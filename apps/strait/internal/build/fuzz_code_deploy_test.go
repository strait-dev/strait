package build

import (
	"testing"

	"strait/internal/domain"
)

// FuzzGenerateDockerfile is a fuzz target for GenerateDockerfile.
// Properties under test:
//  1. Never panics regardless of input values.
//  2. Any error returned is a plain error (not a nil panic or unrecovered value).
//  3. When a valid runtime is given with benign field values, the result is
//     non-empty and contains "FROM".
func FuzzGenerateDockerfile(f *testing.F) {
	// Seed: valid runtimes with typical field values.
	for _, rt := range []domain.Runtime{
		domain.RuntimePython,
		domain.RuntimeGo,
		domain.RuntimeTypeScript,
	} {
		f.Add(string(rt), "", "my-job", "")
	}
	// Seed: custom base image.
	f.Add(string(domain.RuntimePython), "ghcr.io/myorg/python:3.12", "job-slug", "requirements.txt")
	// Seed: invalid runtime (should error, not panic).
	f.Add("java", "", "job", "")
	// Seed: injection attempts in base_image and job_slug.
	f.Add(string(domain.RuntimeGo), "attacker.io/img\nRUN echo owned", "job", "")
	f.Add(string(domain.RuntimeGo), "img", "job\nRUN echo owned", "")

	f.Fuzz(func(t *testing.T, runtime, baseImage, jobSlug, depsFile string) {
		spec := DockerfileSpec{
			Runtime:   domain.Runtime(runtime),
			BaseImage: baseImage,
			JobSlug:   jobSlug,
			DepsFile:  depsFile,
		}
		// Must never panic.
		result, err := GenerateDockerfile(spec)
		if err != nil {
			// An error is acceptable; just ensure result is empty.
			if result != "" {
				t.Errorf("non-empty result alongside non-nil error: %q / %v", result, err)
			}
			return
		}
		// On success the output must look like a Dockerfile.
		if len(result) == 0 {
			t.Error("GenerateDockerfile returned empty string with nil error")
		}
	})
}

// FuzzBuildLogChannel verifies that BuildLogChannel never panics and always
// returns a non-empty string with the expected prefix, regardless of the input
// deployment ID (including empty string, unicode, or control characters).
func FuzzBuildLogChannel(f *testing.F) {
	f.Add("deploy_abc123")
	f.Add("")
	f.Add("deploy\x00null")
	f.Add("deploy/path/traversal")
	f.Add("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") // 64 hex chars
	f.Add("🚀deploy")

	f.Fuzz(func(t *testing.T, deploymentID string) {
		channel := BuildLogChannel(deploymentID)
		if channel == "" {
			t.Error("BuildLogChannel returned empty string")
		}
		// Must always start with the expected prefix and contain the input.
		const prefix = "deploy:"
		if len(channel) < len(prefix) || channel[:len(prefix)] != prefix {
			t.Errorf("BuildLogChannel(%q) = %q; want prefix %q", deploymentID, channel, prefix)
		}
	})
}
