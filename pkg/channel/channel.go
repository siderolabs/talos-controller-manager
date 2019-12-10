// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package channel

import "fmt"

const (
	LatestChannel = "latest"
	EdgeChannel   = "edge"
	AlphaChannel  = "alpha"
	BetaChannel   = "beta"
	StableChannel = "stable"
)

type Channel = string

type InvalidChannelError struct {
	value string
}

func NewInvalidChannelError(c string) InvalidChannelError {
	return InvalidChannelError{c}
}

func (i InvalidChannelError) Error() string {
	return fmt.Sprintf("invalid channel: %s", i.value)
}
