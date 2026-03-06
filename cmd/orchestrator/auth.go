package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	cliauth "orchestrator/internal/cli/auth"
	cliconfig "orchestrator/internal/cli/config"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newLoginCommand(state *appState) *cobra.Command {
	var apiKey string
	var withToken bool
	var contextName string
	var server string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with an API key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			targetContext := contextName
			if targetContext == "" {
				targetContext = state.opts.contextName
			}
			if targetContext == "" {
				targetContext = "default"
			}

			targetServer := server
			if targetServer == "" {
				targetServer = state.opts.serverURL
			}

			resolvedKey, err := resolveAPIKeyInput(apiKey, withToken)
			if err != nil {
				return err
			}

			if err := cliauth.ValidateAPIKey(cmd.Context(), targetServer, resolvedKey, state.opts.timeout); err != nil {
				return err
			}

			if err := cliauth.SaveAPIKey(targetContext, resolvedKey); err != nil {
				return fmt.Errorf("save api key: %w", err)
			}

			cfg, path, err := loadConfigForWrite(state)
			if err != nil {
				return err
			}
			ctx := cfg.Contexts[targetContext]
			if targetServer != "" {
				ctx.Server = targetServer
			}
			cfg.Contexts[targetContext] = ctx
			cfg.ActiveContext = targetContext
			if err := cliconfig.Save(path, cfg); err != nil {
				return err
			}

			return printData(state, map[string]any{
				"authenticated": true,
				"context":       targetContext,
				"server":        targetServer,
			})
		},
	}

	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key")
	cmd.Flags().BoolVar(&withToken, "with-token", false, "read API key from stdin")
	cmd.Flags().StringVar(&contextName, "context", "", "context to save API key under")
	cmd.Flags().StringVar(&server, "server", "", "server URL to validate against")

	return cmd
}

func newLogoutCommand(state *appState) *cobra.Command {
	var contextName string

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove stored API key from keychain",
		RunE: func(_ *cobra.Command, _ []string) error {
			targetContext := contextName
			if targetContext == "" {
				targetContext = state.opts.contextName
			}
			if targetContext == "" {
				targetContext = "default"
			}

			if err := cliauth.DeleteAPIKey(targetContext); err != nil {
				return fmt.Errorf("delete api key: %w", err)
			}

			return printData(state, map[string]any{
				"logged_out": true,
				"context":    targetContext,
			})
		},
	}

	cmd.Flags().StringVar(&contextName, "context", "", "context to remove API key from")

	return cmd
}

func newAuthCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication helper commands",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		RunE: func(_ *cobra.Command, _ []string) error {
			targetContext := state.opts.contextName
			if targetContext == "" {
				targetContext = "default"
			}

			_, err := cliauth.LoadAPIKey(targetContext)
			authed := err == nil
			return printData(state, map[string]any{
				"authenticated": authed,
				"context":       targetContext,
				"server":        state.opts.serverURL,
			})
		},
	})

	return cmd
}

func resolveAPIKeyInput(flagValue string, withToken bool) (string, error) {
	if v := strings.TrimSpace(flagValue); v != "" {
		return v, nil
	}

	if withToken {
		reader := bufio.NewReader(os.Stdin)
		token, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		if v := strings.TrimSpace(token); v != "" {
			return v, nil
		}
	}

	if v := strings.TrimSpace(os.Getenv("ORCHESTRATOR_API_KEY")); v != "" {
		return v, nil
	}

	fmt.Fprint(os.Stderr, "API key: ")
	secret, err := term.ReadPassword(syscall.Stdin)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if v := strings.TrimSpace(string(secret)); v != "" {
		return v, nil
	}

	return "", fmt.Errorf("api key is required")
}
