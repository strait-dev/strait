package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/store"
)

func TestHandleEventVolume_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getEventVolumeFn: func(_ context.Context, _ string, _, _ time.Time, bucket string) ([]store.EventVolumeBucket, error) {
			if bucket != "day" {
				t.Fatalf("expected default bucket 'day', got %q", bucket)
			}
			return []store.EventVolumeBucket{
				{Period: "2026-01-01T00:00:00Z", Created: 100, Received: 90, TimedOut: 10},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("events/volume", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEventVolume_InvalidBucket(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("events/volume", validFrom(), validTo(), "bucket", "week"), "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleEventVolume_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/events/volume", "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleEventVolume_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getEventVolumeFn: func(_ context.Context, _ string, _, _ time.Time, _ string) ([]store.EventVolumeBucket, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("events/volume", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleEventLatency_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getEventLatencyFn: func(_ context.Context, _ string, _, _ time.Time) (*store.EventLatencyStats, error) {
			return &store.EventLatencyStats{
				AvgMs: 150, P50Ms: 100, P95Ms: 500, P99Ms: 1200, Count: 1000,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("events/latency", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result store.EventLatencyStats
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.Count != 1000 {
		t.Errorf("expected count 1000, got %d", result.Count)
	}
}

func TestHandleEventLatency_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getEventLatencyFn: func(_ context.Context, _ string, _, _ time.Time) (*store.EventLatencyStats, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("events/latency", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleCostForecast_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getCostForecastFn: func(_ context.Context, _ string, _, _ time.Time) (*store.CostForecast, error) {
			return &store.CostForecast{DailyRate: 10000, ProjectedMonthly: 300000, TrendPct: 5.2}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("costs/forecast", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCostForecast_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/costs/forecast", "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCostByTrigger_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getCostByTriggerFn: func(_ context.Context, _ string, _, _ time.Time) ([]store.CostByTrigger, error) {
			return []store.CostByTrigger{
				{Trigger: "api", Cost: 50000, RunCount: 100, Pct: 60},
				{Trigger: "schedule", Cost: 30000, RunCount: 50, Pct: 40},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("costs/by-trigger", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCostByMachine_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getCostByMachineFn: func(_ context.Context, _ string, _, _ time.Time) ([]store.CostByMachine, error) {
			return []store.CostByMachine{
				{Preset: "large", Cost: 80000, DurationSecs: 3600, RunCount: 20},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("costs/by-machine", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCostByMachine_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getCostByMachineFn: func(_ context.Context, _ string, _, _ time.Time) ([]store.CostByMachine, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("costs/by-machine", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
