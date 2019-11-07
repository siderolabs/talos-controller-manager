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

		if len(v2.Pre) > 0 {
			// The stable channel forbids prereleases.
			if ch == channel.StableChannel {
				continue
			}
			// Ensure that we are comparing the prerelease with the requested
			// channel.
			pre := v2.Pre[0].String()
			if pre != ch {
				continue
			}
		}

		switch ch {
		case channel.AlphaChannel, channel.BetaChannel:
			if len(v2.Pre) < 2 {
				continue
			}

			if v2.Pre[1].VersionStr != "" {
				continue
			}
		case channel.LatestChannel, channel.EdgeChannel:
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
