package main

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "strait",
		Short:         "Strait server runtime",
		Long:          "Strait job orchestration server. Runs in api, worker, or all mode.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), "")
		},
	}

	cmd.AddCommand(newServeCommand())
	cmd.AddCommand(newServerCommand())
	cmd.AddCommand(newMigrateCommand())
	cmd.AddCommand(newVersionCommand())
	cmd.AddCommand(newHealthCommand())
	cmd.AddCommand(newBackfillHistoryCommand())
	cmd.AddCommand(newBackfillEntitlementsCommand())
	// Pentest seeding/cleanup commands are compiled in only under the `pentest`
	// build tag, so production binaries do not ship them.
	registerPentestCommands(cmd)

	rawArgs := os.Args[1:]
	cmd.SetArgs(normalizeLegacyArgs(rawArgs))

	return cmd
}

func normalizeLegacyArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}

	subcommands := map[string]struct{}{
		"serve":                 {},
		"server":                {},
		"migrate":               {},
		"version":               {},
		"health":                {},
		"help":                  {},
		"backfill-history":      {},
		"backfill-entitlements": {},
		"seed-pentest":          {},
		"revoke-pentest":        {},
	}

	first := args[0]
	if _, ok := subcommands[first]; ok {
		return args
	}

	if strings.HasPrefix(first, "-") || containsModeFlag(args) {
		return append([]string{"serve"}, args...)
	}

	return args
}

func containsModeFlag(args []string) bool {
	for i := range args {
		if args[i] == "--mode" || strings.HasPrefix(args[i], "--mode=") {
			return true
		}
	}

	return false
}

func newServeCommand() *cobra.Command {
	var mode string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start strait server components",
		Long:  "Starts strait runtime in api, worker, or all mode.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), mode)
		},
	}

	cmd.Flags().StringVar(&mode, "mode", "", "run mode: api, worker, or all (overrides MODE env)")

	return cmd
}

func newHealthCommand() *cobra.Command {
	var quiet bool
	var healthURL string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check strait readiness",
		RunE: func(cmd *cobra.Command, _ []string) error {
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, healthURL, nil)
			if err != nil {
				return fmt.Errorf("create health request: %w", err)
			}

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("readiness check failed: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
				return fmt.Errorf("readiness check returned HTTP %d", resp.StatusCode)
			}
			if !quiet {
				fmt.Fprintln(cmd.OutOrStdout(), "ok")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress success output")
	cmd.Flags().StringVar(&healthURL, "url", "http://localhost:8080/health/ready", "readiness URL")

	return cmd
}

func newVersionCommand() *cobra.Command {
	var short bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			if short {
				fmt.Println(version)
				return nil
			}

			fmt.Printf("version: %s\n", version)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("date: %s\n", date)
			fmt.Printf("go: %s\n", runtime.Version())
			fmt.Printf("os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "print only the version number")

	return cmd
}
