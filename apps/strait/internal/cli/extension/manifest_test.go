package extension

import (
	"testing"
)

func TestParseManifest_Valid(t *testing.T) {
	t.Parallel()

	data := []byte(`{
		"name": "my-plugin",
		"version": "1.0.0",
		"description": "A test plugin",
		"commands": ["greet"],
		"hooks": ["pre-deploy"]
	}`)

	m, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "my-plugin" {
		t.Errorf("expected name %q, got %q", "my-plugin", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("expected version %q, got %q", "1.0.0", m.Version)
	}
	if len(m.Commands) != 1 || m.Commands[0] != "greet" {
		t.Errorf("expected commands [greet], got %v", m.Commands)
	}
	if len(m.Hooks) != 1 || m.Hooks[0] != "pre-deploy" {
		t.Errorf("expected hooks [pre-deploy], got %v", m.Hooks)
	}
}

func TestParseManifest_MissingName(t *testing.T) {
	t.Parallel()

	data := []byte(`{"commands": ["hello"]}`)

	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

func TestParseManifest_InvalidJSON(t *testing.T) {
	t.Parallel()

	data := []byte(`{not json}`)

	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseManifest_EmptyCommands(t *testing.T) {
	t.Parallel()

	data := []byte(`{"name": "test", "commands": []}`)

	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("expected error for empty commands, got nil")
	}
}

func TestValidateManifest_ValidHooks(t *testing.T) {
	t.Parallel()

	m := &PluginManifest{
		Name:     "test",
		Commands: []string{"cmd"},
		Hooks:    []string{"pre-deploy", "post-deploy", "pre-trigger", "post-trigger", "pre-build", "post-build"},
	}

	if err := ValidateManifest(m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateManifest_InvalidHook(t *testing.T) {
	t.Parallel()

	m := &PluginManifest{
		Name:     "test",
		Commands: []string{"cmd"},
		Hooks:    []string{"pre-deploy", "on-crash"},
	}

	err := ValidateManifest(m)
	if err == nil {
		t.Fatal("expected error for unknown hook, got nil")
	}
}

func TestValidateManifest_NoCommands(t *testing.T) {
	t.Parallel()

	m := &PluginManifest{
		Name: "test",
	}

	err := ValidateManifest(m)
	if err == nil {
		t.Fatal("expected error for no commands, got nil")
	}
}
