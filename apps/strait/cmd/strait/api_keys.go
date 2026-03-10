package main

import (
	"fmt"
	"strings"

	"strait/internal/cli/client"

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
	cmd.AddCommand(newAPIKeysRotateCommand(state))

	return cmd
}

func newAPIKeysCreateCommand(state *appState) *cobra.Command {
	var projectID string
	var name string
	var scopes string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create API key",
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			created, err := cli.CreateAPIKey(cmd.Context(), req)
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			keys, err := cli.ListAPIKeys(cmd.Context(), projectID)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}
			if err := cli.RevokeAPIKey(cmd.Context(), args[0]); err != nil {
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

func newAPIKeysRotateCommand(state *appState) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "rotate <key-id>",
		Short: "Rotate an API key (create new, revoke old)",
		Long: `Creates a new API key with the same project scope and revokes the old key.

The new key is printed to stdout. The old key is immediately revoked.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			// Get the old key info to determine project ID
			keys, err := cli.ListAPIKeys(cmd.Context(), state.opts.projectID)
			if err != nil {
				return fmt.Errorf("failed to list keys: %w", err)
			}

			var oldKey *struct {
				ID        string
				ProjectID string
				Name      string
			}
			for _, k := range keys {
				if k.ID == args[0] {
					oldKey = &struct {
						ID        string
						ProjectID string
						Name      string
					}{ID: k.ID, ProjectID: k.ProjectID, Name: k.Name}
					break
				}
			}
			if oldKey == nil {
				return fmt.Errorf("API key %s not found", args[0])
			}

			keyName := name
			if keyName == "" {
				keyName = oldKey.Name + "-rotated"
			}

			// Create new key
			newKey, err := cli.CreateAPIKey(cmd.Context(), client.CreateAPIKeyRequest{
				ProjectID: oldKey.ProjectID,
				Name:      keyName,
			})
			if err != nil {
				return fmt.Errorf("failed to create replacement key: %w", err)
			}

			// Revoke old key
			if err := cli.RevokeAPIKey(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("created new key %s but failed to revoke old key: %w", newKey.ID, err)
			}

			return printData(state, map[string]any{
				"rotated":    true,
				"old_key_id": args[0],
				"new_key":    newKey,
			})
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "name for the new key (default: <old-name>-rotated)")

	return cmd
}
