package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func newInitCommand(state *appState) *cobra.Command {
	var yes bool
	var template string
	var name string

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Initialize local orchestrator project files",
		Long:    "Creates baseline orchestrator project files such as config, env, and declarative definitions.",
		Example: "orchestrator init\n  orchestrator init --yes --name demo-project --template minimal\n  orchestrator init --template full",
		RunE: func(_ *cobra.Command, _ []string) error {
			if !yes {
				if !stdoutIsTTY() {
					return fmt.Errorf("init requires --yes when stdout is not a TTY")
				}

				reader := bufio.NewReader(os.Stdin)
				projectInput, err := promptWithDefault(reader, "project name", name)
				if err != nil {
					return err
				}
				templateInput, err := promptWithDefault(reader, "template (minimal|full)", template)
				if err != nil {
					return err
				}

				name = projectInput
				template = templateInput
			}

			if template == "" {
				template = "minimal"
			}
			template = strings.ToLower(strings.TrimSpace(template))
			if template != "minimal" && template != "full" {
				return fmt.Errorf("invalid template %q; supported values: minimal, full", template)
			}

			name = strings.TrimSpace(name)
			if name == "" {
				return fmt.Errorf("project name is required")
			}

			configStatus, err := writeInitConfig(name)
			if err != nil {
				return err
			}
			envStatus, err := writeInitEnv()
			if err != nil {
				return err
			}
			manifestStatus, err := writeInitJobManifest(template, name)
			if err != nil {
				return err
			}

			return printData(state, map[string]any{
				"project":  name,
				"template": template,
				"files": []map[string]any{
					{"path": ".orchestrator.yaml", "status": configStatus},
					{"path": ".env", "status": envStatus},
					{"path": "definitions/jobs.yaml", "status": manifestStatus},
				},
			})
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "run non-interactive initialization")
	cmd.Flags().StringVar(&template, "template", "minimal", "template name")
	cmd.Flags().StringVar(&name, "name", "demo-project", "project name")

	return cmd
}

func writeInitConfig(projectName string) (string, error) {
	path := ".orchestrator.yaml"
	if _, err := os.Stat(path); err == nil {
		return "skipped", nil
	}

	cfg := initConfigFile{
		Server:        "http://localhost:8080",
		Project:       projectName,
		Format:        "table",
		ActiveContext: "default",
	}

	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return "", err
	}

	return "created", nil
}

func writeInitEnv() (string, error) {
	if _, err := os.Stat(".env"); err == nil {
		return "skipped", nil
	}

	if _, err := os.Stat(".env.example"); err == nil {
		content, readErr := os.ReadFile(".env.example")
		if readErr != nil {
			return "", readErr
		}
		if err := os.WriteFile(".env", content, 0o600); err != nil {
			return "", err
		}
		return "created", nil
	}

	defaultEnv := []byte("DATABASE_URL=postgres://orchestrator:orchestrator@localhost:5432/orchestrator?sslmode=disable\nREDIS_URL=redis://localhost:6379\n")
	if err := os.WriteFile(".env", defaultEnv, 0o600); err != nil {
		return "", err
	}

	return "created", nil
}

func writeInitJobManifest(template, projectName string) (string, error) {

	dir := "definitions"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}

	target := filepath.Join(dir, "jobs.yaml")
	if _, err := os.Stat(target); err == nil {
		return "skipped", nil
	}

	jobName := "example-job"
	jobSlug := "example-job"
	jobDescription := "example job definition"
	jobCron := ""
	if template == "full" {
		jobName = "example-full-job"
		jobSlug = "example-full-job"
		jobDescription = "example full template job"
		jobCron = "*/5 * * * *"
	}

	m := initJobManifest{
		APIVersion: "v1",
		Kind:       "Job",
		Metadata:   map[string]string{"name": jobName},
		Spec: map[string]any{
			"project_id":   projectName,
			"slug":         jobSlug,
			"description":  jobDescription,
			"cron":         jobCron,
			"endpoint_url": "http://localhost:3000/webhook",
			"timeout_secs": 60,
			"max_attempts": 3,
		},
	}

	encoded, err := yaml.Marshal(m)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(target, encoded, 0o600); err != nil {
		return "", err
	}

	return "created", nil
}

func promptWithDefault(reader *bufio.Reader, label, defaultValue string) (string, error) {
	_, _ = fmt.Fprintf(os.Stderr, "%s [%s]: ", label, defaultValue)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue, nil
	}
	return line, nil
}
