// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package version

import (
	"sync"

	"github.com/talos-systems/talos-controller-manager/pkg/channel"
)

type VersionMap map[channel.Channel]string

type V1Alpha1 struct {
	v VersionMap

	mu sync.Mutex
}

func (v1alpha1 *V1Alpha1) Get(channel channel.Channel) (string, bool) {
	v1alpha1.mu.Lock()
	defer v1alpha1.mu.Unlock()

	if v1alpha1.v == nil {
		v1alpha1.v = VersionMap{}
	}

	version, ok := v1alpha1.v[channel]

	return version, ok
}

func (v1alpha1 *V1Alpha1) Set(channel channel.Channel, value string) {
	v1alpha1.mu.Lock()
	defer v1alpha1.mu.Unlock()

	if v1alpha1.v == nil {
		v1alpha1.v = VersionMap{}
	}

	v1alpha1.v[channel] = value
}
