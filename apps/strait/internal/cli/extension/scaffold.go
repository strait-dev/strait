package extension

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Scaffold creates a new plugin directory with boilerplate files:
// strait-plugin.json, main.go, and README.md.
func Scaffold(name, dir string) error {
	pluginDir := filepath.Join(dir, name)
	if _, err := os.Stat(pluginDir); err == nil {
		return fmt.Errorf("directory %q already exists", pluginDir)
	}

	if err := os.MkdirAll(pluginDir, 0o750); err != nil {
		return fmt.Errorf("creating plugin directory: %w", err)
	}

	if err := writeManifest(pluginDir, name); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}
	if err := writeMainGo(pluginDir, name); err != nil {
		return fmt.Errorf("writing main.go: %w", err)
	}
	if err := writeReadme(pluginDir, name); err != nil {
		return fmt.Errorf("writing README.md: %w", err)
	}

	return nil
}

func writeManifest(dir, name string) error {
	m := PluginManifest{
		Name:        name,
		Version:     "0.1.0",
		Description: fmt.Sprintf("Strait CLI extension: %s", name),
		Commands:    []string{name},
		Hooks:       []string{},
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	return os.WriteFile(filepath.Join(dir, "strait-plugin.json"), append(data, '\n'), 0o600)
}

func writeMainGo(dir, name string) error {
	content := fmt.Sprintf(`package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("strait-%s extension")
	if len(os.Args) > 1 {
		fmt.Printf("args: %%v\n", os.Args[1:])
	}
}
`, name)

	return os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0o600)
}

func writeReadme(dir, name string) error {
	content := fmt.Sprintf(`# strait-%s

A Strait CLI extension.

## Installation

Copy this directory to ~/.config/strait/extensions/%s or add the built
binary to your PATH as strait-%s.

## Usage

    strait extension run %s
`, name, name, name, name)

	return os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0o600)
}
