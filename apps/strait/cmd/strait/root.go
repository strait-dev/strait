package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

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
	cmd.AddCommand(newAuditCommand())

	rawArgs := os.Args[1:]
	cmd.SetArgs(normalizeLegacyArgs(rawArgs))

	return cmd
}

func normalizeLegacyArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}

	subcommands := map[string]struct{}{
		"serve":   {},
		"server":  {},
		"migrate": {},
		"version": {},
		"help":    {},
		"audit":   {},
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
