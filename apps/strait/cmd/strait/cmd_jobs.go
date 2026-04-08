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

func newJobsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "List and inspect jobs in the current project",
	}
	cmd.AddCommand(newJobsListCommand())
	cmd.AddCommand(newJobsGetCommand())
	return cmd
}

// list subcommand.

func newJobsListCommand() *cobra.Command {
	var (
		limit       int
		outputJSON  bool
		profileName string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List jobs in the current project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runJobsList(cmd.Context(), limit, outputJSON, profileName)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum number of jobs to return")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "output raw JSON")
	cmd.Flags().StringVar(&profileName, "profile", "", "auth profile")
	return cmd
}

type listJobsResponse struct {
	Data    []domain.Job `json:"data"`
	HasMore bool         `json:"has_more"`
}

func runJobsList(ctx context.Context, limit int, outputJSON bool, profileName string) error {
	c, err := resolveClientFromContext(profileName)
	if err != nil {
		return err
	}

	var resp listJobsResponse
	if err := c.Do(ctx, "GET", fmt.Sprintf("/v1/jobs?limit=%d", limit), nil, &resp); err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}

	if outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SLUG\tNAME\tSTATUS\tSOURCE\tVERSION\tUPDATED")
	for _, j := range resp.Data {
		status := "enabled"
		if j.Paused {
			status = "paused"
		} else if !j.Enabled {
			status = "disabled"
		}
		source := j.SourceType
		if source == "" {
			source = "image"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
			j.Slug,
			j.Name,
			status,
			source,
			j.Version,
			j.UpdatedAt.Format(time.RFC3339),
		)
	}
	return tw.Flush()
}

// get subcommand.

func newJobsGetCommand() *cobra.Command {
	var (
		outputJSON  bool
		profileName string
	)
	cmd := &cobra.Command{
		Use:   "get <slug>",
		Short: "Get details for a specific job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJobsGet(cmd.Context(), args[0], outputJSON, profileName)
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "output raw JSON")
	cmd.Flags().StringVar(&profileName, "profile", "", "auth profile")
	return cmd
}

type getJobResponse struct {
	Job *domain.Job `json:"job"`
}

func runJobsGet(ctx context.Context, slug string, outputJSON bool, profileName string) error {
	c, err := resolveClientFromContext(profileName)
	if err != nil {
		return err
	}

	// Resolve slug → ID via list endpoint.
	var listResp listJobsResponse
	if err := c.Do(ctx, "GET", fmt.Sprintf("/v1/jobs?slug=%s", slug), nil, &listResp); err != nil {
		return fmt.Errorf("look up job: %w", err)
	}
	if len(listResp.Data) == 0 {
		return fmt.Errorf("job %q not found", slug)
	}
	job := listResp.Data[0]

	// Fetch full detail by ID.
	var resp getJobResponse
	if err := c.Do(ctx, "GET", fmt.Sprintf("/v1/jobs/%s", job.ID), nil, &resp); err != nil {
		return fmt.Errorf("get job: %w", err)
	}

	if outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}

	j := resp.Job
	status := "enabled"
	if j.Paused {
		status = "paused"
	} else if !j.Enabled {
		status = "disabled"
	}
	source := j.SourceType
	if source == "" {
		source = "image"
	}

	fmt.Printf("ID:         %s\n", j.ID)
	fmt.Printf("Name:       %s\n", j.Name)
	fmt.Printf("Slug:       %s\n", j.Slug)
	fmt.Printf("Status:     %s\n", status)
	fmt.Printf("Source:     %s\n", source)
	fmt.Printf("Version:    %d\n", j.Version)
	fmt.Printf("Created:    %s\n", j.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated:    %s\n", j.UpdatedAt.Format(time.RFC3339))
	if j.ActiveDeploymentID != "" {
		fmt.Printf("Deployment: %s\n", j.ActiveDeploymentID)
	}
	if j.Description != "" {
		fmt.Printf("Desc:       %s\n", j.Description)
	}
	return nil
}

// resolveClientFromContext loads auth from profile + strait.json for read-only
// commands that don't need to resolve a specific job.
func resolveClientFromContext(profileName string) (*cli.Client, error) {
	startDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	var apiURL string
	configPath, _, cfgErr := cli.FindStraitConfig(startDir)
	if cfgErr == nil {
		sc, scErr := cli.LoadStraitConfig(configPath)
		if scErr == nil {
			apiURL = sc.EffectiveAPIURL("")
		}
	}

	authCfg, err := cli.LoadConfig()
	if err != nil {
		return nil, err
	}
	profile := authCfg.ActiveProfileData(profileName)
	if apiURL != "" {
		profile.APIURL = apiURL
	}
	if profile.APIKey == "" {
		profile.APIKey = os.Getenv("STRAIT_API_KEY")
	}
	if profile.APIKey == "" {
		return nil, fmt.Errorf("no API key found — set STRAIT_API_KEY or run 'strait auth login'")
	}
	return cli.NewClient(profile), nil
}
