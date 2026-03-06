package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/cli/styles"

	"github.com/spf13/cobra"
)

func newTraceCommand(state *appState) *cobra.Command {
	var showPayload bool
	var showResult bool
	var eventLimit int

	cmd := &cobra.Command{
		Use:   "trace <run-id>",
		Short: "Show ASCII timeline for a run",
		Long: `Renders an ASCII timeline visualization of a run's lifecycle events.

Shows state transitions, log messages, and timing information in a
visual timeline format.`,
		Example: `  orchestrator trace run_abc123
  orchestrator trace run_abc123 --show-payload --show-result
  orchestrator trace run_abc123 --event-limit 100`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			runID := args[0]

			run, err := cli.GetRun(context.Background(), runID)
			if err != nil {
				return fmt.Errorf("failed to get run: %w", err)
			}

			events, err := cli.ListRunEvents(context.Background(), runID, "", "")
			if err != nil {
				return fmt.Errorf("failed to get events: %w", err)
			}

			sort.Slice(events, func(i, j int) bool {
				return events[i].CreatedAt.Before(events[j].CreatedAt)
			})
			if len(events) > eventLimit {
				events = events[:eventLimit]
			}

			// Header
			var b strings.Builder
			b.WriteString(fmt.Sprintf("Run: %s\n", run.ID))
			b.WriteString(fmt.Sprintf("Job: %s\n", run.JobID))
			b.WriteString(fmt.Sprintf("Status: %s\n", styles.Status(string(run.Status))))
			b.WriteString(fmt.Sprintf("Attempt: %d  Triggered by: %s\n", run.Attempt, run.TriggeredBy))
			b.WriteString(fmt.Sprintf("Created: %s\n", run.CreatedAt.UTC().Format(time.RFC3339)))
			if run.StartedAt != nil {
				b.WriteString(fmt.Sprintf("Started: %s\n", run.StartedAt.UTC().Format(time.RFC3339)))
			}
			if run.FinishedAt != nil {
				b.WriteString(fmt.Sprintf("Finished: %s\n", run.FinishedAt.UTC().Format(time.RFC3339)))
			}

			// Duration
			if run.StartedAt != nil {
				end := time.Now()
				if run.FinishedAt != nil {
					end = *run.FinishedAt
				}
				dur := end.Sub(*run.StartedAt)
				b.WriteString(fmt.Sprintf("Duration: %s\n", dur.Truncate(time.Millisecond)))
			}

			if run.Error != "" {
				b.WriteString(fmt.Sprintf("Error: %s\n", run.Error))
			}

			if showPayload && run.Payload != nil {
				b.WriteString(fmt.Sprintf("Payload: %s\n", string(run.Payload)))
			}
			if showResult && run.Result != nil {
				b.WriteString(fmt.Sprintf("Result: %s\n", string(run.Result)))
			}

			b.WriteString("\n")

			// ASCII timeline
			b.WriteString("Timeline\n")
			b.WriteString("────────\n")

			var anchor time.Time
			if len(events) > 0 {
				anchor = events[0].CreatedAt
			} else {
				anchor = run.CreatedAt
			}

			for _, event := range events {
				offset := event.CreatedAt.Sub(anchor)
				ts := formatOffset(offset)
				marker := timelineMarker(string(event.Type))
				level := event.Level
				if level == "" {
					level = "info"
				}

				b.WriteString(fmt.Sprintf("  %s %s [%s/%s] %s\n",
					ts, marker, level, event.Type, event.Message))
			}

			if len(events) == 0 {
				b.WriteString("  (no events recorded)\n")
			}

			fmt.Print(b.String())
			return nil
		},
	}

	cmd.Flags().BoolVar(&showPayload, "show-payload", false, "include run payload")
	cmd.Flags().BoolVar(&showResult, "show-result", false, "include run result")
	cmd.Flags().IntVar(&eventLimit, "event-limit", 50, "max events to display")

	return cmd
}

func formatOffset(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("+%4dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("+%5.1fs", d.Seconds())
	}
	return fmt.Sprintf("+%5.1fm", d.Minutes())
}

func timelineMarker(eventType string) string {
	switch eventType {
	case "state_change":
		return "◆"
	case "error":
		return "✗"
	case "progress":
		return "▶"
	default:
		return "●"
	}
}
