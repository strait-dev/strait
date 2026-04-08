package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"strait/internal/cli"
)

func TestFindStraitConfig_FoundInCurrentDir(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj_1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, projectDir, err := cli.FindStraitConfig(dir)
	if err != nil {
		t.Fatalf("FindStraitConfig: %v", err)
	}
	if got != configPath {
		t.Errorf("got %q want %q", got, configPath)
	}
	if projectDir != dir {
		t.Errorf("projectDir: got %q want %q", projectDir, dir)
	}
}

func TestFindStraitConfig_FoundInParentDir(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj_1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, _, err := cli.FindStraitConfig(sub)
	if err != nil {
		t.Fatalf("FindStraitConfig from subdir: %v", err)
	}
	if got != configPath {
		t.Errorf("got %q want %q", got, configPath)
	}
}

func TestFindStraitConfig_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, _, err := cli.FindStraitConfig(dir)
	if !errors.Is(err, cli.ErrStraitConfigNotFound) {
		t.Errorf("expected ErrStraitConfigNotFound, got %v", err)
	}
}

func TestLoadStraitConfig_ParsesDeploySection(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	content := `{
		"project": {"id": "proj_abc"},
		"deploy": {"runtime": "python", "build_command": "pip install ."}
	}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sc, err := cli.LoadStraitConfig(configPath)
	if err != nil {
		t.Fatalf("LoadStraitConfig: %v", err)
	}
	if sc.Project.ID != "proj_abc" {
		t.Errorf("project.id: got %q", sc.Project.ID)
	}
	if sc.Deploy == nil {
		t.Fatal("deploy section missing")
	}
	if sc.Deploy.Runtime != "python" {
		t.Errorf("runtime: got %q", sc.Deploy.Runtime)
	}
}

func TestLoadStraitConfig_MissingProjectIDErrors(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := cli.LoadStraitConfig(configPath)
	if err == nil {
		t.Fatal("expected error for missing project.id")
	}
}

func TestEffectiveAPIURL_EnvWins(t *testing.T) {
	t.Setenv("STRAIT_API_URL", "https://custom.example.com")
	sc := &cli.StraitConfig{Project: cli.StraitProject{ID: "p"}}
	if got := sc.EffectiveAPIURL(""); got != "https://custom.example.com" {
		t.Errorf("expected env URL, got %q", got)
	}
}

func TestEffectiveAPIURL_FallsBackToDefault(t *testing.T) {
	_ = os.Unsetenv("STRAIT_API_URL")
	sc := &cli.StraitConfig{Project: cli.StraitProject{ID: "p"}}
	if got := sc.EffectiveAPIURL(""); got != cli.DefaultAPIURL {
		t.Errorf("expected default URL, got %q", got)
	}
}
