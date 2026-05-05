package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

type mockWorkflowTrigger struct {
	triggerWorkflowFn func(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride) (*domain.WorkflowRun, error)
}

func (m *mockWorkflowTrigger) TriggerWorkflow(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride, extraTags map[string]string) (*domain.WorkflowRun, error) {
	if m.triggerWorkflowFn != nil {
		return m.triggerWorkflowFn(ctx, workflowID, projectID, payload, triggeredBy, stepOverrides)
	}
	return nil, nil
}

func TestNewCronScheduler(t *testing.T) {
	t.Parallel()
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, &mockQueue{}, nil)
	if cs == nil {
		t.Fatal("expected scheduler to be non-nil")
	}
}

func TestCronScheduler_LoadJobs_Success(t *testing.T) {
	t.Parallel()
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{
				{ID: "job-1", ProjectID: "proj-1", Cron: "* * * * *"},
				{ID: "job-2", ProjectID: "proj-2", Cron: "* * * * *"},
			}, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)
	err := cs.LoadJobs(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCronScheduler_LoadJobs_NoJobs(t *testing.T) {
	t.Parallel()
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{}, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)
	err := cs.LoadJobs(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCronScheduler_LoadJobs_StoreError(t *testing.T) {
	t.Parallel()
	storeErr := errors.New("store error")
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return nil, storeErr
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)
	err := cs.LoadJobs(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "list cron jobs") {
		t.Fatalf("expected wrapped list error, got %v", err)
	}
}

func TestCronScheduler_LoadJobs_InvalidCron(t *testing.T) {
	t.Parallel()
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{{ID: "job-1", ProjectID: "proj-1", Cron: "bad"}}, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)
	err := cs.LoadJobs(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "register cron job job-1") {
		t.Fatalf("expected register cron job error, got %v", err)
	}
}

func TestCronScheduler_StartStop(t *testing.T) {
	t.Parallel()
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{{ID: "job-1", ProjectID: "proj-1", Cron: "* * * * *"}}, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)
	if err := cs.LoadJobs(context.Background()); err != nil {
		t.Fatalf("load jobs failed: %v", err)
	}

	cs.Start()
	stopCtx := cs.Stop()
	<-stopCtx.Done()
}

func TestCronScheduler_TriggerJob(t *testing.T) {
	t.Parallel()
	var enqueued domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = *run
			return nil
		},
	}

	cs := NewCronScheduler(context.Background(), &mockCronStore{}, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", ExecutionMode: domain.ExecutionModeWorker, Queue: "priority"}
	cs.triggerJob(context.Background(), job)

	if enqueued.JobID != job.ID {
		t.Fatalf("expected job id %q, got %q", job.ID, enqueued.JobID)
	}
	if enqueued.ProjectID != job.ProjectID {
		t.Fatalf("expected project id %q, got %q", job.ProjectID, enqueued.ProjectID)
	}
	if enqueued.TriggeredBy != "cron" {
		t.Fatalf("expected triggered_by cron, got %q", enqueued.TriggeredBy)
	}
	if enqueued.ExecutionMode != domain.ExecutionModeWorker {
		t.Fatalf("expected execution_mode worker, got %q", enqueued.ExecutionMode)
	}
	if enqueued.QueueName != "priority" {
		t.Fatalf("expected queue_name priority, got %q", enqueued.QueueName)
	}
}

func TestCronScheduler_TriggerJob_WithTTL(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, mq, nil)

	job := domain.Job{
		ID:         "job-ttl",
		ProjectID:  "proj-1",
		RunTTLSecs: 600,
	}
	cs.triggerJob(context.Background(), job)

	if capturedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if capturedRun.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	expected := time.Now().Add(600 * time.Second)
	diff := capturedRun.ExpiresAt.Sub(expected)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt diff = %v, want within 5s", diff)
	}
}

func TestCronScheduler_TriggerJob_NoTTL(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, mq, nil)

	job := domain.Job{
		ID:         "job-no-ttl",
		ProjectID:  "proj-1",
		RunTTLSecs: 0,
	}
	cs.triggerJob(context.Background(), job)

	if capturedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if capturedRun.ExpiresAt != nil {
		t.Fatalf("expected ExpiresAt to be nil, got %v", capturedRun.ExpiresAt)
	}
}

