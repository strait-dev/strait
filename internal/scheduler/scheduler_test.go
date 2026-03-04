package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/config"
	"orchestrator/internal/domain"
)

type mockSchedulerStore struct {
	cron   *mockCronStore
	poller *mockPollerStore
	reaper *mockReaperStore
}

func (m *mockSchedulerStore) ListCronJobs(ctx context.Context) ([]domain.Job, error) {
	return m.cron.ListCronJobs(ctx)
}

func (m *mockSchedulerStore) ListDueRuns(ctx context.Context) ([]domain.JobRun, error) {
	return m.poller.ListDueRuns(ctx)
}

func (m *mockSchedulerStore) ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	return m.reaper.ListStaleRuns(ctx, threshold)
}

func (m *mockSchedulerStore) ListExpiredRuns(ctx context.Context) ([]domain.JobRun, error) {
	return m.reaper.ListExpiredRuns(ctx)
}

func (m *mockSchedulerStore) ListStaleDequeued(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	return m.reaper.ListStaleDequeued(ctx, threshold)
}

func (m *mockSchedulerStore) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	return m.poller.UpdateRunStatus(ctx, id, from, to, fields)
}

func testSchedulerConfig() *config.Config {
	return &config.Config{
		PollerInterval: 100 * time.Millisecond,
		ReaperInterval: 100 * time.Millisecond,
		StaleThreshold: 30 * time.Second,
	}
}

func TestScheduler_New(t *testing.T) {
	store := &mockSchedulerStore{
		cron:   &mockCronStore{},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
	}

	s := New(testSchedulerConfig(), store, &mockQueue{}, nil)
	if s == nil {
		t.Fatal("expected scheduler to be non-nil")
	}
}

func TestScheduler_Start_Success(t *testing.T) {
	store := &mockSchedulerStore{
		cron: &mockCronStore{
			listCronJobsFn: func(context.Context) ([]domain.Job, error) { return []domain.Job{}, nil },
		},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
	}

	s := New(testSchedulerConfig(), store, &mockQueue{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	cancel()
	s.Stop()
}

func TestScheduler_Start_LoadJobsError(t *testing.T) {
	storeErr := errors.New("list failed")
	store := &mockSchedulerStore{
		cron: &mockCronStore{
			listCronJobsFn: func(context.Context) ([]domain.Job, error) { return nil, storeErr },
		},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
	}

	s := New(testSchedulerConfig(), store, &mockQueue{}, nil)
	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "load cron jobs") {
		t.Fatalf("expected load cron jobs error, got %v", err)
	}
}

func TestScheduler_Stop(t *testing.T) {
	store := &mockSchedulerStore{
		cron: &mockCronStore{
			listCronJobsFn: func(context.Context) ([]domain.Job, error) { return []domain.Job{}, nil },
		},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
	}

	s := New(testSchedulerConfig(), store, &mockQueue{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	cancel()
	s.Stop()
}
