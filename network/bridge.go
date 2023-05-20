package network

import (
	"fmt"
	"github.com/cox96de/containervm/util"
	"github.com/vishvananda/netlink"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type CreateBridgeOption struct {
	// NICName is the default network interface name, e.g. eth0.
	NICName string
	// NICMac is the default network interface MAC address.
	NICMac net.HardwareAddr
	// NewNICMac is the new MAC address for the default network interface.
	// Original mac (NICMac) will be assigned to the tap device.
	NewNICMac net.HardwareAddr
	// TapName is the name of the tap device.
	TapName string
	// TapDevicePath is the path of the tap device file, e.g. /dev/tap0.
	TapDevicePath string
	// LanName is the name of the macvlan device.
	// ARP server and DHCP server will be started on this device.
	LanName string
}

// CreateBridge creates a MacVTap device and connects it to the given NIC. The tap can be connected to the VM later.
// It will delete all IPs of this NIC, and assign its MAC address to the tap device.
func CreateBridge(opt *CreateBridgeOption) (err error) {
	// Set MAC of NIC to a random one. The original MAC should be assigned to the tap device.
	nicName := opt.NICName
	tapName := opt.TapName
	lanName := opt.LanName
	link, err := netlink.LinkByName(nicName)
	if err != nil {
		return errors.WithMessagef(err, "failed to get link by name %s", nicName)
	}
	// Identical to `ip link set nicName down`.
	if err = netlink.LinkSetDown(link); err != nil {
		return errors.WithMessagef(err, "failed to bring down nic %s", nicName)
	}
	// Identical to `ip link set nicName address opt.NewNICMac.String()`.
	if err = netlink.LinkSetHardwareAddr(link, opt.NewNICMac); err != nil {
		return errors.WithMessagef(err, "failed to set mac '%s' for nic %s ", opt.NewNICMac.String(), nicName)
	}
	// Identical to `ip link set nicName up`.
	if err = netlink.LinkSetUp(link); err != nil {
		return errors.WithMessagef(err, "failed to bring up nic %s", nicName)
	}
	// Create a MacVTap device upon the NIC and assign the original NIC MAC to it.
	log.Infof("creating tap device %s upon nic %s", tapName, nicName)
	attrs := netlink.NewLinkAttrs()
	attrs.ParentIndex = link.Attrs().Index
	attrs.Name = tapName
	attrs.MTU = link.Attrs().MTU
	// Identical to `ip link add link nicName name tapName type macvtap mode bridge`.
	if err = netlink.LinkAdd(&netlink.Macvtap{
		Macvlan: netlink.Macvlan{
			LinkAttrs: attrs,
			Mode:      netlink.MACVLAN_MODE_BRIDGE,
		},
	}); err != nil {
		return errors.WithMessagef(err, "failed to create macvtap device %s", tapName)
	}
	macvtap, err := netlink.LinkByName(tapName)
	if err != nil {
		return errors.WithMessagef(err, "failed to get macvtap link by name %s", tapName)
	}
	// Identical to `ip link set tapName address opt.NICMac.String()`.
	if err = netlink.LinkSetHardwareAddr(macvtap, opt.NICMac); err != nil {
		return errors.WithMessagef(err, "failed to set mac '%s' for macvtap %s ", opt.NICMac.String(), tapName)
	}
	// Identical to `ip link set tapName up`.
	if err = netlink.LinkSetUp(macvtap); err != nil {
		return errors.WithMessagef(err, "failed to bring up macvtap %s", tapName)
	}
	// Flush the original NIC IPs.
	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return errors.WithMessagef(err, "failed to get ip of nic %s", nicName)
	}
	// Identical to `ip addr flush dev nicName`.
	for _, addr := range addrs {
		if err := netlink.AddrDel(link, &addr); err != nil {
			return errors.WithMessagef(err, "failed to delete ip %s of nic %s", addr.String(), nicName)
		}

	}
	major, minor, err := getTapDeviceNum(tapName)
	if err != nil {
		return errors.WithMessagef(err, "failed to get tap device number")
	}
	if output, err := util.Run("mknod", opt.TapDevicePath, "c", major, minor); err != nil {
		return errors.WithMessagef(err, "failed to create dev file: %s", output)
	}
	// Create a MacVLan device with a fake IP. The DHCP server serves on that device.
	log.Infof("creating macvlan device %s upon nic %s", lanName, nicName)
	// Identical to `ip link add link nicName name lanName type macvlan mode bridge`.
	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.ParentIndex = link.Attrs().Index
	linkAttrs.Name = lanName
	linkAttrs.MTU = link.Attrs().MTU
	if err = netlink.LinkAdd(&netlink.Macvlan{
		LinkAttrs: linkAttrs,
		Mode:      netlink.MACVLAN_MODE_BRIDGE,
	}); err != nil {
		return errors.WithMessagef(err, "failed to create macvlan device %s", lanName)
	}
	macvlan, err := netlink.LinkByName(lanName)
	if err != nil {
		return errors.WithMessagef(err, "failed to get macvlan link by name %s", lanName)
	}
	if err = netlink.LinkSetUp(macvlan); err != nil {
		return errors.WithMessagef(err, "failed to bring up macvlan %s", lanName)
	}
	// Identical to `ip addr add xxxx dev lanName`.
	lanIP := "240.0.0.1/32"
	addr, err := netlink.ParseAddr(lanIP)
	if err != nil {
		return errors.WithMessagef(err, "failed to parse ip address '%s'", lanIP)
	}
	if err = netlink.AddrAdd(macvtap, addr); err != nil {
		return errors.WithMessagef(err, "failed to assign ip address '%s' to macvlan device %s", lanIP, lanName)
	}
	return nil
}

// getTapDeviceNum returns tap device major/minor device id.
// The virtual network device cannot be shown in `/dev`, as the files in `/dev` is created by host kernel.
func getTapDeviceNum(tapName string) (string, string, error) {
	devices, err := filepath.Glob(fmt.Sprintf("/sys/devices/virtual/net/%s/tap*/dev", tapName))
	if err != nil {
		fmt.Println("Error:", err)
		return "", "", err
	}
	if len(devices) != 1 {
		return "", "", errors.Errorf("bad device number, got %+v", devices)
	}
	content, err := os.ReadFile(devices[0])
	if err != nil {
		return "", "", errors.WithMessagef(err, "failed to read device number: %s", devices[0])
	}
	split := strings.Split(strings.TrimSpace(string(content)), ":")
	if len(split) != 2 {
		return "", "", errors.Errorf("bad device file, got %+v", split)
	}
	return split[0], split[1], nil
}
