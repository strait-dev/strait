package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"strait/internal/billing"
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

func TestIsPrivateRegistryHost(t *testing.T) {
	t.Parallel()

	blocked := []string{
		"localhost",
		"LOCALHOST",
		"localhost:5000",
		"127.0.0.1",
		"127.0.0.1:5000",
		"::1",
		"0.0.0.0",
		"10.0.0.1",
		"10.0.0.1:5000",
		"192.168.1.1",
		"172.16.0.1",
		"169.254.0.1", // link-local
	}
	for _, host := range blocked {
		if !isPrivateRegistryHost(host) {
			t.Errorf("isPrivateRegistryHost(%q) = false, want true (should be blocked)", host)
		}
	}

	allowed := []string{
		"ghcr.io",
		"ghcr.io:443",
		"registry.example.com",
		"123456789.dkr.ecr.us-east-1.amazonaws.com",
		"gcr.io",
		"docker.io",
	}
	for _, host := range allowed {
		if isPrivateRegistryHost(host) {
			t.Errorf("isPrivateRegistryHost(%q) = true, want false (legitimate public registry)", host)
		}
	}
}

func TestNotificationWorkerEnabled(t *testing.T) {
	t.Helper()

	tests := []struct {
		mode string
		want bool
	}{
		{mode: "api", want: false},
		{mode: "worker", want: true},
		{mode: "all", want: true},
		{mode: "", want: false},
	}

	for _, tt := range tests {
		if got := notificationWorkerEnabled(tt.mode); got != tt.want {
			t.Fatalf("notificationWorkerEnabled(%q) = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestWrapUsageService(t *testing.T) {
	t.Helper()

	var nilSvc *billing.UsageService
	if got := wrapUsageService(nilSvc); got != nil {
		t.Fatalf("wrapUsageService(nil) = %v, want nil", got)
	}

	wrapped := wrapUsageService(&billing.UsageService{})
	if wrapped == nil {
		t.Fatal("wrapUsageService(non-nil) = nil, want non-nil")
	}
}
