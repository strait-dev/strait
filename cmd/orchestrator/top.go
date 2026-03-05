package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"orchestrator/internal/domain"

	"github.com/spf13/cobra"
)

func newTopCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "top [jobs|queue]",
		Short:   "Show live resource usage style stats",
		Long:    "Displays queue runtime metrics and can refresh continuously in watch mode.",
		Example: "orchestrator top\n  orchestrator top --watch\n  orchestrator top jobs --project proj_1\n  orchestrator top queue --watch --interval 5s",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return newTopQueueCommand(state).RunE(cmd, nil)
		},
	}

	cmd.AddCommand(newTopJobsCommand(state))
	cmd.AddCommand(newTopQueueCommand(state))

	return cmd
}

func newTopQueueCommand(state *appState) *cobra.Command {
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Show queue depth snapshot",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if interval <= 0 {
				return fmt.Errorf("interval must be greater than zero")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			return runTopLoop(cmd, watch, interval, func() error {
				stats, err := cli.Stats(context.Background())
				if err != nil {
					return err
				}

				sampledAt := time.Now().UTC().Format(time.RFC3339)
				rows := []map[string]any{
					{"metric": "queued", "value": stats.Queued, "sampled_at": sampledAt},
					{"metric": "executing", "value": stats.Executing, "sampled_at": sampledAt},
					{"metric": "delayed", "value": stats.Delayed, "sampled_at": sampledAt},
				}
				return printData(state, rows)
			})
		},
	}

	cmd.Flags().BoolVar(&watch, "watch", false, "refresh continuously")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "refresh interval in watch mode")

	return cmd
}

func newTopJobsCommand(state *appState) *cobra.Command {
	var watch bool
	var interval time.Duration
	var projectID string
	var limit int

	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "Show job activity leaderboard",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if interval <= 0 {
				return fmt.Errorf("interval must be greater than zero")
			}
			if limit <= 0 {
				return fmt.Errorf("limit must be greater than zero")
			}
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

			return runTopLoop(cmd, watch, interval, func() error {
				jobs, err := cli.ListJobs(context.Background(), projectID)
				if err != nil {
					return err
				}

				runs, err := cli.ListRuns(context.Background(), projectID, "", 500)
				if err != nil {
					return err
				}

				totals := map[string]map[string]int{}
				for _, run := range runs {
					_, ok := totals[run.JobID]
					if !ok {
						totals[run.JobID] = map[string]int{
							"active": 0,
							"failed": 0,
							"total":  0,
						}
					}
					totals[run.JobID]["total"]++
					if run.Status == domain.StatusFailed || run.Status == domain.StatusTimedOut || run.Status == domain.StatusCrashed || run.Status == domain.StatusSystemFailed {
						totals[run.JobID]["failed"]++
					}
					if run.Status == domain.StatusQueued || run.Status == domain.StatusDelayed || run.Status == domain.StatusDequeued || run.Status == domain.StatusExecuting || run.Status == domain.StatusWaiting {
						totals[run.JobID]["active"]++
					}
				}

				type row struct {
					JobID   string
					Name    string
					Slug    string
					Active  int
					Failed  int
					Total   int
					Enabled bool
				}

				rows := make([]row, 0, len(jobs))
				for _, job := range jobs {
					agg := totals[job.ID]
					rows = append(rows, row{
						JobID:   job.ID,
						Name:    job.Name,
						Slug:    job.Slug,
						Active:  agg["active"],
						Failed:  agg["failed"],
						Total:   agg["total"],
						Enabled: job.Enabled,
					})
				}

				sort.Slice(rows, func(i, j int) bool {
					if rows[i].Active == rows[j].Active {
						if rows[i].Failed == rows[j].Failed {
							return rows[i].Name < rows[j].Name
						}
						return rows[i].Failed > rows[j].Failed
					}
					return rows[i].Active > rows[j].Active
				})

				if len(rows) > limit {
					rows = rows[:limit]
				}

				sampledAt := time.Now().UTC().Format(time.RFC3339)
				out := make([]map[string]any, 0, len(rows))
				for _, r := range rows {
					out = append(out, map[string]any{
						"job_id":       r.JobID,
						"name":         r.Name,
						"slug":         r.Slug,
						"active_runs":  r.Active,
						"failed_runs":  r.Failed,
						"sampled_runs": r.Total,
						"enabled":      r.Enabled,
						"sampled_at":   sampledAt,
					})
				}

				return printData(state, out)
			})
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().IntVar(&limit, "limit", 10, "max jobs to show")
	cmd.Flags().BoolVar(&watch, "watch", false, "refresh continuously")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "refresh interval in watch mode")

	return cmd
}

func runTopLoop(cmd *cobra.Command, watch bool, interval time.Duration, render func() error) error {
	for {
		if err := render(); err != nil {
			return err
		}

		if !watch {
			return nil
		}

		fmt.Fprintln(cmd.ErrOrStderr(), "--- press Ctrl+C to stop ---")
		time.Sleep(interval)
	}
}
