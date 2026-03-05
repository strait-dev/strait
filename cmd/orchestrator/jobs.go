package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/cli/client"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
	cmd.AddCommand(newJobsVersionsCommand(state))
	cmd.AddCommand(newJobsDescribeCommand(state))
	cmd.AddCommand(newJobsEditCommand(state))

	return cmd
}

func newJobsDeleteCommand(state *appState) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <job-id-or-slug>",
		Short: "Disable a job by ID or slug",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("delete requires confirmation; rerun with --yes")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			jobID, err := resolveJobIdentifier(context.Background(), cli, state, args[0])
			if err != nil {
				return err
			}

			if err := cli.DeleteJob(context.Background(), jobID); err != nil {
				return err
			}

			return printData(state, map[string]any{"deleted": true, "id": jobID})
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")

	return cmd
}

func newJobsVersionsCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "versions <job-id-or-slug>",
		Short: "List version history for a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			jobID, err := resolveJobIdentifier(context.Background(), cli, state, args[0])
			if err != nil {
				return err
			}

			versions, err := cli.ListJobVersions(context.Background(), jobID)
			if err != nil {
				return err
			}

			return printData(state, versions)
		},
	}

	return cmd
}

func newJobsDescribeCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe <job-id-or-slug>",
		Short: "Show rich details and recent runs for a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			jobID, err := resolveJobIdentifier(context.Background(), cli, state, args[0])
			if err != nil {
				return err
			}

			job, err := cli.GetJob(context.Background(), jobID)
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
	var editor string

	cmd := &cobra.Command{
		Use:   "edit <job-id-or-slug>",
		Short: "Edit a job via --field or interactive editor",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			jobID, err := resolveJobIdentifier(context.Background(), cli, state, args[0])
			if err != nil {
				return err
			}

			if strings.TrimSpace(field) == "" {
				return runInteractiveJobEdit(context.Background(), cli, state, jobID, editor)
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

			job, err := cli.UpdateJob(context.Background(), jobID, upd)
			if err != nil {
				return err
			}

			return printData(state, job)
		},
	}

	cmd.Flags().StringVar(&field, "field", "", "field update in key=value form")
	cmd.Flags().StringVar(&editor, "editor", "", "editor command for interactive mode")

	return cmd
}

type editableJob struct {
	Name        string `yaml:"name"`
	Slug        string `yaml:"slug"`
	Description string `yaml:"description,omitempty"`
	Cron        string `yaml:"cron,omitempty"`
	EndpointURL string `yaml:"endpoint_url"`
	MaxAttempts int    `yaml:"max_attempts"`
	TimeoutSecs int    `yaml:"timeout_secs"`
	RunTTLSecs  int    `yaml:"run_ttl_secs,omitempty"`
	Enabled     bool   `yaml:"enabled"`
}

func runInteractiveJobEdit(ctx context.Context, cli *client.Client, state *appState, jobID, editorOverride string) error {
	job, err := cli.GetJob(ctx, jobID)
	if err != nil {
		return err
	}

	original := editableJob{
		Name:        job.Name,
		Slug:        job.Slug,
		Description: job.Description,
		Cron:        job.Cron,
		EndpointURL: job.EndpointURL,
		MaxAttempts: job.MaxAttempts,
		TimeoutSecs: job.TimeoutSecs,
		RunTTLSecs:  job.RunTTLSecs,
		Enabled:     job.Enabled,
	}

	tmp, err := os.CreateTemp("", "orchestrator-job-edit-*.yaml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if closeErr := tmp.Close(); closeErr != nil {
		return closeErr
	}
	defer os.Remove(tmpPath)

	encoded, err := yaml.Marshal(original)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, encoded, 0o600); err != nil { //nolint:gosec
		return err
	}

	editor := strings.TrimSpace(editorOverride)
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, tmpPath) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	raw, err := os.ReadFile(tmpPath) //nolint:gosec
	if err != nil {
		return err
	}
	updated := editableJob{}
	if err := yaml.Unmarshal(raw, &updated); err != nil {
		return err
	}

	if updated == original {
		return printData(state, map[string]any{"updated": false, "reason": "no changes"})
	}

	upd := client.UpdateJobRequest{
		Name:        &updated.Name,
		Slug:        &updated.Slug,
		Description: &updated.Description,
		Cron:        &updated.Cron,
		EndpointURL: &updated.EndpointURL,
		MaxAttempts: &updated.MaxAttempts,
		TimeoutSecs: &updated.TimeoutSecs,
		RunTTLSecs:  &updated.RunTTLSecs,
		Enabled:     &updated.Enabled,
	}
	patched, err := cli.UpdateJob(ctx, jobID, upd)
	if err != nil {
		return err
	}

	return printData(state, patched)
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
		Use:   "get <job-id-or-slug>",
		Short: "Get a job by ID or slug",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			jobID, err := resolveJobIdentifier(context.Background(), cli, state, args[0])
			if err != nil {
				return err
			}

			job, err := cli.GetJob(context.Background(), jobID)
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
		Use:   "trigger <job-id-or-slug>",
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

			jobID, err := resolveJobIdentifier(context.Background(), cli, state, args[0])
			if err != nil {
				return err
			}

			resp, err := cli.TriggerJob(context.Background(), jobID, req, idempotencyKey)
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
