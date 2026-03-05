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
	cmd.AddCommand(newWorkflowsCreateCommand(state))
	cmd.AddCommand(newWorkflowsTriggerCommand(state))

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
