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

type BridgeConfigure struct {
	defaultNIC string
	tapName    string
	lanName    string
	newMac     net.HardwareAddr
	gateways   []net.IP
	addresses  []net.Addr

	macvatpDevicePath string
}

func NewBridgeConfigure(defaultNIC string, newMac net.HardwareAddr, tapName string, lanName string) *BridgeConfigure {
	return &BridgeConfigure{
		defaultNIC:        defaultNIC,
		tapName:           tapName,
		lanName:           lanName,
		newMac:            newMac,
		macvatpDevicePath: filepath.Join("/dev", tapName),
	}
}

func (b *BridgeConfigure) SetupBridge() error {
	// Set MAC of NIC to a random one. The original MAC should be assigned to the tap device.
	nicName := b.defaultNIC
	tapName := b.tapName
	lanName := b.lanName
	link, err := netlink.LinkByName(nicName)
	if err != nil {
		return errors.WithMessagef(err, "failed to get link by name %s", nicName)
	}
	// Flush the original NIC IPs.
	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return errors.WithMessagef(err, "failed to get ip of nic %s", nicName)
	}
	// Identical to `ip addr flush dev nicName`.
	for _, addr := range addrs {
		b.addresses = append(b.addresses, addr.IPNet)
	}
	ip4Gateway, err := util.GetIPv4DefaultGateway()
	if err == nil {
		b.gateways = append(b.gateways, ip4Gateway)
	} else {
		if !errors.Is(err, util.NotFoundError) {
			return errors.WithMessagef(err, "failed to get ipv4 default gateway")
		}
	}
	ip6Gateway, err := util.GetIPv6DefaultGateway()
	if err == nil {
		b.gateways = append(b.gateways, ip6Gateway)
	} else {
		if !errors.Is(err, util.NotFoundError) {
			return errors.WithMessagef(err, "failed to get ipv6 default gateway")
		}
	}

	// Identical to `ip link set nicName down`.
	if err = netlink.LinkSetDown(link); err != nil {
		return errors.WithMessagef(err, "failed to bring down nic %s", nicName)
	}
	// Identical to `ip link set nicName address opt.NewNICMac.String()`.
	if err = netlink.LinkSetHardwareAddr(link, b.newMac); err != nil {
		return errors.WithMessagef(err, "failed to set mac '%s' for nic %s ", b.newMac.String(), nicName)
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
	if err = netlink.LinkSetHardwareAddr(macvtap, link.Attrs().HardwareAddr); err != nil {
		return errors.WithMessagef(err, "failed to set mac '%s' for macvtap %s ", link.Attrs().HardwareAddr.String(), tapName)
	}
	// Identical to `ip link set tapName up`.
	if err = netlink.LinkSetUp(macvtap); err != nil {
		return errors.WithMessagef(err, "failed to bring up macvtap %s", tapName)
	}
	// Flush the original NIC IPs.
	addrs, err = netlink.AddrList(link, netlink.FAMILY_ALL)
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
	if output, err := util.Run("mknod", b.macvatpDevicePath, "c", major, minor); err != nil {
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

func (b *BridgeConfigure) GetMacVtapDevicePath() string {
	return b.macvatpDevicePath
}
func (b *BridgeConfigure) Recover() error {
	tapName := b.tapName
	defaultNIC := b.defaultNIC
	address := b.addresses
	lanName := b.lanName
	gateways := b.gateways
	tapLink, err := netlink.LinkByName(tapName)
	if err != nil {
		return errors.WithMessagef(err, "failed to get link by name %s", tapName)
	}
	log.Infof("set tap device %s down", tapName)
	if err = netlink.LinkSetDown(tapLink); err != nil {
		return errors.WithMessagef(err, "failed to bring down tap device '%s'",
			tapLink.Attrs().Name)
	}
	defaultLink, err := netlink.LinkByName(defaultNIC)
	if err != nil {
		return errors.WithMessagef(err, "failed to get link by name %s", defaultNIC)
	}
	err = netlink.LinkSetHardwareAddr(defaultLink, tapLink.Attrs().HardwareAddr)
	if err != nil {
		return errors.WithMessagef(err, "failed to set mac '%s' for nic %s ",
			tapLink.Attrs().HardwareAddr.String(), defaultNIC)
	}
	for _, addr := range address {
		address, err := netlink.ParseAddr(addr.String())
		if err != nil {
			log.Errorf("failed to parse address: %s", addr.String())
			continue
		}
		log.Infof("add ip %s to nic %s", addr.String(), defaultNIC)
		if err := netlink.AddrAdd(defaultLink, address); err != nil {
			return errors.WithMessagef(err, "failed to assign ip %s to nic %s", addr.String(), defaultNIC)
		}
	}
	if err = netlink.LinkDel(tapLink); err != nil {
		return errors.WithMessagef(err, "failed to delete tap device %s", tapLink.Attrs().Name)
	}
	if err = os.RemoveAll(b.macvatpDevicePath); err != nil {
		log.Warnf("failed to delete tap device file %s: %+v", tapName, err)
	}
	lanLink, err := netlink.LinkByName(lanName)
	if err != nil {
		return errors.WithMessagef(err, "failed to get link by name %s", lanName)
	}
	if err = netlink.LinkDel(lanLink); err != nil {
		return errors.WithMessagef(err, "failed to delete macvlan device %s", lanLink.Attrs().Name)
	}
	for _, gateway := range gateways {
		var dst *net.IPNet
		if gateway.To4() == nil {
			_, dst, err = net.ParseCIDR("::/0")
			if err != nil {
				panic(err)
			}
		}
		route := &netlink.Route{
			LinkIndex: defaultLink.Attrs().Index,
			Gw:        gateway,
			Dst:       dst,
		}
		err = netlink.RouteAdd(route)
		if err != nil {
			return errors.WithMessagef(err, "failed to add default router '%+v', %+v", gateway, route)
		}
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
