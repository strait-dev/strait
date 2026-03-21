package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"strait/internal/cli/devtest"
	climanifest "strait/internal/cli/manifest"
	"strait/internal/cli/tunnel"

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

			// Validate required environment variables. Never silently default secrets.
			required := map[string]string{
				"DATABASE_URL":    "connection string for PostgreSQL (e.g. postgres://strait:strait@localhost:5432/strait?sslmode=disable)",
				"INTERNAL_SECRET": "shared secret for internal API auth (at least 16 characters)",
				"JWT_SIGNING_KEY": "signing key for JWT tokens (at least 32 characters)",
			}
			var missing []string
			for key, desc := range required {
				if os.Getenv(key) == "" {
					missing = append(missing, fmt.Sprintf("  %s - %s", key, desc))
				}
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				return fmt.Errorf("required environment variables are not set:\n%s\n\nCreate a .env file or export them before running strait dev", strings.Join(missing, "\n"))
			}

			// Optional: REDIS_URL (degrades gracefully without it)
			if os.Getenv("REDIS_URL") == "" {
				fmt.Fprintln(os.Stderr, "note: REDIS_URL not set; rate limiting and pub/sub will be disabled")
			}

			// Override only non-secret runtime settings for dev mode.
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
	cmd.AddCommand(newDevTunnelCommand(state))

	cmd.Flags().BoolVar(&noDocker, "no-docker", false, "skip docker compose startup")
	cmd.Flags().IntVar(&port, "port", 8080, "API port for dev mode")
	cmd.Flags().BoolVar(&seed, "seed", false, "attempt to seed example data")

	return cmd
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

func newDevTunnelCommand(state *appState) *cobra.Command {
	var (
		port     int
		job      string
		noUpdate bool
	)

	cmd := &cobra.Command{
		Use:   "tunnel",
		Short: "Create a Cloudflare Quick Tunnel to expose a local port",
		Long: `Creates a Cloudflare Quick Tunnel using cloudflared to expose a local port
to the internet. Useful for testing webhooks and job endpoints during development.

Requires cloudflared to be installed (offers to download if missing).`,
		Example: `  strait dev tunnel --port 8080
  strait dev tunnel --port 3000 --job process-payment
  strait dev tunnel --port 8080 --no-update`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Detect cloudflared binary.
			cfPath := tunnel.DetectCloudflared()
			if cfPath == "" {
				cachePath := tunnel.CachePath()
				// Check cached download location.
				if info, err := os.Stat(cachePath); err == nil && !info.IsDir() {
					cfPath = cachePath
				}
			}

			if cfPath == "" {
				downloadURL, urlErr := tunnel.DownloadURL()
				if urlErr != nil {
					return fmt.Errorf("cloudflared not found and cannot determine download URL: %w", urlErr)
				}
				return fmt.Errorf("cloudflared not found; install it from %s or add it to your PATH", downloadURL)
			}

			// Start cloudflared quick tunnel.
			args := []string{"tunnel", "--url", fmt.Sprintf("http://localhost:%d", port)}
			cfCmd := exec.CommandContext(cmd.Context(), cfPath, args...) //nolint:gosec // cfPath is from LookPath or user-controlled --cloudflared flag

			// Capture stderr where cloudflared prints the tunnel URL.
			cfCmd.Stdout = os.Stdout
			cfCmd.Stderr = os.Stderr

			if err := cfCmd.Start(); err != nil {
				return fmt.Errorf("start cloudflared: %w", err)
			}

			fmt.Fprintf(os.Stderr, "cloudflared started, tunneling localhost:%d\n", port)

			if !noUpdate && job != "" {
				fmt.Fprintf(os.Stderr, "job filter: %s\n", job)
			}

			// Print helpful output.
			result := map[string]any{
				"port":        port,
				"cloudflared": cfPath,
				"status":      "running",
			}
			if job != "" {
				result["job_filter"] = job
			}
			if noUpdate {
				result["update_endpoints"] = false
			}

			if err := printData(state, result); err != nil {
				return err
			}

			// Wait for interrupt.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			select {
			case sig := <-sigCh:
				fmt.Fprintf(os.Stderr, "received %s, stopping tunnel\n", sig)
			case <-cmd.Context().Done():
				fmt.Fprintln(os.Stderr, "context cancelled, stopping tunnel")
			}

			if cfCmd.Process != nil {
				_ = cfCmd.Process.Signal(syscall.SIGTERM)
				_ = cfCmd.Wait()
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "local port to tunnel (required)")
	cmd.Flags().StringVar(&job, "job", "", "filter to specific job slug")
	cmd.Flags().BoolVar(&noUpdate, "no-update", false, "create tunnel without updating job endpoints")

	_ = cmd.MarkFlagRequired("port")

	return cmd
}
