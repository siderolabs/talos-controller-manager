package upgrader

import (
	corev1 "k8s.io/api/core/v1"
)

type UpgradePolicy interface {
	Run(corev1.NodeList, string) error
}
