package compute

import (
	"strings"
	"testing"
)

// TestExtractRegistry_DockerHub verifies bare images default to docker.io.
func TestExtractRegistry_DockerHub(t *testing.T) {
	t.Parallel()
	got := extractRegistry("nginx:latest")
	if got != "docker.io" {
		t.Errorf("extractRegistry(%q) = %q, want %q", "nginx:latest", got, "docker.io")
	}
}

// TestExtractRegistry_ExplicitRegistry verifies an explicit registry is returned.
func TestExtractRegistry_ExplicitRegistry(t *testing.T) {
	t.Parallel()
	got := extractRegistry("ghcr.io/owner/repo:tag")
	if got != "ghcr.io" {
		t.Errorf("extractRegistry(%q) = %q, want %q", "ghcr.io/owner/repo:tag", got, "ghcr.io")
	}
}

// TestExtractRegistry_RegistryWithPort verifies port-bearing registries are preserved.
func TestExtractRegistry_RegistryWithPort(t *testing.T) {
	t.Parallel()
	got := extractRegistry("registry.com:5000/image:tag")
	if got != "registry.com:5000" {
		t.Errorf("extractRegistry(%q) = %q, want %q", "registry.com:5000/image:tag", got, "registry.com:5000")
	}
}

// TestExtractRegistry_DigestURI verifies digest URIs without an explicit registry default to docker.io.
func TestExtractRegistry_DigestURI(t *testing.T) {
	t.Parallel()
	got := extractRegistry("nginx@sha256:abc")
	if got != "docker.io" {
		t.Errorf("extractRegistry(%q) = %q, want %q", "nginx@sha256:abc", got, "docker.io")
	}
}

// TestValidateImageRegistry_Allowed verifies a registry in the allowlist passes.
func TestValidateImageRegistry_Allowed(t *testing.T) {
	t.Parallel()
	err := ValidateImageRegistry("docker.io/library/nginx:latest", []string{"docker.io"})
	if err != nil {
		t.Errorf("ValidateImageRegistry() = %v, want nil", err)
	}
}

// TestValidateImageRegistry_Blocked verifies a registry not in the allowlist is rejected.
func TestValidateImageRegistry_Blocked(t *testing.T) {
	t.Parallel()
	err := ValidateImageRegistry("evil.com/malware:latest", []string{"docker.io", "ghcr.io"})
	if err == nil {
		t.Error("ValidateImageRegistry() = nil, want error for blocked registry")
	}
}

// TestValidateImageRegistry_EmptyAllowlist verifies an empty allowlist allows everything.
func TestValidateImageRegistry_EmptyAllowlist(t *testing.T) {
	t.Parallel()
	err := ValidateImageRegistry("anything.example.com/image:v1", nil)
	if err != nil {
		t.Errorf("ValidateImageRegistry() = %v, want nil for empty allowlist", err)
	}
}

// TestValidateImageRegistry_Wildcard verifies wildcard patterns match subdomains.
func TestValidateImageRegistry_Wildcard(t *testing.T) {
	t.Parallel()
	// Wildcard *.ecr.amazonaws.com should match any ECR subdomain.
	err := ValidateImageRegistry("123.ecr.amazonaws.com/myapp:v1", []string{"*.ecr.amazonaws.com"})
	if err != nil {
		t.Errorf("ValidateImageRegistry() = %v, want nil for wildcard match", err)
	}
}

// TestValidateImageRegistry_CaseInsensitive verifies case-insensitive registry matching.
func TestValidateImageRegistry_CaseInsensitive(t *testing.T) {
	t.Parallel()
	err := ValidateImageRegistry("Docker.IO/library/nginx:latest", []string{"docker.io"})
	if err != nil {
		t.Errorf("ValidateImageRegistry() = %v, want nil for case-insensitive match", err)
	}
}

// TestValidateImageDigest_Present verifies a digest-pinned URI passes.
func TestValidateImageDigest_Present(t *testing.T) {
	t.Parallel()
	err := ValidateImageDigest("nginx@sha256:abc123")
	if err != nil {
		t.Errorf("ValidateImageDigest() = %v, want nil", err)
	}
}

// TestValidateImageDigest_Missing verifies a tag-only URI fails.
func TestValidateImageDigest_Missing(t *testing.T) {
	t.Parallel()
	err := ValidateImageDigest("nginx:latest")
	if err == nil {
		t.Error("ValidateImageDigest() = nil, want error for missing digest")
	}
}

// TestImageURI_CredentialRejected verifies embedded credentials are rejected.
func TestImageURI_CredentialRejected(t *testing.T) {
	t.Parallel()
	err := validateImageURI("user:pass@registry.example.com/image:tag")
	if err == nil {
		t.Error("validateImageURI() = nil, want error for embedded credentials")
	}
	if err != nil && !strings.Contains(err.Error(), "credentials") {
		t.Errorf("validateImageURI() error = %v, want error mentioning credentials", err)
	}
}

// TestImageURI_DigestAllowed verifies digest-pinned URIs are accepted.
func TestImageURI_DigestAllowed(t *testing.T) {
	t.Parallel()
	err := validateImageURI("nginx@sha256:abc123")
	if err != nil {
		t.Errorf("validateImageURI() = %v, want nil for digest URI", err)
	}
}

// TestImageURI_AtWithoutDigest verifies non-digest @ usage is rejected.
func TestImageURI_AtWithoutDigest(t *testing.T) {
	t.Parallel()
	err := validateImageURI("name@registry")
	if err == nil {
		t.Error("validateImageURI() = nil, want error for @ without sha256 digest")
	}
}

// FuzzExtractRegistry fuzzes the registry extraction function.
func FuzzExtractRegistry(f *testing.F) {
	f.Add("nginx:latest")
	f.Add("ghcr.io/owner/repo:tag")
	f.Add("registry.com:5000/image:tag")
	f.Add("nginx@sha256:abc123")
	f.Add("")
	f.Add("docker.io/library/nginx")
	f.Fuzz(func(t *testing.T, uri string) {
		result := extractRegistry(uri)
		// The result must not be empty.
		if result == "" {
			t.Errorf("extractRegistry(%q) returned empty string", uri)
		}
	})
}

// FuzzValidateImageRegistry fuzzes registry validation with a fixed allowlist.
func FuzzValidateImageRegistry(f *testing.F) {
	f.Add("nginx:latest")
	f.Add("ghcr.io/owner/repo:tag")
	f.Add("evil.com/malware:latest")
	f.Add("")
	f.Fuzz(func(t *testing.T, uri string) {
		allowlist := []string{"docker.io", "ghcr.io", "*.ecr.amazonaws.com"}
		// Must not panic.
		_ = ValidateImageRegistry(uri, allowlist)
	})
}
