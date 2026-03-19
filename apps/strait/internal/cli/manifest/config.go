package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// configFileNames is the ordered list of config file names to search for.
var configFileNames = []string{
	"strait.json",
	"strait.config.yaml",
	"strait.config.yml",
	".strait.yaml",
	".strait.yml",
}

// FindConfigFile searches dir for a config file and returns its path.
// Returns an empty string if no config file is found.
func FindConfigFile(dir string) string {
	for _, name := range configFileNames {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// LoadProjectConfig reads and validates a project config from the given path.
func LoadProjectConfig(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // Path is user-provided CLI config file.
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("config file %s is empty", path)
	}

	var cfg ProjectConfig
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	default:
		// Try JSON first, then YAML.
		if err := json.Unmarshal(data, &cfg); err != nil {
			cfg = ProjectConfig{} // Reset to avoid partial state from failed JSON parse.
			if yamlErr := yaml.Unmarshal(data, &cfg); yamlErr != nil {
				return nil, fmt.Errorf("parse config %s: unable to parse as JSON or YAML", path)
			}
		}
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}

	return &cfg, nil
}

func validateConfig(cfg *ProjectConfig) error {
	if cfg.Project.ID == "" {
		return fmt.Errorf("project.id is required")
	}
	return nil
}
