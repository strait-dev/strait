package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/domain"

	"github.com/spf13/cobra"
)

func newWaitCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wait",
		Short: "Wait for conditions",
	}

	cmd.AddCommand(newWaitRunCommand(state))

	return cmd
}

func newWaitRunCommand(state *appState) *cobra.Command {
	var condition string
	var timeout time.Duration
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "run <run-id>",
		Short: "Wait for a run condition",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			expected, err := parseWaitCondition(condition)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			deadline := time.Now().Add(timeout)
			for {
				run, getErr := cli.GetRun(context.Background(), args[0])
				if getErr != nil {
					return getErr
				}

				if run.Status == expected {
					return printData(state, map[string]any{
						"id":     run.ID,
						"status": run.Status,
						"waited": true,
					})
				}

				if time.Now().After(deadline) {
					return fmt.Errorf("timeout waiting for run %s to reach status %s", run.ID, expected)
				}

				time.Sleep(interval)
			}
		},
	}

	cmd.Flags().StringVar(&condition, "for", "status=completed", "condition expression, e.g. status=completed")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "max wait duration")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "poll interval")

	return cmd
}

func parseWaitCondition(raw string) (domain.RunStatus, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), "=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) != "status" {
		return "", fmt.Errorf("unsupported condition %q", raw)
	}
	status := domain.RunStatus(strings.TrimSpace(parts[1]))
	switch status {
	case domain.StatusDelayed, domain.StatusQueued, domain.StatusDequeued, domain.StatusExecuting, domain.StatusWaiting,
		domain.StatusCompleted, domain.StatusFailed, domain.StatusTimedOut, domain.StatusCrashed, domain.StatusSystemFailed,
		domain.StatusCanceled, domain.StatusExpired:
		return status, nil
	default:
		return "", fmt.Errorf("unsupported run status %q", status)
	}
}
