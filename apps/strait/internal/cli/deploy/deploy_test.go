package deploy

import (
	"testing"

	"strait/internal/domain"
)

func TestDeployJob_MissingImageAndDockerfile(t *testing.T) {
	t.Parallel()
	opts := DeployOptions{
		JobSlug:    "my-job",
		ImageURI:   "",
		Dockerfile: "",
	}
	err := DeployJob(t.Context(), nil, opts)
	if err == nil {
		t.Fatal("expected error for missing --image and empty dockerfile")
	}
}

func TestDeployJob_DryRun_NoAPICalls(t *testing.T) {
	t.Parallel()
	opts := DeployOptions{
		JobSlug:  "my-job",
		ImageURI: "registry.fly.io/my-app:abc123",
		DryRun:   true,
	}
	// Should not panic even with nil client.
	err := DeployJob(t.Context(), nil, opts)
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
}

func TestDeployJob_InvalidPreset(t *testing.T) {
	t.Parallel()
	err := UpdateJobImage(t.Context(), nil, "my-job", "img:latest", "nonexistent", "")
	if err == nil {
		t.Fatal("expected error for invalid preset")
	}
}

func TestValidPresets(t *testing.T) {
	t.Parallel()
	presets := []string{"micro", "small-1x", "small-2x", "medium-1x", "medium-2x", "large-1x", "large-2x"}
	for _, p := range presets {
		if !domain.MachinePreset(p).IsValid() {
			t.Errorf("expected %q to be valid", p)
		}
	}
}

func TestGitSHA(t *testing.T) {
	t.Parallel()
	sha, err := gitSHA(t.Context())
	if err != nil {
		t.Skipf("git not available: %v", err)
	}
	if len(sha) < 7 {
		t.Errorf("expected SHA >= 7 chars, got %q", sha)
	}
}
