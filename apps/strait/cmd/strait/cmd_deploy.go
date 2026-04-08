package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"strait/internal/cli"
	"strait/internal/domain"

	"github.com/spf13/cobra"
)

func newDeployCommand() *cobra.Command {
	var (
		jobSlug     string
		envName     string
		profileName string
		noWatch     bool
		dryRun      bool
		dir         string
	)

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Build and deploy a job using code-first deployment",
		Long: `Pack the source directory, upload it, trigger a container build, and
stream build logs until the deployment is ready.

Reads project settings from strait.json in the current directory (or any
parent). Authentication is taken from the STRAIT_API_KEY environment variable
or from a stored profile (~/.config/strait/config.json).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDeploy(cmd.Context(), runDeployOpts{
				jobSlug:     jobSlug,
				envName:     envName,
				profileName: profileName,
				noWatch:     noWatch,
				dryRun:      dryRun,
				dir:         dir,
			})
		},
	}

	cmd.Flags().StringVarP(&jobSlug, "job", "j", "", "job slug to deploy (required unless strait.json has a single job)")
	cmd.Flags().StringVar(&envName, "env", "", "environment to use (maps to strait.json environments section)")
	cmd.Flags().StringVar(&profileName, "profile", "", "auth profile to use (default: active profile)")
	cmd.Flags().BoolVar(&noWatch, "no-watch", false, "return immediately after triggering the build without streaming logs")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "pack and validate the tarball but do not upload or build")
	cmd.Flags().StringVar(&dir, "dir", "", "source directory to pack (defaults to current working directory)")

	return cmd
}

type runDeployOpts struct {
	jobSlug     string
	envName     string
	profileName string
	noWatch     bool
	dryRun      bool
	dir         string
}

// deployListJobsResponse is the paginated envelope from GET /v1/jobs.
type deployListJobsResponse struct {
	Data    []domain.Job `json:"data"`
	HasMore bool         `json:"has_more"`
}

// deployCreateResponse is the response from POST /v1/jobs/{id}/deployments.
type deployCreateResponse struct {
	Deployment *domain.CodeDeployment `json:"deployment"`
	UploadURL  string                 `json:"upload_url"`
}

// deployConfirmResponse is the response from POST .../confirm.
type deployConfirmResponse struct {
	Deployment *domain.CodeDeployment `json:"deployment"`
}

// deployGetResponse is the response from GET .../deployments/{id}.
type deployGetResponse struct {
	Deployment *domain.CodeDeployment `json:"deployment"`
}

// logChunk is the JSON shape of each SSE data payload.
type logChunk struct {
	Chunk string `json:"chunk"`
	Done  bool   `json:"done"`
}

func runDeploy(ctx context.Context, opts runDeployOpts) error {
	// 1. Find strait.json.
	startDir := opts.dir
	if startDir == "" {
		var cwdErr error
		startDir, cwdErr = os.Getwd()
		if cwdErr != nil {
			return fmt.Errorf("get working directory: %w", cwdErr)
		}
	}

	configPath, projectDir, err := cli.FindStraitConfig(startDir)
	if err != nil {
		return err
	}
	sc, err := cli.LoadStraitConfig(configPath)
	if err != nil {
		return err
	}

	// 2. Determine runtime.
	runtime := ""
	if sc.Deploy != nil {
		runtime = sc.Deploy.Runtime
	}
	if runtime == "" {
		return fmt.Errorf("deploy.runtime is required in strait.json (python|typescript|go|ruby|rust)")
	}

	// 3. Pack source.
	sourceDir := projectDir
	fmt.Fprintf(os.Stderr, "Packing %s...\n", sourceDir)

	var buf bytes.Buffer
	packRes, err := cli.Pack(&buf, sourceDir, nil)
	if err != nil {
		return fmt.Errorf("pack source: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Packed: %s (%s SHA-256: %s)\n",
		formatBytes(packRes.SizeBytes), runtime, packRes.SHA256Hex[:16]+"...")

	if opts.dryRun {
		fmt.Fprintln(os.Stderr, "Dry run complete — no upload performed.")
		return nil
	}

	// 4. Resolve auth profile (not needed for dry-run).
	authCfg, err := cli.LoadConfig()
	if err != nil {
		return err
	}
	profile := authCfg.ActiveProfileData(opts.profileName)

	// Per-environment overrides from strait.json.
	envName := opts.envName
	if envName == "" && sc.CLI != nil {
		envName = sc.CLI.DefaultEnvironment
	}
	if apiURL := sc.EffectiveAPIURL(envName); apiURL != "" {
		profile.APIURL = apiURL
	}
	if profile.APIKey == "" {
		keyEnv := sc.EffectiveAPIKeyEnv(envName)
		profile.APIKey = os.Getenv(keyEnv)
	}
	if profile.APIKey == "" {
		return fmt.Errorf("no API key found — set STRAIT_API_KEY or run 'strait auth login'")
	}

	c := cli.NewClient(profile)

	// 5. Resolve job ID from slug.
	if opts.jobSlug == "" {
		return fmt.Errorf("--job <slug> is required")
	}
	fmt.Fprintf(os.Stderr, "Resolving job %q...\n", opts.jobSlug)
	var jobListResp deployListJobsResponse
	if err := c.Do(ctx, "GET", fmt.Sprintf("/v1/jobs?slug=%s", opts.jobSlug), nil, &jobListResp); err != nil {
		return fmt.Errorf("resolve job: %w", err)
	}
	if len(jobListResp.Data) == 0 {
		return fmt.Errorf("job %q not found in project %s", opts.jobSlug, sc.Project.ID)
	}
	job := jobListResp.Data[0]

	// 7. Create deployment (get presigned URL).
	fmt.Fprintln(os.Stderr, "Creating deployment...")
	var createResp deployCreateResponse
	if err := c.Do(ctx, "POST", fmt.Sprintf("/v1/jobs/%s/deployments", job.ID),
		map[string]any{
			"project_id":        sc.Project.ID,
			"job_id":            job.ID,
			"runtime":           runtime,
			"source_hash":       packRes.SHA256Hex,
			"source_size_bytes": packRes.SizeBytes,
		}, &createResp); err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}
	deploymentID := createResp.Deployment.ID

	// 8. Upload tarball to presigned URL.
	fmt.Fprintf(os.Stderr, "Uploading %s...\n", formatBytes(packRes.SizeBytes))
	uploadCtx, uploadCancel := context.WithTimeout(ctx, 30*time.Minute)
	defer uploadCancel()
	if err := c.Upload(uploadCtx, createResp.UploadURL, bytes.NewReader(buf.Bytes()), packRes.SizeBytes); err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Upload complete.")

	// 9. Confirm deployment — triggers the build.
	fmt.Fprintln(os.Stderr, "Confirming deployment...")
	var confirmResp deployConfirmResponse
	if err := c.Do(ctx, "POST",
		fmt.Sprintf("/v1/jobs/%s/deployments/%s/confirm", job.ID, deploymentID),
		nil, &confirmResp); err != nil {
		return fmt.Errorf("confirm deployment: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Build queued: %s\n", deploymentID)

	if opts.noWatch {
		fmt.Fprintf(os.Stdout, "%s\n", deploymentID)
		return nil
	}

	// 10. Stream build logs.
	fmt.Fprintln(os.Stderr, "Building...")
	if streamErr := streamBuildLogs(ctx, c, job.ID, deploymentID); streamErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: log stream error: %v\n", streamErr)
	}

	// 11. Final status check.
	var statusResp deployGetResponse
	if err := c.Do(ctx, "GET",
		fmt.Sprintf("/v1/jobs/%s/deployments/%s", job.ID, deploymentID),
		nil, &statusResp); err != nil {
		return fmt.Errorf("get deployment status: %w", err)
	}
	d := statusResp.Deployment
	switch d.Status {
	case "ready":
		fmt.Fprintf(os.Stderr, "\nDeployment %s ready", deploymentID)
		if d.BuiltImageURI != "" {
			fmt.Fprintf(os.Stderr, " (%s)", d.BuiltImageURI)
		}
		fmt.Fprintln(os.Stderr)
		return nil
	case "failed":
		msg := d.ErrorMessage
		if msg == "" {
			msg = "build failed — check logs for details"
		}
		return fmt.Errorf("deployment failed: %s", msg)
	case "timed_out":
		return fmt.Errorf("deployment timed out — the build exceeded the configured timeout")
	default:
		fmt.Fprintf(os.Stderr, "Deployment %s is in unexpected status %q\n", deploymentID, d.Status)
		return nil
	}
}

// streamBuildLogs opens the SSE log endpoint and prints each log chunk to
// stderr. Returns when the stream ends or ctx is cancelled.
func streamBuildLogs(ctx context.Context, c *cli.Client, jobID, deploymentID string) error {
	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s/logs?stream=true", jobID, deploymentID)
	body, err := c.Stream(ctx, path)
	if err != nil {
		return err
	}
	defer body.Close()

	return cli.ReadEvents(ctx, body, func(data []byte) {
		var chunk logChunk
		if jsonErr := json.Unmarshal(data, &chunk); jsonErr != nil {
			// Not JSON — print raw.
			fmt.Fprint(os.Stderr, string(data))
			return
		}
		if chunk.Chunk != "" {
			// Print raw log output — preserve newlines from BuildKit.
			if !strings.HasSuffix(chunk.Chunk, "\n") {
				fmt.Fprint(os.Stderr, chunk.Chunk+"\n")
			} else {
				fmt.Fprint(os.Stderr, chunk.Chunk)
			}
		}
	})
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(n int64) string {
	const (
		kb = 1024
		mb = kb * 1024
	)
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
