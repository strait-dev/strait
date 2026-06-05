package clickhouse

import (
	"context"
	"testing"
	"time"
)

func TestNewAnalyticsStore(t *testing.T) {
	t.Parallel()

	store := NewAnalyticsStore(nil, nil)
	if store == nil {
		t.Fatal("expected non-nil store")
		return
	}
	if store.client != nil {
		t.Error("expected nil client")
	}
	if store.pgFallback != nil {
		t.Error("expected nil pgFallback")
	}
}

func TestNewAnalyticsStore_WithPgFallback(t *testing.T) {
	t.Parallel()

	mock := &mockPgHealthQuerier{}
	store := NewAnalyticsStore(nil, mock)
	if store.pgFallback == nil {
		t.Fatal("expected non-nil pgFallback")
	}
}

func TestIsShortPeriod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{"1 hour", "2024-01-01T00:00:00Z", "2024-01-01T01:00:00Z", true},
		{"24 hours", "2024-01-01T00:00:00Z", "2024-01-02T00:00:00Z", true},
		{"25 hours", "2024-01-01T00:00:00Z", "2024-01-02T01:00:00Z", false},
		{"7 days", "2024-01-01T00:00:00Z", "2024-01-08T00:00:00Z", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			from := mustParseTime(t, tt.from)
			to := mustParseTime(t, tt.to)
			if got := isShortPeriod(from, to); got != tt.want {
				t.Errorf("isShortPeriod(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

// mockPgHealthQuerier implements PgHealthQuerier for testing.
type mockPgHealthQuerier struct {
	totalJobs  int
	activeJobs int
	queueDepth int
}

func (m *mockPgHealthQuerier) CountJobs(_ context.Context, _ string) (int, int, error) {
	return m.totalJobs, m.activeJobs, nil
}

func (m *mockPgHealthQuerier) QueueDepth(_ context.Context, _ string) (int, error) {
	return m.queueDepth, nil
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("failed to parse time %q: %v", s, err)
	}
	return parsed
}
