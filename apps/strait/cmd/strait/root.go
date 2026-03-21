package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	cliauth "strait/internal/cli/auth"
	cliconfig "strait/internal/cli/config"
	"strait/internal/cli/styles"

	"github.com/spf13/cobra"
)

type rootOptions struct {
	serverURL    string
	apiKey       string
	projectID    string
	outputFormat string
	noHeaders    bool
	outputTpl    string
	outputPath   string
	noColor      bool
	quiet        bool
	verbose      bool
	contextName  string
	configPath   string
	timeout      time.Duration
	ciMode       bool
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
		Use:           "strait",
		Short:         "Strait CLI and server runtime",
		Long:          "Strait manages jobs, runs, workflows, and server runtime operations.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			loaded, err := cliconfig.Load(opts.configPath)
			if err != nil {
				return err
			}

			if loaded.IsLocal && loaded.Exists {
				if fields := cliconfig.HasSensitiveLocalFields(loaded.Data); len(fields) > 0 {
					fmt.Fprintf(os.Stderr, "warning: local config %s overrides: %s\n", loaded.Path, strings.Join(fields, ", "))
				}
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

			if opts.ciMode || os.Getenv("STRAIT_CI") == "true" || os.Getenv("CI") == "true" {
				opts.ciMode = true
				opts.noColor = true
			}

			if opts.noColor {
				styles.ForceNoColor()
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), "")
		},
	}

	cmd.PersistentFlags().StringVar(&opts.serverURL, "server", "", "server URL")
	cmd.PersistentFlags().StringVar(&opts.apiKey, "api-key", "", "API key")
	cmd.PersistentFlags().StringVar(&opts.projectID, "project", "", "default project ID")
	cmd.PersistentFlags().StringVarP(&opts.outputFormat, "format", "o", "", "output format")
	cmd.PersistentFlags().BoolVar(&opts.noHeaders, "no-headers", false, "omit headers for table output")
	cmd.PersistentFlags().StringVar(&opts.outputTpl, "output-template", "", "go template for --format go-template")
	cmd.PersistentFlags().StringVar(&opts.outputPath, "output-jsonpath", "", "jsonpath for --format jsonpath")
	cmd.PersistentFlags().BoolVar(&opts.noColor, "no-color", false, "disable color output")
	cmd.PersistentFlags().BoolVarP(&opts.quiet, "quiet", "q", false, "minimal output")
	cmd.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "verbose output")
	cmd.PersistentFlags().StringVar(&opts.contextName, "context", "", "context name override")
	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "config file path")
	cmd.PersistentFlags().DurationVar(&opts.timeout, "timeout", 30*time.Second, "API request timeout")
	cmd.PersistentFlags().BoolVar(&opts.ciMode, "ci", false, "enable CI mode (no color, no prompts)")

	cmd.AddCommand(newServeCommand())
	cmd.AddCommand(newServerCommand())
	cmd.AddCommand(newDevCommand(state))
	cmd.AddCommand(newInitCommand(state))
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
	cmd.AddCommand(newCheckCommand(state))
	cmd.AddCommand(newCleanupCommand(state))
	cmd.AddCommand(newTopCommand(state))
	cmd.AddCommand(newTUICommand(state))
	cmd.AddCommand(newValidateCommand(state))
	cmd.AddCommand(newApplyCommand(state))
	cmd.AddCommand(newDiffCommand(state))
	cmd.AddCommand(newExportCommand(state))
	cmd.AddCommand(newDBCommand())
	cmd.AddCommand(newRunCommand(state))
	cmd.AddCommand(newSendCommand(state))
	cmd.AddCommand(newTriggersCommand(state))
	cmd.AddCommand(newSecretsCommand(state))
	cmd.AddCommand(newFixturesCommand(state))
	cmd.AddCommand(newExtensionCommand(state))
	cmd.AddCommand(newListenCommand(state))
	cmd.AddCommand(newDrainCommand(state))
	cmd.AddCommand(newTraceCommand(state))
	cmd.AddCommand(newUpgradeCommand())
	cmd.AddCommand(newBackupCommand(state))
	cmd.AddCommand(newProfileCommand(state))
	cmd.AddCommand(newDeployCommand(state))
	cmd.AddCommand(newProjectCommand(state))
	cmd.AddCommand(newBuildCommand(state))
	cmd.AddCommand(newDoctorCommand(state))
	cmd.AddCommand(newOpenCommand(state))
	cmd.AddCommand(newStatusCommand(state))
	cmd.AddCommand(newDebugCommand(state))
	cmd.AddCommand(newCreateCommand(state))
	cmd.AddCommand(newCICommand(state))
	cmd.AddCommand(newPerfCommand(state))
	cmd.AddCommand(newTeamCommand(state))
	cmd.AddCommand(newAuditCommand(state))

	rawArgs := os.Args[1:]
	configPath := extractConfigPath(rawArgs)
	rawArgs = expandAliasArgs(rawArgs, configPath)
	cmd.SetArgs(normalizeLegacyArgs(rawArgs))

	registerRootCompletions(cmd)

	return cmd
}

