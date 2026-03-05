package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newDiagnoseCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Run troubleshooting diagnostics",
		RunE: func(_ *cobra.Command, _ []string) error {
			checks := make([]map[string]any, 0, 6)

			checks = append(checks, map[string]any{"check": "server configured", "ok": state.opts.serverURL != "", "detail": state.opts.serverURL})
			checks = append(checks, map[string]any{"check": "api key present", "ok": state.opts.apiKey != "", "detail": boolString(state.opts.apiKey != "")})
			checks = append(checks, map[string]any{"check": "project set", "ok": state.opts.projectID != "", "detail": state.opts.projectID})

			databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
			checks = append(checks, map[string]any{"check": "DATABASE_URL", "ok": databaseURL != "", "detail": boolString(databaseURL != "")})

			redisURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
			checks = append(checks, map[string]any{"check": "REDIS_URL", "ok": redisURL != "", "detail": boolString(redisURL != "")})

			cli, err := newAPIClient(state)
			if err == nil {
				health, hErr := cli.Health(context.Background())
				checks = append(checks, map[string]any{"check": "health", "ok": hErr == nil, "detail": healthDetail(health, hErr)})
				stats, sErr := cli.Stats(context.Background())
				checks = append(checks, map[string]any{"check": "stats", "ok": sErr == nil, "detail": statsDetail(stats, sErr)})
			}

			if host, port, splitErr := splitHostPortFromURL(state.opts.serverURL); splitErr == nil {
				target := net.JoinHostPort(host, port)
				conn, dialErr := net.DialTimeout("tcp", target, 2*time.Second)
				if dialErr == nil {
					_ = conn.Close()
				}
				checks = append(checks, map[string]any{"check": "tcp connectivity", "ok": dialErr == nil, "detail": errDetail(dialErr)})
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

	return cmd
}

func splitHostPortFromURL(serverURL string) (string, string, error) {
	trimmed := strings.TrimSpace(serverURL)
	if trimmed == "" {
		return "", "", fmt.Errorf("empty server URL")
	}
	trimmed = strings.TrimPrefix(trimmed, "http://")
	trimmed = strings.TrimPrefix(trimmed, "https://")
	parts := strings.Split(trimmed, "/")
	hostPort := parts[0]
	host, port, err := net.SplitHostPort(hostPort)
	if err == nil {
		return host, port, nil
	}
	if strings.Contains(err.Error(), "missing port in address") {
		return hostPort, "80", nil
	}
	return "", "", err
}

func boolString(v bool) string {
	if v {
		return "set"
	}
	return "missing"
}
