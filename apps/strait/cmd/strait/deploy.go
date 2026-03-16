package main

import (
	"fmt"

	"strait/internal/cli/deploy"

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
	)

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a managed job image",
		Long:  "Build, push, and update a managed job's container image.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			// Config-file mode: deploy multiple jobs from strait.config.yaml.
			if configPath != "" && jobSlug == "" {
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
						jobPreset = preset // CLI flag overrides config
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

			// Single job mode.
			if jobSlug == "" {
				return fmt.Errorf("--job is required (or use --config for multi-job deploy)")
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
	cmd.Flags().StringVar(&configPath, "config", "", "path to strait.config.yaml for multi-job deploy")

	return cmd
}
