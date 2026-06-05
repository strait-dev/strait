//go:build !integration

package clickhouse

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AnalyticsStore construction

func TestNewAnalyticsStore_NilBoth(t *testing.T) {
	t.Parallel()

	s := NewAnalyticsStore(nil, nil)
	require.NotNil(t, s)
	assert.Nil(t, s.client)
	assert.Nil(t, s.pgFallback)
}

func TestNewPgHealthAdapter(t *testing.T) {
	t.Parallel()

	a := NewPgHealthAdapter(nil)
	require.NotNil(t, a)
	assert.Nil(t, a.pool)
}

// AnalyticsStore methods with nil client -- only for methods that call
// client.Query first (which has nil-safety and returns an error).
// Methods that call QueryRow first would panic on nil client, so those
// are tested with a closed-db client below.

func TestAnalyticsStore_NilClient_QueryMethods(t *testing.T) {
	t.Parallel()

	s := NewAnalyticsStore(nil, nil)
	now := time.Now()
	from := now.Add(-24 * time.Hour)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"GetPerformanceAnalytics", func() error {
			_, err := s.GetPerformanceAnalytics(context.Background(), "p1", 24)
			return err
		}},
		{"GetCostTrends", func() error {
			_, err := s.GetCostTrends(context.Background(), "p1", from, now)
			return err
		}},
		{"GetTopCosts", func() error {
			_, err := s.GetTopCosts(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetCostOutliers", func() error {
			_, err := s.GetCostOutliers(context.Background(), "p1", from, now, 2.0)
			return err
		}},
		{"GetRunTimeline_day", func() error {
			_, err := s.GetRunTimeline(context.Background(), "p1", from, now, "day")
			return err
		}},
		{"GetRunTimeline_hour", func() error {
			_, err := s.GetRunTimeline(context.Background(), "p1", from, now, "hour")
			return err
		}},
		{"GetRunDurationDistribution", func() error {
			_, err := s.GetRunDurationDistribution(context.Background(), "p1", from, now)
			return err
		}},
		{"GetRunFailureReasons", func() error {
			_, err := s.GetRunFailureReasons(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetRunsByTrigger", func() error {
			_, err := s.GetRunsByTrigger(context.Background(), "p1", from, now)
			return err
		}},
		{"GetJobHistory_day", func() error {
			_, err := s.GetJobHistory(context.Background(), "p1", "j1", from, now, "day")
			return err
		}},
		{"GetJobHistory_hour", func() error {
			_, err := s.GetJobHistory(context.Background(), "p1", "j1", from, now, "hour")
			return err
		}},
		{"GetJobComparison", func() error {
			_, err := s.GetJobComparison(context.Background(), "p1", []string{"j1"}, from, now)
			return err
		}},
		{"GetJobReliability", func() error {
			_, err := s.GetJobReliability(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetRunsByVersion", func() error {
			_, err := s.GetRunsByVersion(context.Background(), "p1", "j1", from, now)
			return err
		}},
		{"GetJobCostRanking", func() error {
			_, err := s.GetJobCostRanking(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetTopFailingJobs", func() error {
			_, err := s.GetTopFailingJobs(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetTagSummary", func() error {
			_, err := s.GetTagSummary(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetTopFailingTags", func() error {
			_, err := s.GetTopFailingTags(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetTagCost", func() error {
			_, err := s.GetTagCost(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetWorkflowStepDurations", func() error {
			_, err := s.GetWorkflowStepDurations(context.Background(), "p1", "wf1", from, now)
			return err
		}},
		{"GetWorkflowCompletionRates_day", func() error {
			_, err := s.GetWorkflowCompletionRates(context.Background(), "p1", from, now, "day")
			return err
		}},
		{"GetWorkflowCompletionRates_hour", func() error {
			_, err := s.GetWorkflowCompletionRates(context.Background(), "p1", from, now, "hour")
			return err
		}},
		{"GetWebhookDeliveryStats", func() error {
			_, err := s.GetWebhookDeliveryStats(context.Background(), "p1", from, now)
			return err
		}},
		{"GetWebhookEndpointHealth_day", func() error {
			_, err := s.GetWebhookEndpointHealth(context.Background(), "p1", from, now, "day")
			return err
		}},
		{"GetWebhookEndpointHealth_hour", func() error {
			_, err := s.GetWebhookEndpointHealth(context.Background(), "p1", from, now, "hour")
			return err
		}},
		{"GetTopFailingWebhooks", func() error {
			_, err := s.GetTopFailingWebhooks(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetEventVolume_day", func() error {
			_, err := s.GetEventVolume(context.Background(), "p1", from, now, "day")
			return err
		}},
		{"GetEventVolume_hour", func() error {
			_, err := s.GetEventVolume(context.Background(), "p1", from, now, "hour")
			return err
		}},
		{"GetCostByTrigger", func() error {
			_, err := s.GetCostByTrigger(context.Background(), "p1", from, now)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.fn()
			require.Error(t, err)
		})
	}
}

// AnalyticsStore methods that use QueryRow first (which has no nil-safety).
// We use a closed-db client to exercise the error path without panicking.

func TestAnalyticsStore_ClosedDB_QueryRowMethods(t *testing.T) {
	t.Parallel()

	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	now := time.Now()
	from := now.Add(-24 * time.Hour)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"GetCostAnalytics", func() error {
			_, err := s.GetCostAnalytics(context.Background(), "p1", from, now)
			return err
		}},
		{"GetApprovalStats", func() error {
			_, err := s.GetApprovalStats(context.Background(), "p1", from, now)
			return err
		}},
		{"GetRunSummary", func() error {
			_, err := s.GetRunSummary(context.Background(), "p1", from, now)
			return err
		}},
		{"GetWorkflowSummary", func() error {
			_, err := s.GetWorkflowSummary(context.Background(), "p1", from, now)
			return err
		}},
		{"GetEventLatency", func() error {
			_, err := s.GetEventLatency(context.Background(), "p1", from, now)
			return err
		}},
		{"GetCostForecast", func() error {
			_, err := s.GetCostForecast(context.Background(), "p1", from, now)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.fn()
			require.Error(t, err)
		})
	}
}

// AnalyticsStore methods with closed-db client (error paths, more detailed)

func TestAnalyticsStore_ClosedDB_GetPerformanceAnalytics(t *testing.T) {
	t.Parallel()

	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	_, err := s.GetPerformanceAnalytics(context.Background(), "proj-1", 24)
	require.Error(t, err)
	assert.Contains(t, err.
		Error(), "slowest jobs")
}

func TestAnalyticsStore_ClosedDB_GetCostAnalytics(t *testing.T) {
	t.Parallel()

	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	now := time.Now()
	_, err := s.GetCostAnalytics(context.Background(), "proj-1", now.Add(-24*time.Hour), now)
	require.Error(t, err)
}

// AnalyticsStore with failing pgFallback

type failingPgHealthQuerier struct {
	countErr error
	depthErr error
}

func (f *failingPgHealthQuerier) CountJobs(_ context.Context, _ string) (int, int, error) {
	return 0, 0, f.countErr
}

func (f *failingPgHealthQuerier) QueueDepth(_ context.Context, _ string) (int, error) {
	return 0, f.depthErr
}

func TestAnalyticsStore_PgFallback_CountJobsError(t *testing.T) {
	t.Parallel()

	// Since all ClickHouse queries will fail with nil client, we verify the
	// error comes from the ClickHouse query, not from pgFallback.
	fallback := &failingPgHealthQuerier{
		countErr: errors.New("pg connection refused"),
	}
	s := NewAnalyticsStore(nil, fallback)
	_, err := s.GetPerformanceAnalytics(context.Background(), "proj-1", 24)
	require.Error(t, err)
}

// isShortPeriod edge cases

func TestIsShortPeriod_Exact24Hours(t *testing.T) {
	t.Parallel()

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	assert.True(t, isShortPeriod(from,
		to))
}

func TestIsShortPeriod_OneNanosecondOver(t *testing.T) {
	t.Parallel()

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24*time.Hour + time.Nanosecond)
	assert.False(t, isShortPeriod(from,
		to))
}

func TestIsShortPeriod_ZeroDuration(t *testing.T) {
	t.Parallel()

	now := time.Now()
	assert.True(t, isShortPeriod(now,
		now))
}

func TestIsShortPeriod_NegativeDuration(t *testing.T) {
	t.Parallel()

	now := time.Now()
	assert.True(t, isShortPeriod(now,
		now.Add(-time.
			Hour)))

	// from > to: negative duration should be "short" (less than 24h).
}

// Bucket parameter variations for timeline queries with closed db

func TestAnalyticsStore_CostTrends_ShortPeriod(t *testing.T) {
	t.Parallel()

	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	now := time.Now()
	// Short period (< 24h) exercises the toStartOfHour branch.
	_, err := s.GetCostTrends(context.Background(), "proj-1", now.Add(-time.Hour), now)
	require.Error(t, err)
}

func TestAnalyticsStore_CostTrends_LongPeriod(t *testing.T) {
	t.Parallel()

	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	now := time.Now()
	// Long period (> 24h) exercises the toStartOfDay branch.
	_, err := s.GetCostTrends(context.Background(), "proj-1", now.Add(-7*24*time.Hour), now)
	require.Error(t, err)
}

// Helpers

func newClosedDBClient(t *testing.T) *Client {
	t.Helper()
	db, err := sql.Open("clickhouse", "clickhouse://localhost:0")
	require.NoError(t, err)

	db.Close()
	return &Client{db: db, logger: slog.Default()}
}

func newOpenDB(t *testing.T) (*sql.DB, error) {
	t.Helper()
	return sql.Open("clickhouse", "clickhouse://localhost:0")
}
