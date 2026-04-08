package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"strait/internal/cli"

	"github.com/spf13/cobra"
)

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate the CLI with the Strait API",
	}
	cmd.AddCommand(newAuthLoginCommand())
	cmd.AddCommand(newAuthLogoutCommand())
	cmd.AddCommand(newAuthStatusCommand())
	return cmd
}

func newAuthLoginCommand() *cobra.Command {
	var (
		profileName string
		apiURL      string
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in via browser-based device code flow",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthLogin(cmd.Context(), profileName, apiURL)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "default", "profile name to write credentials to")
	cmd.Flags().StringVar(&apiURL, "api-url", cli.DefaultAPIURL, "Strait API base URL")
	return cmd
}

func runAuthLogin(ctx context.Context, profileName, apiURL string) error {
	cfg, err := cli.LoadConfig()
	if err != nil {
		return err
	}

	// Use a bare client (no key yet) just for the auth flow.
	c := cli.NewClient(&cli.Profile{APIURL: apiURL})

	fmt.Fprintln(os.Stderr, "Requesting device code...")
	dc, err := cli.RequestDeviceCode(ctx, c)
	if err != nil {
		return fmt.Errorf("request device code: %w", err)
	}

	verifyURL := dc.VerificationURL
	if !strings.HasPrefix(verifyURL, "http") {
		verifyURL = strings.TrimRight(apiURL, "/") + verifyURL
	}

	fmt.Fprintf(os.Stderr, "\nYour confirmation code: %s\n\n", dc.UserCode)
	fmt.Fprintf(os.Stderr, "Open this URL to approve: %s\n\n", verifyURL)
	fmt.Fprintln(os.Stderr, "Attempting to open your browser automatically...")
	cli.OpenBrowser(verifyURL)
	fmt.Fprintln(os.Stderr, "Waiting for approval...")

	interval := time.Duration(dc.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	tok, err := cli.PollForToken(ctx, c, dc.DeviceCode, interval, deadline)
	if err != nil {
		if errors.Is(err, cli.ErrExpiredToken) {
			return fmt.Errorf("login timed out — run 'strait auth login' again")
		}
		return fmt.Errorf("login failed: %w", err)
	}

	p := &cli.Profile{
		APIURL:    apiURL,
		APIKey:    tok.APIKey,
		ProjectID: tok.ProjectID,
	}
	cfg.SetProfile(profileName, p)
	if err := cli.SaveConfig(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nLogged in. Profile %q saved (project: %s).\n", profileName, tok.ProjectID)
	return nil
}

func newAuthLogoutCommand() *cobra.Command {
	var profileName string
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials for a profile",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := cli.LoadConfig()
			if err != nil {
				return err
			}
			cfg.RemoveProfile(profileName)
			if err := cli.SaveConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Profile %q removed.\n", profileName)
			return nil
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "default", "profile to remove")
	return cmd
}

func newAuthStatusCommand() *cobra.Command {
	var profileName string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current authentication status",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := cli.LoadConfig()
			if err != nil {
				return err
			}
			p := cfg.ActiveProfileData(profileName)

			activeName := profileName
			if activeName == "" {
				activeName = cfg.ActiveProfile
				if activeName == "" {
					activeName = "default"
				}
			}

			fmt.Printf("Profile:    %s\n", activeName)
			fmt.Printf("API URL:    %s\n", p.APIURL)
			if p.ProjectID != "" {
				fmt.Printf("Project:    %s\n", p.ProjectID)
			}
			if p.APIKey != "" {
				masked := maskKey(p.APIKey)
				fmt.Printf("API Key:    %s\n", masked)
				fmt.Printf("Status:     authenticated\n")
			} else {
				fmt.Printf("Status:     not authenticated (run 'strait auth login')\n")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "profile to show (defaults to active)")
	return cmd
}

// maskKey shows the first 8 characters and masks the rest.
func maskKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:8] + strings.Repeat("*", len(key)-8)
}