func TestCronScheduler_TriggerWorkflow_SkipIfRunning(t *testing.T) {
	t.Parallel()
	triggered := false
	cs := NewCronScheduler(context.Background(), &mockCronStore{
		countRunningWfRunsFn: func(_ context.Context, workflowID string) (int, error) {
			if workflowID != "wf-1" {
				t.Fatalf("workflowID = %q, want wf-1", workflowID)
			}
			return 1, nil
		},
	}, &mockQueue{}, &mockWorkflowTrigger{
		triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			triggered = true
			return &domain.WorkflowRun{ID: "wr-1"}, nil
		},
	})

	cs.triggerWorkflow(context.Background(), domain.Workflow{ID: "wf-1", ProjectID: "proj-1", SkipIfRunning: true})

	if triggered {
		t.Fatal("expected workflow cron trigger to be skipped")
	}
}

func TestCronScheduler_TriggerWorkflow_Success(t *testing.T) {
	t.Parallel()
	triggered := false
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, &mockQueue{}, &mockWorkflowTrigger{
		triggerWorkflowFn: func(_ context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			if workflowID != "wf-1" || projectID != "proj-1" {
				t.Fatalf("unexpected trigger args: %s %s", workflowID, projectID)
			}
			if payload != nil {
				t.Fatalf("expected nil payload, got %s", string(payload))
			}
			if triggeredBy != domain.TriggerCron {
				t.Fatalf("triggeredBy = %q, want %q", triggeredBy, domain.TriggerCron)
			}
			triggered = true
			return &domain.WorkflowRun{ID: "wr-1"}, nil
		},
	})

	cs.triggerWorkflow(context.Background(), domain.Workflow{ID: "wf-1", ProjectID: "proj-1"})

	if !triggered {
		t.Fatal("expected workflow cron trigger to run")
	}
}

func TestCronScheduler_TriggerJob_OverlapPolicyAllow(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	// CountActiveRunsForJob returns active runs, but allow policy should ignore that.
	store := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, _ string) (int, error) {
			return 5, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyAllow}
	cs.triggerJob(context.Background(), job)

	if !enqueued {
		t.Fatal("expected run to be enqueued with allow policy")
	}
}

func TestCronScheduler_TriggerJob_OverlapPolicySkip_ActiveRuns(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	store := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, jobID string) (int, error) {
			if jobID != "job-1" {
				t.Fatalf("unexpected job_id %q", jobID)
			}
			return 2, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicySkip}
	cs.triggerJob(context.Background(), job)

	if enqueued {
		t.Fatal("expected run to be skipped when active runs exist")
	}
}

func TestCronScheduler_TriggerJob_OverlapPolicySkip_NoActive(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	store := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicySkip}
	cs.triggerJob(context.Background(), job)

	if !enqueued {
		t.Fatal("expected run to be enqueued when no active runs")
	}
}

func TestCronScheduler_TriggerJob_OverlapPolicySkip_CountError(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	store := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, _ string) (int, error) {
			return 0, errors.New("db error")
		},
	}

	cs := NewCronScheduler(context.Background(), store, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicySkip}
	cs.triggerJob(context.Background(), job)

	if enqueued {
		t.Fatal("expected run not to be enqueued on count error")
	}
}

func TestCronScheduler_TriggerJob_OverlapPolicyCancelRunning(t *testing.T) {
	t.Parallel()
	enqueued := false
	cancelCalled := false
	var cancelReason string
	childCancelCalled := false
	wfCallbackCalled := false

	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, jobID string, reason string) ([]store.CanceledRun, error) {
			if jobID != "job-1" {
				t.Fatalf("unexpected job_id %q", jobID)
			}
			cancelCalled = true
			cancelReason = reason
			return []store.CanceledRun{
				{ID: "run-1", ExecutionMode: domain.ExecutionModeWorker},
				{ID: "run-2", ExecutionMode: domain.ExecutionModeHTTP},
			}, nil
		},
		cancelChildRunsByParentIDFn: func(_ context.Context, parentIDs []string, _ time.Time, _ string) (int64, error) {
			childCancelCalled = true
			if len(parentIDs) != 2 {
				t.Fatalf("expected 2 parent IDs, got %d", len(parentIDs))
			}
			return 1, nil
		},
	}
	wfCb := &mockWorkflowCallback{
		onJobRunTerminalFn: func(_ context.Context, run *domain.JobRun) error {
			wfCallbackCalled = true
			return nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil).
		WithWorkflowCallback(wfCb)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)

	if !cancelCalled {
		t.Fatal("expected CancelActiveRunsForJob to be called")
	}
	if !strings.Contains(cancelReason, "cancel_running") {
		t.Fatalf("expected reason to contain cancel_running, got %q", cancelReason)
	}
	if !enqueued {
		t.Fatal("expected run to be enqueued after canceling active runs")
	}
	if !childCancelCalled {
		t.Fatal("expected CancelChildRunsByParentIDs to be called")
	}
	if !wfCallbackCalled {
		t.Fatal("expected workflow callback to be called")
	}
}

