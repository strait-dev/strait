package main

import (
	"fmt"
	"time"

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
	cmd.AddCommand(newWorkflowRunsWatchCommand(state))

	return cmd
}

func newWorkflowRunsWatchCommand(state *appState) *cobra.Command {
	var interval time.Duration
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "watch <workflow-run-id>",
		Short: "Watch workflow run status and step progression",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			deadline := time.Now().Add(timeout)
			for {
				run, err := cli.GetWorkflowRun(ctx, args[0])
				if err != nil {
					return err
				}

				steps, err := cli.ListWorkflowStepRuns(ctx, args[0])
				if err != nil {
					return err
				}

				payload := map[string]any{
					"run":   run,
					"steps": steps,
				}
				if err := printData(state, payload); err != nil {
					return err
				}

				if run.Status.IsTerminal() {
					if run.Status == "completed" {
						return nil
					}
					return fmt.Errorf("workflow run terminal status %s", run.Status)
				}

				if timeout > 0 && time.Now().After(deadline) {
					return fmt.Errorf("workflow watch timeout reached")
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(interval):
				}
			}
		},
	}

	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "poll interval")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "max watch duration")

	return cmd
}

func newWorkflowRunsListCommand(state *appState) *cobra.Command {
	var projectID string
	var status string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflow runs",
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			runs, err := cli.ListWorkflowRunsByProject(cmd.Context(), projectID, status, limit)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			run, err := cli.GetWorkflowRun(cmd.Context(), args[0])
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
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			run, err := cli.CancelWorkflowRun(cmd.Context(), args[0])
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
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			steps, err := cli.ListWorkflowStepRuns(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return printData(state, steps)
		},
	}

	return cmd
}
