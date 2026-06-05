package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func costInsightsURL(from, to string, extras ...string) string {
	v := url.Values{}
	v.Set("from", from)
	v.Set("to", to)
	for i := 0; i+1 < len(extras); i += 2 {
		v.Set(extras[i], extras[i+1])
	}
	return "/v1/analytics/cost-insights?" + v.Encode()
}

func TestHandleGetCostInsights_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetCostOutliersFunc: func(_ context.Context, _ string, _, _ time.Time, threshold float64) ([]store.CostOutlier, error) {
			require.EqualValues(t, 2.0, threshold)

			return []store.CostOutlier{
				{RunID: "run-1", JobID: "job-1", CostMicrousd: 50000, AvgCostMicrousd: 10000, StddevMicrousd: 5000, DeviationsAbove: 8.0},
				{RunID: "run-2", JobID: "job-1", CostMicrousd: 40000, AvgCostMicrousd: 10000, StddevMicrousd: 5000, DeviationsAbove: 6.0},
				{RunID: "run-3", JobID: "job-2", CostMicrousd: 30000, AvgCostMicrousd: 8000, StddevMicrousd: 4000, DeviationsAbove: 5.5},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", costInsightsURL("2025-01-01T00:00:00Z", "2025-01-31T00:00:00Z"), "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 200, w.Code)

	var resp struct {
		Outliers  []store.CostOutlier `json:"outliers"`
		Threshold float64             `json:"threshold"`
	}
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp))
	require.Len(t,
		resp.Outliers,
		3)
	require.EqualValues(t, 2.0, resp.Threshold)

}

func TestHandleGetCostInsights_CustomThreshold(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetCostOutliersFunc: func(_ context.Context, _ string, _, _ time.Time, threshold float64) ([]store.CostOutlier, error) {
			require.EqualValues(t, 3.0, threshold)

			return nil, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", costInsightsURL("2025-01-01T00:00:00Z", "2025-01-31T00:00:00Z", "threshold", "3.0"), "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 200, w.Code)

}

func TestHandleGetCostInsights_DefaultThreshold(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetCostOutliersFunc: func(_ context.Context, _ string, _, _ time.Time, threshold float64) ([]store.CostOutlier, error) {
			require.EqualValues(t, 2.0, threshold)

			return nil, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", costInsightsURL("2025-01-01T00:00:00Z", "2025-01-31T00:00:00Z"), "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 200, w.Code)

}

func TestHandleGetCostInsights_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", "/v1/analytics/cost-insights", "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 400, w.Code)

}

func TestHandleGetCostInsights_NoOutliers(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetCostOutliersFunc: func(_ context.Context, _ string, _, _ time.Time, _ float64) ([]store.CostOutlier, error) {
			return []store.CostOutlier{}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", costInsightsURL("2025-01-01T00:00:00Z", "2025-01-31T00:00:00Z"), "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 200, w.Code)

	var resp struct {
		Outliers []store.CostOutlier `json:"outliers"`
	}
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp))
	require.Len(t,
		resp.Outliers,
		0)

}

func TestHandleGetCostInsights_StoreError(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetCostOutliersFunc: func(_ context.Context, _ string, _, _ time.Time, _ float64) ([]store.CostOutlier, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", costInsightsURL("2025-01-01T00:00:00Z", "2025-01-31T00:00:00Z"), "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 500, w.Code)

}
