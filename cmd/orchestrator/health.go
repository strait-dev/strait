package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newHealthCommand(state *appState) *cobra.Command {
	var ready bool

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check server health",
		RunE: func(_ *cobra.Command, _ []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			if ready {
				status, err := cli.HealthReady(context.Background())
				if err != nil {
					return err
				}
				return printData(state, map[string]any{"ready": status.Status})
			}

			status, err := cli.Health(context.Background())
			if err != nil {
				return err
			}
			return printData(state, map[string]any{"status": status.Status})
		},
	}

	cmd.Flags().BoolVar(&ready, "ready", false, "check readiness endpoint")

	return cmd
}
