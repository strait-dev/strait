package main

import (
	"fmt"

	"strait/internal/cli/client"
	"strait/internal/cli/deploy"
	"strait/internal/cli/styles"

	"github.com/spf13/cobra"
)

func newDeployCommand(state *appState) *cobra.Command {
	var (
		jobSlug      string
		imageURI     string
		dockerfile   string
		registry     string
		tag          string
		buildArgs    []string
		push         bool
		preset       string
		region       string
		dryRun       bool
		cacheEnabled bool
		configPath   string
		env          string
		artifactURI  string
	)

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a managed job image or manifest",
		Long:  "Build, push, and update a managed job's container image, or deploy via manifests.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			// Manifest-based deploy: --config provided and no --job
			if configPath != "" && jobSlug == "" {
				return deploy.DeployManifest(cmd.Context(), cli, deploy.ManifestDeployOptions{
					ConfigPath:  configPath,
					Environment: env,
					ArtifactURI: artifactURI,
					DryRun:      dryRun,
				})
			}

			// Config-file multi-job mode (legacy): --config with implied jobs
			if configPath != "" {
				cfg, cfgErr := deploy.LoadDeployConfig(configPath)
				if cfgErr != nil {
					return cfgErr
				}

				reg := registry
				if cfg.Registry != "" {
					reg = cfg.Registry
				}

				for _, jobCfg := range cfg.Jobs {
					jobPreset := jobCfg.Preset
					if preset != "" {
						jobPreset = preset
					}
					jobRegion := jobCfg.Region
					if region != "" {
						jobRegion = region
					}

					var ba []string
					for k, v := range jobCfg.BuildArgs {
						ba = append(ba, fmt.Sprintf("%s=%s", k, v))
					}

					opts := deploy.DeployOptions{
						JobSlug:      jobCfg.Slug,
						Dockerfile:   jobCfg.Dockerfile,
						Registry:     reg,
						Tag:          tag,
						BuildArgs:    ba,
						Push:         push,
						Preset:       jobPreset,
						Region:       jobRegion,
						DryRun:       dryRun,
						CacheEnabled: cacheEnabled,
					}

					fmt.Printf("deploying %s...\n", jobCfg.Slug)
					if deployErr := deploy.DeployJob(cmd.Context(), cli, opts); deployErr != nil {
						return fmt.Errorf("deploy %s: %w", jobCfg.Slug, deployErr)
					}
					if !dryRun {
						fmt.Printf("deployed %s successfully\n", jobCfg.Slug)
					}
				}
				return nil
			}

			// Single job mode
			if jobSlug == "" {
				return fmt.Errorf("--job is required (or use --config for multi-job/manifest deploy)")
			}

			opts := deploy.DeployOptions{
				JobSlug:      jobSlug,
				ImageURI:     imageURI,
				Dockerfile:   dockerfile,
				Registry:     registry,
				Tag:          tag,
				BuildArgs:    buildArgs,
				Push:         push,
				Preset:       preset,
				Region:       region,
				DryRun:       dryRun,
				CacheEnabled: cacheEnabled,
			}

			if err := deploy.DeployJob(cmd.Context(), cli, opts); err != nil {
				return err
			}

			if !dryRun {
				fmt.Printf("deployed %s successfully\n", jobSlug)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&jobSlug, "job", "", "job slug to deploy (required unless --config)")
	cmd.Flags().StringVar(&imageURI, "image", "", "pre-built image URI (skip build)")
	cmd.Flags().StringVar(&dockerfile, "dockerfile", "./Dockerfile", "path to Dockerfile")
	cmd.Flags().StringVar(&registry, "registry", "registry.fly.io", "container registry")
	cmd.Flags().StringVar(&tag, "tag", "", "image tag (default: git SHA or 'latest')")
	cmd.Flags().StringArrayVar(&buildArgs, "build-arg", nil, "docker build args (repeatable)")
	cmd.Flags().BoolVar(&push, "push", true, "push image after build")
	cmd.Flags().StringVar(&preset, "preset", "", "machine preset override")
	cmd.Flags().StringVar(&region, "region", "", "region override")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print plan without executing")
	cmd.Flags().BoolVar(&cacheEnabled, "cache", true, "enable Docker layer caching via buildx")
	cmd.Flags().StringVar(&configPath, "config", "", "path to config file for manifest/multi-job deploy")
	cmd.Flags().StringVar(&env, "env", "", "deployment environment (default: production)")
	cmd.Flags().StringVar(&artifactURI, "artifact-uri", "", "pre-built artifact URI override")

	cmd.AddCommand(newDeployPromoteCommand(state))
	cmd.AddCommand(newDeployRollbackCommand(state))
	cmd.AddCommand(newDeployListCommand(state))
	cmd.AddCommand(newDeployPreviewCommand(state))

	return cmd
}

func newDeployPromoteCommand(state *appState) *cobra.Command {
	var projectID string
	var env string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "promote <deployment-id>",
		Short: "Promote a deployment to an environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun {
				fmt.Printf("[dry-run] would promote deployment %s to %s\n", args[0], env)
				return nil
			}

			resolvedProject, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			if err := cli.PromoteDeployment(cmd.Context(), args[0], client.PromoteDeploymentRequest{
				ProjectID:   resolvedProject,
				Environment: env,
			}); err != nil {
				return err
			}

			return printData(state, map[string]any{
				"promoted":    true,
				"deployment":  args[0],
				"environment": env,
			})
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&env, "env", "production", "target environment")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print plan without executing")

	return cmd
}

func newDeployRollbackCommand(state *appState) *cobra.Command {
	var projectID string
	var env string
	var toID string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback to a previous deployment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if toID == "" {
				return fmt.Errorf("--to is required (deployment ID to rollback to)")
			}

			if dryRun {
				fmt.Printf("[dry-run] would rollback to deployment %s in %s\n", toID, env)
				return nil
			}

			resolvedProject, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			if err := cli.RollbackDeployment(cmd.Context(), toID, client.RollbackDeploymentRequest{
				ProjectID:   resolvedProject,
				Environment: env,
			}); err != nil {
				return err
			}

			return printData(state, map[string]any{
				"rolled_back": true,
				"deployment":  toID,
				"environment": env,
			})
		},
	}

	cmd.Flags().StringVar(&toID, "to", "", "deployment ID to rollback to")
	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&env, "env", "production", "target environment")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print plan without executing")

	return cmd
}

func newDeployListCommand(state *appState) *cobra.Command {
	var projectID string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deployment history",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedProject, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			deps, err := cli.ListDeployments(cmd.Context(), resolvedProject, limit)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(deps))
			for _, dep := range deps {
				rows = append(rows, map[string]any{
					"id":          dep.ID,
					"environment": dep.Environment,
					"status":      styles.Status(dep.Status),
					"checksum":    dep.Checksum,
					"created_at":  dep.CreatedAt,
				})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().IntVar(&limit, "limit", 20, "max deployments to show")

	return cmd
}

func newDeployPreviewCommand(state *appState) *cobra.Command {
	var projectID string
	var ttl string
	var configPath string

	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Create a preview deployment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if configPath == "" {
				return fmt.Errorf("--config is required for preview deployments")
			}

			_, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			branch := "preview"
			previewEnv := fmt.Sprintf("preview-%s", branch)

			return deploy.DeployManifest(cmd.Context(), cli, deploy.ManifestDeployOptions{
				ConfigPath:  configPath,
				Environment: previewEnv,
			})
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&ttl, "ttl", "24h", "time-to-live for preview (default: 24h)")
	cmd.Flags().StringVar(&configPath, "config", "", "path to config file")

	return cmd
}
