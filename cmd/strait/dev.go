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
		Long:    "Starts local strait development runtime with optional Docker dependencies and sensible local defaults.",
		Example: "strait dev\n  strait dev --no-docker --port 9090\n  strait dev --seed",
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

			setEnvIfEmpty("DATABASE_URL", "postgres://strait:strait@localhost:5432/strait?sslmode=disable")
			setEnvIfEmpty("REDIS_URL", "redis://localhost:6379")
			setEnvIfEmpty("INTERNAL_SECRET", "strait-dev-internal-secret-0123456789")
			setEnvIfEmpty("JWT_SIGNING_KEY", "strait-dev-jwt-signing-key-0123456789")
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
				fmt.Fprintln(os.Stderr, "seed flag noted: run `strait fixtures create --template full` after server startup")
			}

			return runServe("all")
		},
	}

	cmd.AddCommand(newDevStatusCommand(state))

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

func newDevStatusCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Show local dev readiness checks",
		Long:    "Runs local development readiness checks for docker tooling, env vars, and server reachability.",
		Example: "strait dev status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			checks := make([]map[string]any, 0, 8)

			_, dockerErr := exec.LookPath("docker")
			checks = append(checks, diagnoseCheck("docker binary", dockerErr == nil, errDetail(dockerErr), "install docker desktop or docker engine"))

			composeErr := exec.Command("docker", "compose", "version").Run()
			checks = append(checks, diagnoseCheck("docker compose", composeErr == nil, errDetail(composeErr), "install docker compose plugin"))

			databaseURL := os.Getenv("DATABASE_URL")
			checks = append(checks, diagnoseCheck("DATABASE_URL", databaseURL != "", boolString(databaseURL != ""), "set DATABASE_URL or run strait dev"))

			redisURL := os.Getenv("REDIS_URL")
			checks = append(checks, diagnoseCheck("REDIS_URL", redisURL != "", boolString(redisURL != ""), "set REDIS_URL or run strait dev"))

			internalSecret := os.Getenv("INTERNAL_SECRET")
			checks = append(checks, diagnoseCheck("INTERNAL_SECRET", internalSecret != "", boolString(internalSecret != ""), "set INTERNAL_SECRET for auth signing"))

			jwtSigningKey := os.Getenv("JWT_SIGNING_KEY")
			checks = append(checks, diagnoseCheck("JWT_SIGNING_KEY", jwtSigningKey != "", boolString(jwtSigningKey != ""), "set JWT_SIGNING_KEY for auth tokens"))

			cli, err := newAPIClient(state)
			if err == nil {
				health, healthErr := cli.Health(cmd.Context())
				checks = append(checks, diagnoseCheck("server health", healthErr == nil, healthDetail(health, healthErr), "start server with `strait dev`"))
			} else {
				checks = append(checks, diagnoseCheck("api client", false, err.Error(), "set valid --server and --api-key for status checks"))
			}

			if err := printData(state, checks); err != nil {
				return err
			}

			for _, check := range checks {
				if ok, _ := check["ok"].(bool); !ok {
					return fmt.Errorf("dev status found failing checks")
				}
			}

			return nil
		},
	}

	return cmd
}
