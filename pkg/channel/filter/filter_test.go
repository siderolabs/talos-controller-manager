// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package filter

import (
	"testing"

	"github.com/talos-systems/talos-controller-manager/pkg/channel"
)

func TestFilterSemver(t *testing.T) {
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
			name: "latest version",
			args: args{
				ch: channel.LatestChannel,
				tags: []string{
					"v0.1.0",
					"v0.1.0-STRING",
					"v0.2.0-alpha.0-STRING",
					"v0.2.0-alpha.1",
					"v0.2.0-alpha.1-STRING",
				},
			},
			wantTarget: ptr("v0.2.0-alpha.1-STRING"),
		},
		{
			name: "alpha version",
			args: args{
				ch: channel.AlphaChannel,
				tags: []string{
					"v0.1.0",
					"v0.1.0-STRING",
					"v0.2.0-alpha.0-STRING",
					"v0.2.0-alpha.1",
				},
			},
			wantTarget: ptr("v0.2.0-alpha.1"),
		},
		{
			name: "alpha version older than latest stable",
			args: args{
				ch: channel.AlphaChannel,
				tags: []string{
					"v0.1.0",
					"v0.1.0-STRING",
					"v0.2.0-alpha.0-STRING",
					"v0.2.0-alpha.1",
					"v0.2.0",
				},
			},
			wantTarget: ptr("v0.2.0"),
		},
		{
			name: "beta version",
			args: args{
				ch: channel.BetaChannel,
				tags: []string{
					"v0.1.0",
					"v0.1.0-STRING",
					"v0.2.0-alpha.0-STRING",
					"v0.2.0-alpha.1",
					"v0.2.0-beta.0-STRING",
					"v0.2.0-beta.1",
				},
			},
			wantTarget: ptr("v0.2.0-beta.1"),
		},
		{
			name: "beta version older than latest stable",
			args: args{
				ch: channel.BetaChannel,
				tags: []string{
					"v0.1.0",
					"v0.1.0-STRING",
					"v0.2.0-alpha.0-STRING",
					"v0.2.0-alpha.1",
					"v0.2.0-beta.0-STRING",
					"v0.2.0-beta.1",
					"v0.3.0",
					"v0.4.0-alpha.0",
				},
			},
			wantTarget: ptr("v0.3.0"),
		},
		{
			name: "stable version",
			args: args{
				ch: channel.StableChannel,
				tags: []string{
					"v0.1.0",
					"v0.1.0-STRING",
					"v0.2.0-alpha.0-STRING",
					"v0.2.0-alpha.1",
				},
			},
			wantTarget: ptr("v0.1.0"),
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
