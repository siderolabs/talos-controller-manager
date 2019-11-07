package version

import (
	"github.com/talos-systems/talos-controller-manager/pkg/channel"
)

type Cache interface {
	Get(channel.Channel) (string, bool)
	Set(channel.Channel, string)
}
