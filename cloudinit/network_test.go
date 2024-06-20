package cloudinit

import (
	"gotest.tools/v3/assert"
	"net"
	"testing"
)

func TestGenerateNetworkConfig(t *testing.T) {
	mac, _ := net.ParseMAC("02:42:ac:11:00:02")
	_, ip, _ := net.ParseCIDR("2001:db8:1::242:ac11:2/64")
	gateway := net.ParseIP("2001:db8:1::1")
	config, err := GenerateNetworkConfig(&NetworkConfig{
		Mac:       mac,
		Addresses: []*net.IPNet{ip},
		Gateway4:  nil,
		Gateway6:  gateway,
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, string(config), `#cloud-config
version: 2
ethernets:
    net0:
        match:
            macaddress: 02:42:ac:11:00:02
        addresses:
            - 2001:db8:1::/64
        gateway6: 2001:db8:1::1
`)
}
