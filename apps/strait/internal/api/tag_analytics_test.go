package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/store"
)

func TestHandleTagSummary_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetTagSummaryFunc: func(_ context.Context, _ string, _, _ time.Time, limit int) ([]store.TagSummary, error) {
			if limit != 50 {
				t.Fatalf("expected default limit 50, got %d", limit)
			}
			return []store.TagSummary{
				{TagKey: "env", TagValue: "prod", Total: 100, Completed: 95, Failed: 5, AvgDurationMs: 1500},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("tags/summary", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTagSummary_CustomLimit(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetTagSummaryFunc: func(_ context.Context, _ string, _, _ time.Time, limit int) ([]store.TagSummary, error) {
			if limit != 20 {
				t.Fatalf("expected limit 20, got %d", limit)
			}
			return []store.TagSummary{}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("tags/summary", validFrom(), validTo(), "limit", "20"), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleTagSummary_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/tags/summary", "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleTagSummary_StoreError(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetTagSummaryFunc: func(_ context.Context, _ string, _, _ time.Time, _ int) ([]store.TagSummary, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("tags/summary", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleTopFailingTags_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetTopFailingTagsFunc: func(_ context.Context, _ string, _, _ time.Time, _ int) ([]store.TopFailingTag, error) {
			return []store.TopFailingTag{
				{TagKey: "env", TagValue: "staging", Failed: 10, Total: 20, FailureRate: 0.5},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("tags/top-failing", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTagCost_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetTagCostFunc: func(_ context.Context, _ string, _, _ time.Time, _ int) ([]store.TagCost, error) {
			return []store.TagCost{
				{TagKey: "team", TagValue: "platform", TotalCost: 50000, RunCount: 100},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("tags/cost", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTagCost_InvalidLimit(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("tags/cost", validFrom(), validTo(), "limit", "0"), "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
