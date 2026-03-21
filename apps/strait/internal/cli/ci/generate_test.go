package ci

import (
	"strings"
	"testing"
)

func TestGenerate_GitHub(t *testing.T) {
	t.Parallel()

	out, err := Generate("github", GenerateConfig{ProjectID: "my-proj", Environment: "production"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "name: Strait Deploy") {
		t.Fatalf("expected GitHub workflow header, got:\n%s", out)
	}
	if !strings.Contains(out, "strait build") {
		t.Fatalf("expected 'strait build' in output:\n%s", out)
	}
	if !strings.Contains(out, "my-proj") {
		t.Fatalf("expected project ID in output:\n%s", out)
	}
}

func TestGenerate_GitLab(t *testing.T) {
	t.Parallel()

	out, err := Generate("gitlab", GenerateConfig{ProjectID: "my-proj", Environment: "production"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "stages:") {
		t.Fatalf("expected GitLab stages, got:\n%s", out)
	}
	if !strings.Contains(out, "validate") {
		t.Fatalf("expected validate stage, got:\n%s", out)
	}
}

func TestGenerate_Generic(t *testing.T) {
	t.Parallel()

	out, err := Generate("generic", GenerateConfig{ProjectID: "my-proj", Environment: "production"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, "#!/bin/bash") {
		t.Fatalf("expected bash shebang, got:\n%s", out)
	}
}

func TestGenerate_UnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := Generate("bitbucket", GenerateConfig{ProjectID: "my-proj", Environment: "production"})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
	if !strings.Contains(err.Error(), "unsupported CI provider") {
		t.Fatalf("expected 'unsupported CI provider' error, got: %v", err)
	}
}

func TestGenerate_InjectsValues(t *testing.T) {
	t.Parallel()

	out, err := Generate("github", GenerateConfig{ProjectID: "proj-abc", Environment: "staging"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "proj-abc") {
		t.Fatalf("expected project ID in output:\n%s", out)
	}
	if !strings.Contains(out, "staging") {
		t.Fatalf("expected environment in output:\n%s", out)
	}
}

func TestGenerate_UnsafeProjectID_DollarBrace(t *testing.T) {
	t.Parallel()

	_, err := Generate("github", GenerateConfig{ProjectID: "proj-${BAD}", Environment: "prod"})
	if err == nil {
		t.Fatal("expected error for unsafe project ID")
	}
	if !strings.Contains(err.Error(), "unsafe characters") {
		t.Fatalf("expected 'unsafe characters' error, got: %v", err)
	}
}

func TestGenerate_UnsafeProjectID_Backtick(t *testing.T) {
	t.Parallel()

	_, err := Generate("github", GenerateConfig{ProjectID: "proj-`whoami`", Environment: "prod"})
	if err == nil {
		t.Fatal("expected error for unsafe project ID")
	}
	if !strings.Contains(err.Error(), "unsafe characters") {
		t.Fatalf("expected 'unsafe characters' error, got: %v", err)
	}
}

func TestGenerate_UnsafeEnvironment(t *testing.T) {
	t.Parallel()

	_, err := Generate("github", GenerateConfig{ProjectID: "proj-1", Environment: "{{.Evil}}"})
	if err == nil {
		t.Fatal("expected error for unsafe environment")
	}
	if !strings.Contains(err.Error(), "unsafe characters") {
		t.Fatalf("expected 'unsafe characters' error, got: %v", err)
	}
}

func TestGenerate_EmptyConfig(t *testing.T) {
	t.Parallel()

	_, err := Generate("github", GenerateConfig{})
	if err != nil {
		t.Fatalf("unexpected error for empty config: %v", err)
	}
}

func TestGenerate_CleanSpecialChars(t *testing.T) {
	t.Parallel()

	_, err := Generate("github", GenerateConfig{ProjectID: "my-proj_123.test", Environment: "prod"})
	if err != nil {
		t.Fatalf("unexpected error for clean special chars: %v", err)
	}
}
