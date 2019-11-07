package filter

import (
	"testing"

	"github.com/talos-systems/talos-controller-manager/pkg/channel"
)

func TestFilterSemver(t *testing.T) {
	tags := []string{
		"v0.2.0",
		"v0.3.0-beta.0",
		"v0.3.0-alpha.8",
		"v0.3.0-alpha.7-22-gbaaa308b",
		"v0.3.0-alpha.7-21-g8c7fadde",
	}
	type args struct {
		ch   string
		tags []string
	}
	tests := []struct {
		name       string
		args       args
		wantTarget *string
	}{
		{
			name: "alpha version",
			args: args{
				ch:   channel.AlphaChannel,
				tags: tags,
			},
			wantTarget: ptr("v0.3.0-alpha.8"),
		},
		{
			name: "beta version",
			args: args{
				ch:   channel.BetaChannel,
				tags: tags,
			},
			wantTarget: ptr("v0.3.0-beta.0"),
		},
		{
			name: "stable version",
			args: args{
				ch:   channel.StableChannel,
				tags: tags,
			},
			wantTarget: ptr("v0.2.0"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotTarget := FilterSemver(tt.args.ch, tt.args.tags); *gotTarget != *tt.wantTarget {
				t.Errorf("FilterSemver() = %v, want %v", *gotTarget, *tt.wantTarget)
			}
		})
	}
}

func ptr(s string) *string {
	return &s
}
