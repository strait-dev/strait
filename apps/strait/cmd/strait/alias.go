package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	cliconfig "strait/internal/cli/config"
	"strait/internal/cli/styles"

	"github.com/spf13/cobra"
)

func newAliasCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Manage command aliases",
	}

	cmd.AddCommand(newAliasSetCommand(state))
	cmd.AddCommand(newAliasListCommand(state))
	cmd.AddCommand(newAliasDeleteCommand(state))

	return cmd
}

func loadHomeConfigForWrite() (*cliconfig.File, string, error) {
	homePath, err := cliconfig.HomePath()
	if err != nil {
		return nil, "", err
	}
	loaded, err := cliconfig.Load(homePath)
	if err != nil {
		return nil, "", err
	}
	if loaded.Data == nil {
		return nil, "", fmt.Errorf("unable to load home config")
	}
	return loaded.Data, loaded.Path, nil
}

func newAliasSetCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <name> <expansion>",
		Short: "Set a command alias",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, path, err := loadHomeConfigForWrite()
			if err != nil {
				return err
			}
			if cfg.Aliases == nil {
				cfg.Aliases = map[string]string{}
			}
			cfg.Aliases[args[0]] = args[1]
			if err := cliconfig.Save(path, cfg); err != nil {
				return err
			}
			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.Success("Set alias "+styles.Bold.Render(args[0])+" -> "+args[1]))
				return nil
			}
			return printData(state, map[string]any{"alias": args[0], "expansion": args[1]})
		},
	}

	return cmd
}

func newAliasListCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured aliases",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg := state.config
			if cfg == nil {
				cfg = &cliconfig.File{}
			}

			keys := make([]string, 0, len(cfg.Aliases))
			for k := range cfg.Aliases {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.SectionHeader("Aliases", len(keys)))
				for _, k := range keys {
					fmt.Fprintf(os.Stderr, "  %s -> %s\n",
						styles.Bold.Render(k),
						styles.MutedStyle.Render(cfg.Aliases[k]),
					)
				}
				return nil
			}
			rows := make([]map[string]any, 0, len(keys))
			for _, k := range keys {
				rows = append(rows, map[string]any{"alias": k, "expansion": cfg.Aliases[k]})
			}
			return printData(state, rows)
		},
	}

	return cmd
}

func newAliasDeleteCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete command alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, path, err := loadHomeConfigForWrite()
			if err != nil {
				return err
			}
			if cfg.Aliases == nil {
				return fmt.Errorf("alias %q not found", args[0])
			}
			if _, ok := cfg.Aliases[args[0]]; !ok {
				return fmt.Errorf("alias %q not found", args[0])
			}
			delete(cfg.Aliases, args[0])
			if err := cliconfig.Save(path, cfg); err != nil {
				return err
			}
			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.Success("Deleted alias "+styles.Bold.Render(args[0])))
				return nil
			}
			return printData(state, map[string]any{"deleted": args[0], "ok": true})
		},
	}

	return cmd
}

func expandAliasArgs(args []string, configPath string) []string {
	if len(args) == 0 {
		return args
	}

	loadPath := configPath
	if loadPath == "" {
		homePath, err := cliconfig.HomePath()
		if err != nil {
			return args
		}
		loadPath = homePath
	}

	loaded, err := cliconfig.Load(loadPath)
	if err != nil || loaded == nil || loaded.Data == nil || len(loaded.Data.Aliases) == 0 {
		return args
	}

	first := strings.TrimSpace(args[0])
	expansion, ok := loaded.Data.Aliases[first]
	if !ok {
		return args
	}

	expanded := strings.Fields(expansion)
	if len(expanded) == 0 {
		return args
	}

	return append(expanded, args[1:]...)
}
