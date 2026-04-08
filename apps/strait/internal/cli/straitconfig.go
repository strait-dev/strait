package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// StraitConfig represents the parsed strait.json project config file.
type StraitConfig struct {
	Schema       string                        `json:"$schema,omitempty"`
	Project      StraitProject                 `json:"project"`
	EndpointURL  string                        `json:"endpoint_url,omitempty"`
	Dirs         []string                      `json:"dirs,omitempty"`
	Deploy       *StraitDeploy                 `json:"deploy,omitempty"`
	Environments map[string]*StraitEnvironment `json:"environments,omitempty"`
	CLI          *StraitCLIConfig              `json:"cli,omitempty"`
}

// StraitProject holds project identity fields from strait.json.
type StraitProject struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// StraitDeploy holds code-first deployment settings from strait.json.
type StraitDeploy struct {
	Runtime      string `json:"runtime,omitempty"`
	BuildCommand string `json:"build_command,omitempty"`
	OutputDir    string `json:"output_dir,omitempty"`
}

// StraitEnvironment holds per-environment overrides.
type StraitEnvironment struct {
	APIURL      string `json:"base_url,omitempty"`
	EndpointURL string `json:"endpoint_url,omitempty"`
	APIKeyEnv   string `json:"api_key_env,omitempty"`
}

// StraitCLIConfig holds CLI-specific preferences from strait.json.
type StraitCLIConfig struct {
	DefaultEnvironment string `json:"default_environment,omitempty"`
	OutputFormat       string `json:"output_format,omitempty"`
}

// ErrStraitConfigNotFound is returned when no strait.json is found in the
// directory tree.
var ErrStraitConfigNotFound = errors.New("strait.json not found — run 'strait init' to create one")

// FindStraitConfig walks up from dir looking for a strait.json file.
// Returns the path to the file and its parent directory.
func FindStraitConfig(startDir string) (configPath, projectDir string, err error) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, "strait.json")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", ErrStraitConfigNotFound
}

// LoadStraitConfig reads and parses the strait.json at path.
func LoadStraitConfig(path string) (*StraitConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read strait.json: %w", err)
	}
	var sc StraitConfig
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parse strait.json: %w", err)
	}
	if sc.Project.ID == "" {
		return nil, fmt.Errorf("strait.json: project.id is required")
	}
	return &sc, nil
}

// EffectiveAPIURL returns the API URL for the given environment, falling back
// through environment config, STRAIT_API_URL, and finally DefaultAPIURL.
func (sc *StraitConfig) EffectiveAPIURL(envName string) string {
	if v := os.Getenv("STRAIT_API_URL"); v != "" {
		return v
	}
	if envName != "" && sc.Environments != nil {
		if env, ok := sc.Environments[envName]; ok && env.APIURL != "" {
			return env.APIURL
		}
	}
	return DefaultAPIURL
}

// EffectiveAPIKeyEnv returns the env var name that holds the API key for the
// given environment. Falls back to "STRAIT_API_KEY".
func (sc *StraitConfig) EffectiveAPIKeyEnv(envName string) string {
	if envName != "" && sc.Environments != nil {
		if env, ok := sc.Environments[envName]; ok && env.APIKeyEnv != "" {
			return env.APIKeyEnv
		}
	}
	return "STRAIT_API_KEY"
}
