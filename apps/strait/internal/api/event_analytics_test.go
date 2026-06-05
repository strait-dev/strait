package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServerWithAnalytics(t *testing.T, s APIStore, as AnalyticsStore, q *mockQueue) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:         cfg,
		Store:          s,
		AnalyticsStore: as,
		Queue:          q,
		Edition:        domain.EditionCloud,
		BillingEnforcer: &tunableLimitsEnforcer{
			limits: billing.GetPlanLimits(domain.PlanEnterprise),
		},
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestHandleEventVolume_Success(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetEventVolumeFunc: func(_ context.Context, _ string, _, _ time.Time, bucket string) ([]store.EventVolumeBucket, error) {
			require.Equal(t, "day", bucket)

			return []store.EventVolumeBucket{
				{Period: "2026-01-01T00:00:00Z", Created: 100, Received: 90, TimedOut: 10},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("events/volume", validFrom(), validTo()), "", "proj-1"))
	require.EqualValues(t, 200, w.Code)

}

func TestHandleEventVolume_InvalidBucket(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("events/volume", validFrom(), validTo(), "bucket", "week"), "", "proj-1"))
	require.EqualValues(t, 400, w.Code)

}

func TestHandleEventVolume_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/events/volume", "", "proj-1"))
	require.EqualValues(t, 400, w.Code)

}

func TestHandleEventVolume_StoreError(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetEventVolumeFunc: func(_ context.Context, _ string, _, _ time.Time, _ string) ([]store.EventVolumeBucket, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("events/volume", validFrom(), validTo()), "", "proj-1"))
	require.EqualValues(t, 500, w.Code)

}

func TestHandleEventLatency_Success(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetEventLatencyFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.EventLatencyStats, error) {
			return &store.EventLatencyStats{
				AvgMs: 150, P50Ms: 100, P95Ms: 500, P99Ms: 1200, Count: 1000,
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("events/latency", validFrom(), validTo()), "", "proj-1"))
	require.EqualValues(t, 200, w.Code)

	var result store.EventLatencyStats
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &result))
	assert.EqualValues(t, 1000, result.Count)

}

func TestHandleEventLatency_StoreError(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetEventLatencyFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.EventLatencyStats, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("events/latency", validFrom(), validTo()), "", "proj-1"))
	require.EqualValues(t, 500, w.Code)

}

func TestHandleCostForecast_Success(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetCostForecastFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.CostForecast, error) {
			return &store.CostForecast{DailyRate: 10000, ProjectedMonthly: 300000, TrendPct: 5.2}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("costs/forecast", validFrom(), validTo()), "", "proj-1"))
	require.EqualValues(t, 200, w.Code)

}

func TestHandleCostForecast_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/costs/forecast", "", "proj-1"))
	require.EqualValues(t, 400, w.Code)

}

func TestHandleCostByTrigger_Success(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetCostByTriggerFunc: func(_ context.Context, _ string, _, _ time.Time) ([]store.CostByTrigger, error) {
			return []store.CostByTrigger{
				{Trigger: "api", Cost: 50000, RunCount: 100, Pct: 60},
				{Trigger: "schedule", Cost: 30000, RunCount: 50, Pct: 40},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("costs/by-trigger", validFrom(), validTo()), "", "proj-1"))
	require.EqualValues(t, 200, w.Code)

}
