package main

import (
	"fmt"
	"os"

	cliauth "strait/internal/cli/auth"
	cliconfig "strait/internal/cli/config"
	"strait/internal/cli/output"
	"strait/internal/cli/styles"

	"github.com/spf13/cobra"
)

func newContextCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage CLI contexts",
	}

	cmd.AddCommand(newContextCreateCommand(state))
	cmd.AddCommand(newContextUseCommand(state))
	cmd.AddCommand(newContextListCommand(state))
	cmd.AddCommand(newContextCurrentCommand(state))

	return cmd
}

func newContextCreateCommand(state *appState) *cobra.Command {
	var server string
	var project string
	var format string
	var apiKey string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create or update a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			cfg, path, err := loadConfigForWrite(state)
			if err != nil {
				return err
			}

			ctx := cfg.Contexts[name]
			if server != "" {
				ctx.Server = server
			} else if ctx.Server == "" {
				ctx.Server = state.opts.serverURL
			}
			if project != "" {
				ctx.Project = project
			}
			if format != "" {
				ctx.Format = format
			}
			cfg.Contexts[name] = ctx
			if cfg.ActiveContext == "" {
				cfg.ActiveContext = name
			}

			if err := cliconfig.Save(path, cfg); err != nil {
				return err
			}

			if apiKey != "" {
				if err := cliauth.SaveAPIKey(name, apiKey); err != nil {
					return fmt.Errorf("save api key to keychain: %w", err)
				}
			}

			if stdoutIsTTY() && state.opts.outputFormat == "" {
				fmt.Fprintln(os.Stderr, styles.Success("Created context "+styles.Bold.Render(name)))
				return nil
			}
			return printData(state, map[string]any{
				"name":    name,
				"server":  ctx.Server,
				"project": ctx.Project,
				"format":  ctx.Format,
				"active":  cfg.ActiveContext == name,
			})
		},
	}

	cmd.Flags().StringVar(&server, "server", "", "context server URL")
	cmd.Flags().StringVar(&project, "project", "", "default project for context")
	cmd.Flags().StringVar(&format, "format", "", "default output format for context")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key to store in keychain for this context")

	return cmd
}

func newContextUseCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <name>",
		Short: "Set active context",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			cfg, path, err := loadConfigForWrite(state)
			if err != nil {
				return err
			}
			if _, ok := cfg.Contexts[name]; !ok {
				return fmt.Errorf("context %q does not exist", name)
			}
			cfg.ActiveContext = name
			if err := cliconfig.Save(path, cfg); err != nil {
				return err
			}

			if stdoutIsTTY() && state.opts.outputFormat == "" {
				fmt.Fprintln(os.Stderr, styles.Success("Switched to context "+styles.Bold.Render(name)))
				return nil
			}
			return printData(state, map[string]any{"active_context": name})
		},
	}

	return cmd
}

func newContextListCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all contexts",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg := state.config
			if cfg == nil {
				cfg = &cliconfig.File{}
			}

			rows := make([]map[string]any, 0, len(cfg.Contexts))
			for name, ctx := range cfg.Contexts {
				displayName := name
				if stdoutIsTTY() && state.opts.outputFormat == "" && cfg.ActiveContext == name {
					displayName = styles.SelectedStyle.Render(name + " *")
				}
				rows = append(rows, map[string]any{
					"name":    displayName,
					"active":  cfg.ActiveContext == name,
					"server":  ctx.Server,
					"project": ctx.Project,
					"format":  ctx.Format,
				})
			}

			return printData(state, rows)
		},
	}

	return cmd
}

func newContextCurrentCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Show active context",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg := state.config
			if cfg == nil || cfg.ActiveContext == "" {
				return printData(state, map[string]any{"active_context": ""})
			}

			ctx := cfg.Contexts[cfg.ActiveContext]
			return printData(state, map[string]any{
				"name":    cfg.ActiveContext,
				"server":  ctx.Server,
				"project": ctx.Project,
				"format":  ctx.Format,
			})
		},
	}

	return cmd
}

func printData(state *appState, data any) error {
	if state.opts.quiet {
		return printQuietIDs(data)
	}

	tty := stdoutIsTTY()
	format := state.opts.outputFormat
	if format == "" {
		format = "table"
	}
	if !tty && (format == "" || format == "table") {
		format = "json"
	}

	return output.Render(os.Stdout, data, output.Options{
		Format:    format,
		NoHeaders: state.opts.noHeaders,
		Template:  state.opts.outputTpl,
		JSONPath:  state.opts.outputPath,
		TTY:       tty,
	})
}

// printQuietIDs prints only the id field from each row, one per line.
func printQuietIDs(data any) error {
	switch v := data.(type) {
	case []map[string]any:
		for _, row := range v {
			if id, ok := row["id"]; ok {
				fmt.Fprintln(os.Stdout, id)
			}
		}
	default:
		return output.Render(os.Stdout, data, output.Options{Format: "json"})
	}
	return nil
}
