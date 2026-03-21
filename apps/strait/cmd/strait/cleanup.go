package main

import (
	"fmt"
	"os"
	"time"

	"strait/internal/cli/styles"

	"github.com/spf13/cobra"
)

func newCleanupCommand(state *appState) *cobra.Command {
	var projectID string
	var olderThan time.Duration
	var status string
	var dryRun bool
	var yes bool
	var limit int

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up old runs",
		Long: `Remove completed, failed, or expired runs older than a specified duration.

By default only targets terminal-state runs (completed, failed, timed_out,
crashed, system_failed, canceled, expired). Use --status to target a specific
status. Use --dry-run to preview what would be removed.`,
		Example: `  strait cleanup --runs-older-than 720h --dry-run
  strait cleanup --runs-older-than 720h --yes
  strait cleanup --runs-older-than 168h --status failed --yes`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var err error
			projectID, err = requireProjectID(state, projectID)
			if err != nil {
				return err
			}
			if olderThan <= 0 {
				return fmt.Errorf("--runs-older-than is required and must be positive")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			cutoff := time.Now().Add(-olderThan)
			targetStatuses := []string{"completed", "failed", "timed_out", "crashed", "system_failed", "canceled", "expired"}
			if status != "" {
				targetStatuses = []string{status}
			}

			var candidates []string
			for _, s := range targetStatuses {
				runs, listErr := cli.ListRuns(cmd.Context(), projectID, s, limit, nil)
				if listErr != nil {
					return fmt.Errorf("listing runs with status %s: %w", s, listErr)
				}
				for _, run := range runs {
					if run.CreatedAt.Before(cutoff) {
						candidates = append(candidates, run.ID)
					}
				}
			}

			if len(candidates) == 0 {
				return printData(state, map[string]any{
					"message":  "no runs matched cleanup criteria",
					"cutoff":   cutoff.Format(time.RFC3339),
					"statuses": targetStatuses,
				})
			}

			if dryRun {
				rows := make([]map[string]any, 0, len(candidates))
				for _, id := range candidates {
					rows = append(rows, map[string]any{
						"id":     id,
						"action": "would-delete",
					})
				}
				return printData(state, map[string]any{
					"dry_run": true,
					"count":   len(candidates),
					"runs":    rows,
				})
			}

			if err := requireConfirmation(state, fmt.Sprintf("Delete %d runs older than %s?", len(candidates), olderThan), yes); err != nil {
				return err
			}

			results := make([]map[string]any, 0, len(candidates))
			for _, id := range candidates {
				_, cancelErr := cli.CancelRun(cmd.Context(), id)
				if cancelErr != nil {
					results = append(results, map[string]any{"id": id, "cleaned": false, "error": cancelErr.Error()})
				} else {
					results = append(results, map[string]any{"id": id, "cleaned": true})
				}
			}

			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.Success(fmt.Sprintf("Cleaned up %d run(s)", len(results))))
				return nil
			}
			return printData(state, map[string]any{
				"cleaned": len(results),
				"results": results,
			})
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().DurationVar(&olderThan, "runs-older-than", 0, "remove runs older than this duration (e.g. 720h for 30 days)")
	cmd.Flags().StringVar(&status, "status", "", "target specific status (default: all terminal statuses)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview what would be removed without deleting")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation prompt")
	cmd.Flags().IntVar(&limit, "limit", 100, "max runs to fetch per status")

	return cmd
}
