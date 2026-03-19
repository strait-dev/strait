package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"strait/internal/cli/client"
	"strait/internal/cli/dag"
	"strait/internal/cli/styles"

	"github.com/spf13/cobra"
)

func newWorkflowsCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflows",
		Short: "Manage workflows",
	}

	cmd.AddCommand(newWorkflowsListCommand(state))
	cmd.AddCommand(newWorkflowsGetCommand(state))
	cmd.AddCommand(newWorkflowsDescribeCommand(state))
	cmd.AddCommand(newWorkflowsCreateCommand(state))
	cmd.AddCommand(newWorkflowsUpdateCommand(state))
	cmd.AddCommand(newWorkflowsDeleteCommand(state))
	cmd.AddCommand(newWorkflowsRunsCommand(state))
	cmd.AddCommand(newWorkflowsTriggerCommand(state))
	cmd.AddCommand(newWorkflowsVisualizeCommand(state))

	return cmd
}

func newWorkflowsDescribeCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe <workflow-id-or-slug>",
		Short: "Show workflow details and step dependency view",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			workflowID, err := resolveWorkflowIdentifier(cmd.Context(), cli, state, args[0])
			if err != nil {
				return err
			}

			wf, err := cli.GetWorkflow(cmd.Context(), workflowID)
			if err != nil {
				return err
			}

			steps := make([]map[string]any, 0, len(wf.Steps))
			for _, step := range wf.Steps {
				deps := "-"
				if len(step.DependsOn) > 0 {
					deps = strings.Join(step.DependsOn, ",")
				}
				steps = append(steps, map[string]any{
					"step_ref":   step.StepRef,
					"job_id":     step.JobID,
					"depends_on": deps,
					"on_failure": step.OnFailure,
				})
			}

			payload := map[string]any{
				"workflow": map[string]any{
					"id":          wf.ID,
					"project_id":  wf.ProjectID,
					"name":        wf.Name,
					"slug":        wf.Slug,
					"description": wf.Description,
					"enabled":     wf.Enabled,
					"version":     wf.Version,
				},
				"steps": steps,
			}

			return printData(state, payload)
		},
	}

	return cmd
}

func newWorkflowsListCommand(state *appState) *cobra.Command {
	var projectID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflows",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			workflows, err := cli.ListWorkflows(cmd.Context(), projectID)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(workflows))
			for _, wf := range workflows {
				rows = append(rows, map[string]any{
					"id":      wf.ID,
					"name":    wf.Name,
					"slug":    wf.Slug,
					"enabled": styles.Enabled(wf.Enabled),
				})
			}
			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")

	return cmd
}

func newWorkflowsGetCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <workflow-id-or-slug>",
		Short: "Get workflow by ID or slug",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			workflowID, err := resolveWorkflowIdentifier(cmd.Context(), cli, state, args[0])
			if err != nil {
				return err
			}

			wf, err := cli.GetWorkflow(cmd.Context(), workflowID)
			if err != nil {
				return err
			}
			return printData(state, wf)
		},
	}

	return cmd
}

func newWorkflowsCreateCommand(state *appState) *cobra.Command {
	var projectID string
	var name string
	var slug string
	var description string
	var stepsJSON string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create workflow",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if projectID == "" {
				projectID = state.opts.projectID
			}
			if projectID == "" || name == "" || slug == "" {
				return fmt.Errorf("project, name, and slug are required")
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
	cmd.Flags().StringVar(&slug, "slug", "", "workflow slug")
	cmd.Flags().StringVar(&description, "description", "", "workflow description")
	cmd.Flags().StringVar(&stepsJSON, "steps-json", "", "JSON array of workflow steps")

	return cmd
}

func newWorkflowsUpdateCommand(state *appState) *cobra.Command {
	var name string
	var slug string
	var description string
	var enabled bool
	var stepsJSON string

	cmd := &cobra.Command{
		Use:   "update <workflow-id-or-slug>",
		Short: "Update a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := client.UpdateWorkflowRequest{}

			if cmd.Flags().Changed("name") {
				req.Name = &name
			}
			if cmd.Flags().Changed("slug") {
				req.Slug = &slug
			}
			if cmd.Flags().Changed("description") {
				req.Description = &description
			}
			if cmd.Flags().Changed("enabled") {
				req.Enabled = &enabled
			}
			if cmd.Flags().Changed("steps-json") {
				steps := make([]client.WorkflowStepRequest, 0)
				if strings.TrimSpace(stepsJSON) != "" {
					if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
						return fmt.Errorf("invalid --steps-json: %w", err)
					}
				}
				req.Steps = &steps
			}

			if req.Name == nil && req.Slug == nil && req.Description == nil && req.Enabled == nil && req.Steps == nil {
				return fmt.Errorf("at least one update flag is required")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			workflowID, err := resolveWorkflowIdentifier(cmd.Context(), cli, state, args[0])
			if err != nil {
				return err
			}

			wf, err := cli.UpdateWorkflow(cmd.Context(), workflowID, req)
			if err != nil {
				return err
			}

			return printData(state, wf)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "workflow name")
	cmd.Flags().StringVar(&slug, "slug", "", "workflow slug")
	cmd.Flags().StringVar(&description, "description", "", "workflow description")
	cmd.Flags().BoolVar(&enabled, "enabled", false, "workflow enabled state")
	cmd.Flags().StringVar(&stepsJSON, "steps-json", "", "JSON array of workflow steps (set empty string to clear)")

	return cmd
}

