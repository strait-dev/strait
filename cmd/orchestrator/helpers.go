package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"orchestrator/internal/cli/client"
	cliconfig "orchestrator/internal/cli/config"
)

func loadConfigForWrite(state *appState) (*cliconfig.File, string, error) {
	path := state.configPath
	if path == "" {
		loaded, err := cliconfig.Load("")
		if err != nil {
			return nil, "", err
		}
		path = loaded.Path
		state.config = loaded.Data
		state.configPath = loaded.Path
	}

	loaded, err := cliconfig.Load(path)
	if err != nil {
		return nil, "", err
	}
	if loaded.Data == nil {
		return nil, "", fmt.Errorf("unable to load config")
	}

	state.config = loaded.Data
	state.configPath = loaded.Path

	return loaded.Data, loaded.Path, nil
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func newAPIClient(state *appState) (*client.Client, error) {
	return client.New(state.opts.serverURL, state.opts.apiKey, state.opts.timeout)
}

// requireConfirmation checks CI mode and prompts interactively if needed.
// Pass yes=true when the user provided --yes flag.
func requireConfirmation(state *appState, msg string, yes bool) error {
	if yes {
		return nil
	}
	if state.opts.ciMode {
		return fmt.Errorf("interactive prompt blocked in CI mode; use --yes to confirm")
	}
	if !stdoutIsTTY() {
		return fmt.Errorf("non-interactive terminal detected; use --yes to confirm")
	}
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", msg)
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("operation cancelled")
	}
	return nil
}

// requireProjectID resolves the project ID from the flag value or appState default.
func requireProjectID(state *appState, flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if state.opts.projectID != "" {
		return state.opts.projectID, nil
	}
	return "", fmt.Errorf("project ID is required (use --project)")
}
