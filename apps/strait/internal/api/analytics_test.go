package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/store"
)

func TestHandleGetPerformanceAnalytics_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetPerformanceAnalyticsFunc: func(_ context.Context, _ string, periodHours int) (*store.PerformanceAnalytics, error) {
			if periodHours != 24 {
				t.Fatalf("expected default 24h period, got %d", periodHours)
			}
			return &store.PerformanceAnalytics{
				SlowestJobs: []store.JobPerformance{
					{JobID: "job-1", JobSlug: "slow-job", AvgDurationSecs: 45.2, TotalRuns: 100},
				},
				Throughput:    store.ThroughputStats{Completed: 500, Failed: 10, PeriodHours: 24},
				HealthSummary: store.HealthSummary{TotalJobs: 5, ActiveJobs: 4, SuccessRate: 0.98},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/analytics/performance", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetPerformanceAnalytics_CustomPeriod(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetPerformanceAnalyticsFunc: func(_ context.Context, _ string, periodHours int) (*store.PerformanceAnalytics, error) {
			if periodHours != 72 {
				t.Fatalf("expected 72h period, got %d", periodHours)
			}
			return &store.PerformanceAnalytics{
				SlowestJobs: make([]store.JobPerformance, 0),
				Throughput:  store.ThroughputStats{PeriodHours: 72},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/analytics/performance?period_hours=72", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetPerformanceAnalytics_InvalidPeriod(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/analytics/performance?period_hours=0", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleGetPerformanceAnalytics_NilStore_Returns503(t *testing.T) {
	t.Parallel()
	// Use a plain test server without an analytics store; neither the store
	// nor an explicit analytics store implement AnalyticsStore.
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/analytics/performance", ""))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when analytics is nil, got %d: %s", w.Code, w.Body.String())
	}
}
