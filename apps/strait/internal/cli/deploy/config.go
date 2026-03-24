package deploy

import (
	"fmt"
	"os"
	"strings"

	"strait/internal/domain"

	"gopkg.in/yaml.v3"
)

// DeployConfig represents the top-level strait.config.yaml structure.
type DeployConfig struct {
	Version  int               `yaml:"version"`
	Project  string            `yaml:"project"`
	Registry string            `yaml:"registry"`
	Jobs     []DeployJobConfig `yaml:"jobs"`
}

// DeployJobConfig represents a single job entry within the deploy config.
type DeployJobConfig struct {
	Slug       string            `yaml:"slug"`
	Dockerfile string            `yaml:"dockerfile"`
	Preset     string            `yaml:"preset"`
	Region     string            `yaml:"region"`
	BuildArgs  map[string]string `yaml:"build_args"`
}

// LoadDeployConfig reads and validates a strait.config.yaml file.
func LoadDeployConfig(path string) (*DeployConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg DeployConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := cfg.validate(path); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate checks all invariants on a parsed DeployConfig.
func (c *DeployConfig) validate(configPath string) error {
	var errs []string

	if c.Version != 1 {
		errs = append(errs, fmt.Sprintf("unsupported config version %d, expected 1", c.Version))
	}

	if len(c.Jobs) == 0 {
		errs = append(errs, "jobs list must not be empty")
	}

	slugs := make(map[string]struct{}, len(c.Jobs))
	for i, job := range c.Jobs {
		prefix := fmt.Sprintf("jobs[%d]", i)

		if job.Slug == "" {
			errs = append(errs, fmt.Sprintf("%s: slug is required", prefix))
		} else if _, exists := slugs[job.Slug]; exists {
			errs = append(errs, fmt.Sprintf("%s: duplicate slug %q", prefix, job.Slug))
		} else {
			slugs[job.Slug] = struct{}{}
		}

		if job.Dockerfile == "" {
			errs = append(errs, fmt.Sprintf("%s (%s): dockerfile is required", prefix, job.Slug))
		} else if _, err := os.Stat(job.Dockerfile); err != nil {
			errs = append(errs, fmt.Sprintf("%s (%s): dockerfile not found: %s", prefix, job.Slug, job.Dockerfile))
		}

		if job.Preset != "" && !domain.MachinePreset(job.Preset).IsValid() {
			errs = append(errs, fmt.Sprintf("%s (%s): invalid preset %q", prefix, job.Slug, job.Preset))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid deploy config %s:\n  %s", configPath, strings.Join(errs, "\n  "))
	}

	return nil
}
