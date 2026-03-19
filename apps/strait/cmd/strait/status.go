package main

import (
	"github.com/spf13/cobra"
)

func newStatusCommand(state *appState) *cobra.Command {
	var projectID string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show project status overview",
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

			stats, err := cli.Stats(cmd.Context())
			if err != nil {
				return err
			}

			recentFailed, failedErr := cli.ListRuns(cmd.Context(), projectID, "failed", 5, nil)
			if failedErr != nil {
				recentFailed = nil // Degrade gracefully: show queue stats without failures.
			}

			failedRows := make([]map[string]any, 0, len(recentFailed))
			for _, r := range recentFailed {
				failedRows = append(failedRows, map[string]any{
					"id":         r.ID,
					"job_id":     r.JobID,
					"status":     r.Status,
					"created_at": r.CreatedAt,
				})
			}

			return printData(state, map[string]any{
				"queue": map[string]any{
					"queued":    stats.Queued,
					"executing": stats.Executing,
					"delayed":   stats.Delayed,
				},
				"recent_failures": failedRows,
			})
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")

	return cmd
}