func newWorkflowsDeleteCommand(state *appState) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <workflow-id-or-slug>",
		Short: "Delete a workflow by ID or slug",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireConfirmation(state, "Delete this workflow?", yes); err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			workflowID, err := resolveWorkflowIdentifier(cmd.Context(), cli, state, args[0])
			if err != nil {
				return err
			}

			if err := cli.DeleteWorkflow(cmd.Context(), workflowID); err != nil {
				return err
			}

			return printData(state, map[string]any{"deleted": true, "id": workflowID})
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")

	return cmd
}

func newWorkflowsRunsCommand(state *appState) *cobra.Command {
	var limit int
	var offset int

	cmd := &cobra.Command{
		Use:   "runs <workflow-id-or-slug>",
		Short: "List runs for a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 0 {
				return fmt.Errorf("limit must be non-negative")
			}
			if offset < 0 {
				return fmt.Errorf("offset must be non-negative")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			workflowID, err := resolveWorkflowIdentifier(cmd.Context(), cli, state, args[0])
			if err != nil {
				return err
			}

			runs, err := cli.ListWorkflowRuns(cmd.Context(), workflowID, limit, offset)
			if err != nil {
				return err
			}

			return printData(state, runs)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "max runs to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")

	return cmd
}

func newWorkflowsTriggerCommand(state *appState) *cobra.Command {
	var payload string
	var payloadFile string

	cmd := &cobra.Command{
		Use:   "trigger <workflow-id-or-slug>",
		Short: "Trigger workflow by ID or slug",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			workflowID, err := resolveWorkflowIdentifier(cmd.Context(), cli, state, args[0])
			if err != nil {
				return err
			}

			req := client.TriggerWorkflowRequest{ProjectID: state.opts.projectID}
			if payloadFile != "" {
				raw, err := os.ReadFile(payloadFile) //nolint:gosec // payloadFile is from --payload-file CLI flag
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

			run, err := cli.TriggerWorkflow(cmd.Context(), workflowID, req)
			if err != nil {
				return err
			}
			return printData(state, run)
		},
	}

	cmd.Flags().StringVar(&payload, "payload", "", "inline JSON payload")
	cmd.Flags().StringVar(&payloadFile, "payload-file", "", "path to payload JSON file")

	return cmd
}

func resolveWorkflowIdentifier(ctx context.Context, cli *client.Client, state *appState, idOrSlug string) (string, error) {
	if _, err := cli.GetWorkflow(ctx, idOrSlug); err == nil {
		return idOrSlug, nil
	}

	projectID, err := requireProjectID(state, "")
	if err != nil {
		return "", fmt.Errorf("project is required to resolve slug %q", idOrSlug)
	}

	workflows, err := cli.ListWorkflows(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("resolving workflow %q: %w", idOrSlug, err)
	}

	for _, workflow := range workflows {
		if workflow.Slug == idOrSlug {
			return workflow.ID, nil
		}
	}

	return "", fmt.Errorf("workflow %q not found", idOrSlug)
}

func newWorkflowsVisualizeCommand(state *appState) *cobra.Command {
	var workflowRunID string

	cmd := &cobra.Command{
		Use:     "visualize <workflow-id-or-slug>",
		Short:   "Render workflow step DAG as a text diagram",
		Long:    "Fetches the workflow definition and renders a topologically sorted DAG with box-drawing characters. Optionally colors nodes by step run status.",
		Example: "  strait workflows visualize my-workflow\n  strait workflows visualize my-workflow --run wfr_123",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			workflowID, err := resolveWorkflowIdentifier(cmd.Context(), cli, state, args[0])
			if err != nil {
				return err
			}

			wf, err := cli.GetWorkflow(cmd.Context(), workflowID)
			if err != nil {
				return err
			}

			steps := make([]dag.Step, 0, len(wf.Steps))
			for _, s := range wf.Steps {
				steps = append(steps, dag.Step{
					StepRef:   s.StepRef,
					DependsOn: s.DependsOn,
				})
			}

			// Optionally fetch step run statuses for coloring
			var statusMap map[string]string
			if workflowRunID != "" {
				stepRuns, listErr := cli.ListWorkflowStepRuns(cmd.Context(), workflowRunID)
				if listErr != nil {
					return fmt.Errorf("fetching step runs: %w", listErr)
				}
				statusMap = make(map[string]string, len(stepRuns))
				for _, sr := range stepRuns {
					statusMap[sr.StepRef] = string(sr.Status)
				}
			}

			rendered := dag.RenderDAG(steps, statusMap)
			fmt.Print(rendered)
			return nil
		},
	}

	cmd.Flags().StringVar(&workflowRunID, "run", "", "workflow run ID to color nodes by step status")

	return cmd
}
