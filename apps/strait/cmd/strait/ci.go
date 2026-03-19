package main

import (
	"fmt"
	"os"
	"path/filepath"

	"strait/internal/cli/ci"

	"github.com/spf13/cobra"
)

func newCICommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "CI/CD pipeline tools",
	}

	cmd.AddCommand(newCISetupCommand(state))
	cmd.AddCommand(newCICheckCommand(state))

	return cmd
}

func newCISetupCommand(state *appState) *cobra.Command {
	var provider string
	var dryRun bool
	var env string
	var projectID string

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Generate CI/CD pipeline configuration",
		Long: `Detects your CI provider and generates a deployment workflow file.
Supported providers: github, gitlab, generic.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if provider == "" {
				provider = ci.DetectProvider(".")
			}

			pid := projectID
			if pid == "" {
				pid = state.opts.projectID
			}

			content, err := ci.Generate(provider, ci.GenerateConfig{
				ProjectID:   pid,
				Environment: env,
			})
			if err != nil {
				return err
			}

			if dryRun {
				fmt.Println(content)
				return nil
			}

			var outputPath string
			switch provider {
			case "github":
				outputPath = ".github/workflows/strait-deploy.yml"
			case "gitlab":
				outputPath = ".gitlab-ci.strait.yml"
			default:
				outputPath = "strait-deploy.sh"
			}

			dir := filepath.Dir(outputPath)
			if dir != "." {
				if err := os.MkdirAll(dir, 0o750); err != nil {
					return fmt.Errorf("create directory %s: %w", dir, err)
				}
			}

			if err := os.WriteFile(outputPath, []byte(content), 0o600); err != nil {
				return fmt.Errorf("write %s: %w", outputPath, err)
			}

			return printData(state, map[string]any{
				"provider": provider,
				"file":     outputPath,
				"written":  true,
			})
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "CI provider (github, gitlab, generic)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print generated config without writing")
	cmd.Flags().StringVar(&env, "env", "production", "deployment environment")
	cmd.Flags().StringVar(&projectID, "project", "", "project ID")

	return cmd
}

func newCICheckCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate CI/CD readiness",
		Long:  "Checks config validity, manifest validation, server connectivity, and API key.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			checks := make([]map[string]any, 0, 4)

			// Config check
			checks = append(checks, diagnoseCheck("server configured", state.opts.serverURL != "", state.opts.serverURL, "set --server or STRAIT_SERVER"))
			checks = append(checks, diagnoseCheck("api key present", state.opts.apiKey != "", boolString(state.opts.apiKey != ""), "set STRAIT_API_KEY"))

			// Connectivity check
			cli, err := newAPIClient(state)
			if err == nil {
				health, hErr := cli.Health(cmd.Context())
				checks = append(checks, diagnoseCheck("server reachable", hErr == nil, healthDetail(health, hErr), "verify server is running"))

				_, sErr := cli.Stats(cmd.Context())
				checks = append(checks, diagnoseCheck("auth valid", sErr == nil, errDetail(sErr), "check API key validity"))
			} else {
				checks = append(checks, diagnoseCheck("api client", false, err.Error(), "check --server and --api-key"))
			}

			if err := printData(state, checks); err != nil {
				return err
			}

			for _, item := range checks {
				if ok, _ := item["ok"].(bool); !ok {
					return fmt.Errorf("CI check found failures")
				}
			}

			return nil
		},
	}

	return cmd
}
