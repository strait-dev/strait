package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestHandleTagSummary_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetTagSummaryFunc: func(_ context.Context, _ string, _, _ time.Time, limit int) ([]store.TagSummary, error) {
			require.Equal(t, 50, limit)

			return []store.TagSummary{
				{TagKey: "env", TagValue: "prod", Total: 100, Completed: 95, Failed: 5, AvgDurationMs: 1500},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("tags/summary", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 200, w.
		Code)
}

func TestHandleTagSummary_CustomLimit(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetTagSummaryFunc: func(_ context.Context, _ string, _, _ time.Time, limit int) ([]store.TagSummary, error) {
			require.Equal(t, 20, limit)

			return []store.TagSummary{}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("tags/summary", validFrom(), validTo(), "limit", "20"), "", "proj-1"))
	require.Equal(t, 200, w.
		Code)
}

func TestHandleTagSummary_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/tags/summary", "", "proj-1"))
	require.Equal(t, 400, w.
		Code)
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
	require.Equal(t, 500, w.
		Code)
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
	require.Equal(t, 200, w.
		Code)
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
	require.Equal(t, 200, w.
		Code)
}

func TestHandleTagCost_InvalidLimit(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("tags/cost", validFrom(), validTo(), "limit", "0"), "", "proj-1"))
	require.Equal(t, 400, w.
		Code)
}
