package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"syscall"

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
	}

	cmd.AddCommand(newSecretsCreateCommand(state))
	cmd.AddCommand(newSecretsListCommand(state))
	cmd.AddCommand(newSecretsDeleteCommand(state))

	return cmd
}

func newSecretsCreateCommand(state *appState) *cobra.Command {
	var fromEnv string
	var fromFile string
	var projectID string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create or update a secret value",
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

func newSecretsListCommand(state *appState) *cobra.Command {
	var projectID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secret names for a project",
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

func newSecretsDeleteCommand(state *appState) *cobra.Command {
	var projectID string
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a secret",
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
