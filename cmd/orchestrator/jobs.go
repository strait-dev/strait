package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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
	cmd.AddCommand(newJobsDeleteCommand(state))
	cmd.AddCommand(newJobsDescribeCommand(state))
	cmd.AddCommand(newJobsEditCommand(state))

	return cmd
}

func newJobsDeleteCommand(state *appState) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <job-id>",
		Short: "Disable a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("delete requires confirmation; rerun with --yes")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			if err := cli.DeleteJob(context.Background(), args[0]); err != nil {
				return err
			}

			return printData(state, map[string]any{"deleted": true, "id": args[0]})
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")

	return cmd
}

func newJobsDescribeCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe <job-id>",
		Short: "Show rich details and recent runs for a job",
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

			runs, err := cli.ListRuns(context.Background(), job.ProjectID, "", 100)
			if err != nil {
				return err
			}

			recent := make([]map[string]any, 0, 10)
			for _, run := range runs {
				if run.JobID != job.ID {
					continue
				}
				recent = append(recent, map[string]any{
					"id":          run.ID,
					"status":      run.Status,
					"attempt":     run.Attempt,
					"triggeredBy": run.TriggeredBy,
					"createdAt":   run.CreatedAt,
				})
				if len(recent) >= 10 {
					break
				}
			}

			payload := map[string]any{
				"job":         job,
				"recent_runs": recent,
			}
			return printData(state, payload)
		},
	}

	return cmd
}

func newJobsEditCommand(state *appState) *cobra.Command {
	var field string

	cmd := &cobra.Command{
		Use:   "edit <job-id>",
		Short: "Update a job field via --field key=value",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if strings.TrimSpace(field) == "" {
				return fmt.Errorf("--field key=value is required")
			}

			parts := strings.SplitN(field, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid --field format, expected key=value")
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			upd := client.UpdateJobRequest{}
			switch key {
			case "name":
				upd.Name = &val
			case "slug":
				upd.Slug = &val
			case "description":
				upd.Description = &val
			case "cron":
				upd.Cron = &val
			case "endpoint_url", "endpoint":
				upd.EndpointURL = &val
			case "enabled":
				parsed, err := strconv.ParseBool(val)
				if err != nil {
					return fmt.Errorf("enabled must be true|false")
				}
				upd.Enabled = &parsed
			case "max_attempts":
				parsed, err := strconv.Atoi(val)
				if err != nil {
					return fmt.Errorf("max_attempts must be an integer")
				}
				upd.MaxAttempts = &parsed
			case "timeout_secs":
				parsed, err := strconv.Atoi(val)
				if err != nil {
					return fmt.Errorf("timeout_secs must be an integer")
				}
				upd.TimeoutSecs = &parsed
			case "run_ttl_secs":
				parsed, err := strconv.Atoi(val)
				if err != nil {
					return fmt.Errorf("run_ttl_secs must be an integer")
				}
				upd.RunTTLSecs = &parsed
			default:
				return fmt.Errorf("unsupported field %q", key)
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			job, err := cli.UpdateJob(context.Background(), args[0], upd)
			if err != nil {
				return err
			}

			return printData(state, job)
		},
	}

	cmd.Flags().StringVar(&field, "field", "", "field update in key=value form")

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
