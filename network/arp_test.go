package network

import (
	"github.com/cox96de/containervm/util"
	"github.com/mdlayher/arp"
	"gotest.tools/v3/assert"
	"net"
	"testing"
	"time"
)

func TestServeARP(t *testing.T) {
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
	output, err = util.Run("ip", "addr", "add", "192.168.1.1/24", "dev", clientIface)
	assert.NilError(t, err, output)
	output, err = util.Run("ip", "link", "set", clientIface, "address", "12:34:56:78:9a:bc")
	assert.NilError(t, err, output)
	output, err = util.Run("ip", "link", "set", clientIface, "up")
	assert.NilError(t, err, output)
	output, err = util.Run("ip", "link", "set", serverIface, "up")
	assert.NilError(t, err, output)
	go func() {
		ip, err := net.ResolveIPAddr("", "192.168.1.1")
		assert.NilError(t, err)
		hw, err := net.ParseMAC("12:34:56:78:9a:bc")
		assert.NilError(t, err)
		gwHW, err := net.ParseMAC("12:34:56:78:9a:02")
		assert.NilError(t, err)
		err = ServeARP(serverIface, ip, hw, gwHW)
		assert.NilError(t, err)
	}()
	// Wait for server to start
	time.Sleep(time.Millisecond * 50)
	iface, err := net.InterfaceByName(clientIface)
	assert.NilError(t, err)
	client, err := arp.Dial(iface)
	assert.NilError(t, err)
	t.Run("outer_ip", func(t *testing.T) {
		err := client.SetDeadline(time.Now().Add(time.Second))
		assert.NilError(t, err)
		ip := net.ParseIP("39.156.66.14")
		err = client.Request(ip)
		assert.NilError(t, err)
		packet, _, err := client.Read()
		assert.NilError(t, err)
		t.Logf("%+v", packet)
	})
	t.Run("loop", func(t *testing.T) {
		err := client.SetDeadline(time.Now().Add(time.Second))
		assert.NilError(t, err)
		ip := net.ParseIP("192.168.1.1")
		err = client.Request(ip)
		assert.NilError(t, err)
		_, _, err = client.Read()
		assert.ErrorContains(t, err, "i/o timeout")
	})

}
