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
			if threshold != 2.0 {
				t.Fatalf("expected threshold 2.0, got %f", threshold)
			}
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

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Outliers  []store.CostOutlier `json:"outliers"`
		Threshold float64             `json:"threshold"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Outliers) != 3 {
		t.Fatalf("expected 3 outliers, got %d", len(resp.Outliers))
	}
	if resp.Threshold != 2.0 {
		t.Fatalf("expected threshold 2.0, got %f", resp.Threshold)
	}
}

func TestHandleGetCostInsights_CustomThreshold(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetCostOutliersFunc: func(_ context.Context, _ string, _, _ time.Time, threshold float64) ([]store.CostOutlier, error) {
			if threshold != 3.0 {
				t.Fatalf("expected threshold 3.0, got %f", threshold)
			}
			return nil, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", costInsightsURL("2025-01-01T00:00:00Z", "2025-01-31T00:00:00Z", "threshold", "3.0"), "", "proj-1")
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetCostInsights_DefaultThreshold(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetCostOutliersFunc: func(_ context.Context, _ string, _, _ time.Time, threshold float64) ([]store.CostOutlier, error) {
			if threshold != 2.0 {
				t.Fatalf("expected default threshold 2.0, got %f", threshold)
			}
			return nil, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", costInsightsURL("2025-01-01T00:00:00Z", "2025-01-31T00:00:00Z"), "", "proj-1")
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetCostInsights_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", "/v1/analytics/cost-insights", "", "proj-1")
	srv.ServeHTTP(w, r)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
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

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Outliers []store.CostOutlier `json:"outliers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Outliers) != 0 {
		t.Fatalf("expected 0 outliers, got %d", len(resp.Outliers))
	}
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

	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
