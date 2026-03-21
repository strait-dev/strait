package extension

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScaffold_CreatesFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := Scaffold("my-ext", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pluginDir := filepath.Join(dir, "my-ext")
	for _, name := range []string{"strait-plugin.json", "main.go", "README.md"} {
		path := filepath.Join(pluginDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s to exist: %v", name, err)
		}
	}
}

func TestScaffold_ManifestValid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := Scaffold("valid-ext", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "valid-ext", "strait-plugin.json"))
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}

	m, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("generated manifest failed validation: %v", err)
	}
	if m.Name != "valid-ext" {
		t.Errorf("expected name %q, got %q", "valid-ext", m.Name)
	}
}

func TestScaffold_DirectoryExists_Errors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Pre-create the directory.
	if err := os.MkdirAll(filepath.Join(dir, "existing"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := Scaffold("existing", dir)
	if err == nil {
		t.Fatal("expected error when directory already exists, got nil")
	}
}
