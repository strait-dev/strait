package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig is a test helper that writes a YAML config file and returns its path.
func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "strait.config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeDockerfile is a test helper that creates a placeholder Dockerfile.
func writeDockerfile(t *testing.T, dir, relPath string) {
	t.Helper()
	abs := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDeployConfig_ValidMultiJob(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeDockerfile(t, dir, "Dockerfile")
	writeDockerfile(t, dir, "worker/Dockerfile")

	cfg := `
version: 1
project: proj-123
registry: registry.fly.io/my-app
jobs:
  - slug: data-processor
    dockerfile: ` + filepath.Join(dir, "Dockerfile") + `
    preset: small-1x
    region: iad
    build_args:
      NODE_ENV: production
  - slug: report-gen
    dockerfile: ` + filepath.Join(dir, "worker/Dockerfile") + `
    preset: micro
`
	path := writeConfig(t, dir, cfg)

	dc, err := LoadDeployConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dc.Version != 1 {
		t.Errorf("version = %d, want 1", dc.Version)
	}
	if dc.Project != "proj-123" {
		t.Errorf("project = %q, want %q", dc.Project, "proj-123")
	}
	if dc.Registry != "registry.fly.io/my-app" {
		t.Errorf("registry = %q, want %q", dc.Registry, "registry.fly.io/my-app")
	}
	if len(dc.Jobs) != 2 {
		t.Fatalf("jobs count = %d, want 2", len(dc.Jobs))
	}

	j0 := dc.Jobs[0]
	if j0.Slug != "data-processor" {
		t.Errorf("jobs[0].slug = %q, want %q", j0.Slug, "data-processor")
	}
	if j0.Preset != "small-1x" {
		t.Errorf("jobs[0].preset = %q, want %q", j0.Preset, "small-1x")
	}
	if j0.Region != "iad" {
		t.Errorf("jobs[0].region = %q, want %q", j0.Region, "iad")
	}
	if j0.BuildArgs["NODE_ENV"] != "production" {
		t.Errorf("jobs[0].build_args[NODE_ENV] = %q, want %q", j0.BuildArgs["NODE_ENV"], "production")
	}

	j1 := dc.Jobs[1]
	if j1.Slug != "report-gen" {
		t.Errorf("jobs[1].slug = %q, want %q", j1.Slug, "report-gen")
	}
	if j1.Preset != "micro" {
		t.Errorf("jobs[1].preset = %q, want %q", j1.Preset, "micro")
	}
	if j1.Region != "" {
		t.Errorf("jobs[1].region = %q, want empty", j1.Region)
	}
}

func TestLoadDeployConfig_InvalidPreset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeDockerfile(t, dir, "Dockerfile")

	cfg := `
version: 1
project: proj-1
registry: r.io/app
jobs:
  - slug: my-job
    dockerfile: ` + filepath.Join(dir, "Dockerfile") + `
    preset: ultra-mega
`
	path := writeConfig(t, dir, cfg)

	_, err := LoadDeployConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid preset")
	}
	if !strings.Contains(err.Error(), "ultra-mega") {
		t.Errorf("error should mention invalid value %q, got: %v", "ultra-mega", err)
	}
}

func TestLoadDeployConfig_DuplicateSlugs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeDockerfile(t, dir, "Dockerfile")
	df := filepath.Join(dir, "Dockerfile")

	cfg := `
version: 1
project: proj-1
registry: r.io/app
jobs:
  - slug: dupe
    dockerfile: ` + df + `
    preset: micro
  - slug: dupe
    dockerfile: ` + df + `
    preset: micro
`
	path := writeConfig(t, dir, cfg)

	_, err := LoadDeployConfig(path)
	if err == nil {
		t.Fatal("expected error for duplicate slugs")
	}
	if !strings.Contains(err.Error(), "duplicate slug") {
		t.Errorf("error should mention duplicate slug, got: %v", err)
	}
}

func TestLoadDeployConfig_MissingDockerfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missingPath := filepath.Join(dir, "nonexistent", "Dockerfile")

	cfg := `
version: 1
project: proj-1
registry: r.io/app
jobs:
  - slug: my-job
    dockerfile: ` + missingPath + `
    preset: micro
`
	path := writeConfig(t, dir, cfg)

	_, err := LoadDeployConfig(path)
	if err == nil {
		t.Fatal("expected error for missing dockerfile")
	}
	if !strings.Contains(err.Error(), missingPath) {
		t.Errorf("error should contain dockerfile path %q, got: %v", missingPath, err)
	}
}

func TestLoadDeployConfig_SingleJob(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeDockerfile(t, dir, "Dockerfile")

	cfg := `
version: 1
project: proj-solo
registry: r.io/solo
jobs:
  - slug: only-job
    dockerfile: ` + filepath.Join(dir, "Dockerfile") + `
    preset: medium-2x
    region: ord
`
	path := writeConfig(t, dir, cfg)

	dc, err := LoadDeployConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dc.Jobs) != 1 {
		t.Fatalf("jobs count = %d, want 1", len(dc.Jobs))
	}
	if dc.Jobs[0].Slug != "only-job" {
		t.Errorf("slug = %q, want %q", dc.Jobs[0].Slug, "only-job")
	}
}

func TestLoadDeployConfig_UnsupportedVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeDockerfile(t, dir, "Dockerfile")

	cfg := `
version: 99
project: proj-1
registry: r.io/app
jobs:
  - slug: my-job
    dockerfile: ` + filepath.Join(dir, "Dockerfile") + `
    preset: micro
`
	path := writeConfig(t, dir, cfg)

	_, err := LoadDeployConfig(path)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !strings.Contains(err.Error(), "unsupported config version 99") {
		t.Errorf("error should mention version 99, got: %v", err)
	}
}

func TestLoadDeployConfig_EmptyJobs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cfg := `
version: 1
project: proj-1
registry: r.io/app
jobs: []
`
	path := writeConfig(t, dir, cfg)

	_, err := LoadDeployConfig(path)
	if err == nil {
		t.Fatal("expected error for empty jobs list")
	}
	if !strings.Contains(err.Error(), "jobs list must not be empty") {
		t.Errorf("error should mention empty jobs, got: %v", err)
	}
}
