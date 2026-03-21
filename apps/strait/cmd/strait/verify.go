package main

import (
	"fmt"
	"os"

	"strait/internal/cli/client"
	"strait/internal/cli/styles"

	"github.com/spf13/cobra"
)

func newVerifyCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Run post-deployment verification checks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			checks := make([]map[string]any, 0, 4)

			health, err := cli.Health(cmd.Context())
			checks = append(checks, map[string]any{"check": "health", "ok": err == nil, "detail": healthDetail(health, err)})

			ready, err := cli.HealthReady(cmd.Context())
			checks = append(checks, map[string]any{"check": "readiness", "ok": err == nil, "detail": healthDetail(ready, err)})

			stats, err := cli.Stats(cmd.Context())
			checks = append(checks, map[string]any{"check": "stats", "ok": err == nil, "detail": statsDetail(stats, err)})

			if state.opts.projectID != "" {
				_, err = cli.ListJobs(cmd.Context(), state.opts.projectID)
				checks = append(checks, map[string]any{"check": "auth/jobs list", "ok": err == nil, "detail": errDetail(err)})
			}

			if stdoutIsTTY() && state.opts.outputFormat == "" {
				for _, c := range checks {
					name, _ := c["check"].(string)
					ok, _ := c["ok"].(bool)
					detail, _ := c["detail"].(string)
					if ok {
						fmt.Fprintf(os.Stderr, "  %s %s: %s\n", styles.StatusBadge("ok"), name, detail)
					} else {
						fmt.Fprintf(os.Stderr, "  %s %s: %s\n", styles.StatusBadge("fail"), name, detail)
					}
				}
			} else if err := printData(state, checks); err != nil {
				return err
			}

			for _, check := range checks {
				if ok, _ := check["ok"].(bool); !ok {
					return fmt.Errorf("verify failed")
				}
			}

			return nil
		},
	}

	return cmd
}

func healthDetail(status *client.HealthStatus, err error) string {
	if err != nil {
		return err.Error()
	}
	return status.Status
}

func statsDetail(stats *client.QueueStats, err error) string {
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("queued=%d executing=%d delayed=%d", stats.Queued, stats.Executing, stats.Delayed)
}

func errDetail(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}
