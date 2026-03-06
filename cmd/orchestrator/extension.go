package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const extensionPrefix = "orchestrator-"

func newExtensionCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extension",
		Short: "Manage CLI extensions",
		Long: `Discover and run CLI extensions.

Extensions are executables named orchestrator-<name> found in your PATH.
They are invoked as subcommands: orchestrator extension run <name> [args...]`,
	}

	cmd.AddCommand(newExtensionListCommand(state))
	cmd.AddCommand(newExtensionRunCommand())

	return cmd
}

func newExtensionListCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discovered extensions",
		RunE: func(_ *cobra.Command, _ []string) error {
			extensions := discoverExtensions()
			if len(extensions) == 0 {
				return printData(state, map[string]any{
					"message": "no extensions found in PATH",
					"hint":    fmt.Sprintf("extensions are executables named %s<name>", extensionPrefix),
				})
			}

			rows := make([]map[string]any, 0, len(extensions))
			for _, ext := range extensions {
				rows = append(rows, map[string]any{
					"name": ext.name,
					"path": ext.path,
				})
			}
			return printData(state, rows)
		},
	}

	return cmd
}

func newExtensionRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "run <extension-name> [args...]",
		Short:              "Run an extension",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			binName := extensionPrefix + name

			binPath, err := exec.LookPath(binName)
			if err != nil {
				return fmt.Errorf("extension %q not found in PATH (looking for %s)", name, binName)
			}

			extCmd := exec.Command(binPath, args[1:]...) //nolint:gosec // extension name from CLI args, path-resolved
			extCmd.Stdin = os.Stdin
			extCmd.Stdout = os.Stdout
			extCmd.Stderr = os.Stderr
			return extCmd.Run()
		},
	}

	return cmd
}

type extensionInfo struct {
	name string
	path string
}

func discoverExtensions() []extensionInfo {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil
	}

	seen := make(map[string]bool)
	var extensions []extensionInfo

	for _, dir := range filepath.SplitList(pathEnv) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasPrefix(name, extensionPrefix) {
				continue
			}

			extName := strings.TrimPrefix(name, extensionPrefix)
			if extName == "" || seen[extName] {
				continue
			}

			fullPath := filepath.Join(dir, name)
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.Mode()&0o111 == 0 {
				continue // not executable
			}

			seen[extName] = true
			extensions = append(extensions, extensionInfo{name: extName, path: fullPath})
		}
	}

	return extensions
}
