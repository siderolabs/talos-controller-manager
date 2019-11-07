package upgrader

import (
	corev1 "k8s.io/api/core/v1"
)

type SerialPolicy struct {
	Upgrader
}

func (policy SerialPolicy) Run(nodes corev1.NodeList, version string) error {
	for _, node := range nodes.Items {
		if err := policy.Upgrade(node, version); err != nil {
			return err
		}
	}

	return nil
}
