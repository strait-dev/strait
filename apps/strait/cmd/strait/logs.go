package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newLogsCommand(state *appState) *cobra.Command {
	var runID string
	var projectID string
	var level string
	var eventType string
	var follow bool
	var interval time.Duration
	var jobGlob string
	var since string
	var search string
	var tail int
	var group bool
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View run logs/events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			// Parse --since duration
			var sinceTime time.Time
			if since != "" {
				d, parseErr := time.ParseDuration(since)
				if parseErr != nil {
					return fmt.Errorf("invalid --since duration %q: %w", since, parseErr)
				}
				sinceTime = time.Now().Add(-d)
			}

			// Resolve --job glob to matching job IDs
			var matchedJobIDs map[string]string // jobID -> slug
			if jobGlob != "" {
				projectID, err = requireProjectID(state, projectID)
				if err != nil {
					return err
				}
				jobs, listErr := cli.ListJobs(ctx, projectID)
				if listErr != nil {
					return fmt.Errorf("listing jobs for --job filter: %w", listErr)
				}
				matchedJobIDs = make(map[string]string)
				for _, job := range jobs {
					matched, matchErr := filepath.Match(jobGlob, job.Slug)
					if matchErr != nil {
						return fmt.Errorf("invalid --job glob pattern %q: %w", jobGlob, matchErr)
					}
					if matched {
						matchedJobIDs[job.ID] = job.Slug
					}
				}
				if len(matchedJobIDs) == 0 {
					return fmt.Errorf("no jobs matched glob pattern %q", jobGlob)
				}
			}

			seen := map[string]struct{}{}
			for {
				runsToRead := []string{}
				runJobMap := map[string]string{} // runID -> jobSlug for grouping
				if runID != "" {
					runsToRead = append(runsToRead, runID)
				} else {
					projectID, err = requireProjectID(state, projectID)
					if err != nil {
						return err
					}
					runs, listErr := cli.ListRuns(ctx, projectID, "", 20, nil)
					if listErr != nil {
						return listErr
					}
					for _, run := range runs {
						// Filter by matched job IDs if --job is set
						if matchedJobIDs != nil {
							if _, ok := matchedJobIDs[run.JobID]; !ok {
								continue
							}
							runJobMap[run.ID] = matchedJobIDs[run.JobID]
						}
						runsToRead = append(runsToRead, run.ID)
					}
				}

				rows := make([]map[string]any, 0)
				for _, rid := range runsToRead {
					events, eventsErr := cli.ListRunEvents(ctx, rid, level, eventType)
					if eventsErr != nil {
						continue
					}
					for _, event := range events {
						if _, ok := seen[event.ID]; ok {
							continue
						}
						seen[event.ID] = struct{}{}

						// Filter by --since
						if !sinceTime.IsZero() && event.CreatedAt.Before(sinceTime) {
							continue
						}

						// Filter by --search (case-insensitive)
						if search != "" && !strings.Contains(strings.ToLower(event.Message), strings.ToLower(search)) {
							continue
						}

						row := map[string]any{
							"run_id":    rid,
							"timestamp": event.CreatedAt,
							"level":     event.Level,
							"type":      event.Type,
							"message":   event.Message,
						}
						if slug, ok := runJobMap[rid]; ok {
							row["job_slug"] = slug
						}
						rows = append(rows, row)
					}
				}

				sort.Slice(rows, func(i, j int) bool {
					ti, _ := rows[i]["timestamp"].(time.Time)
					tj, _ := rows[j]["timestamp"].(time.Time)
					return ti.Before(tj)
				})

				// Apply --tail
				if tail > 0 && len(rows) > tail {
					rows = rows[len(rows)-tail:]
				}

				if len(rows) > 0 {
					if group {
						if err := printGroupedLogs(state, rows); err != nil {
							return err
						}
					} else if outputFmt == "ndjson" {
						enc := json.NewEncoder(os.Stdout)
						for _, row := range rows {
							if err := enc.Encode(row); err != nil {
								return err
							}
						}
					} else {
						if err := printData(state, rows); err != nil {
							return err
						}
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

	cmd.Flags().StringVar(&runID, "run", "", "run ID to scope logs")
	cmd.Flags().StringVar(&projectID, "project", "", "project ID for aggregate logs")
	cmd.Flags().StringVar(&level, "level", "", "event level filter")
	cmd.Flags().StringVar(&eventType, "type", "", "event type filter")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log stream")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "poll interval in follow mode")
	cmd.Flags().StringVar(&jobGlob, "job", "", "filter by job slug glob pattern")
	cmd.Flags().StringVar(&since, "since", "", "show events after this duration ago (e.g. 5m, 1h)")
	cmd.Flags().StringVar(&search, "search", "", "grep-like case-insensitive message filter")
	cmd.Flags().IntVar(&tail, "tail", 0, "show only last N events")
	cmd.Flags().BoolVar(&group, "group", false, "group events by job slug with summary")
	cmd.Flags().StringVar(&outputFmt, "output", "", "output format (ndjson)")

	return cmd
}

// printGroupedLogs groups log rows by job_slug and prints a summary per group.
func printGroupedLogs(state *appState, rows []map[string]any) error {
	groups := make(map[string][]map[string]any)
	order := make([]string, 0)

	for _, row := range rows {
		slug, _ := row["job_slug"].(string)
		if slug == "" {
			slug = "(unknown)"
		}
		if _, exists := groups[slug]; !exists {
			order = append(order, slug)
		}
		groups[slug] = append(groups[slug], row)
	}

	summary := make([]map[string]any, 0, len(groups))
	for _, slug := range order {
		events := groups[slug]
		levelCounts := make(map[string]int)
		for _, e := range events {
			l, _ := e["level"].(string)
			if l == "" {
				l = "unknown"
			}
			levelCounts[l]++
		}
		summary = append(summary, map[string]any{
			"job_slug":     slug,
			"total_events": len(events),
			"levels":       levelCounts,
		})
	}

	return printData(state, summary)
}
