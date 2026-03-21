package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	cliauth "strait/internal/cli/auth"
	"strait/internal/cli/client"
	cliconfig "strait/internal/cli/config"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newLoginCommand(state *appState) *cobra.Command {
	var apiKey string
	var withToken bool
	var token string
	var contextName string
	var server string
	var browser bool
	var noBrowser bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with the Strait API",
		Long: `Authenticate with the Strait API using browser-based device code flow or a direct API key.

By default, opens a browser for device code authorization. Use --token to provide
an API key directly, or --with-token to read one from stdin.`,
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

			// Merge --token into apiKey for unified handling.
			if token != "" && apiKey == "" {
				apiKey = token
			}

			// Direct token mode: --token or --api-key or --with-token provided.
			if apiKey != "" || withToken {
				return loginWithAPIKey(cmd, state, apiKey, withToken, targetContext, targetServer)
			}

			// Non-TTY without explicit token: error with guidance.
			if !term.IsTerminal(syscall.Stdin) {
				return fmt.Errorf("non-interactive terminal detected; use --token <api-key> or pipe a key via --with-token")
			}

			// Browser-based device code flow (default for interactive terminals).
			useDeviceFlow := !noBrowser
			if useDeviceFlow {
				err := loginWithDeviceCode(cmd, state, targetContext, targetServer)
				if err == nil {
					return nil
				}
				// If device code flow fails, fall back to manual key entry
				// unless --browser was explicitly set (user expected it to work).
				if browser {
					return err
				}
				fmt.Fprintf(os.Stderr, "Device code login unavailable: %v\n", err)
				fmt.Fprintln(os.Stderr, "Falling back to manual API key entry.")
			}

			// Legacy browser-open flow or manual entry fallback.
			useLegacyBrowser := browser && !noBrowser
			if useLegacyBrowser {
				dashURL := cliauth.DashboardURL(targetServer)
				if dashURL != "" {
					keysURL := dashURL + "/settings/api-keys"
					fmt.Fprintf(os.Stderr, "Opening %s in your browser...\n", keysURL)
					if err := openBrowser(keysURL); err != nil {
						fmt.Fprintf(os.Stderr, "Could not open browser. Visit %s manually.\n", keysURL)
					}
					fmt.Fprintln(os.Stderr, "Create an API key, then paste it below.")
				}
			}

			return loginWithAPIKey(cmd, state, "", false, targetContext, targetServer)
		},
	}

	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key (deprecated, use --token)")
	cmd.Flags().BoolVar(&withToken, "with-token", false, "read API key from stdin")
	cmd.Flags().StringVar(&token, "token", "", "API key for direct authentication")
	cmd.Flags().StringVar(&contextName, "context", "", "context to save API key under")
	cmd.Flags().StringVar(&server, "server", "", "server URL to validate against")
	cmd.Flags().BoolVar(&browser, "browser", false, "open the dashboard API key page in your browser")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "do not open browser")

	return cmd
}

// loginWithDeviceCode performs the OAuth device code authorization flow.
func loginWithDeviceCode(cmd *cobra.Command, state *appState, targetContext, targetServer string) error {
	c, err := client.New(targetServer, "", state.opts.timeout)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	resp, err := c.RequestDeviceCode(cmd.Context())
	if err != nil {
		return fmt.Errorf("request device code: %w", err)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "Your one-time code: %s\n", resp.UserCode)
	fmt.Fprintf(os.Stderr, "Open this URL to authenticate: %s\n", resp.VerificationURL)
	fmt.Fprintln(os.Stderr, "")

	// Try to open the browser automatically.
	if err := openBrowser(resp.VerificationURL); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open browser automatically. Visit the URL above.\n")
	} else {
		fmt.Fprintln(os.Stderr, "Waiting for browser authorization...")
	}

	// Poll with progress indicator.
	ctx := cmd.Context()
	interval := resp.Interval
	if interval <= 0 {
		interval = 5
	}

	// Start a goroutine to print dots as a progress indicator.
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				fmt.Fprint(os.Stderr, ".")
			}
		}
	}()

	tokenResp, err := c.PollDeviceToken(ctx, resp.DeviceCode, interval, resp.ExpiresIn)
	close(done)
	fmt.Fprintln(os.Stderr, "") // newline after dots

	if err != nil {
		if strings.Contains(err.Error(), "expired") {
			fmt.Fprintln(os.Stderr, "Authorization timed out. You can try again or use --token <api-key> instead.")
		}
		return fmt.Errorf("device code authorization: %w", err)
	}

	if err := cliauth.SaveAPIKey(targetContext, tokenResp.APIKey); err != nil {
		return fmt.Errorf("save api key: %w", err)
	}

	cfg, cfgPath, err := loadConfigForWrite(state)
	if err != nil {
		return err
	}
	if cfg.Contexts == nil {
		cfg.Contexts = make(map[string]cliconfig.Context)
	}
	cfgCtx := cfg.Contexts[targetContext]
	if targetServer != "" {
		cfgCtx.Server = targetServer
	}
	if tokenResp.ProjectID != "" {
		cfgCtx.Project = tokenResp.ProjectID
	}
	cfg.Contexts[targetContext] = cfgCtx
	cfg.ActiveContext = targetContext
	if err := cliconfig.Save(cfgPath, cfg); err != nil {
		return err
	}

	return printData(state, map[string]any{
		"authenticated": true,
		"context":       targetContext,
		"server":        targetServer,
	})
}

// loginWithAPIKey handles direct API key authentication (--token, --api-key, --with-token, or manual entry).
func loginWithAPIKey(cmd *cobra.Command, state *appState, apiKey string, withToken bool, targetContext, targetServer string) error {
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

	cfg, cfgPath, err := loadConfigForWrite(state)
	if err != nil {
		return err
	}
	if cfg.Contexts == nil {
		cfg.Contexts = make(map[string]cliconfig.Context)
	}
	cfgCtx := cfg.Contexts[targetContext]
	if targetServer != "" {
		cfgCtx.Server = targetServer
	}
	cfg.Contexts[targetContext] = cfgCtx
	cfg.ActiveContext = targetContext
	if err := cliconfig.Save(cfgPath, cfg); err != nil {
		return err
	}

	return printData(state, map[string]any{
		"authenticated": true,
		"context":       targetContext,
		"server":        targetServer,
	})
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

	if v := strings.TrimSpace(os.Getenv("STRAIT_API_KEY")); v != "" {
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
