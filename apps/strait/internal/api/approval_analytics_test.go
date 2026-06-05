package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func approvalStatsURL(from, to string) string {
	v := url.Values{}
	v.Set("from", from)
	v.Set("to", to)
	return "/v1/analytics/approvals?" + v.Encode()
}

func TestHandleGetApprovalStats_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetApprovalStatsFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.ApprovalStats, error) {
			return &store.ApprovalStats{
				TotalRequested:      10,
				TotalApproved:       7,
				TotalTimedOut:       2,
				TotalPending:        1,
				AvgApprovalTimeSecs: 120.5,
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})

	from := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", approvalStatsURL(from, to), "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 200, w.Code)

	var stats store.ApprovalStats
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &stats,
	))
	assert.EqualValues(t, 10, stats.TotalRequested)
	assert.EqualValues(t, 7, stats.TotalApproved)

}

func TestHandleGetApprovalStats_FreeTierRejected(t *testing.T) {
	t.Parallel()

	ms := &AnalyticsStoreMock{
		GetApprovalStatsFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.ApprovalStats, error) {
			require.Fail(t,

				"GetApprovalStats must not be called when approval-gates plan gate rejects")
			return nil, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	srv.edition = domain.EditionCloud
	srv.billingEnforcer = &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanFree)}

	from := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, approvalStatsURL(from, to), "", "proj-1")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden,
		w.
			Code)
	require.True(
		t, strings.Contains(w.Body.
			String(), "Approval gates",
		))

}

func TestHandleGetApprovalStats_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})

	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", "/v1/analytics/approvals", "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 400, w.Code)

}

func TestHandleGetApprovalStats_InvalidTimeRange(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})

	to := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	from := time.Now().UTC().Format(time.RFC3339)
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", approvalStatsURL(from, to), "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 400, w.Code)

}

func TestHandleGetApprovalStats_TooWideRange(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})

	from := time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", approvalStatsURL(from, to), "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 400, w.Code)

}

func TestHandleGetApprovalStats_StoreError(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetApprovalStatsFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.ApprovalStats, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})

	from := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", approvalStatsURL(from, to), "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 500, w.Code)

}

func TestHandleGetApprovalStats_EmptyResults(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetApprovalStatsFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.ApprovalStats, error) {
			return &store.ApprovalStats{}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})

	from := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	w := httptest.NewRecorder()
	r := authedProjectRequest("GET", approvalStatsURL(from, to), "", "proj-1")
	srv.ServeHTTP(w, r)
	require.EqualValues(t, 200, w.Code)

	var stats store.ApprovalStats
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &stats,
	))
	assert.EqualValues(t, 0, stats.TotalRequested)

}
