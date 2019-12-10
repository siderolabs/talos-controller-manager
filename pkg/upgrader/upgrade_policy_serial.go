// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

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
