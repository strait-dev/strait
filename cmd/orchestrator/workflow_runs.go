package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newWorkflowRunsCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow-runs",
		Short: "Manage workflow runs",
	}

	cmd.AddCommand(newWorkflowRunsListCommand(state))
	cmd.AddCommand(newWorkflowRunsGetCommand(state))
	cmd.AddCommand(newWorkflowRunsCancelCommand(state))
	cmd.AddCommand(newWorkflowRunsStepsCommand(state))

	return cmd
}

func newWorkflowRunsListCommand(state *appState) *cobra.Command {
	var projectID string
	var status string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflow runs",
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
			runs, err := cli.ListWorkflowRunsByProject(context.Background(), projectID, status, limit)
			if err != nil {
				return err
			}
			return printData(state, runs)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&status, "status", "", "status filter")
	cmd.Flags().IntVar(&limit, "limit", 50, "max runs")

	return cmd
}

func newWorkflowRunsGetCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <workflow-run-id>",
		Short: "Get workflow run by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			run, err := cli.GetWorkflowRun(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printData(state, run)
		},
	}

	return cmd
}

func newWorkflowRunsCancelCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel <workflow-run-id>",
		Short: "Cancel workflow run",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			run, err := cli.CancelWorkflowRun(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printData(state, run)
		},
	}

	return cmd
}

func newWorkflowRunsStepsCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "steps <workflow-run-id>",
		Short: "List workflow step runs",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			steps, err := cli.ListWorkflowStepRuns(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printData(state, steps)
		},
	}

	return cmd
}
