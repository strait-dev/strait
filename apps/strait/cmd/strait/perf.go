package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newPerfCommand(state *appState) *cobra.Command {
	var projectID string
	var period string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "perf",
		Short: "Show project performance analytics",
		Long: `Displays project-wide performance analytics including throughput,
health summary, and the slowest jobs over a fixed analysis window.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}
			periodHours, err := parsePerfPeriodHours(period)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			analytics, err := cli.GetPerformanceAnalytics(cmd.Context(), projectID, periodHours)
			if err != nil {
				return err
			}

			if asJSON {
				return printData(state, analytics)
			}

			slowestJobs := make([]map[string]any, 0, len(analytics.SlowestJobs))
			for _, job := range analytics.SlowestJobs {
				slowestJobs = append(slowestJobs, map[string]any{
					"job_id":            job.JobID,
					"job_slug":          job.JobSlug,
					"avg_duration_secs": fmt.Sprintf("%.2f", job.AvgDurationSecs),
					"p95_duration_secs": fmt.Sprintf("%.2f", job.P95DurationSecs),
					"total_runs":        job.TotalRuns,
					"failed_runs":       job.FailedRuns,
					"failure_rate":      fmt.Sprintf("%.1f%%", failureRate(job.TotalRuns, job.FailedRuns)*100),
				})
			}

			return printData(state, map[string]any{
				"throughput": map[string]any{
					"completed":    analytics.Throughput.Completed,
					"failed":       analytics.Throughput.Failed,
					"timed_out":    analytics.Throughput.TimedOut,
					"canceled":     analytics.Throughput.Canceled,
					"period_hours": analytics.Throughput.PeriodHours,
				},
				"health_summary": map[string]any{
					"total_jobs":        analytics.HealthSummary.TotalJobs,
					"active_jobs":       analytics.HealthSummary.ActiveJobs,
					"success_rate":      fmt.Sprintf("%.1f%%", analytics.HealthSummary.SuccessRate*100),
					"avg_duration_secs": fmt.Sprintf("%.2f", analytics.HealthSummary.AvgDurationSecs),
					"queue_depth":       analytics.HealthSummary.QueueDepth,
				},
				"slowest_jobs": slowestJobs,
			})
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&period, "period", "7d", "analytics period (24h, 72h, 7d, 30d, 90d)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output raw JSON")

	return cmd
}

func parsePerfPeriodHours(raw string) (int, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "24h":
		return 24, nil
	case "72h":
		return 72, nil
	case "7d":
		return 24 * 7, nil
	case "30d":
		return 24 * 30, nil
	case "90d":
		return 24 * 90, nil
	default:
		return 0, fmt.Errorf("invalid --period %q: use one of 24h, 72h, 7d, 30d, 90d", raw)
	}
}

func failureRate(totalRuns, failedRuns int) float64 {
	if totalRuns == 0 {
		return 0
	}
	return float64(failedRuns) / float64(totalRuns)
}
