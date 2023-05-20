package network

import (
	"bytes"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"github.com/insomniacslk/dhcp/rfc1035label"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"net"
)

type DHCPOption struct {
	// Only response dhcp request from HardwareAddr.
	HardwareAddr net.HardwareAddr
	// Return that IP in dhcp response
	IP net.Addr
	// Return GatewayIP in dhcp response.
	GatewayIP net.IP
	// Return GatewayAddr in dhcp response.
	GatewayAddr net.HardwareAddr
	// Return DNSServers in dhcp response.
	DNSServers []string
	// Return SearchDomains in dhcp response.
	SearchDomains []string
	// Return Hostname in dhcp response
	Hostname string
}

// NewDHCPServerFromAddr creates a DHCPServer to distribute `addr` and `gateway`.
func NewDHCPServerFromAddr(opt *DHCPOption) (*DHCPServer, error) {
	clientIP, subnetMask, err := getIPAndMask(opt.IP)
	if clientIP.To4() == nil {
		return nil, errors.New("ipv6 is not supported")
	}
	broadcastAddr := getBroadcastAddress(clientIP, subnetMask)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get hostname")
	}
	nameServers := make([]net.IP, 0, len(opt.DNSServers))
	for _, dn := range opt.DNSServers {
		nameServers = append(nameServers, net.ParseIP(dn))
	}
	return &DHCPServer{
		clientIP:      clientIP.To4(),
		clientHwAddr:  opt.HardwareAddr,
		hostname:      opt.Hostname,
		subnetMask:    subnetMask,
		broadcastAddr: broadcastAddr.To4(),
		router:        opt.GatewayIP.To4(),
		routerHwAddr:  opt.GatewayAddr,
		dnsServers:    nameServers,
		domains:       opt.SearchDomains,
	}, nil
}

// DHCPServer is a simplified DHCP server that only provides a single IP address.
// It listens on the macvlan NIC and pretends to be the router.
// It only supports IPv4.
type DHCPServer struct {
	ifName        string
	clientIP      net.IP
	clientHwAddr  net.HardwareAddr
	hostname      string
	subnetMask    net.IPMask
	broadcastAddr net.IP
	router        net.IP
	routerHwAddr  net.HardwareAddr
	dnsServers    []net.IP
	domains       []string
}

type logger struct{}

func (l *logger) PrintMessage(prefix string, message *dhcpv4.DHCPv4) {
	log.Debugf("%s: %s", prefix, message.Message())
}

func (l *logger) Printf(format string, v ...interface{}) {
	log.Debugf(format, v...)
}

// Run starts the server on the interface `ifName`.
func (s *DHCPServer) Run(ifName string) error {
	s.ifName = ifName
	server, err := server4.NewServer(ifName, nil, s.handle, server4.WithLogger(&logger{}))
	if err != nil {
		return errors.WithMessage(err, "failed to initialize server")
	}
	log.Infof("dhcp server runs on %s", ifName)
	log.Debugf("client ip: %v", s.clientIP)
	log.Debugf("client hardware addr: %v", s.clientHwAddr)
	log.Debugf("subnet mask: %v", s.subnetMask)
	log.Debugf("broadcast addr: %v", s.broadcastAddr)
	log.Debugf("router: %v", s.router)
	log.Debugf("router hw addr: %v", s.routerHwAddr)
	log.Debugf("hostname: %s", s.hostname)
	log.Debugf("dns servers: %+v", s.dnsServers)
	log.Debugf("search domains: %+v", s.domains)
	return server.Serve()
}

func (s *DHCPServer) handle(conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4) {
	var (
		replyMsg *dhcpv4.DHCPv4
		err      error
	)
	if !bytes.Equal(s.clientHwAddr, msg.ClientHWAddr) {
		log.Debugf("ignoring a dhcp packet from unexpected source, expect '%s', got '%s'",
			s.clientHwAddr.String(), msg.ClientHWAddr.String())
		return
	}
	msgType := msg.Options.Get(dhcpv4.OptionDHCPMessageType)
	switch {
	case bytes.Equal(msgType, dhcpv4.MessageTypeDiscover.ToBytes()):
		// Get a DISCOVER message. Send an OFFER.
		log.Debugf("get DISCOVER: %s", msg.Summary())
		replyMsg, err = s.composeReply(msg, dhcpv4.MessageTypeOffer)
		if err != nil {
			log.Errorf("failed to build OFFER message: %+v", err)
			return
		}
		log.Debugf("sending OFFER: %s", replyMsg.Summary())
	case bytes.Equal(msgType, dhcpv4.MessageTypeRequest.ToBytes()):
		// Get a REQUEST message. Send an ACK.
		log.Debugf("get REQUEST: %s", msg.Summary())
		replyMsg, err = s.composeReply(msg, dhcpv4.MessageTypeAck)
		if err != nil {
			log.Errorf("failed to build ACK message: %+v", err)
			return
		}
		log.Debugf("sending ACK: %s", replyMsg.Summary())
	default:
		// Get an unrelated message. Just ignore.
		log.Debugf("ignoring message: %s", msg.Summary())
		return
	}
	_, err = conn.WriteTo(replyMsg.ToBytes(), peer)
	if err != nil {
		log.Errorf("failed to send reply: %+v", err)
		return
	}
	return
}

func (s *DHCPServer) composeReply(msg *dhcpv4.DHCPv4, msgType dhcpv4.MessageType) (*dhcpv4.DHCPv4, error) {
	opts := []dhcpv4.Modifier{
		dhcpv4.WithReply(msg),
		dhcpv4.WithClientIP(msg.ClientIPAddr),
		dhcpv4.WithYourIP(s.clientIP),
		dhcpv4.WithServerIP(s.router),
		dhcpv4.WithOption(dhcpv4.OptMessageType(msgType)),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(s.router)),
		dhcpv4.WithOption(dhcpv4.OptIPAddressLeaseTime(dhcpv4.MaxLeaseTime)),
		dhcpv4.WithOption(dhcpv4.OptBroadcastAddress(s.broadcastAddr)),
		dhcpv4.WithOption(dhcpv4.OptRouter(s.router)),
		dhcpv4.WithOption(dhcpv4.OptSubnetMask(s.subnetMask)),
		dhcpv4.WithOption(dhcpv4.OptHostName(s.hostname)),
	}
	// Windows DHCP client doesn't accept empty options.
	if len(s.dnsServers) > 0 {
		opts = append(opts, dhcpv4.WithOption(dhcpv4.OptDNS(s.dnsServers...)))
	}
	if len(s.domains) > 0 {
		opts = append(opts, dhcpv4.WithOption(dhcpv4.OptDomainSearch(&rfc1035label.Labels{Labels: s.domains})))
	}
	return dhcpv4.New(opts...)
}
