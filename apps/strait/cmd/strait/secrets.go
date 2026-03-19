package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"syscall"

	"strait/internal/cli/client"
	cliconfig "strait/internal/cli/config"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const secretServiceName = "strait-secrets"

func newSecretsCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage project secrets",
		Long:  "Manage server-side secrets and local keyring secrets.",
	}

	cmd.AddCommand(newSecretsListCommand(state))
	cmd.AddCommand(newSecretsCreateCommand(state))
	cmd.AddCommand(newSecretsDeleteCommand(state))
	cmd.AddCommand(newSecretsLocalCommand(state))

	return cmd
}

// Server-side secrets commands.

func newSecretsListCommand(state *appState) *cobra.Command {
	var projectID string
	var environment string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List server-side secrets for a project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			secrets, err := cli.ListServerSecrets(cmd.Context(), projectID, environment)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(secrets))
			for _, s := range secrets {
				rows = append(rows, map[string]any{
					"id":          s.ID,
					"key":         s.SecretKey,
					"environment": s.Environment,
					"value":       "***",
					"created_at":  s.CreatedAt,
				})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&environment, "environment", "", "filter by environment")

	return cmd
}

func newSecretsCreateCommand(state *appState) *cobra.Command {
	var projectID string
	var key string
	var value string
	var environment string
	var jobID string
	var valueFromEnv string
	var valueFromFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a server-side secret",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			if key == "" {
				return fmt.Errorf("--key is required")
			}

			resolvedValue, err := resolveServerSecretValue(value, valueFromEnv, valueFromFile)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			req := client.CreateServerSecretRequest{
				ProjectID:   projectID,
				SecretKey:   key,
				SecretValue: resolvedValue,
				Environment: environment,
				JobID:       jobID,
			}

			secret, err := cli.CreateServerSecret(cmd.Context(), req)
			if err != nil {
				return err
			}

			return printData(state, map[string]any{
				"id":          secret.ID,
				"key":         secret.SecretKey,
				"environment": secret.Environment,
				"value":       "***",
				"created":     true,
			})
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&key, "key", "", "secret key name")
	cmd.Flags().StringVar(&value, "value", "", "secret value")
	cmd.Flags().StringVar(&environment, "environment", "", "environment scope")
	cmd.Flags().StringVar(&jobID, "job-id", "", "scope secret to a specific job")
	cmd.Flags().StringVar(&valueFromEnv, "value-from-env", "", "read value from environment variable")
	cmd.Flags().StringVar(&valueFromFile, "value-from-file", "", "read value from file")

	return cmd
}

func newSecretsDeleteCommand(state *appState) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <secret-id>",
		Short: "Delete a server-side secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireConfirmation(state, "Delete this secret?", yes); err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			if err := cli.DeleteServerSecret(cmd.Context(), args[0]); err != nil {
				return err
			}

			return printData(state, map[string]any{"id": args[0], "deleted": true})
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")

	return cmd
}

// Local keyring secrets commands.

func newSecretsLocalCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "local",
		Short: "Manage local keyring secrets",
	}

	cmd.AddCommand(newSecretsLocalCreateCommand(state))
	cmd.AddCommand(newSecretsLocalListCommand(state))
	cmd.AddCommand(newSecretsLocalDeleteCommand(state))

	return cmd
}

func newSecretsLocalCreateCommand(state *appState) *cobra.Command {
	var fromEnv string
	var fromFile string
	var projectID string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create or update a local secret value",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			name := strings.TrimSpace(args[0])
			if name == "" {
				return fmt.Errorf("secret name is required")
			}

			value, err := resolveSecretValue(fromEnv, fromFile)
			if err != nil {
				return err
			}

			if err := keyring.Set(secretServiceName, secretKey(projectID, name), value); err != nil {
				return err
			}

			cfg, path, err := loadConfigForWrite(state)
			if err != nil {
				return err
			}
			cfg.Secrets[projectID] = addUnique(cfg.Secrets[projectID], name)
			if err := cliconfig.Save(path, cfg); err != nil {
				return err
			}

			return printData(state, map[string]any{"project": projectID, "name": name, "stored": true})
		},
	}

	cmd.Flags().StringVar(&fromEnv, "from-env", "", "read secret value from environment variable name")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "read secret value from file path")
	cmd.Flags().StringVar(&projectID, "project", "", "project ID")

	return cmd
}

func newSecretsLocalListCommand(state *appState) *cobra.Command {
	var projectID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List local secret names for a project",
		RunE: func(_ *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cfg := state.config
			if cfg == nil {
				cfg = &cliconfig.File{}
			}

			names := append([]string(nil), cfg.Secrets[projectID]...)
			sort.Strings(names)
			rows := make([]map[string]any, 0, len(names))
			for _, name := range names {
				rows = append(rows, map[string]any{"project": projectID, "name": name})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")

	return cmd
}

func newSecretsLocalDeleteCommand(state *appState) *cobra.Command {
	var projectID string
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a local secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := requireConfirmation(state, "Delete this secret?", yes); err != nil {
				return err
			}
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			name := strings.TrimSpace(args[0])
			if name == "" {
				return fmt.Errorf("secret name is required")
			}

			err = keyring.Delete(secretServiceName, secretKey(projectID, name))
			if err != nil && !errors.Is(err, keyring.ErrNotFound) {
				return err
			}

			cfg, path, err := loadConfigForWrite(state)
			if err != nil {
				return err
			}
			cfg.Secrets[projectID] = removeValue(cfg.Secrets[projectID], name)
			if err := cliconfig.Save(path, cfg); err != nil {
				return err
			}

			return printData(state, map[string]any{"project": projectID, "name": name, "deleted": true})
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")

	return cmd
}

// Helper functions.

func resolveServerSecretValue(directValue, fromEnv, fromFile string) (string, error) {
	if strings.TrimSpace(directValue) != "" {
		return strings.TrimSpace(directValue), nil
	}
	return resolveSecretValue(fromEnv, fromFile)
}

func resolveSecretValue(fromEnv, fromFile string) (string, error) {
	if strings.TrimSpace(fromEnv) != "" {
		value := strings.TrimSpace(os.Getenv(strings.TrimSpace(fromEnv)))
		if value == "" {
			return "", fmt.Errorf("environment variable %q is empty", fromEnv)
		}
		return value, nil
	}

	if strings.TrimSpace(fromFile) != "" {
		raw, err := os.ReadFile(fromFile) //nolint:gosec // fromFile is from --from-file CLI flag
		if err != nil {
			return "", err
		}
		value := strings.TrimSpace(string(raw))
		if value == "" {
			return "", fmt.Errorf("file %q is empty", fromFile)
		}
		return value, nil
	}

	fmt.Fprint(os.Stderr, "Secret value: ")
	secret, err := term.ReadPassword(syscall.Stdin)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(secret))
	if value == "" {
		return "", fmt.Errorf("secret value is required")
	}
	return value, nil
}

func secretKey(projectID, name string) string {
	return projectID + ":" + name
}

func addUnique(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func removeValue(values []string, value string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != value {
			out = append(out, v)
		}
	}
	return out
}
