package main

import (
	"fmt"
	"os"

	"strait/internal/cli/styles"

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
				if stdoutIsTTY() && state.opts.outputFormat == "" {
					if status.Status == "ok" || status.Status == "healthy" {
						fmt.Fprintln(os.Stderr, styles.Success("Server is ready"))
					} else {
						fmt.Fprintln(os.Stderr, styles.Err("Server readiness: "+status.Status))
					}
					return nil
				}
				return printData(state, map[string]any{"ready": status.Status})
			}

			status, err := cli.Health(cmd.Context())
			if err != nil {
				return err
			}
			if stdoutIsTTY() && state.opts.outputFormat == "" {
				if status.Status == "ok" || status.Status == "healthy" {
					fmt.Fprintln(os.Stderr, styles.Success("Server is healthy"))
				} else {
					fmt.Fprintln(os.Stderr, styles.Err("Server status: "+status.Status))
				}
				return nil
			}
			return printData(state, map[string]any{"status": status.Status})
		},
	}

	cmd.Flags().BoolVar(&ready, "ready", false, "check readiness endpoint")

	return cmd
}
