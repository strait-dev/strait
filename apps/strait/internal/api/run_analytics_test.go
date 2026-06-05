package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func analyticsURL(path, from, to string, extra ...string) string {
	v := url.Values{}
	v.Set("from", from)
	v.Set("to", to)
	for i := 0; i+1 < len(extra); i += 2 {
		v.Set(extra[i], extra[i+1])
	}
	return "/v1/analytics/" + path + "?" + v.Encode()
}

func validFrom() string { return time.Now().Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339) }
func validTo() string   { return time.Now().UTC().Format(time.RFC3339) }

func TestHandleRunTimeline_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetRunTimelineFunc: func(_ context.Context, _ string, _, _ time.Time, bucket string) ([]store.RunTimelineBucket, error) {
			require.Equal(t, "day", bucket)

			return []store.RunTimelineBucket{
				{Period: "2026-01-01T00:00:00Z", Completed: 10, Failed: 2, TimedOut: 1, Total: 13},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/timeline", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 200, w.Code)

	var result []store.RunTimelineBucket
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &result))
	assert.False(
		t, len(result) !=
			1 || result[0].Total != 13)
}

func TestHandleRunTimeline_HourBucket(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetRunTimelineFunc: func(_ context.Context, _ string, _, _ time.Time, bucket string) ([]store.RunTimelineBucket, error) {
			require.Equal(t, "hour", bucket)

			return []store.RunTimelineBucket{}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/timeline", validFrom(), validTo(), "bucket", "hour"), "", "proj-1"))
	require.Equal(t, 200, w.Code)
}

func TestHandleRunTimeline_InvalidBucket(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/timeline", validFrom(), validTo(), "bucket", "week"), "", "proj-1"))
	require.Equal(t, 400, w.Code)
}

func TestHandleRunTimeline_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/runs/timeline", "", "proj-1"))
	require.Equal(t, 400, w.Code)
}

func TestHandleRunTimeline_StoreError(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetRunTimelineFunc: func(_ context.Context, _ string, _, _ time.Time, _ string) ([]store.RunTimelineBucket, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/timeline", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 500, w.Code)
}

func TestHandleRunDurationDistribution_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetRunDurationDistributionFunc: func(_ context.Context, _ string, _, _ time.Time) ([]store.RunDurationBucket, error) {
			return []store.RunDurationBucket{
				{Range: "<1s", Count: 50, Pct: 50},
				{Range: "1-5s", Count: 50, Pct: 50},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/duration-distribution", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 200, w.Code)
}

func TestHandleRunDurationDistribution_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/runs/duration-distribution", "", "proj-1"))
	require.Equal(t, 400, w.Code)
}

func TestHandleRunFailureReasons_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetRunFailureReasonsFunc: func(_ context.Context, _ string, _, _ time.Time, limit int) ([]store.RunFailureReason, error) {
			require.Equal(t, 10, limit)

			return []store.RunFailureReason{
				{Message: "timeout", Count: 5, LastSeen: "2026-01-01T00:00:00Z", ExampleRunID: "run-1"},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/failure-reasons", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 200, w.Code)
}

func TestHandleRunFailureReasons_CustomLimit(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetRunFailureReasonsFunc: func(_ context.Context, _ string, _, _ time.Time, limit int) ([]store.RunFailureReason, error) {
			require.Equal(t, 5, limit)

			return []store.RunFailureReason{}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/failure-reasons", validFrom(), validTo(), "limit", "5"), "", "proj-1"))
	require.Equal(t, 200, w.Code)
}

func TestHandleRunFailureReasons_InvalidLimit(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/failure-reasons", validFrom(), validTo(), "limit", "0"), "", "proj-1"))
	require.Equal(t, 400, w.Code)
}

func TestHandleRunSummary_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetRunSummaryFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.RunSummary, error) {
			return &store.RunSummary{
				Total: 100, Completed: 90, Failed: 8, TimedOut: 2,
				SuccessRate: 0.9, AvgDurationMs: 1500, P95DurationMs: 5000,
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/summary", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 200, w.Code)

	var result store.RunSummary
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &result))
	assert.Equal(t, 100, result.Total)
}

func TestHandleRunSummary_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/runs/summary", "", "proj-1"))
	require.Equal(t, 400, w.Code)
}

func TestHandleRunSummary_ExceedsMaxWindow(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	from := time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/summary", from, to), "", "proj-1"))
	require.Equal(t, 400, w.Code)
}

func TestHandleRunsByTrigger_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetRunsByTriggerFunc: func(_ context.Context, _ string, _, _ time.Time) ([]store.RunsByTrigger, error) {
			return []store.RunsByTrigger{
				{TriggerType: "api", Total: 50, Completed: 48, Failed: 2, AvgDurationMs: 1200},
				{TriggerType: "schedule", Total: 30, Completed: 28, Failed: 2, AvgDurationMs: 2000},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/by-trigger", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 200, w.Code)
}

func TestHandleRunsByTrigger_StoreError(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetRunsByTriggerFunc: func(_ context.Context, _ string, _, _ time.Time) ([]store.RunsByTrigger, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("runs/by-trigger", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 500, w.Code)
}
