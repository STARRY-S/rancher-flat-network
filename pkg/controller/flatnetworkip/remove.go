package flatnetworkip

import (
	"fmt"
	"slices"
	"time"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	"github.com/cnrancher/rancher-flat-network/pkg/ipcalc"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

const (
	maxWaitForPodRemovePeriod = 60
)

func (h *handler) handleIPRemove(_ string, ip *flv1.FlatNetworkIP) (*flv1.FlatNetworkIP, error) {
	if ip == nil || ip.Name == "" {
		return ip, nil
	}

	// Wait until pod deleted
	i := 0
	for ; i < maxWaitForPodRemovePeriod; i++ {
		pod, err := h.podCache.Get(ip.Namespace, ip.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				break
			}
			return ip, fmt.Errorf("failed to get pod from cache: %w", err)
		}
		if pod.DeletionTimestamp == nil {
			break
		}
		if pod.UID != types.UID(ip.Spec.PodID) {
			break
		}
		logrus.WithFields(fieldsIP(ip)).
			Debugf("waiting for pod deleted...")
		time.Sleep(time.Second)
	}
	if i >= maxWaitForPodRemovePeriod {
		return ip, fmt.Errorf("failed to wait for pod [%v/%v] remove after [%v] times retry",
			ip.Namespace, ip.Name, maxWaitForPodRemovePeriod)
	}

	unlock := wrangler.IPAllocateLock(ip.Spec.Subnet)
	defer unlock()

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		result, err := h.subnetCache.Get(flv1.SubnetNamespace, ip.Spec.Subnet)
		if err != nil {
			if errors.IsNotFound(err) {
				// The subnet is deleted, return directly.
				return nil
			}
			return fmt.Errorf("failed to get subnet from cache: %w", err)
		}

		result = result.DeepCopy()
		if ipcalc.IPInRanges(ip.Status.Addr, result.Status.UsedIP) {
			result.Status.UsedIP = ipcalc.RemoveIPFromRange(ip.Status.Addr, result.Status.UsedIP)
			result.Status.UsedIPCount--
		}
		if len(ip.Status.MAC) != 0 {
			result.Status.UsedMAC = slices.DeleteFunc(result.Status.UsedMAC, func(m string) bool {
				return m == ip.Status.MAC
			})
			slices.Sort(result.Status.UsedMAC)
		}
		_, err = h.subnetClient.UpdateStatus(result)
		return err
	})
	if err != nil {
		logrus.WithFields(fieldsIP(ip)).
			Errorf("failed to remove usedIP & usedMAC from subnet: %v", err)
	}
	if ip.Status.MAC != "" {
		logrus.WithFields(fieldsIP(ip)).
			Infof("remove IP [%v] MAC [%v] from subnet [%v]",
				ip.Status.Addr, ip.Status.MAC, ip.Spec.Subnet)
	} else {
		logrus.WithFields(fieldsIP(ip)).
			Infof("remove IP [%v] from subnet [%v]",
				ip.Status.Addr, ip.Spec.Subnet)
	}
	return ip, nil
}
