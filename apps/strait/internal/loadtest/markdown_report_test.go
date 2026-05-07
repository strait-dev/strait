//go:build loadtest

package loadtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteMarkdownReport_CreatesFileWithMetrics(t *testing.T) {
	dir := t.TempDir()
	finished := time.Date(2026, 4, 12, 10, 30, 0, 0, time.UTC)
	started := finished.Add(-15 * time.Minute)
	path, err := WriteMarkdownReport(dir, MarkdownSummary{
		Scenario:       "backpressure_ceiling",
		StartedAt:      started,
		FinishedAt:     finished,
		Tier:           2,
		Description:    "Rejection rate test",
		MaxThroughput:  1200,
		P99LatencyMS:   42.5,
		ErrorRate:      0.012,
		RejectionRate:  0.08,
		QueueDepthPeak: 2500,
		Notes:          []string{"circuit breaker stayed closed"},
	})
	if err != nil {
		t.Fatalf("WriteMarkdownReport: %v", err)
	}
	if !strings.HasSuffix(path, ".md") {
		t.Errorf("path does not end in .md: %s", path)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("wrong dir: %s", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(body)
	for _, want := range []string{
		"# Load test: backpressure_ceiling",
		"| Max throughput (jobs/sec) | 1200 |",
		"| P99 latency (ms) | 42.50 |",
		"| Error rate | 1.20% |",
		"| Rejection rate | 8.00% |",
		"circuit breaker stayed closed",
		"2026-04-12T10:30:00Z",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in report:\n%s", want, s)
		}
	}
}

func TestWriteMarkdownReport_ZeroValuesRenderNA(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteMarkdownReport(dir, MarkdownSummary{Scenario: "empty"})
	if err != nil {
		t.Fatalf("WriteMarkdownReport: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "| Error rate | n/a |") {
		t.Errorf("expected n/a for error rate:\n%s", s)
	}
}

func TestSafeFilename(t *testing.T) {
	cases := map[string]string{
		"simple":        "simple",
		"with spaces":   "with_spaces",
		"weird/path.go": "weird_path_go",
		"":              "scenario",
	}
	for in, want := range cases {
		if got := safeFilename(in); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewScenarioRegistry(t *testing.T) {
	names := map[string]bool{}
	for _, s := range AllScenarios() {
		names[s.Name] = true
	}
	for _, want := range []string{
		"backpressure_ceiling",
		"circuit_breaker_chaos",
		"outbox_burst",
		"denormalized_dequeue_delta",
		"partition_cycle_matrix",
	} {
		if !names[want] {
			t.Errorf("AllScenarios missing %s", want)
		}
	}
}
