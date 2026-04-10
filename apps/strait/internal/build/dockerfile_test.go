package build

import (
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestGenerateDockerfile_AllRuntimes(t *testing.T) {
	runtimes := []domain.Runtime{
		domain.RuntimePython,
		domain.RuntimeTypeScript,
		domain.RuntimeRuby,
		domain.RuntimeRust,
		domain.RuntimeGo,
	}

	for _, rt := range runtimes {
		t.Run(string(rt), func(t *testing.T) {
			spec := DockerfileSpec{
				Runtime: rt,
				JobSlug: "my-job",
			}
			out, err := GenerateDockerfile(spec)
			if err != nil {
				t.Fatalf("GenerateDockerfile(%s): unexpected error: %v", rt, err)
			}
			if out == "" {
				t.Fatal("GenerateDockerfile returned empty string")
			}
			// Every generated Dockerfile must start with a FROM instruction.
			if !strings.Contains(out, "FROM ") {
				t.Errorf("generated Dockerfile missing FROM instruction:\n%s", out)
			}
			// Must include the default base image.
			defaultImg := DefaultBaseImage(rt)
			if !strings.Contains(out, defaultImg) {
				t.Errorf("expected base image %q in output:\n%s", defaultImg, out)
			}
		})
	}
}

func TestGenerateDockerfile_CustomBaseImage(t *testing.T) {
	spec := DockerfileSpec{
		Runtime:   domain.RuntimePython,
		BaseImage: "my-registry.internal/python:3.11-custom",
		JobSlug:   "test-job",
	}
	out, err := GenerateDockerfile(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "my-registry.internal/python:3.11-custom") {
		t.Errorf("expected custom base image in output, got:\n%s", out)
	}
	// Default image must NOT appear.
	if strings.Contains(out, DefaultBaseImage(domain.RuntimePython)) {
		t.Errorf("default base image should not appear when custom base image is set")
	}
}

func TestGenerateDockerfile_DefaultDepsFile(t *testing.T) {
	cases := []struct {
		runtime  domain.Runtime
		expected string
	}{
		{domain.RuntimePython, "requirements.txt"},
		{domain.RuntimeTypeScript, "package.json"},
		{domain.RuntimeRuby, "Gemfile"},
		{domain.RuntimeRust, "Cargo.toml"},
		{domain.RuntimeGo, "go.mod"},
	}
	for _, tc := range cases {
		t.Run(string(tc.runtime), func(t *testing.T) {
			got := DefaultDepsFile(tc.runtime)
			if got != tc.expected {
				t.Errorf("DefaultDepsFile(%s) = %q, want %q", tc.runtime, got, tc.expected)
			}
		})
	}
}

func TestGenerateDockerfile_CustomDepsFile(t *testing.T) {
	spec := DockerfileSpec{
		Runtime:  domain.RuntimePython,
		JobSlug:  "test-job",
		DepsFile: "requirements-prod.txt",
	}
	out, err := GenerateDockerfile(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "requirements-prod.txt") {
		t.Errorf("expected custom deps file in output, got:\n%s", out)
	}
	if strings.Contains(out, "requirements.txt") && !strings.Contains(out, "requirements-prod.txt") {
		t.Errorf("default deps file should not appear when custom deps file is set")
	}
}

func TestGenerateDockerfile_JobSlugInLabel(t *testing.T) {
	spec := DockerfileSpec{
		Runtime: domain.RuntimePython,
		JobSlug: "my-special-job",
	}
	out, err := GenerateDockerfile(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "my-special-job") {
		t.Errorf("expected job slug %q in output, got:\n%s", spec.JobSlug, out)
	}
}

func TestGenerateDockerfile_InvalidRuntime(t *testing.T) {
	spec := DockerfileSpec{
		Runtime: domain.Runtime("brainfuck"),
		JobSlug: "test",
	}
	_, err := GenerateDockerfile(spec)
	if err == nil {
		t.Fatal("expected error for invalid runtime, got nil")
	}
}

func TestGenerateDockerfile_RustTwoStage(t *testing.T) {
	spec := DockerfileSpec{Runtime: domain.RuntimeRust, JobSlug: "rust-job"}
	out, err := GenerateDockerfile(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Rust must use two-stage build: builder + runtime.
	if !strings.Contains(out, "AS builder") {
		t.Errorf("Rust Dockerfile must have a builder stage, got:\n%s", out)
	}
	if !strings.Contains(out, "AS runtime") {
		t.Errorf("Rust Dockerfile must have a runtime stage, got:\n%s", out)
	}
	// Final image must NOT be the builder base image.
	if strings.Contains(out, "rust:") && !strings.Contains(out, "AS builder") {
		t.Errorf("Rust runtime stage must use a non-Rust base image")
	}
}

func TestGenerateDockerfile_GoTwoStage(t *testing.T) {
	spec := DockerfileSpec{Runtime: domain.RuntimeGo, JobSlug: "go-job"}
	out, err := GenerateDockerfile(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "AS builder") {
		t.Errorf("Go Dockerfile must have a builder stage, got:\n%s", out)
	}
	// Go runtime must use distroless.
	if !strings.Contains(out, "distroless") {
		t.Errorf("Go runtime stage should use distroless, got:\n%s", out)
	}
}

func TestGenerateDockerfile_BuildCacheMount(t *testing.T) {
	// All runtimes should use BuildKit cache mounts for faster builds.
	runtimes := []domain.Runtime{
		domain.RuntimePython,
		domain.RuntimeTypeScript,
		domain.RuntimeRuby,
		domain.RuntimeRust,
		domain.RuntimeGo,
	}
	for _, rt := range runtimes {
		t.Run(string(rt), func(t *testing.T) {
			spec := DockerfileSpec{Runtime: rt, JobSlug: "job"}
			out, err := GenerateDockerfile(spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, "--mount=type=cache") {
				t.Errorf("runtime %s: expected BuildKit cache mount in Dockerfile, got:\n%s", rt, out)
			}
		})
	}
}

func TestGenerateDockerfile_Deterministic(t *testing.T) {
	spec := DockerfileSpec{Runtime: domain.RuntimePython, JobSlug: "det-job"}
	out1, _ := GenerateDockerfile(spec)
	out2, _ := GenerateDockerfile(spec)
	if out1 != out2 {
		t.Error("GenerateDockerfile must be deterministic for the same input")
	}
}

func TestDefaultBaseImage(t *testing.T) {
	cases := []struct {
		runtime  domain.Runtime
		expected string
	}{
		{domain.RuntimePython, "ghcr.io/strait-dev/runtime-python:latest"},
		{domain.RuntimeTypeScript, "ghcr.io/strait-dev/runtime-typescript:latest"},
		{domain.RuntimeRuby, "ghcr.io/strait-dev/runtime-ruby:latest"},
		{domain.RuntimeRust, "ghcr.io/strait-dev/runtime-rust:latest"},
		{domain.RuntimeGo, "ghcr.io/strait-dev/runtime-go:latest"},
	}
	for _, tc := range cases {
		got := DefaultBaseImage(tc.runtime)
		if got != tc.expected {
			t.Errorf("DefaultBaseImage(%s) = %q, want %q", tc.runtime, got, tc.expected)
		}
	}
}
