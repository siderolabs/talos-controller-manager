package upgrader

import (
	corev1 "k8s.io/api/core/v1"
)

type Upgrader interface {
	Upgrade(corev1.Node, string) error
}
