// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package version

type GetOptions struct{}

type GetOption func(*GetOptions)

func NewGetOptions(setters ...GetOption) *GetOptions {
	opts := &GetOptions{}

	for _, setter := range setters {
		setter(opts)
	}

	return opts
}
