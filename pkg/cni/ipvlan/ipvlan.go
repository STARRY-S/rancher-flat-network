package ipvlan

import (
	"fmt"
	"net"

	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type Options struct {
	Mode   string
	Flag   string
	Master string
	MTU    int
	IfName string
	NetNS  ns.NetNS
	MAC    string
}

func Create(o *Options) (*types100.Interface, error) {
	mode, err := ModeFromString(o.Mode)
	if err != nil {
		return nil, err
	}

	flag, err := FlagFromString(o.Flag)
	if err != nil {
		return nil, err
	}

	m, err := netlink.LinkByName(o.Master)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup master %q: %w",
			o.Master, err)
	}

	// due to kernel bug we have to create with tmpname or it might
	// collide with the name on the host and error out
	tmpName, err := ip.RandomVethName()
	if err != nil {
		return nil, err
	}
	var mac net.HardwareAddr
	if o.MAC != "" {
		mac, err = net.ParseMAC(o.MAC)
		if err != nil {
			return nil, fmt.Errorf("failed to parse MAC %q: %w", o.MAC, err)
		}
	}
	iv := &netlink.IPVlan{
		LinkAttrs: netlink.LinkAttrs{
			MTU:          o.MTU,
			Name:         tmpName,
			ParentIndex:  m.Attrs().Index,
			Namespace:    netlink.NsFd(int(o.NetNS.Fd())),
			HardwareAddr: mac,
		},
		Mode: mode,
		Flag: flag,
	}
	if err := netlink.LinkAdd(iv); err != nil {
		return nil, fmt.Errorf("failed to create ipvlan: %v", err)
	}
	logrus.Debugf("created ipvlan iface: %v", utils.Print(iv))
	var result *types100.Interface
	if err := o.NetNS.Do(func(_ ns.NetNS) error {
		err := ip.RenameLink(tmpName, o.IfName)
		if err != nil {
			return fmt.Errorf("failed to rename ipvlan ifave to %q: %v", o.IfName, err)
		}
		logrus.Debugf("rename link %v to %v", tmpName, o.IfName)

		// Re-fetch ipvlan to get all properties/attributes
		ipvlan, err := netlink.LinkByName(o.IfName)
		if err != nil {
			netlink.LinkDel(iv)
			return fmt.Errorf("failed to refetch ipvlan %q: %v", o.IfName, err)
		}
		logrus.Debugf("refetch ipvlan link object: %v", utils.Print(ipvlan))

		result = &types100.Interface{
			Name:    o.IfName,
			Mac:     ipvlan.Attrs().HardwareAddr.String(),
			Sandbox: o.NetNS.Path(),
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func ModeToString(mode netlink.IPVlanMode) (string, error) {
	switch mode {
	case netlink.IPVLAN_MODE_L2:
		return "l2", nil
	case netlink.IPVLAN_MODE_L3:
		return "l3", nil
	case netlink.IPVLAN_MODE_L3S:
		return "l3s", nil
	default:
		return "", fmt.Errorf("unknown ipvlan mode: %v", mode)
	}
}

func ModeFromString(s string) (netlink.IPVlanMode, error) {
	switch s {
	case "", "l2":
		return netlink.IPVLAN_MODE_L2, nil
	case "l3":
		return netlink.IPVLAN_MODE_L3, nil
	case "l3s":
		return netlink.IPVLAN_MODE_L3S, nil
	default:
		return 0, fmt.Errorf("unknown ipvlan mode: %q", s)
	}
}

func FlagFromString(s string) (netlink.IPVlanFlag, error) {
	switch s {
	case "", "bridge":
		return netlink.IPVLAN_FLAG_BRIDGE, nil
	case "private":
		return netlink.IPVLAN_FLAG_PRIVATE, nil
	case "vepa":
		return netlink.IPVLAN_FLAG_VEPA, nil
	default:
		return 0, fmt.Errorf("unknow ipvlan flag: %q", s)
	}
}

func FlagToString(flag netlink.IPVlanFlag) (string, error) {
	switch flag {
	case netlink.IPVLAN_FLAG_BRIDGE:
		return "bridge", nil
	case netlink.IPVLAN_FLAG_PRIVATE:
		return "private", nil
	case netlink.IPVLAN_FLAG_VEPA:
		return "vepa", nil
	default:
		return "", fmt.Errorf("unknown ipvlan flag: %v", flag)
	}
}
