package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestSDKGuardrails_Iteration_Success(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{ID: "job-1", MaxIterationsPerRun: 100}
	quota := &store.ProjectQuota{ProjectID: "proj-1", MaxIterationsPerRun: 50}

	var iterCreated bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return quota, nil
		},
		CountRunIterationsFunc: func(_ context.Context, _ string) (int, error) {
			return 5, nil
		},
		CreateRunIterationFunc: func(_ context.Context, iter *domain.RunIteration) error {
			iterCreated = true
			if iter.RunID != "run-1" {
				t.Fatalf("expected run id run-1, got %s", iter.RunID)
			}
			if iter.Iteration != 6 {
				t.Fatalf("expected iteration 6, got %d", iter.Iteration)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/iteration", "run-1", `{"iteration":6,"description":"processing batch"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !iterCreated {
		t.Fatal("expected CreateRunIteration to be called")
	}
}

func TestSDKGuardrails_Iteration_AtLimit(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{ID: "job-1", MaxIterationsPerRun: 10}

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1"}, nil
		},
		CountRunIterationsFunc: func(_ context.Context, _ string) (int, error) {
			return 10, nil // at the limit
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/iteration", "run-1", `{"iteration":11}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["error"] != "iteration_limit_exceeded" {
		t.Fatalf("expected iteration_limit_exceeded error, got %v", body["error"])
	}
	if int(body["limit"].(float64)) != 10 {
		t.Fatalf("expected limit=10, got %v", body["limit"])
	}
}

func TestSDKGuardrails_Iteration_NoLimitConfigured(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	// No limits set on job or quota.
	job := &domain.Job{ID: "job-1"}
	quota := &store.ProjectQuota{ProjectID: "proj-1"}

	var iterCreated bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return quota, nil
		},
		CreateRunIterationFunc: func(_ context.Context, _ *domain.RunIteration) error {
			iterCreated = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/iteration", "run-1", `{"iteration":999}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with no limit, got %d: %s", w.Code, w.Body.String())
	}
	if !iterCreated {
		t.Fatal("expected CreateRunIteration to be called")
	}
}

func TestSDKGuardrails_Iteration_JobLimitOverridesQuota(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	// Job limit is 5, quota limit is 100. Job limit should take precedence.
	job := &domain.Job{ID: "job-1", MaxIterationsPerRun: 5}
	quota := &store.ProjectQuota{ProjectID: "proj-1", MaxIterationsPerRun: 100}

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return quota, nil
		},
		CountRunIterationsFunc: func(_ context.Context, _ string) (int, error) {
			return 5, nil // at job limit of 5
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/iteration", "run-1", `{"iteration":6}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSDKGuardrails_Iteration_QuotaLimitUsedWhenNoJobLimit(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	// No job limit, quota limit is 3.
	job := &domain.Job{ID: "job-1"}
	quota := &store.ProjectQuota{ProjectID: "proj-1", MaxIterationsPerRun: 3}

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return quota, nil
		},
		CountRunIterationsFunc: func(_ context.Context, _ string) (int, error) {
			return 3, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/iteration", "run-1", `{"iteration":4}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 from quota limit, got %d: %s", w.Code, w.Body.String())
	}
}

func TestResolveGuardrailInt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		quotaLimit int
		jobLimit   int
		want       int
	}{
		{"job limit takes precedence", 100, 5, 5},
		{"quota limit when no job limit", 50, 0, 50},
		{"zero when both zero", 0, 0, 0},
		{"job limit only", 0, 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveGuardrailInt(tt.quotaLimit, tt.jobLimit)
			if got != tt.want {
				t.Fatalf("resolveGuardrailInt(%d, %d) = %d, want %d", tt.quotaLimit, tt.jobLimit, got, tt.want)
			}
		})
	}
}

func TestResolveGuardrailInt64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		quotaLimit int64
		jobLimit   int64
		want       int64
	}{
		{"job limit takes precedence", 100, 5, 5},
		{"quota limit when no job limit", 50, 0, 50},
		{"zero when both zero", 0, 0, 0},
		{"job limit only", 0, 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveGuardrailInt64(tt.quotaLimit, tt.jobLimit)
			if got != tt.want {
				t.Fatalf("resolveGuardrailInt64(%d, %d) = %d, want %d", tt.quotaLimit, tt.jobLimit, got, tt.want)
			}
		})
	}
}
