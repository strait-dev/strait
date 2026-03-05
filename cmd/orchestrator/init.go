package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
)

type initConfigFile struct {
	Server        string `yaml:"server"`
	Project       string `yaml:"project"`
	Format        string `yaml:"format"`
	ActiveContext string `yaml:"active_context"`
}

type initJobManifest struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   map[string]string `yaml:"metadata"`
	Spec       map[string]any    `yaml:"spec"`
}

func newInitCommand() *cobra.Command {
	var yes bool
	var template string
	var name string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize local orchestrator project files",
		RunE: func(_ *cobra.Command, _ []string) error {
			if !yes {
				return fmt.Errorf("init requires --yes in this non-interactive baseline")
			}

			if template == "" {
				template = "minimal"
			}

			if err := writeInitConfig(name); err != nil {
				return err
			}
			if err := writeInitEnv(); err != nil {
				return err
			}
			return writeInitJobManifest(template, name)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "run non-interactive initialization")
	cmd.Flags().StringVar(&template, "template", "minimal", "template name")
	cmd.Flags().StringVar(&name, "name", "demo-project", "project name")

	return cmd
}

func writeInitConfig(projectName string) error {
	path := ".orchestrator.yaml"
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	cfg := initConfigFile{
		Server:        "http://localhost:8080",
		Project:       projectName,
		Format:        "table",
		ActiveContext: "default",
	}

	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, encoded, 0o600)
}

func writeInitEnv() error {
	if _, err := os.Stat(".env"); err == nil {
		return nil
	}

	if _, err := os.Stat(".env.example"); err == nil {
		content, readErr := os.ReadFile(".env.example")
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(".env", content, 0o600)
	}

	defaultEnv := []byte("DATABASE_URL=postgres://orchestrator:orchestrator@localhost:5432/orchestrator?sslmode=disable\nREDIS_URL=redis://localhost:6379\n")
	return os.WriteFile(".env", defaultEnv, 0o600)
}

func writeInitJobManifest(template, projectName string) error {
	_ = template

	dir := "definitions"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	target := filepath.Join(dir, "jobs.yaml")
	if _, err := os.Stat(target); err == nil {
		return nil
	}

	m := initJobManifest{
		APIVersion: "v1",
		Kind:       "Job",
		Metadata:   map[string]string{"name": "example-job"},
		Spec: map[string]any{
			"project_id":   projectName,
			"slug":         "example-job",
			"endpoint_url": "http://localhost:3000/webhook",
			"timeout_secs": 60,
			"max_attempts": 3,
		},
	}

	encoded, err := yaml.Marshal(m)
	if err != nil {
		return err
	}

	return os.WriteFile(target, encoded, 0o600)
}
