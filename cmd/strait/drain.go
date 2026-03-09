package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newDrainCommand(state *appState) *cobra.Command {
	var timeout time.Duration
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "drain",
		Short: "Wait for executing runs to complete",
		Long: `Polls the queue stats and waits until all executing runs finish.

Useful before shutting down workers or performing maintenance.
Exits 0 when executing count reaches 0, exits 1 on timeout.`,
		Example: `  strait drain
  strait drain --timeout 5m
  strait drain --interval 3s --timeout 10m`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			deadline := time.Now().Add(timeout)
			attempt := 0

			for {
				attempt++
				stats, err := cli.Stats(ctx)
				if err != nil {
					return fmt.Errorf("failed to fetch stats: %w", err)
				}

				if state.opts.verbose {
					fmt.Printf("[%s] queued=%d executing=%d delayed=%d\n",
						time.Now().UTC().Format(time.RFC3339), stats.Queued, stats.Executing, stats.Delayed)
				}

				if stats.Executing == 0 {
					return printData(state, map[string]any{
						"drained":   true,
						"queued":    stats.Queued,
						"executing": stats.Executing,
						"delayed":   stats.Delayed,
						"polls":     attempt,
					})
				}

				if time.Now().After(deadline) {
					return fmt.Errorf("drain timeout: %d runs still executing after %s", stats.Executing, timeout)
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(interval):
				}
			}
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "max time to wait for drain")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "poll interval")

	return cmd
}
