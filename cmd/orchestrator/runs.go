package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newRunsCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "Manage runs",
	}

	cmd.AddCommand(newRunsListCommand(state))
	cmd.AddCommand(newRunsGetCommand(state))
	cmd.AddCommand(newRunsCancelCommand(state))
	cmd.AddCommand(newRunsLogsCommand(state))

	return cmd
}

func newRunsListCommand(state *appState) *cobra.Command {
	var projectID string
	var status string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List runs",
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

			runs, err := cli.ListRuns(context.Background(), projectID, status, limit)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(runs))
			for _, run := range runs {
				rows = append(rows, map[string]any{
					"id":           run.ID,
					"job_id":       run.JobID,
					"status":       run.Status,
					"attempt":      run.Attempt,
					"triggered_by": run.TriggeredBy,
					"created_at":   run.CreatedAt,
				})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&status, "status", "", "status filter")
	cmd.Flags().IntVar(&limit, "limit", 50, "max runs to return")

	return cmd
}

func newRunsGetCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <run-id>",
		Short: "Get run by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			run, err := cli.GetRun(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printData(state, run)
		},
	}

	return cmd
}

func newRunsCancelCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel <run-id>",
		Short: "Cancel run by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			run, err := cli.CancelRun(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printData(state, run)
		},
	}

	return cmd
}

func newRunsLogsCommand(state *appState) *cobra.Command {
	var follow bool
	var interval time.Duration
	var level string
	var eventType string

	cmd := &cobra.Command{
		Use:   "logs <run-id>",
		Short: "Show run events/logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			seen := map[string]struct{}{}
			for {
				events, err := cli.ListRunEvents(context.Background(), args[0], level, eventType)
				if err != nil {
					return err
				}

				rows := make([]map[string]any, 0, len(events))
				for _, event := range events {
					if _, ok := seen[event.ID]; ok {
						continue
					}
					seen[event.ID] = struct{}{}
					rows = append(rows, map[string]any{
						"timestamp": event.CreatedAt,
						"level":     event.Level,
						"type":      event.Type,
						"message":   event.Message,
					})
				}
				if len(rows) > 0 {
					if err := printData(state, rows); err != nil {
						return err
					}
				}

				if !follow {
					return nil
				}
				time.Sleep(interval)
			}
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream logs by polling events")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "poll interval when following")
	cmd.Flags().StringVar(&level, "level", "", "event level filter")
	cmd.Flags().StringVar(&eventType, "type", "", "event type filter")

	return cmd
}
