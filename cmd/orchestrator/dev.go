package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/spf13/cobra"
)

func newDevCommand() *cobra.Command {
	var noDocker bool
	var port int
	var seed bool

	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Run local development mode",
		RunE: func(_ *cobra.Command, _ []string) error {
			if !noDocker {
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

			if seed {
				fmt.Fprintln(os.Stderr, "seed requested; no fixture loader is implemented yet, continuing without seeding")
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
