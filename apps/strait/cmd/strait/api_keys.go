package main

import (
	"fmt"
	"os"
	"strings"

	"strait/internal/cli/client"
	"strait/internal/cli/styles"

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
			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.Success("Created API key "+styles.Bold.Render(created.Name)))
				fmt.Fprintln(os.Stderr, styles.KeyValue("Key", created.Key))
				fmt.Fprintln(os.Stderr, styles.MutedStyle.Render("  Store this key securely; it will not be shown again."))
				return nil
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
			var err error
			projectID, err = requireProjectID(state, projectID)
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
			if isTTYRich(state) {
				rows := make([]map[string]any, 0, len(keys))
				for _, k := range keys {
					rows = append(rows, map[string]any{
						"id":     k.ID,
						"name":   k.Name,
						"prefix": styles.MutedStyle.Render(k.KeyPrefix),
						"scopes": strings.Join(k.Scopes, ","),
					})
				}
				return printData(state, rows)
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
			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.Success("Revoked API key "+styles.Bold.Render(args[0])))
				return nil
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
	var gracePeriodMinutes int

	cmd := &cobra.Command{
		Use:   "rotate <key-id>",
		Short: "Rotate an API key with a grace window",
		Long: `Rotates an API key via the API rotate endpoint.

The command returns the newly issued key and keeps the old key valid until the grace window expires.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			resp, err := cli.RotateAPIKey(cmd.Context(), args[0], client.RotateAPIKeyRequest{GracePeriodMinutes: gracePeriodMinutes})
			if err != nil {
				return fmt.Errorf("failed to rotate key: %w", err)
			}

			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.Success("Rotated API key "+styles.Bold.Render(args[0])))
				fmt.Fprintln(os.Stderr, styles.KeyValue("Grace Period", fmt.Sprintf("%d minutes", gracePeriodMinutes)))
				return nil
			}
			return printData(state, map[string]any{
				"rotated": true,
				"result":  resp,
			})
		},
	}

	cmd.Flags().IntVar(&gracePeriodMinutes, "grace-period-minutes", 60, "grace period in minutes for old key validity")

	return cmd
}
