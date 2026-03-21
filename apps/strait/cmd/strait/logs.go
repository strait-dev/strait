package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"strait/internal/cli/client"
	"strait/internal/domain"

	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"
)

func newLogsCommand(state *appState) *cobra.Command {
	var runID string
	var projectID string
	var level string
	var eventType string
	var follow bool
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
			// Auto-enable NDJSON in non-TTY environments for pipeline-friendly output.
			if outputFmt == "" && !stdoutIsTTY() {
				outputFmt = "ndjson"
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			var sinceTime time.Time
			if since != "" {
				d, parseErr := time.ParseDuration(since)
				if parseErr != nil {
					return fmt.Errorf("invalid --since duration %q: %w", since, parseErr)
				}
				sinceTime = time.Now().Add(-d)
			}

			if follow {
				if group {
					return fmt.Errorf("--group is not supported with --follow")
				}
				if runID == "" {
					return fmt.Errorf("--follow requires --run in this release")
				}
				if err := ensureRunStreamable(ctx, cli, runID); err != nil {
					return err
				}

				rows, err := listRunEventRows(ctx, cli, runID, level, eventType, search, sinceTime)
				if err != nil {
					return err
				}
				if err := printLogRows(state, rows, false, outputFmt, tail); err != nil {
					return err
				}
				return streamRunLogs(ctx, cli, state, runID, level, eventType, search, sinceTime, outputFmt)
			}

			var matchedJobIDs map[string]string
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

			rows := make([]map[string]any, 0)
			if runID != "" {
				rows, err = listRunEventRows(ctx, cli, runID, level, eventType, search, sinceTime)
				if err != nil {
					return err
				}
			} else {
				projectID, err = requireProjectID(state, projectID)
				if err != nil {
					return err
				}
				runs, listErr := cli.ListRuns(ctx, projectID, "", 20, nil)
				if listErr != nil {
					return listErr
				}
				filteredRuns := runs
				if matchedJobIDs != nil {
					filteredRuns = make([]domain.JobRun, 0, len(runs))
					for _, run := range runs {
						if _, ok := matchedJobIDs[run.JobID]; ok {
							filteredRuns = append(filteredRuns, run)
						}
					}
				}

				type runEvents struct {
					runID  string
					jobID  string
					events []domain.RunEvent
				}
				var mu sync.Mutex
				var allRunEvents []runEvents

				p := pool.New().WithMaxGoroutines(5).WithContext(ctx)
				for _, run := range filteredRuns {
					p.Go(func(ctx context.Context) error {
						events, err := cli.ListRunEvents(ctx, run.ID, level, eventType)
						if err != nil {
							fmt.Fprintf(os.Stderr, "warning: failed to fetch events for run %s: %v\n", run.ID, err)
							return nil
						}
						mu.Lock()
						allRunEvents = append(allRunEvents, runEvents{runID: run.ID, jobID: run.JobID, events: events})
						mu.Unlock()
						return nil
					})
				}
				if err := p.Wait(); err != nil {
					return err
				}

				for _, re := range allRunEvents {
					for _, event := range re.events {
						row := runEventRow(re.runID, event)
						if matchedJobIDs != nil {
							row["job_slug"] = matchedJobIDs[re.jobID]
						}
						if !matchesLogRow(row, level, eventType, search, sinceTime) {
							continue
						}
						rows = append(rows, row)
					}
				}
			}

			return printLogRows(state, rows, group, outputFmt, tail)
		},
	}

	cmd.Flags().StringVar(&runID, "run", "", "run ID to scope logs")
	cmd.Flags().StringVar(&projectID, "project", "", "project ID for aggregate logs")
	cmd.Flags().StringVar(&level, "level", "", "event level filter")
	cmd.Flags().StringVar(&eventType, "type", "", "event type filter")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow a specific run log stream over SSE")
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

func listRunEventRows(ctx context.Context, cli *client.Client, runID, level, eventType, search string, sinceTime time.Time) ([]map[string]any, error) {
	events, err := cli.ListRunEvents(ctx, runID, level, eventType)
	if err != nil {
		return nil, err
	}

	rows := make([]map[string]any, 0, len(events))
	for _, event := range events {
		row := runEventRow(runID, event)
		if !matchesLogRow(row, level, eventType, search, sinceTime) {
			continue
		}
		rows = append(rows, row)
	}
	sortLogRows(rows)
	return rows, nil
}

