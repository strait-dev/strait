package main

import (
	"fmt"

	"strait/internal/cli/styles"

	"github.com/spf13/cobra"
)

func newEventsCommand(state *appState) *cobra.Command {
	var runID string
	var level string
	var eventType string

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Inspect run events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runID == "" {
				return fmt.Errorf("--run is required")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			events, err := cli.ListRunEvents(cmd.Context(), runID, level, eventType)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(events))
			for _, event := range events {
				lvl := event.Level
				if isTTYRich(state) {
					lvl = styles.LogLevel(event.Level)
				}
				rows = append(rows, map[string]any{
					"id":        event.ID,
					"run_id":    event.RunID,
					"timestamp": event.CreatedAt,
					"level":     lvl,
					"type":      event.Type,
					"message":   event.Message,
				})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&runID, "run", "", "run ID")
	cmd.Flags().StringVar(&level, "level", "", "event level filter")
	cmd.Flags().StringVar(&eventType, "type", "", "event type filter")

	return cmd
}
