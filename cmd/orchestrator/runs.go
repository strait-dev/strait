package main

import (
	"fmt"
	"time"

	"orchestrator/internal/cli/client"
	"orchestrator/internal/cli/styles"
	"orchestrator/internal/domain"

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
	cmd.AddCommand(newRunsWatchCommand(state))
	cmd.AddCommand(newRunsReplayCommand(state))

	return cmd
}

func newRunsListCommand(state *appState) *cobra.Command {
	var projectID string
	var status string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List runs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			runs, err := cli.ListRuns(cmd.Context(), projectID, status, limit, nil)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(runs))
			for _, run := range runs {
				rows = append(rows, map[string]any{
					"id":           run.ID,
					"job_id":       run.JobID,
					"status":       styles.Status(string(run.Status)),
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
	_ = cmd.RegisterFlagCompletionFunc("status", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"delayed", "queued", "dequeued", "executing", "waiting", "completed", "failed", "timed_out", "crashed", "system_failed", "canceled", "expired"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func newRunsGetCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <run-id>",
		Short: "Get run by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			run, err := cli.GetRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return printData(state, run)
		},
	}

	return cmd
}

func newRunsCancelCommand(state *appState) *cobra.Command {
	var all bool
	var projectID string
	var status string
	var limit int
	var yes bool

	cmd := &cobra.Command{
		Use:   "cancel <run-id> [run-id...]",
		Short: "Cancel one or more runs",
		Args: func(_ *cobra.Command, args []string) error {
			if all || len(args) > 0 {
				return nil
			}
			return fmt.Errorf("provide run IDs or use --all")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			targetIDs := make([]string, 0)
			if all {
				projectID, err = requireProjectID(state, projectID)
				if err != nil {
					return err
				}
				runs, listErr := cli.ListRuns(cmd.Context(), projectID, status, limit, nil)
				if listErr != nil {
					return listErr
				}
				for _, run := range runs {
					targetIDs = append(targetIDs, run.ID)
				}
			} else {
				targetIDs = append(targetIDs, args...)
			}

			if len(targetIDs) == 0 {
				return fmt.Errorf("no runs matched cancellation criteria")
			}
			if len(targetIDs) > 1 {
				if err := requireConfirmation(state, fmt.Sprintf("Cancel %d runs?", len(targetIDs)), yes); err != nil {
					return err
				}
			}

			results := make([]map[string]any, 0, len(targetIDs))
			for _, id := range targetIDs {
				run, cancelErr := cli.CancelRun(cmd.Context(), id)
				if cancelErr != nil {
					results = append(results, map[string]any{"id": id, "canceled": false, "error": cancelErr.Error()})
					continue
				}
				results = append(results, map[string]any{"id": id, "canceled": true, "status": run.Status})
			}

			return printData(state, results)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "cancel all runs matching filters")
	cmd.Flags().StringVar(&projectID, "project", "", "project ID for --all mode")
	cmd.Flags().StringVar(&status, "status", "", "status filter for --all mode")
	cmd.Flags().IntVar(&limit, "limit", 100, "max runs to consider for --all mode")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm bulk cancellation")

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
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			seen := map[string]struct{}{}
			for {
				events, err := cli.ListRunEvents(ctx, args[0], level, eventType)
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
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(interval):
				}
			}
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream logs by polling events")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "poll interval when following")
	cmd.Flags().StringVar(&level, "level", "", "event level filter")
	cmd.Flags().StringVar(&eventType, "type", "", "event type filter")
	_ = cmd.RegisterFlagCompletionFunc("level", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"debug", "info", "warn", "error"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("type", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"log", "state_change", "error", "progress"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func newRunsWatchCommand(state *appState) *cobra.Command {
	var interval time.Duration
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "watch <run-id>",
		Short: "Watch a run until it reaches a terminal state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			deadline := time.Now().Add(timeout)
			for {
				run, err := cli.GetRun(ctx, args[0])
				if err != nil {
					return err
				}

				if err := printData(state, map[string]any{
					"id":      run.ID,
					"status":  run.Status,
					"attempt": run.Attempt,
				}); err != nil {
					return err
				}

				if run.Status.IsTerminal() {
					if run.Status == domain.StatusCompleted {
						return nil
					}
					return fmt.Errorf("run reached terminal status %q", run.Status)
				}

				if timeout > 0 && time.Now().After(deadline) {
					return fmt.Errorf("watch timeout reached")
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
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "max watch duration (0 disables timeout)")

	return cmd
}

func newRunsReplayCommand(state *appState) *cobra.Command {
	var wait bool

	cmd := &cobra.Command{
		Use:   "replay <run-id>",
		Short: "Replay a run using its original payload",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			original, err := cli.GetRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			triggered, err := cli.TriggerJob(cmd.Context(), original.JobID, client.TriggerJobRequest{Payload: original.Payload}, "")
			if err != nil {
				return err
			}

			if err := printData(state, triggered); err != nil {
				return err
			}

			if !wait {
				return nil
			}

			watchCmd := newRunsWatchCommand(state)
			return watchCmd.RunE(watchCmd, []string{triggered.ID})
		},
	}

	cmd.Flags().BoolVar(&wait, "wait", false, "wait for replayed run to reach terminal state")

	return cmd
}
