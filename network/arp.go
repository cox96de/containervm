package network

import (
	"bytes"
	"net"

	"github.com/mdlayher/arp"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// ServeARP starts an ARP answerer on `ifName` to answer ARP requests with `hardwareAddr`.
// It replies gateway's hardware address.
func ServeARP(ifName string, addr net.Addr, hardwareAddr, gatewayHardAddr net.HardwareAddr) error {
	ip, mask, err := getIPAndMask(addr)
	if err != nil {
		return errors.WithMessagef(err, "failed to parse addr %v", addr)
	}
	ipNet := &net.IPNet{IP: ip.To4(), Mask: mask}
	i, err := net.InterfaceByName(ifName)
	if err != nil {
		return errors.WithMessagef(err, "failed to get interface %s", ifName)
	}
	cli, err := arp.Dial(i)
	if err != nil {
		return errors.WithMessagef(err, "failed to listen to arp at %s", ifName)
	}
	log.Infof("arp answerer started at %s", ifName)
	for {
		p, _, err := cli.Read()
		if err != nil {
			log.Errorf("failed to read arp packet: %v", err)
			continue
		}
		log.Debugf("get an arp request: %v", p)
		if p.Operation != arp.OperationRequest {
			log.Debugf("get an arp packet with operation %v", p.Operation)
			continue
		}
		if !bytes.Equal(p.SenderHardwareAddr, hardwareAddr) {
			continue
		}
		// Ignore the arp request from VM's ip, and it's subnet.
		if p.TargetIP.Equal(ip) || ipNet.Contains(p.TargetIP) {
			continue
		}
		if err := cli.Reply(p, gatewayHardAddr, p.TargetIP); err != nil {
			log.Errorf("failed to answer arp request: %v", err)
			continue
		}
		log.Debugf("answered arp request to %v", gatewayHardAddr)
	}
}