func runEventRow(runID string, event domain.RunEvent) map[string]any {
	return map[string]any{
		"run_id":    runID,
		"timestamp": event.CreatedAt,
		"level":     event.Level,
		"type":      string(event.Type),
		"message":   event.Message,
	}
}

func printLogRows(state *appState, rows []map[string]any, group bool, outputFmt string, tail int) error {
	sortLogRows(rows)
	if tail > 0 && len(rows) > tail {
		rows = rows[len(rows)-tail:]
	}
	if len(rows) == 0 {
		return nil
	}
	if group {
		return printGroupedLogs(state, rows)
	}
	if outputFmt == "ndjson" {
		enc := json.NewEncoder(os.Stdout)
		for _, row := range rows {
			if err := enc.Encode(row); err != nil {
				return err
			}
		}
		return nil
	}
	return printData(state, rows)
}

func sortLogRows(rows []map[string]any) {
	sort.Slice(rows, func(i, j int) bool {
		return logRowTimestamp(rows[i]).Before(logRowTimestamp(rows[j]))
	})
}

func logRowTimestamp(row map[string]any) time.Time {
	timestamp, _ := row["timestamp"].(time.Time)
	return timestamp
}

func matchesLogRow(row map[string]any, level, eventType, search string, sinceTime time.Time) bool {
	if !sinceTime.IsZero() && logRowTimestamp(row).Before(sinceTime) {
		return false
	}
	if level != "" {
		rowLevel, _ := row["level"].(string)
		if !strings.EqualFold(rowLevel, level) {
			return false
		}
	}
	if eventType != "" {
		rowType, _ := row["type"].(string)
		if rowType != eventType {
			return false
		}
	}
	if search != "" {
		message, _ := row["message"].(string)
		if !strings.Contains(strings.ToLower(message), strings.ToLower(search)) {
			return false
		}
	}
	return true
}

func ensureRunStreamable(ctx context.Context, cli *client.Client, runID string) error {
	run, err := cli.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status.IsTerminal() {
		return fmt.Errorf("run %s is already in a terminal state", runID)
	}
	return nil
}

func streamRunLogs(ctx context.Context, cli *client.Client, state *appState, runID, level, eventType, search string, sinceTime time.Time, outputFmt string) error {
	return cli.StreamRunEvents(ctx, runID, func(msg client.RunStreamMessage) error {
		row, ok := runStreamRow(runID, msg)
		if !ok {
			return nil
		}
		if !matchesLogRow(row, level, eventType, search, sinceTime) {
			return nil
		}
		return renderFollowLogRow(state, outputFmt, row)
	})
}

func runStreamRow(runID string, msg client.RunStreamMessage) (map[string]any, bool) {
	switch msg.Type {
	case "event":
		return map[string]any{
			"run_id":    runID,
			"timestamp": msg.Timestamp,
			"level":     msg.Level,
			"type":      msg.EventType,
			"message":   msg.Message,
		}, true
	case "status_change":
		message := fmt.Sprintf("status changed from %s to %s", msg.From, msg.To)
		if msg.Error != "" {
			message = fmt.Sprintf("%s: %s", message, msg.Error)
		}
		return map[string]any{
			"run_id":    runID,
			"timestamp": msg.Timestamp,
			"level":     "info",
			"type":      msg.Type,
			"message":   message,
		}, true
	default:
		if msg.Type == "" && msg.Message == "" {
			return nil, false
		}
		return map[string]any{
			"run_id":    runID,
			"timestamp": msg.Timestamp,
			"level":     msg.Level,
			"type":      msg.Type,
			"message":   msg.Message,
		}, true
	}
}

func renderFollowLogRow(state *appState, outputFmt string, row map[string]any) error {
	if outputFmt == "ndjson" || state.opts.outputFormat == "json" {
		return json.NewEncoder(os.Stdout).Encode(row)
	}

	timestamp := logRowTimestamp(row).Format(time.RFC3339)
	level, _ := row["level"].(string)
	eventType, _ := row["type"].(string)
	message, _ := row["message"].(string)
	_, err := fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\n", timestamp, level, eventType, message)
	return err
}
