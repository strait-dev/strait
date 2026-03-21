// Package extension provides plugin manifest parsing, hook execution,
// and extension lifecycle management for the Strait CLI.
package extension

import (
	"encoding/json"
	"fmt"
	"strings"
)

// KnownHooks lists the hook names that plugins may register.
var KnownHooks = []string{
	"pre-deploy",
	"post-deploy",
	"pre-trigger",
	"post-trigger",
	"pre-build",
	"post-build",
}

// PluginManifest represents the contents of a strait-plugin.json file.
type PluginManifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Commands    []string `json:"commands"`
	Hooks       []string `json:"hooks,omitempty"`
}

// ParseManifest decodes JSON bytes into a PluginManifest and validates it.
func ParseManifest(data []byte) (*PluginManifest, error) {
	var m PluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing plugin manifest: %w", err)
	}
	if err := ValidateManifest(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ValidateManifest checks that a PluginManifest satisfies all constraints:
// name is required, at least one command, and hooks must be from the known set.
func ValidateManifest(m *PluginManifest) error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("plugin manifest: name is required")
	}
	if len(m.Commands) == 0 {
		return fmt.Errorf("plugin manifest: at least one command is required")
	}
	known := make(map[string]bool, len(KnownHooks))
	for _, h := range KnownHooks {
		known[h] = true
	}
	for _, h := range m.Hooks {
		if !known[h] {
			return fmt.Errorf("plugin manifest: unknown hook %q", h)
		}
	}
	return nil
}
