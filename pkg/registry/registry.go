package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/client"
	"github.com/docker/distribution/registry/client/auth"
	"github.com/docker/distribution/registry/client/auth/challenge"
	"github.com/docker/distribution/registry/client/transport"
	digest "github.com/opencontainers/go-digest"
)

type Repository struct {
	repository distribution.Repository
}

type CredentialStore struct {
	username      string
	password      string
	refreshTokens map[string]string
}

type TagFilter interface {
	Filter([]string) (string, error)
}

func (c *CredentialStore) Basic(*url.URL) (string, string) {
	return c.username, c.password
}

func (c *CredentialStore) RefreshToken(u *url.URL, service string) string {
	return c.refreshTokens[service]
}

func (c *CredentialStore) SetRefreshToken(u *url.URL, service string, token string) {
	if c.refreshTokens != nil {
		c.refreshTokens[service] = token
	}
}

// ping pings the provided endpoint to determine its required authorization challenges.
// If a version header is provided, the versions will be returned.
func Ping(manager challenge.Manager, endpoint, versionHeader string) ([]auth.APIVersion, error) {
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := manager.AddResponse(resp); err != nil {
		return nil, err
	}

	return auth.APIVersions(resp, versionHeader), err
}

func New(base, name string) (*Repository, error) {
	ref, err := reference.WithName(name)
	if err != nil {
		log.Fatal(err)
	}

	manager := challenge.NewSimpleManager()
	handler := auth.NewTokenHandler(http.DefaultTransport, &CredentialStore{}, ref.Name(), "pull")
	authorizer := auth.NewAuthorizer(manager, handler)
	transport := transport.NewTransport(http.DefaultTransport, authorizer)

	versions, err := Ping(manager, base+"/v2/", "Docker-Distribution-Api-Version")
	if err != nil {
		return nil, err
	}
	if len(versions) != 1 {
		return nil, fmt.Errorf("Unexpected version count: %d, expected 1", len(versions))
	}
	if check := (auth.APIVersion{Type: "registry", Version: "2.0"}); versions[0] != check {
		return nil, fmt.Errorf("Unexpected api version: %q, expected %q", versions[0], check)
	}

	r, err := client.NewRepository(ref, base, transport)

	return &Repository{r}, nil
}

type Configuration struct {
	Config struct {
		Labels map[string]string `json:"labels"`
	} `json:"config"`
}

func (r *Repository) Configuration(dgst digest.Digest) (*Configuration, error) {
	blobs := r.repository.Blobs(context.Background())

	b, err := blobs.Get(context.Background(), dgst)
	if err != nil {
		return nil, err
	}

	m := &Configuration{}
	if err := json.Unmarshal(b, m); err != nil {
		return nil, err
	}

	return m, nil
}

func (r *Repository) Manifest(tag string) (*distribution.Descriptor, error) {
	tags := r.repository.Tags(context.Background())
	descriptor, err := tags.Get(context.Background(), tag)
	if err != nil {
		return nil, err
	}

	manifests, err := r.repository.Manifests(context.Background())
	if err != nil {
		return nil, err
	}

	manifest, err := manifests.Get(context.Background(), descriptor.Digest, distribution.WithTagOption{Tag: tag}, distribution.WithManifestMediaTypesOption{MediaTypes: []string{schema2.MediaTypeManifest}})
	if err != nil {
		return nil, err
	}

	if len(manifest.References()) == 0 {
		return nil, fmt.Errorf("expected at least 1 manifest")
	}

	return &manifest.References()[0], nil
}

func (r *Repository) Tags() ([]string, error) {
	tags := r.repository.Tags(context.Background())
	all, err := tags.All(context.Background())
	if err != nil {
		return nil, err
	}

	return all, nil
}

func (r *Repository) Tag(filter TagFilter) (string, error) {
	all, err := r.Tags()
	if err != nil {
		return "", err
	}

	return filter.Filter(all)
}
