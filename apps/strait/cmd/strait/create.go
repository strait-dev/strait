package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"strait/internal/cli/client"

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
		Long:  "Create a new job via flags or JSON input.",
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
				return printData(state, job)
			}

			var err error
			projectID, err = requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			if name == "" {
				return fmt.Errorf("--name is required")
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
		Long:  "Create a new workflow via flags or JSON input.",
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
				return printData(state, wf)
			}

			var err error
			projectID, err = requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			if name == "" {
				return fmt.Errorf("--name is required")
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
