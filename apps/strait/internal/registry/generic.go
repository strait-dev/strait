package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// GenericConfig holds configuration for a Docker Registry API v2 compatible registry.
// Compatible with Harbor, GHCR, self-hosted registries, and Docker Hub.
type GenericConfig struct {
	// RegistryURL is the base URL of the registry (e.g. "https://harbor.example.com").
	RegistryURL string
	// Username and Password are the registry credentials.
	Username string
	Password string
	// RepositoryPrefix is prepended to all repository names.
	// Defaults to "strait-jobs" when empty.
	RepositoryPrefix string
}

// GenericRegistry implements ContainerRegistry using the Docker Registry API v2.
// Repository creation is not possible via the standard API — repositories are
// created implicitly on first push. EnsureRepository returns the push URI.
type GenericRegistry struct {
	registryURL      string
	username         string
	password         string
	repositoryPrefix string
	client           *http.Client
}

// NewGenericRegistry creates a new GenericRegistry from cfg.
func NewGenericRegistry(cfg GenericConfig) (*GenericRegistry, error) {
	if cfg.RegistryURL == "" {
		return nil, errors.New("registry: registry URL is required")
	}
	prefix := cfg.RepositoryPrefix
	if prefix == "" {
		prefix = "strait-jobs"
	}
	return &GenericRegistry{
		registryURL:      strings.TrimRight(cfg.RegistryURL, "/"),
		username:         cfg.Username,
		password:         cfg.Password,
		repositoryPrefix: prefix,
		client:           &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// EnsureRepository returns the push URI for the named repository.
// Docker registries create repositories implicitly on first push, so this
// method only constructs and validates the URI without making any API calls.
func (g *GenericRegistry) EnsureRepository(_ context.Context, name string) (string, error) {
	host := strings.TrimPrefix(strings.TrimPrefix(g.registryURL, "https://"), "http://")
	repoName := g.repositoryPrefix + "/" + name
	return host + "/" + repoName, nil
}

// GetAuthToken returns a "username:password" credential string.
// Static credentials don't expire; the returned time is a far-future sentinel.
func (g *GenericRegistry) GetAuthToken(_ context.Context) (string, time.Time, error) {
	return g.username + ":" + g.password, time.Now().Add(87600 * time.Hour), nil
}

// GetImageDigest fetches the digest of the image at repositoryURI:tag using
// the Docker Registry API v2 HEAD /v2/{name}/manifests/{tag} endpoint.
func (g *GenericRegistry) GetImageDigest(ctx context.Context, repositoryURI, tag string) (string, error) {
	// repositoryURI is "registry.example.com/prefix/project/job"
	// Extract the name part after the registry host.
	host := strings.TrimPrefix(strings.TrimPrefix(g.registryURL, "https://"), "http://")
	repoName := strings.TrimPrefix(repositoryURI, host+"/")

	url := g.registryURL + "/v2/" + repoName + "/manifests/" + tag

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", fmt.Errorf("registry: build manifest request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	if g.username != "" {
		req.SetBasicAuth(g.username, g.password)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("registry: manifest request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrImageNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry: manifest request: unexpected status %d", resp.StatusCode)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("registry: manifest response missing Docker-Content-Digest header")
	}
	return digest, nil
}

// DockerConfigJSON returns a Docker config.json-compatible JSON blob suitable
// for use as a BuildKit secret mount or imagePullSecret.
func (g *GenericRegistry) DockerConfigJSON() ([]byte, error) {
	host := strings.TrimPrefix(strings.TrimPrefix(g.registryURL, "https://"), "http://")
	auth := map[string]any{
		"auths": map[string]any{
			host: map[string]string{
				"auth": base64.StdEncoding.EncodeToString([]byte(g.username + ":" + g.password)),
			},
		},
	}
	return json.Marshal(auth)
}
