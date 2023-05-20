package network

import (
	"github.com/cox96de/containervm/util"
	"github.com/insomniacslk/dhcp/dhcpv4/client4"
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
	clientIface := "vetha0"
	serverIface := "vetha1"
	clean := func() {
		_, _ = util.Run("ip", "link", "set", clientIface, "down")
		_, _ = util.Run("ip", "link", "set", serverIface, "down")
		_, _ = util.Run("ip", "link", "del", clientIface)
		_, _ = util.Run("ip", "link", "del", serverIface)
	}
	clean()
	t.Cleanup(func() {
		clean()
	})
	output, err := util.Run("ip", "link", "add", clientIface, "type", "veth", "peer", "name", serverIface)
	assert.NilError(t, err, output)
	output, err = util.Run("ip", "addr", "add", "192.168.1.2/24", "dev", serverIface)
	assert.NilError(t, err, output)
	output, err = util.Run("ip", "link", "set", clientIface, "address", "12:34:56:78:9a:bc")
	assert.NilError(t, err, output)
	output, err = util.Run("ip", "addr", "add", "0.0.0.0", "dev", clientIface)
	assert.NilError(t, err, output)
	output, err = util.Run("ip", "link", "set", clientIface, "up")
	assert.NilError(t, err, output)
	output, err = util.Run("ip", "link", "set", serverIface, "up")
	assert.NilError(t, err, output)
	go func() {
		ip := net.ParseIP("192.168.1.3")
		hw, err := net.ParseMAC("12:34:56:78:9a:bc")
		assert.NilError(t, err)
		gwHW, err := net.ParseMAC("12:34:56:78:9a:bd")
		assert.NilError(t, err)
		dhcpServer, err := NewDHCPServerFromAddr(&DHCPOption{
			IP: &net.IPAddr{
				IP: ip,
			},
			HardwareAddr:  hw,
			GatewayIP:     net.ParseIP("192.168.1.1"),
			GatewayAddr:   gwHW,
			DNSServers:    []string{},
			SearchDomains: []string{},
		})
		assert.NilError(t, err)
		err = dhcpServer.Run("")
		assert.NilError(t, err)
	}()
	// Wait for server to start
	time.Sleep(time.Millisecond * 50)
	cli := client4.NewClient()
	cli.ReadTimeout = time.Second * 3
	//loopbackInterfaces, err := interfaces.GetLoopbackInterfaces()
	//assert.NilError(t, err)
	exchange, err := cli.Exchange(clientIface)
	assert.NilError(t, err)
	t.Logf("%+v", exchange)
}
