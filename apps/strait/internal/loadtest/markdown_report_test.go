//go:build loadtest

package loadtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t,

		err)
	assert.True(t, strings.HasSuffix(path,
		".md"))
	assert.Equal(t, dir,

		filepath.
			Dir(
				path))

	body, err := os.ReadFile(path)
	require.NoError(t,

		err)

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
		assert.True(t, strings.Contains(s,
			want))

	}
}

func TestWriteMarkdownReport_ZeroValuesRenderNA(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteMarkdownReport(dir, MarkdownSummary{Scenario: "empty"})
	require.NoError(t,

		err)

	body, err := os.ReadFile(path)
	require.NoError(t,

		err)

	s := string(body)
	assert.True(t, strings.Contains(s,
		"| Error rate | n/a |",
	))

}

func TestSafeFilename(t *testing.T) {
	cases := map[string]string{
		"simple":        "simple",
		"with spaces":   "with_spaces",
		"weird/path.go": "weird_path_go",
		"":              "scenario",
	}
	for in, want := range cases {
		assert.Equal(t, want, safeFilename(in))
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
		"partition_cycle_matrix",
	} {
		assert.True(t, names[want])

	}
}
