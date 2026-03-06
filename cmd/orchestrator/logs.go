package main

import (
	"sort"
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

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View run logs/events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			seen := map[string]struct{}{}
			for {
				runsToRead := []string{}
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
						rows = append(rows, map[string]any{
							"run_id":    rid,
							"timestamp": event.CreatedAt,
							"level":     event.Level,
							"type":      event.Type,
							"message":   event.Message,
						})
					}
				}

				sort.Slice(rows, func(i, j int) bool {
					ti, _ := rows[i]["timestamp"].(time.Time)
					tj, _ := rows[j]["timestamp"].(time.Time)
					return ti.Before(tj)
				})

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

	cmd.Flags().StringVar(&runID, "run", "", "run ID to scope logs")
	cmd.Flags().StringVar(&projectID, "project", "", "project ID for aggregate logs")
	cmd.Flags().StringVar(&level, "level", "", "event level filter")
	cmd.Flags().StringVar(&eventType, "type", "", "event type filter")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log stream")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "poll interval in follow mode")

	return cmd
}
