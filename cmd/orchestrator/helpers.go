package main

import (
	"fmt"
	"os"

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
