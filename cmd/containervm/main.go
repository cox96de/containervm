package main

import (
	"fmt"
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
	tapDevicePath, bridgeMacAddr, mtu, cleanFunc := configureNetwork(ns, searchDomains)
	defer func() {
		log.Infof("cleaning up network...")
		if err := cleanFunc(); err != nil {
			log.Errorf("failed to clean up network: %+v", err)
		}
	}()
	tapFile, err := os.Open(tapDevicePath)
	if err != nil {
		log.Fatalf("failed to open tap dev(%s): %+v", tapDevicePath, err)
	}
	qemuNetworkOpt := generateQEMUNetworkOpt(tapFile, bridgeMacAddr, mtu)
	args = append(args, qemuNetworkOpt...)
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

func configureNetwork(dnsServers []net.IP, searchDomains []string) (bridgeName string, bridgeMacAddr net.HardwareAddr, mtu int,
	clean func() error) {
	nic, err := util.GetDefaultNIC()
	if err != nil {
		log.Fatalf("failed to get default nic: %+v", err)
	}
	ipv4Gateway, err := util.GetIPv4DefaultGateway()
	if err != nil {
		log.Warnf("failed to get default ipv4 gateway address: %+v", err)
	}
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
	return configure.GetMacVtapDevicePath(), nic.HardwareAddr, nic.MTU, func() error {
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
