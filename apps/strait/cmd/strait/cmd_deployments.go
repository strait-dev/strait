package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"strait/internal/cli"
	"strait/internal/domain"

	"github.com/spf13/cobra"
)

func newDeploymentsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployments",
		Short: "Manage code-first deployments",
	}
	cmd.AddCommand(newDeploymentsListCommand())
	cmd.AddCommand(newDeploymentsLogsCommand())
	cmd.AddCommand(newDeploymentsRollbackCommand())
	return cmd
}

// list subcommand.

func newDeploymentsListCommand() *cobra.Command {
	var (
		jobSlug     string
		limit       int
		outputJSON  bool
		profileName string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deployments for a job",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDeploymentsList(cmd.Context(), jobSlug, limit, outputJSON, profileName)
		},
	}
	cmd.Flags().StringVarP(&jobSlug, "job", "j", "", "job slug (required)")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum number of deployments to return")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "output raw JSON")
	cmd.Flags().StringVar(&profileName, "profile", "", "auth profile")
	_ = cmd.MarkFlagRequired("job")
	return cmd
}

type listDeploymentsResponse struct {
	Data    []domain.CodeDeployment `json:"data"`
	HasMore bool                    `json:"has_more"`
}

func runDeploymentsList(ctx context.Context, jobSlug string, limit int, outputJSON bool, profileName string) error {
	c, jobID, err := resolveClientAndJob(ctx, jobSlug, profileName)
	if err != nil {
		return err
	}

	var resp listDeploymentsResponse
	if err := c.Do(ctx, "GET",
		fmt.Sprintf("/v1/jobs/%s/deployments?limit=%d", jobID, limit),
		nil, &resp); err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	if outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tVERSION\tSTATUS\tRUNTIME\tCREATED\tACTIVE")
	for _, d := range resp.Data {
		active := ""
		if d.Status == "ready" {
			active = "(ready)"
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\n",
			d.ID,
			d.Version,
			d.Status,
			d.Runtime,
			d.CreatedAt.Format(time.RFC3339),
			active,
		)
	}
	return tw.Flush()
}

// logs subcommand.

func newDeploymentsLogsCommand() *cobra.Command {
	var (
		jobSlug      string
		deploymentID string
		stream       bool
		profileName  string
	)
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show or stream build logs for a deployment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDeploymentsLogs(cmd.Context(), jobSlug, deploymentID, stream, profileName)
		},
	}
	cmd.Flags().StringVarP(&jobSlug, "job", "j", "", "job slug (required)")
	cmd.Flags().StringVarP(&deploymentID, "deployment", "d", "", "deployment ID (required)")
	cmd.Flags().BoolVar(&stream, "stream", false, "stream logs (for in-progress builds)")
	cmd.Flags().StringVar(&profileName, "profile", "", "auth profile")
	_ = cmd.MarkFlagRequired("job")
	_ = cmd.MarkFlagRequired("deployment")
	return cmd
}

func runDeploymentsLogs(ctx context.Context, jobSlug, deploymentID string, stream bool, profileName string) error {
	c, jobID, err := resolveClientAndJob(ctx, jobSlug, profileName)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s/logs", jobID, deploymentID)
	if stream {
		path += "?stream=true"
		body, streamErr := c.Stream(ctx, path)
		if streamErr != nil {
			return fmt.Errorf("stream logs: %w", streamErr)
		}
		defer body.Close()
		return cli.ReadEvents(ctx, body, func(data []byte) {
			var chunk logChunk
			if jsonErr := json.Unmarshal(data, &chunk); jsonErr == nil && chunk.Chunk != "" {
				fmt.Print(chunk.Chunk)
			}
		})
	}

	// Terminal deployment — fetch stored logs as plain text.
	body, streamErr := c.Stream(ctx, path)
	if streamErr != nil {
		return fmt.Errorf("get logs: %w", streamErr)
	}
	defer body.Close()
	_, err = os.Stdout.ReadFrom(body)
	return err
}

// rollback subcommand.

func newDeploymentsRollbackCommand() *cobra.Command {
	var (
		jobSlug      string
		deploymentID string
		yes          bool
		profileName  string
	)
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Roll back to a previous deployment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDeploymentsRollback(cmd.Context(), jobSlug, deploymentID, yes, profileName)
		},
	}
	cmd.Flags().StringVarP(&jobSlug, "job", "j", "", "job slug (required)")
	cmd.Flags().StringVarP(&deploymentID, "deployment", "d", "", "deployment ID to roll back to (required)")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation prompt")
	cmd.Flags().StringVar(&profileName, "profile", "", "auth profile")
	_ = cmd.MarkFlagRequired("job")
	_ = cmd.MarkFlagRequired("deployment")
	return cmd
}

type rollbackResponse struct {
	Deployment *domain.CodeDeployment `json:"deployment"`
}

func runDeploymentsRollback(ctx context.Context, jobSlug, deploymentID string, yes bool, profileName string) error {
	c, jobID, err := resolveClientAndJob(ctx, jobSlug, profileName)
	if err != nil {
		return err
	}

	if !yes {
		fmt.Fprintf(os.Stderr, "Roll back job %q to deployment %s? [y/N] ", jobSlug, deploymentID)
		var answer string
		if _, scanErr := fmt.Scanln(&answer); scanErr != nil || (answer != "y" && answer != "Y") {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil //nolint:nilerr // scan error means no input; treat as abort, not failure
		}
	}

	var resp rollbackResponse
	if err := c.Do(ctx, "POST",
		fmt.Sprintf("/v1/jobs/%s/deployments/%s/rollback", jobID, deploymentID),
		nil, &resp); err != nil {
		return fmt.Errorf("rollback: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Rolled back to deployment %s (status: %s)\n",
		resp.Deployment.ID, resp.Deployment.Status)
	return nil
}

// resolveClientAndJob loads the auth profile, finds strait.json, and resolves
// the job ID from a slug. Returns the client and job ID, or an error.
func resolveClientAndJob(ctx context.Context, jobSlug, profileName string) (*cli.Client, string, error) {
	startDir, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("get working directory: %w", err)
	}

	configPath, _, cfgErr := cli.FindStraitConfig(startDir)
	if cfgErr != nil {
		return nil, "", cfgErr
	}
	sc, err := cli.LoadStraitConfig(configPath)
	if err != nil {
		return nil, "", err
	}

	authCfg, err := cli.LoadConfig()
	if err != nil {
		return nil, "", err
	}
	profile := authCfg.ActiveProfileData(profileName)
	if v := sc.EffectiveAPIURL(""); v != "" {
		profile.APIURL = v
	}
	if profile.APIKey == "" {
		keyEnv := sc.EffectiveAPIKeyEnv("")
		profile.APIKey = os.Getenv(keyEnv)
	}
	if profile.APIKey == "" {
		return nil, "", fmt.Errorf("no API key found — set STRAIT_API_KEY or run 'strait auth login'")
	}

	c := cli.NewClient(profile)

	var jobListResp deployListJobsResponse
	if err := c.Do(ctx, "GET", fmt.Sprintf("/v1/jobs?slug=%s", jobSlug), nil, &jobListResp); err != nil {
		return nil, "", fmt.Errorf("resolve job: %w", err)
	}
	if len(jobListResp.Data) == 0 {
		return nil, "", fmt.Errorf("job %q not found in project %s", jobSlug, sc.Project.ID)
	}
	return c, jobListResp.Data[0].ID, nil
}
