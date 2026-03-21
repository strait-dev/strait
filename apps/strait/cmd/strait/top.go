package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"strait/internal/cli/styles"
	"strait/internal/domain"

	"github.com/spf13/cobra"
)

func newTopCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "top [jobs|queue]",
		Short:   "Show live resource usage style stats",
		Long:    "Displays queue runtime metrics and can refresh continuously in watch mode.",
		Example: "strait top\n  strait top --watch\n  strait top jobs --project proj_1\n  strait top queue --watch --interval 5s",
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
	var projectID string
	var limit int

	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Show queue depth snapshot",
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

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			ttyMode := stdoutIsTTY() && state.opts.outputFormat == ""
			return runTopLoop(cmd, watch, interval, func() error {
				sampledAt := time.Now().UTC().Format(time.RFC3339)
				rows := make([]map[string]any, 0)

				if projectID == "" {
					stats, err := cli.Stats(cmd.Context())
					if err != nil {
						return err
					}
					if ttyMode {
						fmt.Fprintln(os.Stderr, styles.SectionHeader("Queue", -1))
						fmt.Fprintln(os.Stderr, styles.KeyValue("Queued", fmt.Sprintf("%d", stats.Queued)))
						fmt.Fprintln(os.Stderr, styles.KeyValue("Executing", fmt.Sprintf("%d", stats.Executing)))
						fmt.Fprintln(os.Stderr, styles.KeyValue("Delayed", fmt.Sprintf("%d", stats.Delayed)))
						return nil
					}
					rows = append(rows,
						map[string]any{"metric": "queued", "value": stats.Queued, "scope": "global", "sampled_at": sampledAt},
						map[string]any{"metric": "executing", "value": stats.Executing, "scope": "global", "sampled_at": sampledAt},
						map[string]any{"metric": "delayed", "value": stats.Delayed, "scope": "global", "sampled_at": sampledAt},
					)
				} else {
					runs, err := cli.ListRuns(cmd.Context(), projectID, "", limit, nil)
					if err != nil {
						return err
					}

					counts := map[string]int{}
					for _, run := range runs {
						counts[string(run.Status)]++
					}

					rows = append(rows,
						map[string]any{"metric": "queued", "value": counts["queued"], "scope": projectID, "sampled_at": sampledAt},
						map[string]any{"metric": "executing", "value": counts["executing"], "scope": projectID, "sampled_at": sampledAt},
						map[string]any{"metric": "delayed", "value": counts["delayed"], "scope": projectID, "sampled_at": sampledAt},
						map[string]any{"metric": "waiting", "value": counts["waiting"], "scope": projectID, "sampled_at": sampledAt},
						map[string]any{"metric": "failed", "value": counts["failed"] + counts["timed_out"] + counts["crashed"] + counts["system_failed"], "scope": projectID, "sampled_at": sampledAt},
					)
				}
				return printData(state, rows)
			})
		},
	}

	cmd.Flags().BoolVar(&watch, "watch", false, "refresh continuously")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "refresh interval in watch mode")
	cmd.Flags().StringVar(&projectID, "project", "", "project ID for project-scoped queue breakdown")
	cmd.Flags().IntVar(&limit, "limit", 500, "max runs sampled for project-scoped breakdown")

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
			var err error
			projectID, err = requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			return runTopLoop(cmd, watch, interval, func() error {
				jobs, err := cli.ListJobs(cmd.Context(), projectID)
				if err != nil {
					return err
				}

				runs, err := cli.ListRuns(cmd.Context(), projectID, "", 500, nil)
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
	ctx := cmd.Context()
	for {
		if err := render(); err != nil {
			return err
		}

		if !watch {
			return nil
		}

		fmt.Fprintln(cmd.ErrOrStderr(), "--- press Ctrl+C to stop ---")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
