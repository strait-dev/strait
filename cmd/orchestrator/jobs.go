package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"orchestrator/internal/cli/client"

	"github.com/spf13/cobra"
)

func newJobsCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "Manage jobs",
	}

	cmd.AddCommand(newJobsListCommand(state))
	cmd.AddCommand(newJobsGetCommand(state))
	cmd.AddCommand(newJobsCreateCommand(state))
	cmd.AddCommand(newJobsTriggerCommand(state))

	return cmd
}

func newJobsListCommand(state *appState) *cobra.Command {
	var projectID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List jobs",
		RunE: func(_ *cobra.Command, _ []string) error {
			if projectID == "" {
				projectID = state.opts.projectID
			}
			if projectID == "" {
				return fmt.Errorf("project ID is required (use --project)")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			jobs, err := cli.ListJobs(context.Background(), projectID)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(jobs))
			for _, job := range jobs {
				rows = append(rows, map[string]any{
					"id":      job.ID,
					"name":    job.Name,
					"slug":    job.Slug,
					"cron":    job.Cron,
					"enabled": job.Enabled,
				})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")

	return cmd
}

func newJobsGetCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <job-id>",
		Short: "Get a job by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			job, err := cli.GetJob(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printData(state, job)
		},
	}

	return cmd
}

func newJobsCreateCommand(state *appState) *cobra.Command {
	var req client.CreateJobRequest

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a job",
		RunE: func(_ *cobra.Command, _ []string) error {
			if req.ProjectID == "" {
				req.ProjectID = state.opts.projectID
			}
			if req.ProjectID == "" || req.Name == "" || req.Slug == "" || req.EndpointURL == "" {
				return fmt.Errorf("project, name, slug, and endpoint are required")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			job, err := cli.CreateJob(context.Background(), req)
			if err != nil {
				return err
			}

			return printData(state, job)
		},
	}

	cmd.Flags().StringVar(&req.ProjectID, "project", "", "project ID")
	cmd.Flags().StringVar(&req.Name, "name", "", "job name")
	cmd.Flags().StringVar(&req.Slug, "slug", "", "job slug")
	cmd.Flags().StringVar(&req.Description, "description", "", "job description")
	cmd.Flags().StringVar(&req.Cron, "cron", "", "cron schedule")
	cmd.Flags().StringVar(&req.EndpointURL, "endpoint", "", "job endpoint URL")
	cmd.Flags().IntVar(&req.TimeoutSecs, "timeout-secs", 60, "execution timeout in seconds")
	cmd.Flags().IntVar(&req.MaxAttempts, "max-attempts", 3, "max attempts")
	cmd.Flags().IntVar(&req.RunTTLSecs, "run-ttl-secs", 0, "run TTL in seconds")

	return cmd
}

func newJobsTriggerCommand(state *appState) *cobra.Command {
	var payload string
	var payloadFile string
	var priority int
	var scheduledAt string
	var idempotencyKey string

	cmd := &cobra.Command{
		Use:   "trigger <job-id>",
		Short: "Trigger a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			req := client.TriggerJobRequest{Priority: priority}

			if strings.TrimSpace(scheduledAt) != "" {
				ts, err := time.Parse(time.RFC3339, scheduledAt)
				if err != nil {
					return fmt.Errorf("invalid scheduled-at: %w", err)
				}
				req.ScheduledAt = &ts
			}

			if payloadFile != "" {
				raw, err := os.ReadFile(payloadFile) //nolint:gosec // user-provided local file path for explicit CLI input
				if err != nil {
					return err
				}
				req.Payload = json.RawMessage(raw)
			} else if strings.TrimSpace(payload) != "" {
				req.Payload = json.RawMessage(payload)
			}

			if len(req.Payload) > 0 && !json.Valid(req.Payload) {
				return fmt.Errorf("payload must be valid JSON")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			resp, err := cli.TriggerJob(context.Background(), args[0], req, idempotencyKey)
			if err != nil {
				return err
			}

			return printData(state, resp)
		},
	}

	cmd.Flags().StringVar(&payload, "payload", "", "inline JSON payload")
	cmd.Flags().StringVar(&payloadFile, "payload-file", "", "path to payload JSON file")
	cmd.Flags().IntVar(&priority, "priority", 0, "run priority")
	cmd.Flags().StringVar(&scheduledAt, "scheduled-at", "", "RFC3339 timestamp")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "idempotency key")

	return cmd
}