func TestCronScheduler_TriggerJob_OverlapPolicyCancelRunning_CancelError(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return nil, errors.New("cancel failed")
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)

	if enqueued {
		t.Fatal("expected run not to be enqueued on cancel error")
	}
}

func TestCronScheduler_TriggerJob_OverlapPolicyDefault(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}

	cs := NewCronScheduler(context.Background(), &mockCronStore{}, q, nil)
	// Empty CronOverlapPolicy should behave like allow.
	job := domain.Job{ID: "job-1", ProjectID: "proj-1"}
	cs.triggerJob(context.Background(), job)

	if !enqueued {
		t.Fatal("expected run to be enqueued with empty/default policy")
	}
}

// Adversarial and edge-case tests for cron overlap policy.

func TestCronScheduler_TriggerJob_UnknownPolicyBehavesLikeAllow(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	// A garbage policy value that doesn't match any constant should fall
	// through the default case and behave like allow.
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, q, nil)
	job := domain.Job{
		ID:                "job-1",
		ProjectID:         "proj-1",
		CronOverlapPolicy: domain.CronOverlapPolicy("bogus_value"),
	}
	cs.triggerJob(context.Background(), job)

	if !enqueued {
		t.Fatal("unknown policy should behave like allow and enqueue")
	}
}

func TestCronScheduler_TriggerJob_CancelRunning_EmptyResultStillEnqueues(t *testing.T) {
	t.Parallel()
	// CancelActiveRunsForJob returns an empty slice (no active runs to cancel).
	// The new run should still be enqueued.
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{}, nil // empty, not nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)

	if !enqueued {
		t.Fatal("cancel_running with zero active runs should still enqueue")
	}
}

func TestCronScheduler_TriggerJob_CancelRunning_NilResultStillEnqueues(t *testing.T) {
	t.Parallel()
	// CancelActiveRunsForJob returns nil slice (no active runs).
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return nil, nil // nil slice
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)

	if !enqueued {
		t.Fatal("cancel_running with nil result should still enqueue")
	}
}

func TestCronScheduler_TriggerJob_CancelRunning_ChildCancelErrorDoesNotPreventEnqueue(t *testing.T) {
	t.Parallel()
	// Child cancel fails, but the run should still be enqueued.
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{
				{ID: "run-1", ExecutionMode: domain.ExecutionModeHTTP},
			}, nil
		},
		cancelChildRunsByParentIDFn: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, errors.New("child cancel db error")
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)

	if !enqueued {
		t.Fatal("child cancel error should not prevent new run from being enqueued")
	}
}

func TestCronScheduler_TriggerJob_CancelRunning_WorkflowCallbackErrorDoesNotPreventEnqueue(t *testing.T) {
	t.Parallel()
	// Workflow callback fails, but the run should still be enqueued.
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{
				{ID: "run-1", ExecutionMode: domain.ExecutionModeHTTP},
			}, nil
		},
	}
	wfCb := &mockWorkflowCallback{
		onJobRunTerminalFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("workflow engine down")
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil).WithWorkflowCallback(wfCb)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)

	if !enqueued {
		t.Fatal("workflow callback error should not prevent new run from being enqueued")
	}
}

func TestCronScheduler_TriggerJob_CancelRunning_NilDependenciesAreGraceful(t *testing.T) {
	t.Parallel()
	// No workflowCallback set. Should not panic.
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{
				{ID: "run-1", ExecutionMode: domain.ExecutionModeWorker},
			}, nil
		},
	}

	// Intentionally no WithWorkflowCallback.
	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)

	if !enqueued {
		t.Fatal("should enqueue even without optional dependencies")
	}
}

func TestCronScheduler_TriggerJob_Skip_EnqueueError(t *testing.T) {
	t.Parallel()
	// skip policy, no active runs, but enqueue fails. Should not panic.
	s := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("queue full")
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicySkip}
	// Should not panic.
	cs.triggerJob(context.Background(), job)
}

