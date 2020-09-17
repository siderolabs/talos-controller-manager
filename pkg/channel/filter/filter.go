// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package filter

import (
	"log"

	"github.com/talos-systems/talos-controller-manager/pkg/channel"
	"github.com/talos-systems/talos-controller-manager/pkg/constants"
	"github.com/talos-systems/talos-controller-manager/pkg/registry"

	"github.com/blang/semver"
	digest "github.com/opencontainers/go-digest"
)

func FilterTagsFor(s, reg, repository string) (target *string) {
	repo, err := registry.New(reg, repository)
	if err != nil {
		log.Println(err)
		return nil
	}

	manifest, err := repo.Manifest(s)
	if err != nil {
		log.Println(err)
		return nil
	}

	dgst := digest.NewDigestFromHex(
		manifest.Digest.Algorithm().String(),
		manifest.Digest.Encoded(),
	)

	config, err := repo.Configuration(dgst)
	if err != nil {
		log.Println(err)
		return nil
	}

	t := config.Config.Labels[constants.InstallerVersionLabel]
	target = &t

	return target
}

// FilterSemver filters a set of tags by enforcing alpha >= beta >= stable.
func FilterSemver(ch string, tags []string) (target *string) {
	v1, _ := semver.New("0.0.0")

	for _, tag := range tags {
		v2, err := semver.ParseTolerant(tag)
		if err != nil {
			continue
		}

		// In the case of a commit SHA that is all numbers, the semver will
		// successfully parse. This is a filter to ensure that we skip this
		// case.
		if v2.Major > 1 {
			continue
		}

		switch ch {
		case channel.StableChannel:
			if len(v2.Pre) > 0 {
				// Filter out all prereleases.
				continue
			}
		case channel.BetaChannel, channel.AlphaChannel:
			// Skip releases that could be X number of commits ahead of a stable
			// release (e.g. v0.1.0-X-gSHA, notice it is missing the alpha/beta).
			if len(v2.Pre) != 2 {
				break
			}

			// If the requested channel is beta, filter out all alphas.
			if ch == channel.BetaChannel && v2.Pre[0].String() == channel.AlphaChannel {
				continue
			}

			// All alpha and beta channels should never have a VersionStr, as that
			// would indicate that the version is of the form
			// major.minor.patch-pre.STRING (e.g. v0.1.0-alpha.0-abc) opposed to
			// major.minor.patch-pre.NUMBER (e.g. v0.1.0-alpha.0). The former means
			// this tag is in the "latest" channel.
			if v2.Pre[1].VersionStr != "" {
				continue
			}
		case channel.LatestChannel, channel.EdgeChannel:
			// Nothing to do.
		}

		if v1.LT(v2) {
			v1 = &v2
		}
	}

	// Ensure that we don't return the target if no tag was found.
	if v1.Major == 0 && v1.Minor == 0 && v1.Patch == 0 {
		return nil
	}

	// The "v" prefix is used in upstream tagging scheme.
	s := "v" + v1.String()
	target = &s

	return target
}
