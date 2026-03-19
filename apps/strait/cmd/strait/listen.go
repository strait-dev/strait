package main

import (
	"fmt"
	"os"
	"time"

	"strait/internal/cli/styles"

	"github.com/spf13/cobra"
)

func newListenCommand(state *appState) *cobra.Command {
	var projectID string
	var status string
	var interval time.Duration
	var limit int

	cmd := &cobra.Command{
		Use:   "listen",
		Short: "Watch for new runs in real time",
		Long: `Continuously polls for new runs and prints them as they appear.

Acts as a simple event stream for monitoring run creation and status changes.
Deduplicates by run ID so each run appears only once (or when status changes).`,
		Example: `  strait listen --project proj_1
  strait listen --project proj_1 --status executing
  strait listen --interval 3s`,
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
			ctx := cmd.Context()

			seen := make(map[string]string) // runID -> last seen status

			for {
				runs, err := cli.ListRuns(ctx, projectID, status, limit, nil)
				if err != nil {
					fmt.Fprintf(os.Stderr, "[error] %v\n", err)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(interval):
					}
					continue
				}

				for i := len(runs) - 1; i >= 0; i-- {
					run := runs[i]
					lastStatus, known := seen[run.ID]
					currentStatus := string(run.Status)

					if !known || lastStatus != currentStatus {
						seen[run.ID] = currentStatus
						ts := run.CreatedAt.UTC().Format(time.RFC3339)
						statusStr := styles.Status(currentStatus)
						fmt.Fprintf(os.Stdout, "%s  %s  %s  attempt=%d  triggered_by=%s\n",
							ts, run.ID, statusStr, run.Attempt, run.TriggeredBy)
					}
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(interval):
				}
			}
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID to watch")
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "poll interval")
	cmd.Flags().IntVar(&limit, "limit", 50, "max runs per poll")

	return cmd
}
