package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func mockAPIStoreWithWorkflow(projectID string) *APIStoreMock {
	return &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: projectID}, nil
		},
	}
}

func TestHandleWorkflowStepDurations_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetWorkflowStepDurationsFunc: func(_ context.Context, _, workflowID string, _, _ time.Time) ([]store.StepDuration, error) {
			require.Equal(t, "wf-1",
				workflowID)

			return []store.StepDuration{
				{StepRef: "step-a", AvgMs: 500, P95Ms: 1200, Count: 50, FailureRate: 0.02},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, mockAPIStoreWithWorkflow("proj-1"), ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/wf-1/step-durations", validFrom(), validTo()), "", "proj-1"))
	require.EqualValues(t, 200, w.
		Code)

}

func TestHandleWorkflowStepDurations_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/workflows/wf-1/step-durations", "", "proj-1"))
	require.EqualValues(t, 400, w.
		Code)

}

func TestHandleWorkflowStepDurations_StoreError(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetWorkflowStepDurationsFunc: func(_ context.Context, _, _ string, _, _ time.Time) ([]store.StepDuration, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, mockAPIStoreWithWorkflow("proj-1"), ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/wf-1/step-durations", validFrom(), validTo()), "", "proj-1"))
	require.EqualValues(t, 500, w.
		Code)

}

func TestHandleWorkflowCompletionRates_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetWorkflowCompletionRatesFunc: func(_ context.Context, _ string, _, _ time.Time, bucket string) ([]store.WorkflowCompletionBucket, error) {
			require.Equal(t, "day",
				bucket)

			return []store.WorkflowCompletionBucket{
				{Period: "2026-01-01T00:00:00Z", Completed: 10, Failed: 2, TimedOut: 1},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/completion-rates", validFrom(), validTo()), "", "proj-1"))
	require.EqualValues(t, 200, w.
		Code)

}

func TestHandleWorkflowCompletionRates_InvalidBucket(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/completion-rates", validFrom(), validTo(), "bucket", "month"), "", "proj-1"))
	require.EqualValues(t, 400, w.
		Code)

}

func TestHandleWorkflowAnalyticsSummary_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetWorkflowSummaryFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.WorkflowSummary, error) {
			return &store.WorkflowSummary{
				Total: 50, Completed: 45, Failed: 5, SuccessRate: 0.9, AvgDurationMs: 30000,
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/summary", validFrom(), validTo()), "", "proj-1"))
	require.EqualValues(t, 200, w.
		Code)

}

func TestHandleWorkflowAnalyticsSummary_StoreError(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetWorkflowSummaryFunc: func(_ context.Context, _ string, _, _ time.Time) (*store.WorkflowSummary, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/summary", validFrom(), validTo()), "", "proj-1"))
	require.EqualValues(t, 500, w.
		Code)

}
