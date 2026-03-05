package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
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

	cmd.AddCommand(newDiagnoseRunCommand(state))

	cmd.Flags().BoolVar(&verbose, "verbose", false, "show full diagnostics context")
	cmd.Flags().BoolVar(&includeReadiness, "check-readiness", false, "include readiness check")

	return cmd
}

func splitHostPortFromURL(serverURL string) (string, string, error) {
	trimmed := strings.TrimSpace(serverURL)
	if trimmed == "" {
		return "", "", fmt.Errorf("empty server URL")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
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

func newDiagnoseRunCommand(state *appState) *cobra.Command {
	var follow bool
	var interval time.Duration
	var level string
	var eventType string
	var showPayload bool
	var showResult bool
	var eventLimit int

	cmd := &cobra.Command{
		Use:     "run <run-id>",
		Short:   "Diagnose a specific run",
		Long:    "Analyzes run state, timeline, and likely remediations for a single run.",
		Example: "orchestrator diagnose run run_123\n  orchestrator diagnose run run_123 --follow\n  orchestrator diagnose run run_123 --level error",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if interval <= 0 {
				return fmt.Errorf("interval must be greater than zero")
			}
			if eventLimit <= 0 {
				return fmt.Errorf("event-limit must be greater than zero")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			runID := args[0]
			seen := map[string]struct{}{}

			for {
				run, err := cli.GetRun(context.Background(), runID)
				if err != nil {
					return err
				}

				jobDetails := map[string]any{"id": run.JobID}
				if job, jobErr := cli.GetJob(context.Background(), run.JobID); jobErr == nil {
					jobDetails["name"] = job.Name
					jobDetails["slug"] = job.Slug
					jobDetails["enabled"] = job.Enabled
					jobDetails["timeout_secs"] = job.TimeoutSecs
					jobDetails["max_attempts"] = job.MaxAttempts
				}

				events, err := cli.ListRunEvents(context.Background(), runID, level, eventType)
				if err != nil {
					return err
				}

				sort.Slice(events, func(i, j int) bool {
					return events[i].CreatedAt.Before(events[j].CreatedAt)
				})
				if len(events) > eventLimit {
					events = events[len(events)-eventLimit:]
				}

				timeline := make([]map[string]any, 0, len(events))
				for _, event := range events {
					timeline = append(timeline, map[string]any{
						"id":         event.ID,
						"timestamp":  event.CreatedAt,
						"type":       event.Type,
						"level":      event.Level,
						"message":    event.Message,
						"first_seen": hasNotSeen(seen, event.ID),
					})
					seen[event.ID] = struct{}{}
				}

				status := string(run.Status)
				remediation := make([]map[string]any, 0, 4)
				switch run.Status {
				case "failed", "crashed", "system_failed":
					remediation = append(remediation, map[string]any{"issue": "run failed", "action": "inspect recent error events and endpoint behavior"})
					if strings.TrimSpace(run.Error) != "" {
						remediation = append(remediation, map[string]any{"issue": "last error", "action": run.Error})
					}
				case "timed_out":
					remediation = append(remediation, map[string]any{"issue": "run timed out", "action": "increase job timeout or reduce endpoint latency"})
				case "queued", "delayed", "dequeued", "waiting":
					remediation = append(remediation, map[string]any{"issue": "run not started", "action": "check worker availability and queue pressure with `orchestrator top queue`"})
				case "executing":
					remediation = append(remediation, map[string]any{"issue": "run in progress", "action": "follow events with `orchestrator runs logs " + runID + " --follow`"})
				}

				payload := map[string]any{
					"run": map[string]any{
						"id":            run.ID,
						"status":        status,
						"attempt":       run.Attempt,
						"triggered_by":  run.TriggeredBy,
						"created_at":    run.CreatedAt,
						"started_at":    run.StartedAt,
						"finished_at":   run.FinishedAt,
						"next_retry_at": run.NextRetryAt,
						"error":         run.Error,
					},
					"job":         jobDetails,
					"timeline":    timeline,
					"remediation": remediation,
				}
				if showPayload {
					payload["payload"] = run.Payload
				}
				if showResult {
					payload["result"] = run.Result
				}

				if err := printData(state, payload); err != nil {
					return err
				}

				if !follow || run.Status.IsTerminal() {
					if run.Status == "completed" || !run.Status.IsTerminal() {
						return nil
					}
					return fmt.Errorf("run is in terminal status %q", run.Status)
				}

				time.Sleep(interval)
			}
		},
	}

	cmd.Flags().BoolVar(&follow, "follow", false, "continuously refresh diagnostics until terminal state")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "poll interval when following")
	cmd.Flags().StringVar(&level, "level", "", "event level filter")
	cmd.Flags().StringVar(&eventType, "type", "", "event type filter")
	cmd.Flags().BoolVar(&showPayload, "show-payload", false, "include run payload in output")
	cmd.Flags().BoolVar(&showResult, "show-result", false, "include run result in output")
	cmd.Flags().IntVar(&eventLimit, "event-limit", 50, "maximum events to include in timeline")
	_ = cmd.RegisterFlagCompletionFunc("level", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"debug", "info", "warn", "error"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("type", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"log", "state_change", "error", "progress"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func hasNotSeen(seen map[string]struct{}, id string) bool {
	_, ok := seen[id]
	return !ok
}

func boolString(v bool) string {
	if v {
		return "set"
	}
	return "missing"
}
