package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"strait/internal/cli/wizard"

	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
)

type straitConfigJSON struct {
	Project  projectBlock    `json:"project"`
	Runtime  string          `json:"runtime,omitempty"`
	Jobs     []jobBlock      `json:"jobs,omitempty"`
	Workflow []workflowBlock `json:"workflows,omitempty"`
}

type projectBlock struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type jobBlock struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	EndpointURL string `json:"endpointUrl,omitempty"`
	Cron        string `json:"cron,omitempty"`
}

type workflowBlock struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

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

type initWorkflowManifest struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   map[string]string `yaml:"metadata"`
	Spec       map[string]any    `yaml:"spec"`
}

func newInitCommand(state *appState) *cobra.Command {
	var (
		yes         bool
		force       bool
		template    string
		name        string
		runtime     string
		withJob     bool
		jobName     string
		jobEndpoint string
		jobCron     string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new strait project",
		Long: `Initialize a new strait project with configuration files.

In interactive mode (default when TTY), a wizard guides you through setup.
In non-interactive mode (--yes), all values come from flags.`,
		Example: `  strait init
  strait init --yes --name my-api --runtime node
  strait init --yes --name my-api --runtime bun --with-job --job-name process-payment --job-endpoint http://localhost:3000/jobs/payment
  strait init --template full --name demo`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Interactive mode: TTY + no --yes flag
			if !yes && stdoutIsTTY() {
				result, err := wizard.RunInitWizard()
				if err != nil {
					return err
				}
				name = result.ProjectName
				runtime = result.Runtime
				withJob = result.WithJob
				jobName = result.JobName
				jobEndpoint = result.JobEndpoint
				jobCron = result.JobCron
			} else if !yes {
				return fmt.Errorf("interactive mode requires a TTY; use --yes with flags for non-interactive init")
			}

			// Validate inputs
			name = strings.TrimSpace(name)
			if err := wizard.ValidateProjectName(name); err != nil {
				return err
			}
			if runtime != "" {
				if err := wizard.ValidateRuntime(runtime); err != nil {
					return err
				}
			}

			// Check for existing config (unless --force)
			configPath := "strait.config.json"
			if !force {
				if _, err := os.Stat(configPath); err == nil {
					return fmt.Errorf("config file %s already exists (use --force to overwrite)", configPath)
				}
			}

			// Write strait.config.json
			cfg := straitConfigJSON{
				Project: projectBlock{ID: name, Name: name},
				Runtime: runtime,
			}
			if withJob && jobName != "" {
				slug := wizard.GenerateSlug(jobName)
				cfg.Jobs = append(cfg.Jobs, jobBlock{
					Slug:        slug,
					Name:        jobName,
					EndpointURL: jobEndpoint,
					Cron:        jobCron,
				})
			}

			encoded, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return fmt.Errorf("encode config: %w", err)
			}
			if err := os.WriteFile(configPath, append(encoded, '\n'), 0o600); err != nil {
				return fmt.Errorf("write config: %w", err)
			}

			// Write .strait.yaml (local CLI config)
			configStatus, err := writeInitConfig(name)
			if err != nil {
				return fmt.Errorf("writing CLI config: %w", err)
			}

			// Update .gitignore
			gitignoreStatus := updateGitignore()

			// Write declarative definitions (legacy template mode)
			if template == "full" || template == "minimal" {
				envStatus, envErr := writeInitEnv()
				if envErr != nil {
					return fmt.Errorf("writing .env: %w", envErr)
				}
				dcStatus, dcErr := writeInitDockerCompose()
				if dcErr != nil {
					return fmt.Errorf("writing docker-compose: %w", dcErr)
				}
				manifestStatus, mErr := writeInitJobManifest(template, name)
				if mErr != nil {
					return fmt.Errorf("writing job manifest: %w", mErr)
				}
				wfStatus, wfErr := writeInitWorkflowManifest(template, name)
				if wfErr != nil {
					return fmt.Errorf("writing workflow manifest: %w", wfErr)
				}

				return printData(state, map[string]any{
					"project":  name,
					"runtime":  runtime,
					"template": template,
					"files": []map[string]any{
						{"path": configPath, "status": "created"},
						{"path": ".strait.yaml", "status": configStatus},
						{"path": ".gitignore", "status": gitignoreStatus},
						{"path": ".env", "status": envStatus},
						{"path": "docker-compose.yml", "status": dcStatus},
						{"path": "definitions/jobs.yaml", "status": manifestStatus},
						{"path": "definitions/workflows.yaml", "status": wfStatus},
					},
				})
			}

			files := []map[string]any{
				{"path": configPath, "status": "created"},
				{"path": ".strait.yaml", "status": configStatus},
				{"path": ".gitignore", "status": gitignoreStatus},
			}

			return printData(state, map[string]any{
				"project": name,
				"runtime": runtime,
				"files":   files,
			})
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "run non-interactive initialization with flags")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing config files")
	cmd.Flags().StringVar(&template, "template", "", "template mode (minimal|full) for legacy definitions")
	cmd.Flags().StringVar(&name, "name", "", "project name")
	cmd.Flags().StringVar(&runtime, "runtime", "", "project runtime (node, bun, python, go, docker)")
	cmd.Flags().BoolVar(&withJob, "with-job", false, "include a starter job")
	cmd.Flags().StringVar(&jobName, "job-name", "", "starter job name (requires --with-job)")
	cmd.Flags().StringVar(&jobEndpoint, "job-endpoint", "", "starter job endpoint URL (requires --with-job)")
	cmd.Flags().StringVar(&jobCron, "job-cron", "", "starter job cron schedule (optional)")

	return cmd
}

