package main

import (
	"fmt"
	"github.com/cox96de/containervm/network"
	"github.com/cox96de/containervm/util"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"golang.org/x/exp/rand"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func main() {
	pflag.Parse()
	args := pflag.Args()
	log.SetLevel(log.DebugLevel)
	if len(args) == 0 {
		log.Fatalf("qemu launch command is required")
	}
	tapDevicePath, bridgeMacAddr, mtu := configureNetwork()
	tapFile, err := os.Open(tapDevicePath)
	if err != nil {
		log.Fatalf("failed to open tap dev(%s): %+v", tapDevicePath, err)
	}
	qemuNetworkOpt := generateQEMUNetworkOpt(tapFile, bridgeMacAddr, mtu)
	args = append(args, qemuNetworkOpt...)
	log.Infof("run qemu with command: %s", strings.Join(args, " "))
	qemuCMD := exec.Command(args[0], args[1:]...)
	qemuCMD.Stdin = os.Stdin
	qemuCMD.Stdout = os.Stdout
	qemuCMD.Stderr = os.Stderr
	qemuCMD.ExtraFiles = []*os.File{tapFile}
	if err := qemuCMD.Start(); err != nil {
		log.Fatalf("failed to start qemu: %+v", err)
	}
	if err := qemuCMD.Wait(); err != nil {
		log.Fatalf("failed to wait for qemu: %+v", err)
	}
	log.Infof("qemu exited with code %d", qemuCMD.ProcessState.ExitCode())
}

func generateQEMUNetworkOpt(vtapFile *os.File, macAddr net.HardwareAddr, mtu int) []string {
	return []string{"-netdev", fmt.Sprintf("tap,id=net0,vhost=on,fd=%d", vtapFile.Fd()),
		"-device", "virtio-net-pci,netdev=net0,mac=" + macAddr.String() + ",host_mtu=" + strconv.Itoa(mtu)}
}

func configureNetwork() (bridgeName string, bridgeMacAddr net.HardwareAddr, mtu int) {
	nic, err := util.GetDefaultNIC()
	if err != nil {
		log.Fatalf("failed to get default nic: %+v", err)
	}
	log.Infof("reconfiguring nic %s", nic.Name)
	tapName := fmt.Sprintf("macvtap%s", randomString(3))
	lanName := fmt.Sprintf("macvlan%s", randomString(3))
	tapDevicePath := "/dev/" + tapName
	err = network.CreateBridge(&network.CreateBridgeOption{
		NicName:       nic.Name,
		NicMac:        nic.HardwareAddr,
		NewNicMac:     util.GetRandomMAC(),
		TapName:       tapName,
		TapDevicePath: tapDevicePath,
		LanName:       lanName,
	})
	if err != nil {
		log.Fatalf("failed to set up bridge: %+v", err)
	}
	log.Infof("tap device %s is created", tapName)
	// Start a DHCP server.
	hostname, _ := os.Hostname()
	ds, err := network.NewDHCPServerFromAddr(&network.DHCPOption{
		HardwareAddr:  nic.HardwareAddr,
		IP:            nic.Addr,
		GatewayIP:     nic.Gateway,
		GatewayAddr:   nic.GatewayHardwareAddr,
		DNSServers:    []string{},
		SearchDomains: []string{},
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
	go func() {
		if err := network.ServeARP(lanName, nic.Addr, nic.HardwareAddr, nic.GatewayHardwareAddr); err != nil {
			log.Errorf("failed to start arp server: %+v", err)
		}
	}()
	return tapDevicePath, nic.HardwareAddr, nic.MTU
}

func randomString(b int) string {
	bs := make([]byte, b)
	_, _ = rand.Read(bs)
	return fmt.Sprintf("%x", bs)
}
