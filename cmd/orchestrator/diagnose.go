package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newDiagnoseCommand(state *appState) *cobra.Command {
	var verbose bool
	var includeReadiness bool

	cmd := &cobra.Command{
		Use:     "diagnose",
		Short:   "Run troubleshooting diagnostics",
		Long:    "Runs connectivity and configuration checks and reports fixes for failed checks.",
		Example: "orchestrator diagnose\n  orchestrator diagnose --verbose\n  orchestrator diagnose --check-readiness",
		RunE: func(_ *cobra.Command, _ []string) error {
			checks := make([]map[string]any, 0, 10)

			checks = append(checks, diagnoseCheck("server configured", state.opts.serverURL != "", state.opts.serverURL, "set --server or ORCHESTRATOR_SERVER"))
			checks = append(checks, diagnoseCheck("api key present", state.opts.apiKey != "", boolString(state.opts.apiKey != ""), "run `orchestrator login` or set ORCHESTRATOR_API_KEY"))
			checks = append(checks, diagnoseCheck("project set", state.opts.projectID != "", state.opts.projectID, "set --project or ORCHESTRATOR_PROJECT"))

			databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
			checks = append(checks, diagnoseCheck("DATABASE_URL", databaseURL != "", boolString(databaseURL != ""), "export DATABASE_URL=..."))

			redisURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
			checks = append(checks, diagnoseCheck("REDIS_URL", redisURL != "", boolString(redisURL != ""), "export REDIS_URL=..."))

			internalSecret := strings.TrimSpace(os.Getenv("INTERNAL_SECRET"))
			checks = append(checks, diagnoseCheck("INTERNAL_SECRET", internalSecret != "", boolString(internalSecret != ""), "export INTERNAL_SECRET=..."))

			jwtSigningKey := strings.TrimSpace(os.Getenv("JWT_SIGNING_KEY"))
			checks = append(checks, diagnoseCheck("JWT_SIGNING_KEY", jwtSigningKey != "", boolString(jwtSigningKey != ""), "export JWT_SIGNING_KEY=..."))

			cli, err := newAPIClient(state)
			if err == nil {
				health, hErr := cli.Health(context.Background())
				checks = append(checks, diagnoseCheck("health", hErr == nil, healthDetail(health, hErr), "verify server is running and reachable"))

				if includeReadiness {
					ready, rErr := cli.HealthReady(context.Background())
					checks = append(checks, diagnoseCheck("readiness", rErr == nil, healthDetail(ready, rErr), "verify database and redis dependencies are up"))
				}

				stats, sErr := cli.Stats(context.Background())
				checks = append(checks, diagnoseCheck("stats", sErr == nil, statsDetail(stats, sErr), "check server auth and /v1/stats availability"))
			} else {
				checks = append(checks, diagnoseCheck("api client", false, err.Error(), "check --server URL, API key, and timeout"))
			}

			if host, port, splitErr := splitHostPortFromURL(state.opts.serverURL); splitErr == nil {
				target := net.JoinHostPort(host, port)
				conn, dialErr := net.DialTimeout("tcp", target, 2*time.Second)
				if dialErr == nil {
					_ = conn.Close()
				}
				checks = append(checks, diagnoseCheck("tcp connectivity", dialErr == nil, errDetail(dialErr), "check DNS/network and server port reachability"))
			}

			if !verbose {
				trimmed := make([]map[string]any, 0, len(checks))
				for _, check := range checks {
					trimmed = append(trimmed, map[string]any{
						"check":  check["check"],
						"ok":     check["ok"],
						"detail": check["detail"],
						"fix":    check["fix"],
					})
				}
				checks = trimmed
			}

			if err := printData(state, checks); err != nil {
				return err
			}

			for _, item := range checks {
				if ok, _ := item["ok"].(bool); !ok {
					return fmt.Errorf("diagnose found failing checks")
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&verbose, "verbose", false, "show full diagnostics context")
	cmd.Flags().BoolVar(&includeReadiness, "check-readiness", false, "include readiness check")

	return cmd
}

func splitHostPortFromURL(serverURL string) (string, string, error) {
	trimmed := strings.TrimSpace(serverURL)
	if trimmed == "" {
		return "", "", fmt.Errorf("empty server URL")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", "", err
	}

	hostPort := parsed.Host
	host, port, err := net.SplitHostPort(hostPort)
	if err == nil {
		return host, port, nil
	}
	if strings.Contains(err.Error(), "missing port in address") {
		if parsed.Scheme == "https" {
			return hostPort, "443", nil
		}
		return hostPort, "80", nil
	}
	return "", "", err
}

func diagnoseCheck(name string, ok bool, detail, fix string) map[string]any {
	return map[string]any{
		"check":  name,
		"ok":     ok,
		"detail": detail,
		"fix":    fix,
	}
}

func boolString(v bool) string {
	if v {
		return "set"
	}
	return "missing"
}
