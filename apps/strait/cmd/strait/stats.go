package main

import (
	"fmt"
	"os"

	"strait/internal/cli/styles"

	"github.com/spf13/cobra"
)

func newStatsCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show queue statistics",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			stats, err := cli.Stats(cmd.Context())
			if err != nil {
				return err
			}
			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.SectionHeader("Queue Statistics", -1))
				fmt.Fprintln(os.Stderr, styles.KeyValue("Queued", fmt.Sprintf("%d", stats.Queued)))
				fmt.Fprintln(os.Stderr, styles.KeyValue("Executing", fmt.Sprintf("%d", stats.Executing)))
				fmt.Fprintln(os.Stderr, styles.KeyValue("Delayed", fmt.Sprintf("%d", stats.Delayed)))
				return nil
			}
			return printData(state, stats)
		},
	}

	return cmd
}
