package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/domain"
)

func TestNewCronScheduler(t *testing.T) {
	cs := NewCronScheduler(&mockCronStore{}, &mockQueue{})
	if cs == nil {
		t.Fatal("expected scheduler to be non-nil")
	}
}

func TestCronScheduler_LoadJobs_Success(t *testing.T) {
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{
				{ID: "job-1", ProjectID: "proj-1", Cron: "* * * * *"},
				{ID: "job-2", ProjectID: "proj-2", Cron: "* * * * *"},
			}, nil
		},
	}

	cs := NewCronScheduler(store, &mockQueue{})
	err := cs.LoadJobs(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCronScheduler_LoadJobs_NoJobs(t *testing.T) {
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{}, nil
		},
	}

	cs := NewCronScheduler(store, &mockQueue{})
	err := cs.LoadJobs(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCronScheduler_LoadJobs_StoreError(t *testing.T) {
	storeErr := errors.New("store error")
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return nil, storeErr
		},
	}

	cs := NewCronScheduler(store, &mockQueue{})
	err := cs.LoadJobs(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "list cron jobs") {
		t.Fatalf("expected wrapped list error, got %v", err)
	}
}

func TestCronScheduler_LoadJobs_InvalidCron(t *testing.T) {
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{{ID: "job-1", ProjectID: "proj-1", Cron: "bad"}}, nil
		},
	}

	cs := NewCronScheduler(store, &mockQueue{})
	err := cs.LoadJobs(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "register cron job job-1") {
		t.Fatalf("expected register cron job error, got %v", err)
	}
}

func TestCronScheduler_StartStop(t *testing.T) {
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{{ID: "job-1", ProjectID: "proj-1", Cron: "* * * * *"}}, nil
		},
	}

	cs := NewCronScheduler(store, &mockQueue{})
	if err := cs.LoadJobs(context.Background()); err != nil {
		t.Fatalf("load jobs failed: %v", err)
	}

	cs.Start()
	stopCtx := cs.Stop()
	<-stopCtx.Done()
}

func TestCronScheduler_TriggerJob(t *testing.T) {
	var enqueued domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = *run
			return nil
		},
	}

	cs := NewCronScheduler(&mockCronStore{}, q)
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
