package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"strait/internal/cli/client"
	climanifest "strait/internal/cli/manifest"
)

// ManifestDeployOptions configures a manifest-based deployment.
type ManifestDeployOptions struct {
	ConfigPath     string
	Environment    string
	ArtifactURI    string
	DryRun         bool
	OutDir         string
	Strategy       string
	CanaryPercent  int
	CanaryDuration string
}

// DeployManifest runs the create + finalize deployment flow using manifests.
func DeployManifest(ctx context.Context, cli *client.Client, opts ManifestDeployOptions) error {
	deployment, manifest, env, cfg, err := CreateManifestDeployment(ctx, cli, opts)
	if err != nil {
		return err
	}

	if opts.DryRun {
		encoded, encErr := json.MarshalIndent(manifest, "", "  ")
		if encErr != nil {
			return fmt.Errorf("encode manifest: %w", encErr)
		}
		fmt.Println(string(encoded))
		if opts.Strategy == "canary" {
			fmt.Fprintf(os.Stderr, "[dry-run] canary strategy: %d%% traffic for %s\n", opts.CanaryPercent, opts.CanaryDuration)
		}
		return nil
	}

	if err := writeManifest(cfg, manifest, opts.OutDir); err != nil {
		return err
	}

	if err := FinalizeManifestDeployment(ctx, cli, deployment.ID, manifest.ProjectID, env); err != nil {
		return fmt.Errorf("finalize deployment %s: %w", deployment.ID, err)
	}

	fmt.Printf("deployment %s created and finalized (checksum: %s)\n", deployment.ID, manifest.Checksum)
	return nil
}

func CreateManifestDeployment(ctx context.Context, cli *client.Client, opts ManifestDeployOptions) (*client.DeploymentVersion, *climanifest.ProjectManifest, string, *climanifest.ProjectConfig, error) {
	cfg, err := climanifest.LoadProjectConfig(opts.ConfigPath)
	if err != nil {
		return nil, nil, "", nil, err
	}

	manifest := climanifest.BuildManifest(cfg)
	env := resolveManifestEnvironment(cfg, opts.Environment)

	if strings.TrimSpace(manifest.Runtime) == "" {
		return nil, nil, "", nil, fmt.Errorf("manifest deploy requires project.runtime in the config file")
	}

	if opts.DryRun {
		return nil, manifest, env, cfg, nil
	}

	if strings.TrimSpace(opts.ArtifactURI) == "" {
		return nil, nil, "", nil, fmt.Errorf("manifest deploy requires --artifact-uri")
	}

	deployment, err := cli.CreateDeploymentVersion(ctx, client.CreateDeploymentVersionRequest{
		ProjectID:      manifest.ProjectID,
		Environment:    env,
		Runtime:        manifest.Runtime,
		Manifest:       manifest,
		Checksum:       manifest.Checksum,
		ArtifactURI:    strings.TrimSpace(opts.ArtifactURI),
		Strategy:       opts.Strategy,
		CanaryPercent:  opts.CanaryPercent,
		CanaryDuration: opts.CanaryDuration,
	})
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("create deployment: %w", err)
	}

	return deployment, manifest, env, cfg, nil
}

func FinalizeManifestDeployment(ctx context.Context, cli *client.Client, deploymentID, projectID, environment string) error {
	return cli.FinalizeDeployment(ctx, deploymentID, client.FinalizeDeploymentRequest{
		ProjectID:   projectID,
		Environment: environment,
	})
}

func resolveManifestEnvironment(cfg *climanifest.ProjectConfig, override string) string {
	env := strings.TrimSpace(override)
	if env != "" {
		return env
	}
	env = strings.TrimSpace(cfg.Deploy.DefaultEnvironment)
	if env != "" {
		return env
	}
	return "production"
}

func writeManifest(cfg *climanifest.ProjectConfig, manifest *climanifest.ProjectManifest, outDir string) error {
	targetDir := outDir
	if targetDir == "" {
		targetDir = cfg.Build.OutDir
	}
	if targetDir == "" {
		targetDir = ".strait"
	}

	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	target := filepath.Join(targetDir, "manifest.json")
	if err := os.WriteFile(target, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func WriteManifestForCommand(cfg *climanifest.ProjectConfig, manifest *climanifest.ProjectManifest, outDir string) error {
	return writeManifest(cfg, manifest, outDir)
}
