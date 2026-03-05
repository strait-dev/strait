package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

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

func newRootCommand() *cobra.Command {
	opts := &rootOptions{}
	cmd := &cobra.Command{
		Use:           "orchestrator",
		Short:         "Orchestrator CLI and server runtime",
		Long:          "Orchestrator manages jobs, runs, workflows, and server runtime operations.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe("")
		},
	}

	cmd.PersistentFlags().StringVar(&opts.serverURL, "server", "http://localhost:8080", "server URL")
	cmd.PersistentFlags().StringVar(&opts.apiKey, "api-key", "", "API key")
	cmd.PersistentFlags().StringVar(&opts.projectID, "project", "", "default project ID")
	cmd.PersistentFlags().StringVarP(&opts.outputFormat, "format", "o", "table", "output format")
	cmd.PersistentFlags().BoolVar(&opts.noColor, "no-color", false, "disable color output")
	cmd.PersistentFlags().BoolVarP(&opts.quiet, "quiet", "q", false, "minimal output")
	cmd.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "verbose output")
	cmd.PersistentFlags().StringVar(&opts.contextName, "context", "", "context name override")
	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "config file path")
	cmd.PersistentFlags().DurationVar(&opts.timeout, "timeout", 30*time.Second, "API request timeout")

	cmd.AddCommand(newServeCommand())
	cmd.AddCommand(newVersionCommand(opts))
	cmd.AddCommand(newCompletionCommand(cmd))

	cmd.SetArgs(normalizeLegacyArgs(os.Args[1:]))

	return cmd
}

func normalizeLegacyArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}

	subcommands := map[string]struct{}{
		"serve":      {},
		"version":    {},
		"completion": {},
		"help":       {},
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
		Short: "Start orchestrator server components",
		Long:  "Starts orchestrator runtime in api, worker, or all mode.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe(mode)
		},
	}

	cmd.Flags().StringVar(&mode, "mode", "", "run mode: api, worker, or all (overrides MODE env)")

	return cmd
}

func newVersionCommand(opts *rootOptions) *cobra.Command {
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
				client := &http.Client{Timeout: opts.timeout}
				resp, err := client.Get(strings.TrimRight(opts.serverURL, "/") + "/health")
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
