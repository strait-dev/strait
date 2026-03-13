package main

import "github.com/spf13/cobra"

func newServerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Server runtime commands",
	}

	cmd.AddCommand(newServerStartCommand())

	return cmd
}

func newServerStartCommand() *cobra.Command {
	var mode string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start strait server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), mode)
		},
	}

	cmd.Flags().StringVar(&mode, "mode", "", "run mode: api, worker, or all (overrides MODE env)")

	return cmd
}
