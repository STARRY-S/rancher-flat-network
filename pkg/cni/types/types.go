package types

import (
	"net"

	"github.com/containernetworking/cni/pkg/types"
)

// K8sArgs is the valid CNI_ARGS used for Kubernetes
type K8sArgs struct {
	types.CommonArgs

	// IP is pod's ip address
	IP net.IP

	// K8S_POD_NAME is pod's name
	K8S_POD_NAME types.UnmarshallableString

	// K8S_POD_NAMESPACE is pod's namespace
	K8S_POD_NAMESPACE types.UnmarshallableString

	// K8S_POD_INFRA_CONTAINER_ID is pod's container id
	K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString
}

// NetConf describes a network.
type NetConf struct {
	CNIVersion string `json:"cniVersion,omitempty"`

	Name         string          `json:"name,omitempty"`
	Type         string          `json:"type,omitempty"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
	DNS          types.DNS       `json:"dns,omitempty"`

	RawPrevResult map[string]interface{} `json:"prevResult,omitempty"`
	PrevResult    types.Result           `json:"-"`

	// ValidAttachments is only supplied when executing a GC operation
	ValidAttachments []types.GCAttachment `json:"cni.dev/valid-attachments,omitempty"`

	// static-ipam config
	IPAM IPAMConfig `json:"ipam,omitempty"`

	// Rancher FlatNetwork Config
	FlatNetworkConfig FlatNetworkConfig `json:"flatNetwork,omitempty"`
}

type FlatNetworkConfig struct {
	MTU         int    `json:"mtu"`
	ClusterCIDR string `json:"clusterCIDR"`
	ServiceCIDR string `json:"serviceCIDR"`
	ARPPolicy   string `json:"arpPolicy"`
	ProxyARP    bool   `json:"proxyARP"`
}

type Address struct {
	Address string `json:"address"`
	Gateway net.IP `json:"gateway,omitempty"`
	Version string
}

type IPAMConfig struct {
	Type      string         `json:"type"`
	Addresses []Address      `json:"addresses,omitempty"`
	Routes    []*types.Route `json:"routes,omitempty"`
}
