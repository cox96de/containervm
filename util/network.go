package util

import (
	"crypto/rand"
	"net"
	"time"

	"github.com/go-ping/ping"
	"github.com/jackpal/gateway"
	"github.com/pkg/errors"
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
	net.Interface
}

// GetDefaultNIC finds the default network interface of this host.
// The default NIC must have a default gateway.
func GetDefaultNIC() (*NIC, error) {
	ifIP, err := gateway.DiscoverInterface()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to discover default interface")
	}
	if err != nil && errors.Is(err, NotFoundError) {
		return nil, errors.WithMessagef(err, "failed to get ipv6 default gateway")
	}
	config := &NIC{}
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
			config.Interface = nic
			return config, nil
		}
	}
	return nil, errors.WithMessage(err, "failed to find default interface")
}

var NotFoundError = errors.New("not found")

func GetIPv4DefaultGateway() (net.IP, error) {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get ipv4 routes")
	}
	for _, route := range routes {
		if route.Dst == nil && route.Gw != nil {
			return route.Gw, nil
		}
	}
	return nil, NotFoundError
}

func GetIPv6DefaultGateway() (net.IP, error) {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V6)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get ipv6 routes")
	}
	for _, route := range routes {
		if route.Dst == nil && route.Gw != nil {
			return route.Gw, nil
		}
	}
	return nil, NotFoundError
}

func GetHardwareAddr(ifIndex int, ip net.IP) (net.HardwareAddr, error) {
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
