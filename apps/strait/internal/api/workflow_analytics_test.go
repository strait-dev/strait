package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/store"
)

func TestHandleWorkflowStepDurations_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getWorkflowStepDurationsFn: func(_ context.Context, _, workflowID string, _, _ time.Time) ([]store.StepDuration, error) {
			if workflowID != "wf-1" {
				t.Fatalf("expected wf-1, got %s", workflowID)
			}
			return []store.StepDuration{
				{StepRef: "step-a", AvgMs: 500, P95Ms: 1200, Count: 50, FailureRate: 0.02},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/wf-1/step-durations", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWorkflowStepDurations_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/workflows/wf-1/step-durations", "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleWorkflowStepDurations_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getWorkflowStepDurationsFn: func(_ context.Context, _, _ string, _, _ time.Time) ([]store.StepDuration, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/wf-1/step-durations", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleWorkflowCompletionRates_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getWorkflowCompletionRatesFn: func(_ context.Context, _ string, _, _ time.Time, bucket string) ([]store.WorkflowCompletionBucket, error) {
			if bucket != "day" {
				t.Fatalf("expected default bucket 'day', got %q", bucket)
			}
			return []store.WorkflowCompletionBucket{
				{Period: "2026-01-01T00:00:00Z", Completed: 10, Failed: 2, TimedOut: 1},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/completion-rates", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWorkflowCompletionRates_InvalidBucket(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/completion-rates", validFrom(), validTo(), "bucket", "month"), "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleWorkflowAnalyticsSummary_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getWorkflowSummaryFn: func(_ context.Context, _ string, _, _ time.Time) (*store.WorkflowSummary, error) {
			return &store.WorkflowSummary{
				Total: 50, Completed: 45, Failed: 5, SuccessRate: 0.9, AvgDurationMs: 30000,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/summary", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWorkflowAnalyticsSummary_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getWorkflowSummaryFn: func(_ context.Context, _ string, _, _ time.Time) (*store.WorkflowSummary, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("workflows/summary", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
