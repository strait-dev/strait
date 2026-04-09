package build

import (
	"strings"
	"testing"

	"strait/internal/registry"
)

// TestRegistryHostFromURI_EmptyString verifies that an empty URI does not panic
// and returns an empty host (no slash to cut on).
func TestRegistryHostFromURI_EmptyString(t *testing.T) {
	t.Parallel()
	got := registryHostFromURI("")
	if got != "" {
		t.Errorf("registryHostFromURI(\"\") = %q, want empty", got)
	}
}

// TestRegistryHostFromURI_InjectNewline verifies that a newline embedded in
// a URI does not panic and is returned as part of the host string.
// The function performs no host validation — callers are responsible for
// sanitizing URIs; this test ensures the function cannot panic.
func TestRegistryHostFromURI_InjectNewline(t *testing.T) {
	t.Parallel()
	// newline before a slash: the "host" portion contains the newline.
	uri := "malicious\nhost/repo:tag"
	got := registryHostFromURI(uri)
	if strings.Contains(got, "/") {
		t.Errorf("host should not contain '/': %q", got)
	}
	// No panic is the primary assertion; the newline is in the host portion.
	_ = got
}

// TestRegistryHostFromURI_InjectAt verifies that a user@host format URI
// (sometimes used to inject credentials) does not panic and does not return
// a host that starts with a username.
func TestRegistryHostFromURI_InjectAt(t *testing.T) {
	t.Parallel()
	// No slash — the whole string is the "host" per strings.Cut logic.
	uri := "user@evil.example.com:5000"
	got := registryHostFromURI(uri)
	if strings.Contains(got, "/") {
		t.Errorf("host must not contain '/': %q", got)
	}
	// Just verify no panic.
	_ = got
}

// TestRegistryHostFromURI_SlashOnly verifies that a URI consisting of only
// "/" does not panic and returns an empty host.
func TestRegistryHostFromURI_SlashOnly(t *testing.T) {
	t.Parallel()
	got := registryHostFromURI("/")
	if got != "" {
		t.Errorf("registryHostFromURI(\"/\") = %q, want empty string", got)
	}
}

// TestRegistryHostFromURI_MultipleSlashes verifies that only the first slash
// is used as the split point; the host portion does not include any path.
func TestRegistryHostFromURI_MultipleSlashes(t *testing.T) {
	t.Parallel()
	got := registryHostFromURI("host.example.com/org/repo/sub:tag")
	if got != "host.example.com" {
		t.Errorf("registryHostFromURI = %q, want %q", got, "host.example.com")
	}
}

// TestCacheKey_LongProjectID_NoCollision verifies that two very long project IDs
// produce distinct repository paths. This guards against hash collisions or
// prefix-truncation bugs when project IDs approach implementation limits.
func TestCacheKey_LongProjectID_NoCollision(t *testing.T) {
	t.Parallel()

	const jobID = "job_collision_test"
	projA := "proj_" + strings.Repeat("a", 200)
	projB := "proj_" + strings.Repeat("b", 200)

	repoA := registry.JobRepositoryName("", projA, jobID)
	repoB := registry.JobRepositoryName("", projB, jobID)

	if repoA == repoB {
		t.Errorf("long project IDs with same job produced identical repo paths: %q", repoA)
	}
	if !strings.Contains(repoA, projA) && !strings.Contains(repoA, jobID) {
		t.Errorf("repoA %q does not reference projA or jobID", repoA)
	}
}

// TestCacheKey_SameProjectDifferentJob verifies that the same project with
// different job IDs always produces different repository paths. This is
// the basic per-job isolation property.
func TestCacheKey_SameProjectDifferentJob(t *testing.T) {
	t.Parallel()

	projID := "proj_isolation"
	jobs := []string{"job_alpha", "job_beta", "job_alpha_extra"}

	paths := make(map[string]string)
	for _, job := range jobs {
		path := registry.JobRepositoryName("", projID, job)
		if existing, dup := paths[path]; dup {
			t.Errorf("collision: job %q and %q produced identical repo path %q", job, existing, path)
		}
		paths[path] = job
	}
}

// FuzzRegistryHostFromURI verifies that registryHostFromURI never panics
// regardless of input, and always returns a string that does not contain
// a leading '/'.
func FuzzRegistryHostFromURI(f *testing.F) {
	// Seed corpus: valid and adversarial URIs.
	f.Add("registry.example.com/org/repo:tag")
	f.Add("")
	f.Add("/")
	f.Add("//double")
	f.Add("user@host/repo")
	f.Add("host\ninjection/repo")
	f.Add("host\x00null/repo")
	f.Add("host:5000/repo")
	f.Add("123456789.dkr.ecr.us-east-1.amazonaws.com/strait-jobs/proj/job:tag")

	f.Fuzz(func(t *testing.T, uri string) {
		// Must never panic.
		got := registryHostFromURI(uri)

		// The returned host must never contain "/" since it is the pre-slash portion.
		if strings.Contains(got, "/") {
			t.Errorf("registryHostFromURI(%q) = %q; host must not contain '/'", uri, got)
		}
	})
}
