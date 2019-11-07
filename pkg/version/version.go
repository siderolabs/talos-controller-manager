package version

import (
	"log"
	"sync"
	"time"

	"github.com/talos-systems/talos-controller-manager/pkg/channel"
	"github.com/talos-systems/talos-controller-manager/pkg/channel/filter"
	"github.com/talos-systems/talos-controller-manager/pkg/registry"
)

type Version struct {
	Cache

	synced chan struct{}
}

func NewVersion(cache Cache) *Version {
	return &Version{
		Cache:  cache,
		synced: make(chan struct{}, 1),
	}
}

func (v Version) WaitForCacheSync() bool {
	select {
	case <-v.synced:
		return true
	case <-time.After(time.Minute):
		return false
	}
}

func (v Version) Run(reg, repository string, channels []channel.Channel) error {
	repo, err := registry.New(reg, repository)
	if err != nil {
		return err
	}

	for {
		tags, err := repo.Tags()
		if err != nil {
			return err
		}

		var wg sync.WaitGroup

		wg.Add(len(channels))

		for _, channel := range channels {
			go func(c string) {
				defer wg.Done()
				v.discover(c, reg, repository, tags)
			}(channel)
		}

		wg.Wait()

		v.synced <- struct{}{}

		time.Sleep(5 * time.Minute)
	}
}

func (v Version) discover(c, reg, repo string, tags []string) {
	var found *string
	switch c {
	case channel.LatestChannel:
		found = filter.FilterTagsFor(channel.LatestChannel, reg, repo)
	case channel.EdgeChannel:
		found = filter.FilterTagsFor(channel.EdgeChannel, reg, repo)
	case channel.AlphaChannel, channel.BetaChannel, channel.StableChannel:
		found = filter.FilterSemver(c, tags)
	default:
		log.Printf("%v", channel.NewInvalidChannelError(c))
		return
	}

	if found == nil {
		return
	}

	if *found == "" {
		return
	}

	// No change in version.
	version, ok := v.Get(c)
	if ok && version == *found {
		return
	}

	// A new tag has been detected, update the cache.
	v.Set(c, *found)
}
