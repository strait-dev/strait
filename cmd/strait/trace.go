package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"strait/internal/cli/styles"

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
		Example: `  strait trace run_abc123
  strait trace run_abc123 --show-payload --show-result
  strait trace run_abc123 --event-limit 100`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			runID := args[0]

			run, err := cli.GetRun(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("failed to get run: %w", err)
			}

			events, err := cli.ListRunEvents(cmd.Context(), runID, "", "")
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
			fmt.Fprintf(&b, "Run: %s\n", run.ID)
			fmt.Fprintf(&b, "Job: %s\n", run.JobID)
			fmt.Fprintf(&b, "Status: %s\n", styles.Status(string(run.Status)))
			fmt.Fprintf(&b, "Attempt: %d  Triggered by: %s\n", run.Attempt, run.TriggeredBy)
			fmt.Fprintf(&b, "Created: %s\n", run.CreatedAt.UTC().Format(time.RFC3339))
			if run.StartedAt != nil {
				fmt.Fprintf(&b, "Started: %s\n", run.StartedAt.UTC().Format(time.RFC3339))
			}
			if run.FinishedAt != nil {
				fmt.Fprintf(&b, "Finished: %s\n", run.FinishedAt.UTC().Format(time.RFC3339))
			}

			// Duration
			if run.StartedAt != nil {
				end := time.Now()
				if run.FinishedAt != nil {
					end = *run.FinishedAt
				}
				dur := end.Sub(*run.StartedAt)
				fmt.Fprintf(&b, "Duration: %s\n", dur.Truncate(time.Millisecond))
			}

			if run.Error != "" {
				fmt.Fprintf(&b, "Error: %s\n", run.Error)
			}

			if showPayload && run.Payload != nil {
				fmt.Fprintf(&b, "Payload: %s\n", string(run.Payload))
			}
			if showResult && run.Result != nil {
				fmt.Fprintf(&b, "Result: %s\n", string(run.Result))
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

				fmt.Fprintf(&b, "  %s %s [%s/%s] %s\n",
					ts, marker, level, event.Type, event.Message)
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
