package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/sourcegraph/conc"

	"strait/internal/config"
	"strait/internal/queue"
	"strait/internal/telemetry"
)

// SchedulerStore combines the store interfaces required by all scheduler components.
type SchedulerStore interface {
	CronStore
	PollerStore
	ReaperStore
	IndexMaintenanceStore
}

type Scheduler struct {
	cron   *CronScheduler
	poller *DelayedPoller
	reaper *Reaper
	index  *IndexMaintainer
	wg     conc.WaitGroup
}

// New creates a new scheduler that runs the cron, poller, and reaper.
func New(cfg *config.Config, s SchedulerStore, q queue.Queue, wfCallback WorkflowCallback, wfTrigger WorkflowTrigger, opts ...SchedulerOption) *Scheduler {
	sched := &Scheduler{
		cron:   NewCronScheduler(s, q, wfTrigger),
		poller: NewDelayedPoller(s, cfg.PollerInterval),
		reaper: NewReaper(s, cfg.ReaperInterval, cfg.StaleThreshold, cfg.RunRetentionShort, cfg.RunRetentionLong, cfg.FFRunRetention, wfCallback).
			WithWorkflowRetention(cfg.WorkflowRetention).
			WithEventTriggerRetention(cfg.EventTriggerRetention).
			WithDeleteBatchSize(cfg.ReaperDeleteBatchSize),
	}
	if cfg.IndexMaintenanceInterval > 0 {
		sched.index = NewIndexMaintainer(s, cfg.IndexMaintenanceInterval)
	}
	for _, opt := range opts {
		opt(sched)
	}
	return sched
}

// SchedulerOption configures a Scheduler.
type SchedulerOption func(*Scheduler)

// WithSchedulerMetrics attaches telemetry metrics to the reaper.
func WithSchedulerMetrics(m *telemetry.Metrics) SchedulerOption {
	return func(s *Scheduler) {
		s.cron.WithMetrics(m)
		s.poller.WithMetrics(m)
		s.reaper.WithMetrics(m)
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.cron.LoadJobs(ctx); err != nil {
		return fmt.Errorf("load cron jobs: %w", err)
	}

	s.cron.Start()
	s.wg.Go(func() { s.poller.Run(ctx) })
	s.wg.Go(func() { s.reaper.Run(ctx) })
	if s.index != nil {
		s.wg.Go(func() { s.index.Run(ctx) })
	}

	slog.Info("scheduler started")
	return nil
}

func (s *Scheduler) Stop() {
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	s.wg.Wait()
	slog.Info("scheduler stopped")
}

func (s *Scheduler) PollerLastTick() time.Time {
	if s.poller == nil {
		return time.Time{}
	}
	return s.poller.LastTick()
}
