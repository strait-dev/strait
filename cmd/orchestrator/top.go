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
		Use:   "top",
		Short: "Show live resource usage style stats",
		RunE: func(_ *cobra.Command, _ []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			for {
				stats, err := cli.Stats(context.Background())
				if err != nil {
					return err
				}

				rows := []map[string]any{{
					"queued":    stats.Queued,
					"executing": stats.Executing,
					"delayed":   stats.Delayed,
				}}
				if err := printData(state, rows); err != nil {
					return err
				}

				if !watch {
					return nil
				}
				fmt.Println("---")
				time.Sleep(interval)
			}
		},
	}

	cmd.Flags().BoolVar(&watch, "watch", false, "refresh continuously")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "refresh interval in watch mode")

	return cmd
}
