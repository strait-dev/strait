// Package cli provides client-side utilities for the strait CLI: config file
// management, HTTP client, device-code auth, source packing, and SSE streaming.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DefaultAPIURL is the production API endpoint used when no profile overrides it.
	DefaultAPIURL = "https://api.strait.dev"

	configDirName  = "strait"
	configFileName = "config.json"
)

// Profile holds the credentials and endpoint for one named environment.
type Profile struct {
	APIURL    string `json:"api_url"`
	APIKey    string `json:"api_key"`
	ProjectID string `json:"project_id,omitempty"`
}

// Config is the on-disk representation of ~/.config/strait/config.json.
type Config struct {
	Profiles      map[string]*Profile `json:"profiles"`
	ActiveProfile string              `json:"active_profile"`
}

// ActiveProfileData returns the active profile, applying environment variable
// overrides (STRAIT_API_KEY, STRAIT_API_URL) on top of the stored values.
// Returns a zero-value Profile (with DefaultAPIURL) if no profile is configured.
func (c *Config) ActiveProfileData(profileName string) *Profile {
	name := profileName
	if name == "" {
		name = c.ActiveProfile
	}
	if name == "" {
		name = "default"
	}

	p := &Profile{APIURL: DefaultAPIURL}
	if c.Profiles != nil {
		if stored, ok := c.Profiles[name]; ok {
			*p = *stored
		}
	}

	// Environment variables always win.
	if v := os.Getenv("STRAIT_API_URL"); v != "" {
		p.APIURL = v
	}
	if v := os.Getenv("STRAIT_API_KEY"); v != "" {
		p.APIKey = v
	}
	if p.APIURL == "" {
		p.APIURL = DefaultAPIURL
	}
	return p
}

// SetProfile upserts a named profile and, if it is the first profile, sets it
// as the active profile.
func (c *Config) SetProfile(name string, p *Profile) {
	if c.Profiles == nil {
		c.Profiles = make(map[string]*Profile)
	}
	c.Profiles[name] = p
	if c.ActiveProfile == "" {
		c.ActiveProfile = name
	}
}

// RemoveProfile deletes a named profile. If it was the active profile the
// active_profile field is cleared.
func (c *Config) RemoveProfile(name string) {
	if c.Profiles != nil {
		delete(c.Profiles, name)
	}
	if c.ActiveProfile == name {
		c.ActiveProfile = ""
	}
}

// ConfigPath returns the path to the config file.
// The STRAIT_CONFIG environment variable overrides the default location.
func ConfigPath() (string, error) {
	if v := os.Getenv("STRAIT_CONFIG"); v != "" {
		return v, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, configDirName, configFileName), nil
}

// LoadConfig reads the config file from disk. Returns an empty Config if the
// file does not exist yet.
func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// SaveConfig writes the config to disk, creating parent directories as needed.
func SaveConfig(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Write atomically via a temp file in the same directory.
	tmp, err := os.CreateTemp(filepath.Dir(path), "config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}
