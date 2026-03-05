package main

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/cli/client"

	"github.com/spf13/cobra"
)

func newAPIKeysCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api-keys",
		Short: "Manage API keys",
	}

	cmd.AddCommand(newAPIKeysCreateCommand(state))
	cmd.AddCommand(newAPIKeysListCommand(state))
	cmd.AddCommand(newAPIKeysRevokeCommand(state))

	return cmd
}

func newAPIKeysCreateCommand(state *appState) *cobra.Command {
	var projectID string
	var name string
	var scopes string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create API key",
		RunE: func(_ *cobra.Command, _ []string) error {
			if projectID == "" {
				projectID = state.opts.projectID
			}
			if projectID == "" || name == "" {
				return fmt.Errorf("project and name are required")
			}

			req := client.CreateAPIKeyRequest{
				ProjectID: projectID,
				Name:      name,
			}
			if strings.TrimSpace(scopes) != "" {
				req.Scopes = splitCSV(scopes)
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			created, err := cli.CreateAPIKey(context.Background(), req)
			if err != nil {
				return err
			}
			return printData(state, created)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&name, "name", "", "key name")
	cmd.Flags().StringVar(&scopes, "scopes", "", "comma-separated scopes")

	return cmd
}

func newAPIKeysListCommand(state *appState) *cobra.Command {
	var projectID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List API keys",
		RunE: func(_ *cobra.Command, _ []string) error {
			if projectID == "" {
				projectID = state.opts.projectID
			}
			if projectID == "" {
				return fmt.Errorf("project ID is required")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			keys, err := cli.ListAPIKeys(context.Background(), projectID)
			if err != nil {
				return err
			}
			return printData(state, keys)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")

	return cmd
}

func newAPIKeysRevokeCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke <key-id>",
		Short: "Revoke API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			if err := cli.RevokeAPIKey(context.Background(), args[0]); err != nil {
				return err
			}
			return printData(state, map[string]any{"revoked": true, "id": args[0]})
		},
	}

	return cmd
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
