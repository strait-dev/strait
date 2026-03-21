package extension

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstalledPlugin describes a plugin present in the extensions directory.
type InstalledPlugin struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Path     string   `json:"path"`
	Commands []string `json:"commands"`
}

// ExtensionsDir returns the default directory where CLI extensions are stored.
func ExtensionsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "strait", "extensions")
	}
	return filepath.Join(home, ".config", "strait", "extensions")
}

// ListInstalled reads the extensions directory and returns metadata for each
// installed plugin that has a valid strait-plugin.json manifest.
func ListInstalled(dir string) ([]InstalledPlugin, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading extensions directory: %w", err)
	}

	var plugins []InstalledPlugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(dir, entry.Name(), "strait-plugin.json")
		data, err := os.ReadFile(manifestPath) //nolint:gosec // extension dir from config
		if err != nil {
			continue
		}

		m, err := ParseManifest(data)
		if err != nil {
			continue
		}

		plugins = append(plugins, InstalledPlugin{
			Name:     m.Name,
			Version:  m.Version,
			Path:     filepath.Join(dir, entry.Name()),
			Commands: m.Commands,
		})
	}

	return plugins, nil
}

// Install validates the source and prepares for plugin installation.
// Currently this is a stub that validates the source looks like a GitHub URL
// or a local file path.
func Install(_ context.Context, source string) error {
	if source == "" {
		return fmt.Errorf("install source is required")
	}

	isGitHub := strings.HasPrefix(source, "https://github.com/") ||
		strings.HasPrefix(source, "github.com/")
	isPath := strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../")

	if !isGitHub && !isPath {
		return fmt.Errorf("invalid install source %q: must be a github.com URL or local file path", source)
	}

	return nil
}

// Remove deletes an installed extension by name from the given directory.
func Remove(dir, name string) error {
	if name == "" {
		return fmt.Errorf("extension name is required")
	}
	if strings.ContainsAny(name, "/\\") || name == ".." || name == "." {
		return fmt.Errorf("extension name %q contains invalid path characters", name)
	}

	pluginPath := filepath.Join(dir, name)

	// Verify resolved path stays within extensions directory.
	absDir, _ := filepath.Abs(dir)
	absPlugin, _ := filepath.Abs(pluginPath)
	if !strings.HasPrefix(absPlugin, absDir+string(filepath.Separator)) {
		return fmt.Errorf("extension name %q resolves outside extensions directory", name)
	}
	info, err := os.Stat(pluginPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("extension %q is not installed", name)
		}
		return fmt.Errorf("checking extension %q: %w", name, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("extension %q is not a valid plugin directory", name)
	}

	if err := os.RemoveAll(pluginPath); err != nil {
		return fmt.Errorf("removing extension %q: %w", name, err)
	}
	return nil
}
