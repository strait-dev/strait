package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"strait/internal/cli/devtest"
	climanifest "strait/internal/cli/manifest"

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
		RunE: func(cmd *cobra.Command, _ []string) error {
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

			return runServe(cmd.Context(), "all")
		},
	}

	cmd.AddCommand(newDevStatusCommand(state))
	cmd.AddCommand(newDevTestCommand(state))

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

func newDevTestCommand(state *appState) *cobra.Command {
	var (
		payload     string
		payloadFile string
		endpoint    string
		configPath  string
		all         bool
		timeout     time.Duration
	)

	cmd := &cobra.Command{
		Use:   "test [job-slug]",
		Short: "Test a job locally by calling its endpoint",
		Long: `Test a job by making a direct HTTP POST to its endpoint URL.

Simulates the Strait job execution request format with proper headers.
Does not require a running Strait server — just calls the endpoint directly.`,
		Example: `  strait dev test process-payment --payload '{"id": "123"}'
  strait dev test process-payment --payload-file test.json
  strait dev test process-payment --endpoint http://localhost:3000/jobs/payment
  strait dev test --all --config strait.config.json
  echo '{"id":"1"}' | strait dev test process-payment`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var payloadBytes json.RawMessage

			switch {
			case payload != "":
				payloadBytes = json.RawMessage(payload)
			case payloadFile != "":
				content, err := os.ReadFile(payloadFile) //nolint:gosec // User-provided CLI flag path
				if err != nil {
					return fmt.Errorf("read payload file: %w", err)
				}
				payloadBytes = content
			case !stdoutIsTTY():
				content, err := readStdinIfAvailable()
				if err == nil && len(content) > 0 {
					payloadBytes = content
				}
			}

			if all {
				return runAllTests(cmd, state, configPath, payloadBytes, endpoint, timeout)
			}

			if len(args) == 0 {
				return fmt.Errorf("job slug is required (or use --all)")
			}

			jobSlug := args[0]
			endpointURL := endpoint
			if endpointURL == "" {
				endpointURL = resolveEndpointFromConfig(configPath, jobSlug)
			}
			if endpointURL == "" {
				cli, err := newAPIClient(state)
				if err == nil {
					projectID, projErr := requireProjectID(state, "")
					if projErr == nil {
						jobs, listErr := cli.ListJobs(cmd.Context(), projectID)
						if listErr == nil {
							for _, j := range jobs {
								if j.Slug == jobSlug {
									endpointURL = j.EndpointURL
									break
								}
							}
						}
					}
				}
			}

			result, err := devtest.RunTest(cmd.Context(), devtest.TestRequest{
				JobSlug:     jobSlug,
				EndpointURL: endpointURL,
				Payload:     payloadBytes,
				Timeout:     timeout,
			})
			if err != nil {
				return err
			}

			return printData(state, result)
		},
	}

	cmd.Flags().StringVar(&payload, "payload", "", "JSON payload")
	cmd.Flags().StringVar(&payloadFile, "payload-file", "", "path to JSON payload file")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "endpoint URL override")
	cmd.Flags().StringVar(&configPath, "config", "", "path to strait config file")
	cmd.Flags().BoolVar(&all, "all", false, "test all jobs from config")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "HTTP request timeout")

	return cmd
}

func runAllTests(cmd *cobra.Command, state *appState, configPath string, payload json.RawMessage, endpoint string, timeout time.Duration) error {
	if configPath == "" {
		configPath = climanifest.FindConfigFile(".")
	}
	if configPath == "" {
		return fmt.Errorf("--config is required for --all (or have a strait config file in current directory)")
	}

	cfg, err := climanifest.LoadProjectConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var results []any
	for _, job := range cfg.Jobs {
		endpointURL := endpoint
		if endpointURL == "" {
			endpointURL = job.EndpointURL
		}

		result, testErr := devtest.RunTest(cmd.Context(), devtest.TestRequest{
			JobSlug:     job.Slug,
			EndpointURL: endpointURL,
			Payload:     payload,
			Timeout:     timeout,
		})
		if testErr != nil {
			return testErr
		}
		results = append(results, result)
	}

	return printData(state, results)
}

func resolveEndpointFromConfig(configPath, jobSlug string) string {
	if configPath == "" {
		configPath = climanifest.FindConfigFile(".")
	}
	if configPath == "" {
		return ""
	}

	cfg, err := climanifest.LoadProjectConfig(configPath)
	if err != nil {
		return ""
	}

	for _, job := range cfg.Jobs {
		if job.Slug == jobSlug {
			return job.EndpointURL
		}
	}
	return ""
}

func readStdinIfAvailable() ([]byte, error) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeNamedPipe == 0 {
		return nil, nil
	}
	return os.ReadFile("/dev/stdin")
}
