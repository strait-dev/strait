package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindConfigFile_JSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "strait.json"), []byte(`{"project":{"id":"p1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got := FindConfigFile(dir)
	if filepath.Base(got) != "strait.json" {
		t.Fatalf("expected strait.json, got %q", got)
	}
}

func TestFindConfigFile_YAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "strait.config.yaml"), []byte("project:\n  id: p1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := FindConfigFile(dir)
	if filepath.Base(got) != "strait.config.yaml" {
		t.Fatalf("expected strait.config.yaml, got %q", got)
	}
}

func TestFindConfigFile_Priority(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "strait.json"), []byte(`{"project":{"id":"p1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "strait.config.yaml"), []byte("project:\n  id: p1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := FindConfigFile(dir)
	if filepath.Base(got) != "strait.json" {
		t.Fatalf("expected JSON to be preferred, got %q", got)
	}
}

func TestFindConfigFile_NotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	got := FindConfigFile(dir)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestLoadProjectConfig_ValidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `{
		"project": {"id": "proj-123", "name": "My Project"},
		"runtime": "node",
		"jobs": [{"slug": "process-payment", "name": "Process Payment"}],
		"workflows": [{"slug": "order-pipeline", "name": "Order Pipeline"}]
	}`
	path := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}
	if cfg.Project.ID != "proj-123" {
		t.Fatalf("expected project.id=proj-123, got %q", cfg.Project.ID)
	}
	if cfg.Runtime != "node" {
		t.Fatalf("expected runtime=node, got %q", cfg.Runtime)
	}
	if len(cfg.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(cfg.Jobs))
	}
	if len(cfg.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(cfg.Workflows))
	}
}

func TestLoadProjectConfig_ValidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `
project:
  id: proj-123
  name: My Project
runtime: node
jobs:
  - slug: process-payment
    name: Process Payment
`
	path := filepath.Join(dir, "strait.config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}
	if cfg.Project.ID != "proj-123" {
		t.Fatalf("expected project.id=proj-123, got %q", cfg.Project.ID)
	}
}

func TestLoadProjectConfig_MissingProjectID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `{"project": {"name": "No ID"}}`
	path := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadProjectConfig(path)
	if err == nil {
		t.Fatal("expected error for missing project.id")
	}
}

func TestLoadProjectConfig_InvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(path, []byte(`{invalid`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadProjectConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadProjectConfig_EmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadProjectConfig(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}
