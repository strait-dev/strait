package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNormalizeWorkerLogLevel is the regression guard for log-level spoofing:
// worker-supplied levels must be coerced to a known allowlist.
func TestNormalizeWorkerLogLevel(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"info":             "info",
		"INFO":             "info",
		"  Warn  ":         "warn",
		"error":            "error",
		"":                 "info",
		"critical":         "info", // not in allowlist
		"<script>":         "info", // injection attempt
		"info\ndata: evil": "info",
	}
	for in, want := range cases {
		require.Equalf(t, want, normalizeWorkerLogLevel(in), "level %q", in)
	}
}

// TestSanitizeWorkerLogTimestamp is the regression guard for worker-controlled
// timestamps: implausible values fall back to server time, plausible ones pass.
func TestSanitizeWorkerLogTimestamp(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	nowMs := now.UnixMilli()

	// Plausible recent timestamp is preserved.
	recent := nowMs - 5_000
	require.Equal(t, recent, sanitizeWorkerLogTimestamp(recent, now))

	// Implausible values clamp to server time.
	for _, bad := range []int64{0, -1, nowMs + (2 * 24 * 60 * 60 * 1000), nowMs - (60 * 24 * 60 * 60 * 1000)} {
		require.Equal(t, nowMs, sanitizeWorkerLogTimestamp(bad, now))
	}
}
