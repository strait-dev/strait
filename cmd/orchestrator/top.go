package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newTopCommand(state *appState) *cobra.Command {
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:     "top",
		Short:   "Show live resource usage style stats",
		Long:    "Displays queue runtime metrics and can refresh continuously in watch mode.",
		Example: "orchestrator top\n  orchestrator top --watch\n  orchestrator top --watch --interval 5s",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if interval <= 0 {
				return fmt.Errorf("interval must be greater than zero")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			for {
				stats, err := cli.Stats(context.Background())
				if err != nil {
					return err
				}

				sampledAt := time.Now().UTC().Format(time.RFC3339)
				rows := []map[string]any{
					{"metric": "queued", "value": stats.Queued, "sampled_at": sampledAt},
					{"metric": "executing", "value": stats.Executing, "sampled_at": sampledAt},
					{"metric": "delayed", "value": stats.Delayed, "sampled_at": sampledAt},
				}
				if err := printData(state, rows); err != nil {
					return err
				}

				if !watch {
					return nil
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "--- press Ctrl+C to stop ---")
				time.Sleep(interval)
			}
		},
	}

	cmd.Flags().BoolVar(&watch, "watch", false, "refresh continuously")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "refresh interval in watch mode")

	return cmd
}