func registerRootCompletions(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"table", "json", "yaml", "csv", "wide", "go-template", "jsonpath"}, cobra.ShellCompDirectiveNoFileComp
	})

	_ = cmd.RegisterFlagCompletionFunc("context", func(c *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		configPath := ""
		if flag := c.Flag("config"); flag != nil {
			configPath = strings.TrimSpace(flag.Value.String())
		}
		loaded, err := cliconfig.Load(configPath)
		if err != nil || loaded == nil || loaded.Data == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		results := make([]string, 0, len(loaded.Data.Contexts))
		for name := range loaded.Data.Contexts {
			results = append(results, name)
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})
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
		"tui":           {},
		"validate":      {},
		"apply":         {},
		"diff":          {},
		"export":        {},
		"db":            {},
		"run":           {},
		"send":          {},
		"secrets":       {},
		"fixtures":      {},
		"help":          {},
		"check":         {},
		"cleanup":       {},
		"extension":     {},
		"listen":        {},
		"drain":         {},
		"trace":         {},
		"upgrade":       {},
		"backup":        {},
		"profile":       {},
		"deploy":        {},
		"project":       {},
		"build":         {},
		"doctor":        {},
		"open":          {},
		"status":        {},
		"debug":         {},
		"create":        {},
		"ci":            {},
		"perf":          {},
		"team":          {},
		"audit":         {},
		"triggers":      {},
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
		Short: "Start strait server components",
		Long:  "Starts strait runtime in api, worker, or all mode.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), mode)
		},
	}

	cmd.Flags().StringVar(&mode, "mode", "", "run mode: api, worker, or all (overrides MODE env)")

	return cmd
}

func newVersionCommand(state *appState) *cobra.Command {
	var short bool
	var asJSON bool
	var checkServer bool
	var checkUpdate bool

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
				"commit":  commit,
				"date":    date,
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
			fmt.Printf("commit: %s\n", info["commit"])
			fmt.Printf("date: %s\n", info["date"])
			fmt.Printf("go: %s\n", info["go"])
			fmt.Printf("os/arch: %s\n", info["os_arch"])
			if checkServer {
				fmt.Printf("server: %s\n", info["server"])
			}

			if checkUpdate {
				latest, cached := getCachedUpdate()
				if !cached {
					latest = checkForUpdate()
					if latest != "" {
						setCachedUpdate(latest)
					}
				}
				if latest != "" {
					current := strings.TrimPrefix(version, "v")
					if current == latest {
						fmt.Println("update: up to date")
					} else {
						fmt.Printf("update: v%s available (current: v%s)\n", latest, current)
					}
				} else {
					fmt.Println("update: check failed")
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "print only the version number")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print version information as JSON")
	cmd.Flags().BoolVar(&checkServer, "check-server", false, "check configured server health endpoint")
	cmd.Flags().BoolVar(&checkUpdate, "check-update", false, "check for newer CLI version")

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
		"STRAIT_SERVER":  strings.TrimSpace(os.Getenv("STRAIT_SERVER")),
		"STRAIT_API_KEY": strings.TrimSpace(os.Getenv("STRAIT_API_KEY")),
		"STRAIT_PROJECT": strings.TrimSpace(os.Getenv("STRAIT_PROJECT")),
		"STRAIT_FORMAT":  strings.TrimSpace(os.Getenv("STRAIT_FORMAT")),
		"STRAIT_CONTEXT": strings.TrimSpace(os.Getenv("STRAIT_CONTEXT")),
		"NO_COLOR":       strings.TrimSpace(os.Getenv("NO_COLOR")),
		"STRAIT_CI":      strings.TrimSpace(os.Getenv("STRAIT_CI")),
	}
}
