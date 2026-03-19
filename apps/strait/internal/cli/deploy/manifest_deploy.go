package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"strait/internal/cli/client"
	climanifest "strait/internal/cli/manifest"
)

// ManifestDeployOptions configures a manifest-based deployment.
type ManifestDeployOptions struct {
	ConfigPath  string
	Environment string
	ArtifactURI string
	DryRun      bool
	OutDir      string
}

// DeployManifest runs the create + finalize deployment flow using manifests.
func DeployManifest(ctx context.Context, cli *client.Client, opts ManifestDeployOptions) error {
	cfg, err := climanifest.LoadProjectConfig(opts.ConfigPath)
	if err != nil {
		return err
	}

	m := climanifest.BuildManifest(cfg)

	env := opts.Environment
	if env == "" {
		env = cfg.Deploy.DefaultEnvironment
	}
	if env == "" {
		env = "production"
	}

	if opts.DryRun {
		encoded, encErr := json.MarshalIndent(m, "", "  ")
		if encErr != nil {
			return fmt.Errorf("encode manifest: %w", encErr)
		}
		fmt.Printf("[dry-run] would create deployment for project %s environment %s\n", m.ProjectID, env)
		fmt.Println(string(encoded))
		return nil
	}

	// Step 1: Create deployment version
	dep, err := cli.CreateDeploymentVersion(ctx, client.CreateDeploymentVersionRequest{
		ProjectID:   m.ProjectID,
		Environment: env,
		Runtime:     m.Runtime,
		Manifest:    m,
		Checksum:    m.Checksum,
		ArtifactURI: opts.ArtifactURI,
	})
	if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}

	// Write manifest to outDir
	outDir := opts.OutDir
	if outDir == "" {
		outDir = cfg.Build.OutDir
	}
	if outDir == "" {
		outDir = ".strait"
	}

	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	encoded, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	target := filepath.Join(outDir, "manifest.json")
	if err := os.WriteFile(target, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Step 2: Finalize deployment
	if err := cli.FinalizeDeployment(ctx, dep.ID, client.FinalizeDeploymentRequest{
		ProjectID: m.ProjectID,
	}); err != nil {
		return fmt.Errorf("finalize deployment %s: %w", dep.ID, err)
	}

	fmt.Printf("deployment %s created and finalized (checksum: %s)\n", dep.ID, m.Checksum)
	return nil
}
