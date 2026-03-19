package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPerfCommand(state *appState) *cobra.Command {
	var projectID string
	var period string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "perf [job-slug]",
		Short: "Show performance analytics for jobs",
		Long: `Displays performance metrics including latency percentiles,
success rates, and throughput trends for jobs in a project.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			analytics, err := cli.GetPerformanceAnalytics(cmd.Context(), projectID, period)
			if err != nil {
				return err
			}

			// Filter to specific job if slug provided
			if len(args) > 0 {
				slug := args[0]
				filtered := analytics[:0]
				for _, a := range analytics {
					if a.JobSlug == slug {
						filtered = append(filtered, a)
					}
				}
				analytics = filtered
				if len(analytics) == 0 {
					return fmt.Errorf("no analytics found for job %q", slug)
				}
			}

			if asJSON {
				return printData(state, analytics)
			}

			rows := make([]map[string]any, 0, len(analytics))
			for _, a := range analytics {
				rows = append(rows, map[string]any{
					"job":          a.JobSlug,
					"total_runs":   a.TotalRuns,
					"success_rate": fmt.Sprintf("%.1f%%", a.SuccessRate*100),
					"avg_ms":       fmt.Sprintf("%.0f", a.AvgDuration),
					"p50_ms":       fmt.Sprintf("%.0f", a.P50Duration),
					"p95_ms":       fmt.Sprintf("%.0f", a.P95Duration),
					"p99_ms":       fmt.Sprintf("%.0f", a.P99Duration),
				})
			}

			if len(rows) == 0 {
				return printData(state, map[string]any{"message": "no analytics data available"})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&period, "period", "7d", "analytics period (7d, 30d, 90d)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output raw JSON")

	return cmd
}
