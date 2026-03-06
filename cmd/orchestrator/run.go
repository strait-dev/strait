package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func newRunCommand(state *appState) *cobra.Command {
	var contextName string

	cmd := &cobra.Command{
		Use:   "run -- <command> [args...]",
		Short: "Run local command with orchestrator context env vars",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("command is required")
			}

			targetContext := strings.TrimSpace(contextName)
			if targetContext == "" {
				targetContext = strings.TrimSpace(state.opts.contextName)
			}

			env := os.Environ()
			env = append(env,
				"ORCHESTRATOR_URL="+state.opts.serverURL,
				"ORCHESTRATOR_API_KEY="+state.opts.apiKey,
				"ORCHESTRATOR_PROJECT_ID="+state.opts.projectID,
			)
			if targetContext != "" {
				env = append(env, "ORCHESTRATOR_CONTEXT="+targetContext)
			}

			c := exec.Command(args[0], args[1:]...) //nolint:gosec
			c.Env = env
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}

	cmd.Flags().StringVar(&contextName, "context", "", "context override for env injection")

	return cmd
}
