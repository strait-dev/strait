package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"strait/internal/cli/client"
	"strait/internal/cli/styles"
	"strait/internal/domain"

	"github.com/google/go-cmp/cmp"
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
	cmd.AddCommand(newRunsLastCommand(state))
	cmd.AddCommand(newRunsDiffCommand(state))

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
			var err error
			projectID, err = requireProjectID(state, projectID)
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

			if !follow {
				rows, err := listRunEventRows(ctx, cli, args[0], level, eventType, "", time.Time{})
				if err != nil {
					return err
				}
				return printLogRows(state, rows, false, "", 0)
			}

			if err := ensureRunStreamable(ctx, cli, args[0]); err != nil {
				return err
			}

			rows, err := listRunEventRows(ctx, cli, args[0], level, eventType, "", time.Time{})
			if err != nil {
				return err
			}
			if err := printLogRows(state, rows, false, "", 0); err != nil {
				return err
			}

			return streamRunLogs(ctx, cli, state, args[0], level, eventType, "", time.Time{}, "")
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream logs over SSE")
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

// watchRunUntilDone polls a run until it reaches a terminal state. It is used by
// trigger --wait and replay --wait to avoid synthesizing a cobra command context.
func watchRunUntilDone(ctx context.Context, state *appState, runID string, interval, timeout time.Duration) error {
	cli, err := newAPIClient(state)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	for {
		run, err := cli.GetRun(ctx, runID)
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

			return watchRunUntilDone(cmd.Context(), state, triggered.ID, 2*time.Second, 5*time.Minute)
		},
	}

	cmd.Flags().BoolVar(&wait, "wait", false, "wait for replayed run to reach terminal state")

	return cmd
}

func newRunsLastCommand(state *appState) *cobra.Command {
	var projectID string
	var openInBrowser bool

	cmd := &cobra.Command{
		Use:   "last",
		Short: "Show the most recent run",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var err error
			projectID, err = requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			runs, err := cli.ListRuns(cmd.Context(), projectID, "", 1, nil)
			if err != nil {
				return err
			}
			if len(runs) == 0 {
				return fmt.Errorf("no runs found")
			}

			run := runs[0]
			if err := printData(state, map[string]any{
				"id":           run.ID,
				"job_id":       run.JobID,
				"status":       styles.Status(string(run.Status)),
				"attempt":      run.Attempt,
				"triggered_by": run.TriggeredBy,
				"created_at":   run.CreatedAt,
			}); err != nil {
				return err
			}

			if openInBrowser {
				return openBrowser(buildDashboardURL(state, "/runs/"+run.ID))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().BoolVar(&openInBrowser, "open", false, "open the run in the browser")

	return cmd
}

func buildDashboardURL(state *appState, urlPath string) string {
	base := strings.TrimRight(state.opts.serverURL, "/")
	dashURL := strings.Replace(base, ":8080", ":5173", 1)
	dashURL = strings.Replace(dashURL, "api.", "app.", 1)
	return dashURL + urlPath
}

func newRunsDiffCommand(state *appState) *cobra.Command {
	var showPayload bool
	var showEvents bool
	var eventLimit int

	cmd := &cobra.Command{
		Use:   "diff <run1> <run2>",
		Short: "Compare two runs side by side",
		Long:  "Fetches both runs and their events, then compares status, duration, attempts, and optionally payloads and events.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			run1, err := cli.GetRun(ctx, args[0])
			if err != nil {
				return fmt.Errorf("fetching run %s: %w", args[0], err)
			}
			run2, err := cli.GetRun(ctx, args[1])
			if err != nil {
				return fmt.Errorf("fetching run %s: %w", args[1], err)
			}

			result := map[string]any{
				"run1_id": run1.ID,
				"run2_id": run2.ID,
			}

			// Status comparison
			result["status"] = map[string]any{
				"run1": string(run1.Status),
				"run2": string(run2.Status),
				"same": run1.Status == run2.Status,
			}

			// Duration comparison
			d1 := runDuration(run1)
			d2 := runDuration(run2)
			result["duration"] = map[string]any{
				"run1": d1.String(),
				"run2": d2.String(),
				"diff": (d2 - d1).String(),
			}

			// Attempt comparison
			result["attempts"] = map[string]any{
				"run1": run1.Attempt,
				"run2": run2.Attempt,
				"same": run1.Attempt == run2.Attempt,
			}

			// Created at comparison
			result["created_at"] = map[string]any{
				"run1": run1.CreatedAt,
				"run2": run2.CreatedAt,
				"diff": run2.CreatedAt.Sub(run1.CreatedAt).String(),
			}

			// Payload diff (optional)
			if showPayload {
				var p1, p2 any
				if len(run1.Payload) > 0 {
					if err := json.Unmarshal(run1.Payload, &p1); err != nil {
						p1 = string(run1.Payload) // Fall back to raw string.
					}
				}
				if len(run2.Payload) > 0 {
					if err := json.Unmarshal(run2.Payload, &p2); err != nil {
						p2 = string(run2.Payload) // Fall back to raw string.
					}
				}
				diff := cmp.Diff(p1, p2)
				if diff == "" {
					diff = "(identical)"
				}
				result["payload_diff"] = diff
			}

			// Events comparison (optional)
			if showEvents {
				events1, evErr1 := cli.ListRunEvents(ctx, run1.ID, "", "")
				if evErr1 != nil {
					return fmt.Errorf("fetching events for %s: %w", run1.ID, evErr1)
				}
				events2, evErr2 := cli.ListRunEvents(ctx, run2.ID, "", "")
				if evErr2 != nil {
					return fmt.Errorf("fetching events for %s: %w", run2.ID, evErr2)
				}

				sort.Slice(events1, func(i, j int) bool {
					return events1[i].CreatedAt.Before(events1[j].CreatedAt)
				})
				sort.Slice(events2, func(i, j int) bool {
					return events2[i].CreatedAt.Before(events2[j].CreatedAt)
				})

				if eventLimit > 0 && len(events1) > eventLimit {
					events1 = events1[len(events1)-eventLimit:]
				}
				if eventLimit > 0 && len(events2) > eventLimit {
					events2 = events2[len(events2)-eventLimit:]
				}

				e1Summary := make([]map[string]any, 0, len(events1))
				for _, e := range events1 {
					e1Summary = append(e1Summary, map[string]any{
						"type":    e.Type,
						"level":   e.Level,
						"message": e.Message,
					})
				}
				e2Summary := make([]map[string]any, 0, len(events2))
				for _, e := range events2 {
					e2Summary = append(e2Summary, map[string]any{
						"type":    e.Type,
						"level":   e.Level,
						"message": e.Message,
					})
				}

				eventDiff := cmp.Diff(e1Summary, e2Summary)
				if eventDiff == "" {
					eventDiff = "(identical)"
				}
				result["events"] = map[string]any{
					"run1_count": len(events1),
					"run2_count": len(events2),
					"diff":       eventDiff,
				}
			}

			return printData(state, result)
		},
	}

	cmd.Flags().BoolVar(&showPayload, "show-payload", false, "include payload diff")
	cmd.Flags().BoolVar(&showEvents, "show-events", false, "include events diff")
	cmd.Flags().IntVar(&eventLimit, "event-limit", 50, "max events per run to compare")

	return cmd
}

func runDuration(run *domain.JobRun) time.Duration {
	if run.StartedAt == nil {
		return 0
	}
	end := time.Now()
	if run.FinishedAt != nil {
		end = *run.FinishedAt
	}
	return end.Sub(*run.StartedAt)
}
