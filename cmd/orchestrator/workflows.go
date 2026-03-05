package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"orchestrator/internal/cli/client"

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
	cmd.AddCommand(newWorkflowsTriggerCommand(state))

	return cmd
}

func newWorkflowsDescribeCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe <workflow-id>",
		Short: "Show workflow details and step dependency view",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			wf, err := cli.GetWorkflow(context.Background(), args[0])
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
		RunE: func(_ *cobra.Command, _ []string) error {
			if projectID == "" {
				projectID = state.opts.projectID
			}
			if projectID == "" {
				return fmt.Errorf("project ID is required")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			workflows, err := cli.ListWorkflows(context.Background(), projectID)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(workflows))
			for _, wf := range workflows {
				rows = append(rows, map[string]any{
					"id":      wf.ID,
					"name":    wf.Name,
					"slug":    wf.Slug,
					"enabled": wf.Enabled,
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
		Use:   "get <workflow-id>",
		Short: "Get workflow by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			wf, err := cli.GetWorkflow(context.Background(), args[0])
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
		RunE: func(_ *cobra.Command, _ []string) error {
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
			wf, err := cli.CreateWorkflow(context.Background(), req)
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
		Use:   "update <workflow-id>",
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

			wf, err := cli.UpdateWorkflow(context.Background(), args[0], req)
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
		Use:   "delete <workflow-id>",
		Short: "Delete a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("delete requires confirmation; rerun with --yes")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			if err := cli.DeleteWorkflow(context.Background(), args[0]); err != nil {
				return err
			}

			return printData(state, map[string]any{"deleted": true, "id": args[0]})
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")

	return cmd
}

func newWorkflowsTriggerCommand(state *appState) *cobra.Command {
	var payload string

	cmd := &cobra.Command{
		Use:   "trigger <workflow-id>",
		Short: "Trigger workflow by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			req := client.TriggerWorkflowRequest{ProjectID: state.opts.projectID}
			if strings.TrimSpace(payload) != "" {
				req.Payload = json.RawMessage(payload)
				if !json.Valid(req.Payload) {
					return fmt.Errorf("payload must be valid JSON")
				}
			}

			run, err := cli.TriggerWorkflow(context.Background(), args[0], req)
			if err != nil {
				return err
			}
			return printData(state, run)
		},
	}

	cmd.Flags().StringVar(&payload, "payload", "", "inline JSON payload")

	return cmd
}
