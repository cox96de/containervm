package cloudinit

import (
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"
	"net"
)

type NetworkConfig struct {
	// Mac is the MAC address of the network interface in the guest VM.
	Mac net.HardwareAddr
	// Addresses is a list of IP addresses and subnets to assign to the network interface.
	Addresses []*net.IPNet
	// Gateway4 is the IPv4 gateway address. If nil, no ipv4 gateway is set.
	Gateway4 net.IP
	// Gateway6 is the IPv6 gateway address. If nil, no ipv6 gateway is set.
	Gateway6 net.IP
}

// GenerateNetworkConfig generates a network configuration for cloud-init.
func GenerateNetworkConfig(c *NetworkConfig) ([]byte, error) {
	eth := &ethernet{
		Match: &match{
			Macaddress: c.Mac.String(),
		},
		Addresses: lo.Map(c.Addresses, func(item *net.IPNet, index int) string {
			return item.String()
		}),
	}
	if c.Gateway4 != nil {
		eth.Gateway4 = lo.ToPtr(c.Gateway4.String())
	}
	if c.Gateway6 != nil {
		eth.Gateway6 = lo.ToPtr(c.Gateway6.String())
	}
	n := &cloudInitNetwork{
		Version: 2,
		Ethernets: map[string]*ethernet{
			// Use a fixed name.
			"net0": eth,
		},
	}
	out, err := yaml.Marshal(n)
	if err != nil {
		return nil, err
	}
	return append([]byte("#cloud-config\n"), out...), nil
}

type cloudInitNetwork struct {
	Version   int                  `yaml:"version"`
	Ethernets map[string]*ethernet `yaml:"ethernets"`
}

type ethernet struct {
	Match     *match   `yaml:"match,omitempty"`
	Addresses []string `yaml:"addresses,omitempty"`
	Gateway4  *string  `yaml:"gateway4,omitempty"`
	Gateway6  *string  `yaml:"gateway6,omitempty"`
	// For lower version of cloud-init, it's necessary to set the set-name or the name of the network interface
	// must exactly match the name of the network in guest VM.
	SetName string `yaml:"set-name,omitempty"`
}

type match struct {
	Macaddress string `yaml:"macaddress,omitempty"`
}
