package pod

import (
	"fmt"
	"net"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/common"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	eventFlatNetworkIPError = "FlatNetworkIPError"
)

// newFlatNetworkIP returns a new flat-network IP struct object by Pod.
func (h *handler) newFlatNetworkIP(pod *corev1.Pod) (*flv1.FlatNetworkIP, error) {
	// Valid pod annotation
	annotationIP := pod.Annotations[flv1.AnnotationIP]
	annotationMAC := pod.Annotations[flv1.AnnotationMac]
	annotationSubnet := pod.Annotations[flv1.AnnotationSubnet]
	flatNetworkIPType := flv1.AllocateModeSpecific

	var (
		ipAddrs  []net.IP
		macAddrs []string
		err      error
	)
	switch annotationIP {
	case flv1.AllocateModeAuto:
		flatNetworkIPType = annotationIP
	default:
		ipAddrs, err = common.CheckPodAnnotationIPs(annotationIP)
		if err != nil {
			return nil, fmt.Errorf("newFlatNetworkIP: invalid annotation [%v: %v]: %w",
				flv1.AnnotationIP, annotationIP, err)
		}
	}
	macAddrs, err = common.CheckPodAnnotationMACs(annotationMAC)
	if err != nil {
		return nil, fmt.Errorf("newFlatNetworkIP: invalid annotation [%v: %v]: %w",
			flv1.AnnotationMac, annotationMAC, err)
	}

	subnet, err := h.subnetCache.Get(flv1.SubnetNamespace, annotationSubnet)
	if err != nil {
		return nil, fmt.Errorf("newFlatNetworkIP: failed to get subnet [%v]: %w",
			annotationSubnet, err)
	}

	flatNetworkIP := &flv1.FlatNetworkIP{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: map[string]string{},
			Labels: map[string]string{
				"subnet":                    subnet.Name,
				flv1.LabelFlatNetworkIPType: flatNetworkIPType,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Pod",
					UID:        pod.UID,
					Name:       pod.Name,
					Controller: utils.Ptr(true),
				},
			},
		},
		Spec: flv1.IPSpec{
			Addrs:  ipAddrs,
			MACs:   macAddrs,
			PodID:  string(pod.GetUID()),
			Subnet: subnet.Name,
		},
	}
	if subnet.Annotations[flv1.AnnotationsIPv6to4] != "" {
		flatNetworkIP.Annotations[flv1.AnnotationsIPv6to4] = "true"
	}
	return flatNetworkIP, nil
}

func (h *handler) eventFlatNetworkIPError(pod *corev1.Pod, err error) {
	h.recorder.Event(pod, corev1.EventTypeWarning, eventFlatNetworkIPError, err.Error())
}

func flatNetworkIPUpdated(a, b *flv1.FlatNetworkIP) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Name != b.Name || a.Namespace != b.Namespace {
		logrus.Debugf("ip namespace/name [%v/%v] != [%v/%v]",
			a.Namespace, a.Name, b.Namespace, b.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(a.OwnerReferences, b.OwnerReferences) {
		logrus.Debugf("ip OwnerReferences of [%v/%v] mismatch", a.Namespace, a.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(a.Labels, b.Labels) {
		logrus.Debugf("ip Labels of [%v/%v] mismatch", a.Namespace, a.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(a.Annotations, b.Annotations) {
		logrus.Debugf("ip Annotations of [%v/%v] mismatch", a.Namespace, a.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec, b.Spec) {
		logrus.Debugf("ip Spec of [%v/%v] mismatch", a.Namespace, a.Name)
		return false
	}
	return true
}
