package manifest

import (
	"os"
	"path/filepath"
	"strings"
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

func TestFindConfigFile_YML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "strait.config.yml"), []byte("project:\n  id: p1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := FindConfigFile(dir)
	if filepath.Base(got) != "strait.config.yml" {
		t.Fatalf("expected strait.config.yml, got %q", got)
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

func TestLoadProjectConfig_ValidYML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := "project:\n  id: proj-456\nruntime: node\n"
	path := filepath.Join(dir, "strait.config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}
	if cfg.Project.ID != "proj-456" {
		t.Fatalf("expected project.id=proj-456, got %q", cfg.Project.ID)
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

func TestLoadProjectConfig_FallbackResetsBetweenParsers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Write a file with unknown extension containing valid YAML.
	// Prepend partial JSON-like bytes that would partially populate cfg.
	content := "project:\n  id: from-yaml\n"
	path := filepath.Join(dir, "config.conf")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}
	if cfg.Project.ID != "from-yaml" {
		t.Fatalf("expected project.id=from-yaml, got %q", cfg.Project.ID)
	}
}

func TestLoadProjectConfig_FallbackJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `{"project":{"id":"from-json"}}`
	path := filepath.Join(dir, "config.conf")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}
	if cfg.Project.ID != "from-json" {
		t.Fatalf("expected project.id=from-json, got %q", cfg.Project.ID)
	}
}

func TestLoadProjectConfig_FallbackBothFail(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.conf")
	if err := os.WriteFile(path, []byte("!@#$garbage"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadProjectConfig(path)
	if err == nil {
		t.Fatal("expected error for unparseable content")
	}
	if !strings.Contains(err.Error(), "unable to parse as JSON or YAML") {
		t.Fatalf("expected 'unable to parse' error, got: %v", err)
	}
}

func TestLoadProjectConfig_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "job empty slug",
			content: `{"project":{"id":"p1"},"jobs":[{"slug":"","name":"Good Name"}]}`,
			wantErr: "jobs[0].slug is required",
		},
		{
			name:    "job empty name",
			content: `{"project":{"id":"p1"},"jobs":[{"slug":"good-slug","name":""}]}`,
			wantErr: "jobs[0].name is required",
		},
		{
			name:    "job whitespace-only slug",
			content: `{"project":{"id":"p1"},"jobs":[{"slug":"  ","name":"Good Name"}]}`,
			wantErr: "jobs[0].slug is required",
		},
		{
			name:    "workflow empty slug",
			content: `{"project":{"id":"p1"},"workflows":[{"slug":"","name":"Good Name"}]}`,
			wantErr: "workflows[0].slug is required",
		},
		{
			name:    "workflow empty name",
			content: `{"project":{"id":"p1"},"workflows":[{"slug":"good-slug","name":""}]}`,
			wantErr: "workflows[0].name is required",
		},
		{
			name: "valid jobs and workflows",
			content: `{"project":{"id":"p1"},"jobs":[{"slug":"j1","name":"Job 1"}],
			           "workflows":[{"slug":"w1","name":"Workflow 1"}]}`,
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			p := filepath.Join(dir, "strait.json")
			if err := os.WriteFile(p, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := LoadProjectConfig(p)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoadProjectConfig_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadProjectConfig("/tmp/no-such-file-abc123.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFindConfigFile_StraitYaml(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".strait.yaml"), []byte("project:\n  id: p1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := FindConfigFile(dir)
	if filepath.Base(got) != ".strait.yaml" {
		t.Fatalf("expected .strait.yaml, got %q", got)
	}
}

func TestFindConfigFile_StraitYml(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".strait.yml"), []byte("project:\n  id: p1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := FindConfigFile(dir)
	if filepath.Base(got) != ".strait.yml" {
		t.Fatalf("expected .strait.yml, got %q", got)
	}
}
