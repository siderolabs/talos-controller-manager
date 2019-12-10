// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package version

import (
	"github.com/talos-systems/talos-controller-manager/pkg/channel"
)

type Cache interface {
	Get(channel.Channel) (string, bool)
	Set(channel.Channel, string)
}
