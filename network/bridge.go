package network

import (
	"fmt"
	"github.com/cox96de/containervm/util"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type CreateBridgeOption struct {
	// NicName is the default network interface name, e.g. eth0.
	NicName string
	// NicMac is the default network interface MAC address.
	NicMac net.HardwareAddr
	// NewNicMac is the new MAC address for the default network interface.
	// Original mac (NicMac) will be assigned to the tap device.
	NewNicMac net.HardwareAddr
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
	nicName := opt.NicName
	tapName := opt.TapName
	lanName := opt.LanName
	// TODO: use netlink to refactor, ip command might be not installed
	if output, err := util.Run("ip", "link", "set", nicName, "down"); err != nil {
		return errors.WithMessagef(err, "failed to bring down nic: %s", output)
	}
	if output, err := util.Run("ip", "link", "set", nicName, "address", opt.NewNicMac.String()); err != nil {
		return errors.WithMessagef(err, "failed to set mac for nic: %s", output)
	}
	if output, err := util.Run("ip", "link", "set", nicName, "up"); err != nil {
		return errors.WithMessagef(err, "failed to bring up nic: %s", output)
	}
	// Create a MacVTap device upon the NIC and assign the original NIC MAC to it.
	log.Infof("creating tap device %s upon nic %s", tapName, nicName)
	tapCommand := []string{"link", "add", "link", nicName, "name", tapName, "type", "macvtap", "mode", "bridge"}
	if output, err := util.Run("ip", tapCommand...); err != nil {
		return errors.WithMessagef(err, "failed to create macvtap device: %s", output)
	}
	if output, err := util.Run("ip", "link", "set", tapName, "address", opt.NicMac.String()); err != nil {
		return errors.WithMessagef(err, "failed to set mac of tap device to nic: %+v", output)
	}
	if output, err := util.Run("ip", "link", "set", tapName, "up"); err != nil {
		return errors.WithMessagef(err, "failed to bring up tap device: %s", output)
	}
	// Flush the original NIC IPs.
	if output, err := util.Run("ip", "addr", "flush", "dev", nicName); err != nil {
		return errors.WithMessagef(err, "failed to flush ip of nic: %s", output)
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
	lanCommand := []string{"link", "add", "link", nicName, "name", lanName, "type", "macvlan", "mode", "bridge"}
	if output, err := util.Run("ip", lanCommand...); err != nil {
		return errors.WithMessagef(err, "failed to create macvtap device: %s", output)
	}
	if output, err := util.Run("ip", "link", "set", lanName, "up"); err != nil {
		return errors.WithMessagef(err, "failed to bring up macvlan device: %s", output)
	}
	if output, err := util.Run("ip", "addr", "add", "240.0.0.0/32", "dev", lanName); err != nil {
		return errors.WithMessagef(err, "failed to assign ip to macvlan device: %s", output)
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
