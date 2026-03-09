package main

import (
	"fmt"
	"sort"
	"strings"

	cliconfig "strait/internal/cli/config"

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

func newAliasSetCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <name> <expansion>",
		Short: "Set a command alias",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, path, err := loadConfigForWrite(state)
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
			rows := make([]map[string]any, 0, len(keys))
			sort.Strings(keys)
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
			cfg, path, err := loadConfigForWrite(state)
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
			return printData(state, map[string]any{"deleted": args[0], "ok": true})
		},
	}

	return cmd
}

func expandAliasArgs(args []string, configPath string) []string {
	if len(args) == 0 {
		return args
	}

	loaded, err := cliconfig.Load(configPath)
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
