package main

import (
	"fmt"
	"github.com/cox96de/containervm/cloudinit"
	"github.com/cox96de/containervm/network"
	"github.com/cox96de/containervm/resolvconf"
	"github.com/cox96de/containervm/util"
	"github.com/jackpal/gateway"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"golang.org/x/exp/rand"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func main() {
	var (
		inheritResolv    bool
		extraNameservers []string
	)
	pflag.BoolVar(&inheritResolv, "inherit-resolv", true, "inherit resolv.conf from host")
	pflag.StringSliceVar(&extraNameservers, "nameserver", []string{}, "extra nameserver to use")
	pflag.Parse()
	args := pflag.Args()
	log.SetLevel(log.DebugLevel)
	if len(args) == 0 {
		log.Fatalf("qemu launch command is required")
	}
	var (
		nameservers   []string
		searchDomains []string
		err           error
	)
	if inheritResolv {
		nameservers, searchDomains, err = getNameserversAndSearchDomain()
		if err != nil {
			log.Fatalf("failed to get nameservers and search domains: %+v", err)
		}
	}
	// TODO: validate nameservers
	nameservers = append(nameservers, extraNameservers...)
	ns := make([]net.IP, 0, len(nameservers))
	for _, nameserver := range nameservers {
		ip := net.ParseIP(nameserver)
		if ip.IsLoopback() {
			// Nameserver in containers might be a loopback address.
			// It can be seen in docker-compose.
			continue
		}
		ns = append(ns, ip)
	}
	nw, cleanFunc := configureNetwork(ns, searchDomains)
	defer func() {
		log.Infof("cleaning up network...")
		if err := cleanFunc(); err != nil {
			log.Errorf("failed to clean up network: %+v", err)
		}
	}()
	tapFile, err := os.Open(nw.BridgeName)
	if err != nil {
		log.Fatalf("failed to open tap dev(%s): %+v", nw.BridgeName, err)
	}
	qemuNetworkOpt := generateQEMUNetworkOpt(tapFile, nw.BridgeMacAddr, nw.MTU)
	args = append(args, qemuNetworkOpt...)
	if nw.Gateway6 != nil {
		log.Infof("use cloud-init to setup ipv6 network...")
		cloudInitOpt := generateCloudInitOpt(nw)
		args = append(args, cloudInitOpt...)
	}
	log.Infof("run qemu with command: %s", strings.Join(args, " "))
	exitSig := make(chan os.Signal)
	signal.Notify(exitSig, syscall.SIGTERM, syscall.SIGINT)
	qemuCMD := exec.Command(args[0], args[1:]...)
	qemuCMD.Stdin = os.Stdin
	qemuCMD.Stdout = os.Stdout
	qemuCMD.Stderr = os.Stderr
	qemuCMD.ExtraFiles = []*os.File{tapFile}
	if err := qemuCMD.Start(); err != nil {
		log.Fatalf("failed to start qemu: %+v", err)
	}
	go func() {
		sig := <-exitSig
		log.Infof("recieve signal %+v", sig)
		_ = qemuCMD.Process.Kill()
	}()
	if err := qemuCMD.Wait(); err != nil {
		log.Errorf("failed to wait for qemu: %+v", err)
		return
	}
	log.Infof("qemu exited with code %d", qemuCMD.ProcessState.ExitCode())
}

func generateQEMUNetworkOpt(vtapFile *os.File, macAddr net.HardwareAddr, mtu int) []string {
	return []string{"-netdev", fmt.Sprintf("tap,id=net0,vhost=on,fd=%d", vtapFile.Fd()),
		"-device", "virtio-net-pci,netdev=net0,mac=" + macAddr.String() + ",host_mtu=" + strconv.Itoa(mtu)}
}

func generateCloudInitOpt(n *Network) []string {
	c := &cloudinit.NetworkConfig{
		Mac:       n.BridgeMacAddr,
		Addresses: n.Address,
		Gateway4:  n.Gateway,
		Gateway6:  n.Gateway6,
	}
	content, err := cloudinit.GenerateNetworkConfig(c)
	if err != nil {
		log.Fatalf("failed to generate network config: %+v", err)
	}
	tempDir, err := os.MkdirTemp("", "cloud-init-*")
	if err != nil {
		log.Fatalf("failed to create temp dir: %+v", err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "network-config"), content, os.ModePerm)
	if err != nil {
		log.Fatalf("failed to write network-config: %+v", err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "user-data"), []byte(`#cloud-config
`), os.ModePerm)
	if err != nil {
		log.Fatalf("failed to write user-data: %+v", err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "meta-data"), []byte(`#cloud-config
instance-id: someid/somehost
`), os.ModePerm)
	if err != nil {
		log.Fatalf("failed to write meta-data: %+v", err)
	}
	isoFile := "seed.iso"

	err = util.GenISO(tempDir, isoFile, []string{"network-config", "meta-data", "user-data"}, "cidata")
	if err != nil {
		log.Fatalf("failed to write network-config: %+v", err)
	}
	return []string{"-drive", fmt.Sprintf("driver=raw,file=%s,if=virtio", filepath.Join(tempDir, isoFile))}
}

type Network struct {
	NIC           *util.NIC
	Address       []*net.IPNet
	Gateway       net.IP
	Gateway6      net.IP
	BridgeName    string
	BridgeMacAddr net.HardwareAddr
	MTU           int
}

func configureNetwork(dnsServers []net.IP, searchDomains []string) (nw *Network,
	clean func() error) {

	nic, err := util.GetDefaultNIC()
	if err != nil {
		log.Fatalf("failed to get default nic: %+v", err)
	}
	nw = &Network{
		NIC:           nic,
		BridgeMacAddr: nic.HardwareAddr,
		MTU:           nic.MTU,
	}
	ipv4Gateway, err := util.GetIPv4DefaultGateway()
	if err != nil {
		log.Warnf("failed to get default ipv4 gateway address: %+v", err)
	}
	nw.Gateway = ipv4Gateway
	ipv6Gateway, err := util.GetIPv6DefaultGateway()
	if err != nil {
		log.Warnf("failed to get default ipv6 gateway address: %+v", err)
	}
	nw.Gateway6 = ipv6Gateway
	ifIP, err := gateway.DiscoverInterface()
	if err != nil {
		log.Warnf("failed to get default gateway interface: %+v", err)
	}
	log.Infof("reconfiguring nic %s", nic.Name)
	defaultNIC, err := net.InterfaceByName(nic.Name)
	if err != nil {
		log.Fatalf("failed to get nic %s: %+v", nic.Name, err)
	}
	addrs, err := defaultNIC.Addrs()
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		nw.Address = append(nw.Address, ipNet)
	}
	if err != nil {
		log.Fatalf("failed to get addresses of nic %s: %+v", nic.Name, err)
	}
	var ipv4Addr net.Addr
	for _, addr := range addrs {
		switch ip := addr.(type) {
		case *net.IPAddr:
			if !ifIP.Equal(ip.IP) {
				continue
			}
		case *net.IPNet:
			if !ifIP.Equal(ip.IP) {
				continue
			}
		default:
			continue
		}
		ipv4Addr = addr
		break
	}

	tapName := fmt.Sprintf("macvtap%s", randomString(3))
	lanName := fmt.Sprintf("macvlan%s", randomString(3))
	configure := network.NewBridgeConfigure(nic.Name, util.GetRandomMAC(), tapName, lanName)
	err = configure.SetupBridge()
	if err != nil {
		log.Fatalf("failed to set up bridge: %+v", err)
	}

	log.Infof("tap device %s is created", tapName)
	// Start a DHCP server.
	hostname, _ := os.Hostname()

	if ipv4Addr != nil && ipv4Gateway != nil {
		log.Infof("start dhcp server")
		ds, err := network.NewDHCPServerFromAddr(&network.DHCPOption{
			HardwareAddr:  nic.HardwareAddr,
			IP:            ipv4Addr,
			GatewayIP:     ipv4Gateway,
			DNSServers:    dnsServers,
			SearchDomains: searchDomains,
			Hostname:      hostname,
		})
		if err != nil {
			log.Fatalf("failed to create dhcp server: %+v", err)
		}
		go func() {
			if err := ds.Run(lanName); err != nil {
				log.Errorf("failed to start dhcp server: %+v", err)
			}
		}()
	}
	var gatewayMacAddr net.HardwareAddr

	if ipv4Gateway != nil {
		gatewayMacAddr, err = util.GetHardwareAddr(nic.Index, ipv4Gateway)
		if err != nil {
			log.Warnf("failed to get gateway mac address for ipv4 gateway %+v: %+v", ipv4Gateway, err)
		}
	}
	if ipv4Gateway != nil && gatewayMacAddr != nil {
		log.Infof("start arp server")
		go func() {
			if err := network.ServeARP(lanName, ipv4Addr, nic.HardwareAddr, gatewayMacAddr); err != nil {
				log.Errorf("failed to start arp server: %+v", err)
			}
		}()
	}
	nw.BridgeName = configure.GetMacVtapDevicePath()
	return nw, func() error {
		return configure.Recover()
	}
}

func getNameserversAndSearchDomain() (nameservers []string, searchDomains []string, err error) {
	resolvFile, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "failed to read /etc/resolv.conf")
	}
	nameservers = resolvconf.GetNameservers(resolvFile)
	searchDomains = resolvconf.GetSearchDomains(resolvFile)
	return nameservers, searchDomains, nil
}

func randomString(b int) string {
	bs := make([]byte, b)
	_, _ = rand.Read(bs)
	return fmt.Sprintf("%x", bs)
}
