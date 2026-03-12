package scheduler

import (
	"context"
	"fmt"
	"log/slog"

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
}

type Scheduler struct {
	cron   *CronScheduler
	poller *DelayedPoller
	reaper *Reaper
	wg     conc.WaitGroup
}

// New creates a new scheduler that runs the cron, poller, and reaper.
func New(ctx context.Context, cfg *config.Config, s SchedulerStore, q queue.Queue, wfCallback WorkflowCallback, wfTrigger WorkflowTrigger, opts ...SchedulerOption) *Scheduler {
	sched := &Scheduler{
		cron:   NewCronScheduler(ctx, s, q, wfTrigger),
		poller: NewDelayedPoller(s, slog.Default(), cfg.PollerInterval),
		reaper: NewReaper(s, cfg.ReaperInterval, cfg.StaleThreshold, cfg.RunRetentionShort, cfg.RunRetentionLong, cfg.FFRunRetention, wfCallback).
			WithWorkflowRetention(cfg.WorkflowRetention).
			WithEventTriggerRetention(cfg.EventTriggerRetention).
			WithDeleteBatchSize(cfg.ReaperDeleteBatchSize).
			WithStalledThreshold(cfg.StalledWorkflowThreshold).
			WithStalledAction(cfg.StalledWorkflowAction),
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

	slog.Info("scheduler started")
	return nil
}

func (s *Scheduler) Stop() {
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	s.wg.Wait()
	slog.Info("scheduler stopped")
}
