package main

import (
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
			return printData(state, stats)
		},
	}

	return cmd
}