func TestCronScheduler_TriggerJob_CancelRunning_EnqueueError(t *testing.T) {
	t.Parallel()
	// cancel_running succeeds, but enqueue fails. Should not panic.
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{
				{ID: "run-1", ExecutionMode: domain.ExecutionModeHTTP},
			}, nil
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("queue full")
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	// Should not panic. Runs are already canceled but new run fails to enqueue.
	cs.triggerJob(context.Background(), job)
}

func TestCronOverlapPolicy_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		policy domain.CronOverlapPolicy
		valid  bool
	}{
		{domain.OverlapPolicyAllow, true},
		{domain.OverlapPolicySkip, true},
		{domain.OverlapPolicyCancelRunning, true},
		{"", false},
		{"queue", false},
		{"ALLOW", false},
		{"cancel_Running", false},
		{"skip ", false},
		{" allow", false},
	}
	for _, tt := range tests {
		if got := tt.policy.IsValid(); got != tt.valid {
			t.Errorf("CronOverlapPolicy(%q).IsValid() = %v, want %v", tt.policy, got, tt.valid)
		}
	}
}

func TestCronScheduler_TriggerJob_CancelRunning_CorrectRunFields(t *testing.T) {
	t.Parallel()
	// Verify the enqueued run has the correct fields set.
	var capturedRun *domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return nil, nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{
		ID:                "job-1",
		ProjectID:         "proj-1",
		CronOverlapPolicy: domain.OverlapPolicyCancelRunning,
		Tags:              map[string]string{"env": "prod"},
		Version:           3,
		VersionID:         "ver_abc",
	}
	cs.triggerJob(context.Background(), job)

	if capturedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if capturedRun.JobID != "job-1" {
		t.Errorf("JobID = %q, want job-1", capturedRun.JobID)
	}
	if capturedRun.ProjectID != "proj-1" {
		t.Errorf("ProjectID = %q, want proj-1", capturedRun.ProjectID)
	}
	if capturedRun.TriggeredBy != domain.TriggerCron {
		t.Errorf("TriggeredBy = %q, want %q", capturedRun.TriggeredBy, domain.TriggerCron)
	}
	if capturedRun.CreatedBy != "system:cron" {
		t.Errorf("CreatedBy = %q, want system:cron", capturedRun.CreatedBy)
	}
	if capturedRun.JobVersion != 3 {
		t.Errorf("JobVersion = %d, want 3", capturedRun.JobVersion)
	}
	if capturedRun.JobVersionID != "ver_abc" {
		t.Errorf("JobVersionID = %q, want ver_abc", capturedRun.JobVersionID)
	}
	if capturedRun.Tags["env"] != "prod" {
		t.Errorf("Tags[env] = %q, want prod", capturedRun.Tags["env"])
	}
}

func TestCronScheduler_TriggerJob_CancelRunning_ChildCancelReceivesCorrectParentIDs(t *testing.T) {
	t.Parallel()
	var capturedParentIDs []string
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{
				{ID: "run-aaa"},
				{ID: "run-bbb"},
				{ID: "run-ccc"},
			}, nil
		},
		cancelChildRunsByParentIDFn: func(_ context.Context, parentIDs []string, _ time.Time, reason string) (int64, error) {
			capturedParentIDs = parentIDs
			if !strings.Contains(reason, "cron overlap policy") {
				t.Errorf("child cancel reason = %q, want to contain 'cron overlap policy'", reason)
			}
			return 2, nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)

	if !enqueued {
		t.Fatal("expected enqueue")
	}
	if len(capturedParentIDs) != 3 {
		t.Fatalf("expected 3 parent IDs, got %d", len(capturedParentIDs))
	}
	expected := []string{"run-aaa", "run-bbb", "run-ccc"}
	for i, id := range capturedParentIDs {
		if id != expected[i] {
			t.Errorf("parentIDs[%d] = %q, want %q", i, id, expected[i])
		}
	}
}

func FuzzCronExpression(f *testing.F) {
	f.Add("*/5 * * * *")
	f.Add("0 0 * * *")
	f.Add("0 12 * * MON-FRI")
	f.Add("")
	f.Add("not a cron")
	f.Add("* * * * * *")

	f.Fuzz(func(t *testing.T, expr string) {
		cs := NewCronScheduler(context.Background(), nil, nil, nil)
		_, _ = cs.cron.AddFunc(expr, func() {})
	})
}

func BenchmarkCronParse(b *testing.B) {
	cs := NewCronScheduler(context.Background(), nil, nil, nil)
	for b.Loop() {
		_, _ = cs.cron.AddFunc("*/5 * * * *", func() {})
	}
}
