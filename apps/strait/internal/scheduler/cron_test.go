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
	job := domain.Job{ID: "job-1", ProjectID: "proj-1"}
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
	var stoppedMachines []string
	childCancelCalled := false
	wfCallbackCalled := false

	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, jobID string, reason string) ([]store.CancelledRun, error) {
			if jobID != "job-1" {
				t.Fatalf("unexpected job_id %q", jobID)
			}
			cancelCalled = true
			cancelReason = reason
			return []store.CancelledRun{
				{ID: "run-1", MachineID: "mach-1", ExecutionMode: domain.ExecutionModeManaged},
				{ID: "run-2", MachineID: "", ExecutionMode: domain.ExecutionModeHTTP},
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
	stopper := &mockMachineStopper{
		stopFn: func(_ context.Context, machineID string) error {
			stoppedMachines = append(stoppedMachines, machineID)
			return nil
		},
	}
	wfCb := &mockWorkflowCallback{
		onJobRunTerminalFn: func(_ context.Context, run *domain.JobRun) error {
			wfCallbackCalled = true
			return nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil).
		WithMachineStopper(stopper).
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
	// Only the managed run with a machine ID should be stopped.
	if len(stoppedMachines) != 1 || stoppedMachines[0] != "mach-1" {
		t.Fatalf("expected [mach-1] stopped, got %v", stoppedMachines)
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
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CancelledRun, error) {
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

func TestCronScheduler_TriggerJob_OverlapPolicyCancelRunning_NoManagedRuns(t *testing.T) {
	t.Parallel()
	enqueued := false
	stopCalled := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CancelledRun, error) {
			return []store.CancelledRun{
				{ID: "run-1", MachineID: "", ExecutionMode: domain.ExecutionModeHTTP},
			}, nil
		},
	}
	stopper := &mockMachineStopper{
		stopFn: func(_ context.Context, _ string) error {
			stopCalled = true
			return nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil).WithMachineStopper(stopper)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)

	if !enqueued {
		t.Fatal("expected run to be enqueued")
	}
	if stopCalled {
		t.Fatal("expected no container stop for HTTP-only runs")
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
