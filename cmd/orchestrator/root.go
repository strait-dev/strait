package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	cliauth "orchestrator/internal/cli/auth"
	cliconfig "orchestrator/internal/cli/config"

	"github.com/spf13/cobra"
)

type rootOptions struct {
	serverURL    string
	apiKey       string
	projectID    string
	outputFormat string
	noColor      bool
	quiet        bool
	verbose      bool
	contextName  string
	configPath   string
	timeout      time.Duration
}

type appState struct {
	opts       *rootOptions
	configPath string
	config     *cliconfig.File
	resolved   cliconfig.Resolved
}

func newRootCommand() *cobra.Command {
	opts := &rootOptions{}
	state := &appState{opts: opts}

	cmd := &cobra.Command{
		Use:           "orchestrator",
		Short:         "Orchestrator CLI and server runtime",
		Long:          "Orchestrator manages jobs, runs, workflows, and server runtime operations.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			loaded, err := cliconfig.Load(opts.configPath)
			if err != nil {
				return err
			}

			resolved := cliconfig.Resolve(cliconfig.ResolveInput{
				Flags: map[string]string{
					"server":  opts.serverURL,
					"api-key": opts.apiKey,
					"project": opts.projectID,
					"format":  opts.outputFormat,
					"context": opts.contextName,
				},
				BoolFlags: map[string]bool{
					"no-color": opts.noColor,
					"quiet":    opts.quiet,
					"verbose":  opts.verbose,
				},
				DurationFlags: map[string]string{
					"timeout": opts.timeout.String(),
				},
				Changed: map[string]bool{
					"server":   cmd.Flags().Changed("server"),
					"api-key":  cmd.Flags().Changed("api-key"),
					"project":  cmd.Flags().Changed("project"),
					"format":   cmd.Flags().Changed("format"),
					"context":  cmd.Flags().Changed("context"),
					"no-color": cmd.Flags().Changed("no-color"),
					"quiet":    cmd.Flags().Changed("quiet"),
					"verbose":  cmd.Flags().Changed("verbose"),
					"timeout":  cmd.Flags().Changed("timeout"),
				},
				Config:          loaded.Data,
				Env:             cliEnv(),
				ContextOverride: opts.contextName,
			})

			if resolved.Credential == "" {
				if key, keyErr := cliauth.LoadAPIKey(resolved.ContextName); keyErr == nil {
					resolved.Credential = key
				}
			}

			timeout, parseErr := time.ParseDuration(resolved.Timeout)
			if parseErr != nil {
				return fmt.Errorf("invalid timeout %q: %w", resolved.Timeout, parseErr)
			}

			opts.serverURL = resolved.ServerURL
			opts.apiKey = resolved.Credential
			opts.projectID = resolved.ProjectID
			opts.outputFormat = resolved.Format
			opts.contextName = resolved.ContextName
			opts.noColor = resolved.NoColor
			opts.quiet = resolved.Quiet
			opts.verbose = resolved.Verbose
			opts.timeout = timeout
			opts.configPath = loaded.Path

			state.configPath = loaded.Path
			state.config = loaded.Data
			state.resolved = resolved

			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe("")
		},
	}

	cmd.PersistentFlags().StringVar(&opts.serverURL, "server", "", "server URL")
	cmd.PersistentFlags().StringVar(&opts.apiKey, "api-key", "", "API key")
	cmd.PersistentFlags().StringVar(&opts.projectID, "project", "", "default project ID")
	cmd.PersistentFlags().StringVarP(&opts.outputFormat, "format", "o", "", "output format")
	cmd.PersistentFlags().BoolVar(&opts.noColor, "no-color", false, "disable color output")
	cmd.PersistentFlags().BoolVarP(&opts.quiet, "quiet", "q", false, "minimal output")
	cmd.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "verbose output")
	cmd.PersistentFlags().StringVar(&opts.contextName, "context", "", "context name override")
	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "config file path")
	cmd.PersistentFlags().DurationVar(&opts.timeout, "timeout", 30*time.Second, "API request timeout")

	cmd.AddCommand(newServeCommand())
	cmd.AddCommand(newServerCommand())
	cmd.AddCommand(newDevCommand())
	cmd.AddCommand(newInitCommand())
	cmd.AddCommand(newVersionCommand(state))
	cmd.AddCommand(newCompletionCommand(cmd))
	cmd.AddCommand(newContextCommand(state))
	cmd.AddCommand(newAliasCommand(state))
	cmd.AddCommand(newLoginCommand(state))
	cmd.AddCommand(newLogoutCommand(state))
	cmd.AddCommand(newAuthCommand(state))
	cmd.AddCommand(newJobsCommand(state))
	cmd.AddCommand(newRunsCommand(state))
	cmd.AddCommand(newMigrateCommand(state))
	cmd.AddCommand(newTriggerCommand(state))
	cmd.AddCommand(newHealthCommand(state))
	cmd.AddCommand(newWorkflowsCommand(state))
	cmd.AddCommand(newWorkflowRunsCommand(state))
	cmd.AddCommand(newAPIKeysCommand(state))
	cmd.AddCommand(newStatsCommand(state))
	cmd.AddCommand(newAPICommand(state))
	cmd.AddCommand(newWaitCommand(state))
	cmd.AddCommand(newDocsCommand(cmd))
	cmd.AddCommand(newLogsCommand(state))
	cmd.AddCommand(newEventsCommand(state))
	cmd.AddCommand(newVerifyCommand(state))
	cmd.AddCommand(newDiagnoseCommand(state))
	cmd.AddCommand(newTopCommand(state))
	cmd.AddCommand(newValidateCommand(state))
	cmd.AddCommand(newApplyCommand(state))
	cmd.AddCommand(newDiffCommand(state))
	cmd.AddCommand(newExportCommand(state))
	cmd.AddCommand(newDBCommand())
	cmd.AddCommand(newRunCommand(state))

	rawArgs := os.Args[1:]
	configPath := extractConfigPath(rawArgs)
	rawArgs = expandAliasArgs(rawArgs, configPath)
	cmd.SetArgs(normalizeLegacyArgs(rawArgs))

	return cmd
}

func normalizeLegacyArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}

	subcommands := map[string]struct{}{
		"serve":         {},
		"server":        {},
		"dev":           {},
		"init":          {},
		"version":       {},
		"completion":    {},
		"context":       {},
		"alias":         {},
		"auth":          {},
		"login":         {},
		"logout":        {},
		"jobs":          {},
		"runs":          {},
		"migrate":       {},
		"trigger":       {},
		"health":        {},
		"workflows":     {},
		"workflow-runs": {},
		"api-keys":      {},
		"stats":         {},
		"api":           {},
		"wait":          {},
		"docs":          {},
		"logs":          {},
		"events":        {},
		"verify":        {},
		"diagnose":      {},
		"top":           {},
		"validate":      {},
		"apply":         {},
		"diff":          {},
		"export":        {},
		"db":            {},
		"run":           {},
		"help":          {},
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

func extractConfigPath(args []string) string {
	for i := range len(args) {
		if args[i] == "--config" && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}
		if value, ok := strings.CutPrefix(args[i], "--config="); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
		Short: "Start orchestrator server components",
		Long:  "Starts orchestrator runtime in api, worker, or all mode.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe(mode)
		},
	}

	cmd.Flags().StringVar(&mode, "mode", "", "run mode: api, worker, or all (overrides MODE env)")

	return cmd
}

func newVersionCommand(state *appState) *cobra.Command {
	var short bool
	var asJSON bool
	var checkServer bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print CLI version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			if short {
				fmt.Println(version)
				return nil
			}

			info := map[string]string{
				"version": version,
				"go":      runtime.Version(),
				"os_arch": fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
			}

			if checkServer {
				serverStatus := "unreachable"
				client := &http.Client{Timeout: state.opts.timeout}
				resp, err := client.Get(strings.TrimRight(state.opts.serverURL, "/") + "/health")
				if err == nil {
					_ = resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						serverStatus = "reachable"
					} else {
						serverStatus = fmt.Sprintf("http_%d", resp.StatusCode)
					}
				}
				info["server"] = serverStatus
			}

			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(info)
			}

			fmt.Printf("version: %s\n", info["version"])
			fmt.Printf("go: %s\n", info["go"])
			fmt.Printf("os/arch: %s\n", info["os_arch"])
			if checkServer {
				fmt.Printf("server: %s\n", info["server"])
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "print only the version number")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print version information as JSON")
	cmd.Flags().BoolVar(&checkServer, "check-server", false, "check configured server health endpoint")

	return cmd
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion scripts",
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(os.Stdout)
			case "zsh":
				return root.GenZshCompletion(os.Stdout)
			case "fish":
				return root.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}

	return cmd
}

func cliEnv() map[string]string {
	return map[string]string{
		"ORCHESTRATOR_SERVER":  strings.TrimSpace(os.Getenv("ORCHESTRATOR_SERVER")),
		"ORCHESTRATOR_API_KEY": strings.TrimSpace(os.Getenv("ORCHESTRATOR_API_KEY")),
		"ORCHESTRATOR_PROJECT": strings.TrimSpace(os.Getenv("ORCHESTRATOR_PROJECT")),
		"ORCHESTRATOR_FORMAT":  strings.TrimSpace(os.Getenv("ORCHESTRATOR_FORMAT")),
		"ORCHESTRATOR_CONTEXT": strings.TrimSpace(os.Getenv("ORCHESTRATOR_CONTEXT")),
		"NO_COLOR":             strings.TrimSpace(os.Getenv("NO_COLOR")),
	}
}
