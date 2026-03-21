package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"strait/internal/cli/client"
	"strait/internal/cli/styles"
	"strait/internal/cli/wizard"

	"github.com/spf13/cobra"
)

func newCreateCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create resources from flags or JSON input",
	}

	cmd.AddCommand(newCreateJobCommand(state))
	cmd.AddCommand(newCreateWorkflowCommand(state))

	return cmd
}

func newCreateJobCommand(state *appState) *cobra.Command {
	var (
		projectID   string
		name        string
		slug        string
		endpoint    string
		cronExpr    string
		timeout     int
		maxAttempts int
		asJSON      bool
	)

	cmd := &cobra.Command{
		Use:   "job",
		Short: "Create a new job",
		Long: `Create a new job via flags, JSON input, or interactive wizard.

When run without --name or --json in a TTY, an interactive wizard guides you through job creation.`,
		Example: `  strait create job
  strait create job --name my-job --endpoint http://localhost:3000/jobs/my-job --project proj-1
  echo '{"name":"my-job","endpoint_url":"http://localhost:3000"}' | strait create job --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// JSON mode: read from stdin
			if asJSON {
				var req client.CreateJobRequest
				if err := json.NewDecoder(cmd.InOrStdin()).Decode(&req); err != nil {
					return fmt.Errorf("invalid JSON input: %w", err)
				}
				if req.ProjectID == "" {
					resolved, err := requireProjectID(state, projectID)
					if err != nil {
						return err
					}
					req.ProjectID = resolved
				}

				cli, err := newAPIClient(state)
				if err != nil {
					return err
				}
				job, err := cli.CreateJob(cmd.Context(), req)
				if err != nil {
					return err
				}
				if isTTYRich(state) {
					fmt.Fprintln(os.Stderr, styles.Success("Created job "+styles.Bold.Render(job.Slug)))
					fmt.Fprintln(os.Stderr, styles.KeyValue("ID", job.ID))
					return nil
				}
				return printData(state, job)
			}

			// Interactive mode: no name provided and TTY available
			if name == "" && stdoutIsTTY() {
				result, wizErr := wizard.RunCreateJobWizard()
				if wizErr != nil {
					return wizErr
				}
				name = result.Name
				slug = result.Slug
				endpoint = result.Endpoint
				cronExpr = result.Cron
				timeout = result.Timeout
				maxAttempts = result.MaxAttempts
			}

			var err error
			projectID, err = requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			if name == "" {
				return fmt.Errorf("--name is required (or run interactively in a TTY)")
			}

			if slug == "" {
				slug = generateSlug(name)
			}

			if endpoint == "" {
				return fmt.Errorf("--endpoint is required")
			}

			req := client.CreateJobRequest{
				ProjectID:   projectID,
				Name:        name,
				Slug:        slug,
				EndpointURL: endpoint,
				Cron:        cronExpr,
				TimeoutSecs: timeout,
				MaxAttempts: maxAttempts,
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			job, err := cli.CreateJob(cmd.Context(), req)
			if err != nil {
				return err
			}
			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.Success("Created job "+styles.Bold.Render(job.Slug)))
				fmt.Fprintln(os.Stderr, styles.KeyValue("ID", job.ID))
				fmt.Fprintln(os.Stderr, styles.KeyValue("Endpoint", job.EndpointURL))
				if job.Cron != "" {
					fmt.Fprintln(os.Stderr, styles.KeyValue("Schedule", job.Cron))
				}
				fmt.Fprintln(os.Stderr, styles.KeyValue("Timeout", fmt.Sprintf("%ds", job.TimeoutSecs)))
				fmt.Fprintln(os.Stderr, styles.KeyValue("Retries", fmt.Sprintf("%d", job.MaxAttempts)))
				fmt.Fprintln(os.Stderr)
				fmt.Fprintln(os.Stderr, styles.MutedStyle.Render("  Trigger: strait trigger "+job.Slug+" --payload '{}'"))
				return nil
			}
			return printData(state, job)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&name, "name", "", "job name")
	cmd.Flags().StringVar(&slug, "slug", "", "job slug (auto-generated from name if omitted)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "endpoint URL")
	cmd.Flags().StringVar(&cronExpr, "cron", "", "cron schedule expression")
	cmd.Flags().IntVar(&timeout, "timeout", 60, "timeout in seconds")
	cmd.Flags().IntVar(&maxAttempts, "max-attempts", 3, "maximum retry attempts")
	cmd.Flags().BoolVar(&asJSON, "json", false, "read job definition from stdin as JSON")

	return cmd
}

func newCreateWorkflowCommand(state *appState) *cobra.Command {
	var (
		projectID   string
		name        string
		slug        string
		description string
		stepsJSON   string
		asJSON      bool
	)

	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Create a new workflow",
		Long: `Create a new workflow via flags, JSON input, or interactive wizard.

When run without --name or --json in a TTY, an interactive wizard guides you through workflow creation with a step builder.`,
		Example: `  strait create workflow
  strait create workflow --name data-pipeline --project proj-1
  echo '{"name":"pipeline","steps":[...]}' | strait create workflow --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// JSON mode: read from stdin
			if asJSON {
				var req client.CreateWorkflowRequest
				if err := json.NewDecoder(cmd.InOrStdin()).Decode(&req); err != nil {
					return fmt.Errorf("invalid JSON input: %w", err)
				}
				if req.ProjectID == "" {
					resolved, err := requireProjectID(state, projectID)
					if err != nil {
						return err
					}
					req.ProjectID = resolved
				}

				cli, err := newAPIClient(state)
				if err != nil {
					return err
				}
				wf, err := cli.CreateWorkflow(cmd.Context(), req)
				if err != nil {
					return err
				}
				if isTTYRich(state) {
					fmt.Fprintln(os.Stderr, styles.Success("Created workflow "+styles.Bold.Render(wf.Slug)))
					fmt.Fprintln(os.Stderr, styles.KeyValue("ID", wf.ID))
					return nil
				}
				return printData(state, wf)
			}

			// Interactive mode: no name provided and TTY available
			if name == "" && stdoutIsTTY() {
				result, wizErr := wizard.RunCreateWorkflowWizard()
				if wizErr != nil {
					return wizErr
				}
				name = result.Name
				slug = result.Slug
				description = result.Description
				// Convert wizard steps to JSON for the steps field
				if len(result.Steps) > 0 {
					var steps []map[string]any
					for _, s := range result.Steps {
						step := map[string]any{
							"step_ref": s.StepRef,
							"job_ref":  s.JobSlug,
						}
						if s.DependsOn != "" {
							deps := strings.Split(s.DependsOn, ",")
							var trimmed []string
							for _, d := range deps {
								d = strings.TrimSpace(d)
								if d != "" {
									trimmed = append(trimmed, d)
								}
							}
							if len(trimmed) > 0 {
								step["depends_on"] = trimmed
							}
						}
						steps = append(steps, step)
					}
					stepsBytes, marshalErr := json.Marshal(steps)
					if marshalErr != nil {
						return fmt.Errorf("encode steps: %w", marshalErr)
					}
					stepsJSON = string(stepsBytes)
				}
			}

			var err error
			projectID, err = requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			if name == "" {
				return fmt.Errorf("--name is required (or run interactively in a TTY)")
			}

			if slug == "" {
				slug = generateSlug(name)
			}

			req := client.CreateWorkflowRequest{
				ProjectID:   projectID,
				Name:        name,
				Slug:        slug,
				Description: description,
			}

			if strings.TrimSpace(stepsJSON) != "" {
				if err := json.Unmarshal([]byte(stepsJSON), &req.Steps); err != nil {
					return fmt.Errorf("invalid --steps-json: %w", err)
				}
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			wf, err := cli.CreateWorkflow(cmd.Context(), req)
			if err != nil {
				return err
			}
			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.Success("Created workflow "+styles.Bold.Render(wf.Slug)))
				fmt.Fprintln(os.Stderr, styles.KeyValue("ID", wf.ID))
				if len(req.Steps) > 0 {
					fmt.Fprintln(os.Stderr, styles.KeyValue("Steps", fmt.Sprintf("%d", len(req.Steps))))
				}
				return nil
			}
			return printData(state, wf)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&name, "name", "", "workflow name")
	cmd.Flags().StringVar(&slug, "slug", "", "workflow slug (auto-generated from name if omitted)")
	cmd.Flags().StringVar(&description, "description", "", "workflow description")
	cmd.Flags().StringVar(&stepsJSON, "steps-json", "", "JSON array of workflow steps")
	cmd.Flags().BoolVar(&asJSON, "json", false, "read workflow definition from stdin as JSON")

	return cmd
}

func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '_' || r == '-' {
			return '-'
		}
		return -1
	}, slug)
	// Collapse multiple hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	return slug
}
