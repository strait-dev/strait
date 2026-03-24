package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleGetWorkflowRunTimeline_Success(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	start := now.Add(-10 * time.Second)
	mid := now.Add(-5 * time.Second)
	end := now.Add(-1 * time.Second)

	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:         "wr-1",
				Status:     domain.WfStatusCompleted,
				StartedAt:  &start,
				FinishedAt: &end,
			}, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{
					ID:         "sr-1",
					StepRef:    "step-a",
					Status:     domain.StepCompleted,
					StartedAt:  &start,
					FinishedAt: &mid,
				},
				{
					ID:         "sr-2",
					StepRef:    "step-b",
					Status:     domain.StepCompleted,
					StartedAt:  &mid,
					FinishedAt: &end,
				},
			}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/timeline", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.TimelineResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.WorkflowRunID != "wr-1" {
		t.Fatalf("workflow_run_id = %q, want wr-1", resp.WorkflowRunID)
	}
	if resp.Status != "completed" {
		t.Fatalf("status = %q, want completed", resp.Status)
	}
	if len(resp.Steps) != 2 {
		t.Fatalf("steps count = %d, want 2", len(resp.Steps))
	}
	if resp.Steps[0].StepRef != "step-a" {
		t.Fatalf("first step ref = %q, want step-a", resp.Steps[0].StepRef)
	}
	if resp.Steps[0].DurationMs <= 0 {
		t.Fatalf("first step duration_ms = %d, want > 0", resp.Steps[0].DurationMs)
	}
	if resp.TotalMs <= 0 {
		t.Fatalf("total_ms = %d, want > 0", resp.TotalMs)
	}
}

func TestHandleGetWorkflowRunTimeline_ParallelDetection(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	start := now.Add(-10 * time.Second)
	mid := now.Add(-5 * time.Second)
	end := now.Add(-1 * time.Second)

	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:         "wr-1",
				Status:     domain.WfStatusCompleted,
				StartedAt:  &start,
				FinishedAt: &end,
			}, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			// Two overlapping steps (parallel).
			return []domain.WorkflowStepRun{
				{
					ID:         "sr-1",
					StepRef:    "step-a",
					Status:     domain.StepCompleted,
					StartedAt:  &start,
					FinishedAt: &mid,
				},
				{
					ID:         "sr-2",
					StepRef:    "step-b",
					Status:     domain.StepCompleted,
					StartedAt:  &start,
					FinishedAt: &end,
				},
			}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/timeline", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.TimelineResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Both steps should be parallel with each other.
	if len(resp.Steps[0].ParallelWith) != 1 || resp.Steps[0].ParallelWith[0] != "step-b" {
		t.Fatalf("step-a parallel_with = %v, want [step-b]", resp.Steps[0].ParallelWith)
	}
	if len(resp.Steps[1].ParallelWith) != 1 || resp.Steps[1].ParallelWith[0] != "step-a" {
		t.Fatalf("step-b parallel_with = %v, want [step-a]", resp.Steps[1].ParallelWith)
	}
}

func TestHandleGetWorkflowRunTimeline_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return nil, store.ErrWorkflowRunNotFound
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-missing/timeline", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetWorkflowRunTimeline_EmptySteps(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:        "wr-1",
				Status:    domain.WfStatusRunning,
				StartedAt: &now,
			}, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/timeline", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.TimelineResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Steps) != 0 {
		t.Fatalf("steps count = %d, want 0", len(resp.Steps))
	}
}
