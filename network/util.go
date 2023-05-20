package network

import (
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
