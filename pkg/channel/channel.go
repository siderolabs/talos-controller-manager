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
