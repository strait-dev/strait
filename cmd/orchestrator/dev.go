package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/spf13/cobra"
)

func newDevCommand(state *appState) *cobra.Command {
	var noDocker bool
	var port int
	var seed bool

	cmd := &cobra.Command{
		Use:     "dev",
		Short:   "Run local development mode",
		Long:    "Starts local orchestrator development runtime with optional Docker dependencies and sensible local defaults.",
		Example: "orchestrator dev\n  orchestrator dev --no-docker --port 9090\n  orchestrator dev --seed",
		RunE: func(_ *cobra.Command, _ []string) error {
			if !noDocker {
				if _, err := exec.LookPath("docker"); err != nil {
					return fmt.Errorf("docker is required for dev mode; install docker or rerun with --no-docker")
				}

				if err := exec.Command("docker", "compose", "version").Run(); err != nil {
					return fmt.Errorf("docker compose is required for dev mode; ensure docker compose is installed or rerun with --no-docker")
				}

				fmt.Fprintln(os.Stderr, "starting docker dependencies: postgres, redis")
				compose := exec.Command("docker", "compose", "up", "-d", "postgres", "redis")
				compose.Stdout = os.Stdout
				compose.Stderr = os.Stderr
				if err := compose.Run(); err != nil {
					return fmt.Errorf("start docker dependencies: %w", err)
				}
			}

			setEnvIfEmpty("DATABASE_URL", "postgres://orchestrator:orchestrator@localhost:5432/orchestrator?sslmode=disable")
			setEnvIfEmpty("REDIS_URL", "redis://localhost:6379")
			setEnvIfEmpty("INTERNAL_SECRET", "orchestrator-dev-internal-secret-0123456789")
			setEnvIfEmpty("JWT_SIGNING_KEY", "orchestrator-dev-jwt-signing-key-0123456789")
			_ = os.Setenv("LOG_LEVEL", "debug")
			_ = os.Setenv("PORT", strconv.Itoa(port))

			if err := printData(state, map[string]any{
				"mode":       "all",
				"docker":     !noDocker,
				"port":       port,
				"server_url": fmt.Sprintf("http://localhost:%d", port),
			}); err != nil {
				return err
			}

			if seed {
				fmt.Fprintln(os.Stderr, "seed flag noted: run `orchestrator fixtures create --template full` after server startup")
			}

			return runServe("all")
		},
	}

	cmd.Flags().BoolVar(&noDocker, "no-docker", false, "skip docker compose startup")
	cmd.Flags().IntVar(&port, "port", 8080, "API port for dev mode")
	cmd.Flags().BoolVar(&seed, "seed", false, "attempt to seed example data")

	return cmd
}

func setEnvIfEmpty(key, value string) {
	if os.Getenv(key) == "" {
		_ = os.Setenv(key, value)
	}
}
