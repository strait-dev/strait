package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestWorkerShutdownTelemetryLogsContainExpectedFields(t *testing.T) {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	startedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	logWorkerShutdownStart(logger, startedAt, 3, 15*time.Second)
	logWorkerShutdownComplete(logger, nil, startedAt.Add(4*time.Second), 2, "graceful", nil)

	logs := buf.String()
	for _, field := range []string{
		"shutdown_started_at",
		"in_flight_runs",
		"drain_timeout",
		"shutdown_completed_at",
		"runs_drained",
	} {
		if !strings.Contains(logs, field) {
			t.Fatalf("expected logs to contain field %q, got: %s", field, logs)
		}
	}
}

func TestShutdownReason(t *testing.T) {
	t.Helper()

	if got := shutdownReason(nil); got != "graceful" {
		t.Fatalf("shutdownReason(nil) = %q, want graceful", got)
	}
	if got := shutdownReason(context.DeadlineExceeded); got != "timeout" {
		t.Fatalf("shutdownReason(DeadlineExceeded) = %q, want timeout", got)
	}
	if got := shutdownReason(errors.New("forced")); got != "forced" {
		t.Fatalf("shutdownReason(other error) = %q, want forced", got)
	}
}
