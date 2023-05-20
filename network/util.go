package network

import (
	"encoding/binary"
	"github.com/pkg/errors"
	"net"
)

func getIPAndMask(addr net.Addr) (net.IP, net.IPMask, error) {
	switch ip := addr.(type) {
	case *net.IPAddr:
		return ip.IP, ip.IP.DefaultMask(), nil
	case *net.IPNet:
		return ip.IP, ip.Mask, nil
	default:
		return nil, nil, errors.New("unknown addr type")
	}
}

// getBroadcastAddress returns the broadcast address of given IP and subnet mast.
func getBroadcastAddress(ip net.IP, mask net.IPMask) net.IP {
	broadcastAddr := make(net.IP, net.IPv4len)
	binary.BigEndian.PutUint32(broadcastAddr, binary.BigEndian.Uint32(ip.To4())|^binary.BigEndian.Uint32(mask))
	return broadcastAddr
}
