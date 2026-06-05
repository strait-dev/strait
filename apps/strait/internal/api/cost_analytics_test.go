package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestHandleGetCostAnalytics_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetCostAnalyticsFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.CostAnalytics, error) {
			return &store.CostAnalytics{
				TotalSpendMicrousd: 456,
				ByJob:              make([]store.CostByJob, 0),
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	now := time.Now().UTC()
	from := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs?from="+from+"&to="+to, "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&body))
	require.Equal(t, float64(456),
		body["total_spend_microusd"])

	retiredCostField := strings.Join([]string{"total", "ai", "cost", "microusd"}, "_")
	for _, stale := range []string{
		retiredCostField,
		"total_tokens",
		"by_model",
		"total_usage_cost_microusd",
		"total_compute_cost_microusd",
		"usage_cost_microusd",
		"compute_cost_microusd",
	} {
		if _, ok := body[stale]; ok {
			require.Failf(t, "test failure",

				"launch response must not expose %q: %s", stale, w.Body.String())
		}
	}
}

func TestHandleGetCostTrends_SuccessUsesSpendFields(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetCostTrendsFunc: func(_ context.Context, _ string, _, _ time.Time) ([]store.CostTrendPoint, error) {
			return []store.CostTrendPoint{
				{
					Period:        "2026-06-04T10:00:00Z",
					SpendMicrousd: 789,
					RunCount:      3,
				},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs/trends?from="+from+"&to="+to, "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var body []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&body))
	require.Len(t,
		body, 1)
	require.Equal(t, float64(789),
		body[0]["spend_microusd"])

	for _, stale := range []string{"usage_cost_microusd", "compute_cost_microusd"} {
		if _, ok := body[0][stale]; ok {
			require.Failf(t, "test failure",

				"launch trend response must not expose %q: %s", stale, w.Body.String())
		}
	}
}

func TestHandleGetCostAnalytics_MissingFrom(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs?to=2025-01-01T00:00:00Z", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleGetCostAnalytics_MissingTo(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs?from=2025-01-01T00:00:00Z", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleGetCostAnalytics_InvalidFromFormat(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs?from=not-a-date&to=2025-01-01T00:00:00Z", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleGetCostAnalytics_InvalidToFormat(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs?from=2025-01-01T00:00:00Z&to=not-a-date", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleGetCostAnalytics_ToBeforeFrom(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs?from=2025-06-01T00:00:00Z&to=2025-01-01T00:00:00Z", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleGetCostAnalytics_ExceedsMaxWindow(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs?from=2025-01-01T00:00:00Z&to=2025-04-02T00:00:00Z", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.True(
		t, strings.Contains(w.Body.
			String(),
			"90 days"))

}

func TestHandleGetCostAnalytics_ExactlyMaxWindow(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetCostAnalyticsFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.CostAnalytics, error) {
			return &store.CostAnalytics{
				ByJob: make([]store.CostByJob, 0),
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs?from=2025-01-01T00:00:00Z&to=2025-04-01T00:00:00Z", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestHandleGetCostTrends_ExceedsMaxWindow(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs/trends?from=2025-01-01T00:00:00Z&to=2025-04-02T00:00:00Z", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleGetTopCosts_ExceedsMaxWindow(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs/top?from=2025-01-01T00:00:00Z&to=2025-04-02T00:00:00Z", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleGetCostInsights_ExceedsMaxWindow(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/cost-insights?from=2025-01-01T00:00:00Z&to=2025-04-02T00:00:00Z", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleGetTopCosts_ValidLimit(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetTopCostsFunc: func(_ context.Context, _ string, _, _ time.Time, _ int) ([]store.TopCostItem, error) {
			return []store.TopCostItem{}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	now := time.Now().UTC()
	from := now.Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs/top?from="+from+"&to="+to+"&limit=50", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestHandleGetTopCosts_LimitTooHigh(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	now := time.Now().UTC()
	from := now.Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs/top?from="+from+"&to="+to+"&limit=200", "", "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleGetTopCosts_StoreError(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetTopCostsFunc: func(_ context.Context, _ string, _, _ time.Time, _ int) ([]store.TopCostItem, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	now := time.Now().UTC()
	from := now.Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/analytics/costs/top?from="+from+"&to="+to, "", "proj-1"))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}
