package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
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
