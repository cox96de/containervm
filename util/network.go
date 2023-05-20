package util

import (
	"crypto/rand"
	"net"
	"time"

	"github.com/go-ping/ping"
	"github.com/jackpal/gateway"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// GetRandomMAC generates a random uni-cast MAC address.
func GetRandomMAC() net.HardwareAddr {
	addr := make(net.HardwareAddr, 6)
	_, _ = rand.Read(addr)
	addr[0] = (addr[0] | 2) & 0xfe
	return addr
}

// NIC describes a NIC with its name, address, gateway & MAC address.
type NIC struct {
	// Name is the interface name, like eth0, eth1, etc.
	Name string
	// Addr is the IP address of the interface.
	Addr net.Addr
	// HardwareAddr is the MAC address of the interface.
	HardwareAddr net.HardwareAddr
	// Gateway is the default gateway of the interface.
	Gateway net.IP
	// GatewayHardwareAddr is the MAC address of the gateway.
	GatewayHardwareAddr net.HardwareAddr
	// MTU is the MTU of the interface.
	MTU int
}

// GetDefaultNIC finds the default network interface of this host.
// The default NIC must have a default gateway.
func GetDefaultNIC() (*NIC, error) {
	gwIP, err := gateway.DiscoverGateway()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to discover default gateway")
	}
	ifIP, err := gateway.DiscoverInterface()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to discover default interface")
	}
	config := &NIC{
		Gateway: gwIP,
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to list interfaces")
	}
	for _, nic := range interfaces {
		addrs, err := nic.Addrs()
		if err != nil {
			continue
		}
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
			config.Name = nic.Name
			config.Addr = addr
			config.HardwareAddr = nic.HardwareAddr
			config.MTU = nic.MTU
			gwHwAddr, err := getHardwareAddr(nic.Index, gwIP)
			if err != nil {
				log.Warningf("failed to get MAC address of gateway %v at %s: %v", gwIP, nic.Name, err)
			}
			config.GatewayHardwareAddr = gwHwAddr
			return config, nil
		}
	}
	return nil, errors.WithMessage(err, "failed to find default interface")
}

func getHardwareAddr(ifIndex int, ip net.IP) (net.HardwareAddr, error) {
	pinger, err := ping.NewPinger(ip.String())
	if err == nil {
		pinger.Count = 1
		pinger.Timeout = time.Millisecond * 100
		pinger.SetNetwork("ipv4")
		pinger.SetPrivileged(true)
		_ = pinger.Run()
	}
	neighs, err := netlink.NeighList(ifIndex, netlink.FAMILY_V4)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get neighbors of %d", ifIndex)
	}
	for _, neigh := range neighs {
		if neigh.IP.Equal(ip) {
			return neigh.HardwareAddr, nil
		}
	}
	return nil, errors.WithMessagef(err, "failed to get hardware address of %v at %d", ip, ifIndex)
}