func updateGitignore() string {
	const entry = ".strait/"
	path := ".gitignore"

	content, err := os.ReadFile(path)
	if err != nil {
		// No .gitignore — create one
		if err := os.WriteFile(path, []byte(entry+"\n"), 0o600); err != nil {
			return "error"
		}
		return "created"
	}

	// Check if already present
	for line := range strings.SplitSeq(string(content), "\n") {
		if strings.TrimSpace(line) == entry {
			return "skipped"
		}
	}

	// Append
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	content = append(content, []byte(entry+"\n")...)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return "error"
	}
	return "updated"
}

func writeInitConfig(projectName string) (string, error) {
	path := ".strait.yaml"
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

	defaultEnv := []byte("DATABASE_URL=postgres://strait:strait@localhost:5432/strait?sslmode=disable\nREDIS_URL=redis://localhost:6379\n")
	if err := os.WriteFile(".env", defaultEnv, 0o600); err != nil {
		return "", err
	}

	return "created", nil
}

func writeInitDockerCompose() (string, error) {
	path := "docker-compose.yml"
	if _, err := os.Stat(path); err == nil {
		return "skipped", nil
	}

	compose := []byte("services:\n  postgres:\n    image: postgres:16\n    environment:\n      POSTGRES_USER: strait\n      POSTGRES_PASSWORD: strait\n      POSTGRES_DB: strait\n    ports:\n      - \"5432:5432\"\n\n  redis:\n    image: redis:7\n    ports:\n      - \"6379:6379\"\n")
	if err := os.WriteFile(path, compose, 0o600); err != nil {
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

func writeInitWorkflowManifest(template, projectName string) (string, error) {
	if template != "full" {
		return "not_applicable", nil
	}

	dir := "definitions"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}

	target := filepath.Join(dir, "workflows.yaml")
	if _, err := os.Stat(target); err == nil {
		return "skipped", nil
	}

	m := initWorkflowManifest{
		APIVersion: "v1",
		Kind:       "Workflow",
		Metadata:   map[string]string{"name": "example-full-workflow"},
		Spec: map[string]any{
			"project_id":  projectName,
			"slug":        "example-full-workflow",
			"description": "example full template workflow",
			"enabled":     true,
			"steps": []map[string]any{
				{
					"step_ref":   "send_webhook",
					"job_id":     "example-full-job",
					"depends_on": []string{},
					"on_failure": "fail",
				},
			},
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
