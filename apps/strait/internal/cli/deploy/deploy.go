// Package deploy provides the core logic for the `strait deploy` command.
package deploy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"strait/internal/cli/client"
	"strait/internal/domain"
)

// DeployOptions configures a single job deployment.
type DeployOptions struct {
	JobSlug      string
	ImageURI     string
	Dockerfile   string
	Registry     string
	Tag          string
	BuildArgs    []string
	Push         bool
	Preset       string
	Region       string
	DryRun       bool
	CacheEnabled bool
}

// DeployJob orchestrates the build → push → update flow for a single job.
func DeployJob(ctx context.Context, cli *client.Client, opts DeployOptions) error {
	imageURI := opts.ImageURI

	if imageURI == "" && opts.Dockerfile != "" {
		if opts.Tag == "" {
			sha, err := gitSHA(ctx)
			if err != nil {
				opts.Tag = "latest"
			} else {
				opts.Tag = sha
			}
		}

		var alreadyPushed bool
		var err error
		imageURI, alreadyPushed, err = BuildImage(ctx, opts.Dockerfile, opts.Tag, opts.Registry, opts.JobSlug, opts.BuildArgs, opts.CacheEnabled)
		if err != nil {
			return fmt.Errorf("build failed: %w", err)
		}

		if opts.Push && !alreadyPushed {
			if err := PushImage(ctx, imageURI); err != nil {
				return fmt.Errorf("push failed: %w", err)
			}
		}
	}

	if imageURI == "" {
		return fmt.Errorf("either --image or --dockerfile is required")
	}

	if opts.DryRun {
		fmt.Printf("[dry-run] would update job %s with image %s\n", opts.JobSlug, imageURI)
		if opts.Preset != "" {
			fmt.Printf("[dry-run]   preset: %s\n", opts.Preset)
		}
		if opts.Region != "" {
			fmt.Printf("[dry-run]   region: %s\n", opts.Region)
		}
		return nil
	}

	return UpdateJobImage(ctx, cli, opts.JobSlug, imageURI, opts.Preset, opts.Region)
}

// UpdateJobImage patches a job with the new image URI and optional preset/region.
func UpdateJobImage(ctx context.Context, cli *client.Client, slug, imageURI, preset, region string) error {
	if preset != "" && !domain.MachinePreset(preset).IsValid() {
		return fmt.Errorf("invalid preset: %s", preset)
	}

	req := client.UpdateJobRequest{
		ImageURI: &imageURI,
	}
	if preset != "" {
		req.MachinePreset = &preset
	}
	if region != "" {
		req.Region = &region
	}

	_, err := cli.UpdateJob(ctx, slug, req)
	if err != nil {
		return fmt.Errorf("update job %s: %w", slug, err)
	}
	return nil
}

// BuildImage runs `docker build` (or `docker buildx build` with caching) and
// returns the tagged image URI and whether the image was already pushed (buildx --push).
func BuildImage(ctx context.Context, dockerfile, tag, registry, slug string, buildArgs []string, cacheEnabled bool) (string, bool, error) {
	imageURI := fmt.Sprintf("%s/%s:%s", registry, slug, tag)

	useBuildx := cacheEnabled && hasBuildx(ctx)

	args := make([]string, 0, 12+2*len(buildArgs))
	if useBuildx {
		cacheRef := fmt.Sprintf("%s/%s:cache", registry, slug)
		args = append(args, "buildx", "build",
			"--cache-from", fmt.Sprintf("type=registry,ref=%s", cacheRef),
			"--cache-to", fmt.Sprintf("type=registry,ref=%s,mode=max", cacheRef),
			"--push",
			"-t", imageURI, "-f", dockerfile, ".")
	} else {
		if cacheEnabled {
			fmt.Println("warning: docker buildx not available, falling back to docker build")
		}
		args = append(args, "build", "-t", imageURI, "-f", dockerfile, ".")
	}

	for _, arg := range buildArgs {
		args = append(args, "--build-arg", arg)
	}

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // Args from trusted CLI flags.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", false, fmt.Errorf("docker build failed: %w", err)
	}

	return imageURI, useBuildx, nil
}

// hasBuildx checks if docker buildx is available.
func hasBuildx(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "docker", "buildx", "version")
	return cmd.Run() == nil
}

// PushImage pushes a Docker image to its registry.
func PushImage(ctx context.Context, imageURI string) error {
	cmd := exec.CommandContext(ctx, "docker", "push", imageURI) //nolint:gosec // Image URI from trusted CLI input.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker push failed: %w", err)
	}
	return nil
}

// gitSHA returns the current git HEAD short SHA.
func gitSHA(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
