package network

import (
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/client4"
	"github.com/insomniacslk/dhcp/interfaces"
	log "github.com/sirupsen/logrus"
	"gotest.tools/v3/assert"
	"net"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	log.SetLevel(log.DebugLevel)
	os.Exit(m.Run())
}

func TestNewDHCPServerFromAddr(t *testing.T) {
	link, err := interfaces.GetLoopbackInterfaces()
	assert.NilError(t, err)
	nic := link[0]
	ip := net.ParseIP("192.168.1.3")
	gwIP := net.ParseIP("192.168.1.1")
	go func() {
		dhcpServer, err := NewDHCPServerFromAddr(&DHCPOption{
			IP: &net.IPAddr{
				IP: ip,
			},
			HardwareAddr:  nic.HardwareAddr,
			GatewayIP:     gwIP,
			DNSServers:    []string{},
			SearchDomains: []string{},
		})
		assert.NilError(t, err)
		err = dhcpServer.Run(nic.Name)
		assert.NilError(t, err)
	}()
	// Wait for server to start
	time.Sleep(time.Millisecond * 50)
	cli := client4.NewClient()
	cli.ReadTimeout = time.Second * 3
	exchange, err := cli.Exchange(nic.Name)
	assert.NilError(t, err)
	t.Logf("%+v", exchange)
	assert.Assert(t, exchange[1].YourIPAddr.Equal(ip))
	assert.Assert(t, net.IP(exchange[1].Options.Get(dhcpv4.OptionRouter)).Equal(gwIP))
}
