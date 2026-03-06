package main

import (
	"github.com/spf13/cobra"
)

func newHealthCommand(state *appState) *cobra.Command {
	var ready bool

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check server health",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			if ready {
				status, err := cli.HealthReady(cmd.Context())
				if err != nil {
					return err
				}
				return printData(state, map[string]any{"ready": status.Status})
			}

			status, err := cli.Health(cmd.Context())
			if err != nil {
				return err
			}
			return printData(state, map[string]any{"status": status.Status})
		},
	}

	cmd.Flags().BoolVar(&ready, "ready", false, "check readiness endpoint")

	return cmd
}
