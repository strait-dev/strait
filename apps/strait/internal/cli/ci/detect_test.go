package ci

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProvider_GitHub(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".github"), 0o750); err != nil {
		t.Fatal(err)
	}

	got := DetectProvider(dir)
	if got != "github" {
		t.Fatalf("expected github, got %q", got)
	}
}

func TestDetectProvider_GitLab(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitlab-ci.yml"), []byte("stages:\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := DetectProvider(dir)
	if got != "gitlab" {
		t.Fatalf("expected gitlab, got %q", got)
	}
}

func TestDetectProvider_CircleCI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".circleci"), 0o750); err != nil {
		t.Fatal(err)
	}

	got := DetectProvider(dir)
	if got != "circleci" {
		t.Fatalf("expected circleci, got %q", got)
	}
}

func TestDetectProvider_None(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	got := DetectProvider(dir)
	if got != "generic" {
		t.Fatalf("expected generic, got %q", got)
	}
}

func TestDetectProvider_Priority(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".github"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitlab-ci.yml"), []byte("stages:\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := DetectProvider(dir)
	if got != "github" {
		t.Fatalf("expected github (priority), got %q", got)
	}
}
