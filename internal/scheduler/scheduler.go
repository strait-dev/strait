package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sourcegraph/conc"

	"strait/internal/config"
	"strait/internal/queue"
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
func New(cfg *config.Config, s SchedulerStore, q queue.Queue, wfCallback WorkflowCallback, wfTrigger WorkflowTrigger) *Scheduler {
	return &Scheduler{
		cron:   NewCronScheduler(s, q, wfTrigger),
		poller: NewDelayedPoller(s, cfg.PollerInterval),
		reaper: NewReaper(s, cfg.ReaperInterval, cfg.StaleThreshold, cfg.RunRetentionShort, cfg.RunRetentionLong, cfg.FFRunRetention, wfCallback).
			WithWorkflowRetention(cfg.WorkflowRetention).
			WithDeleteBatchSize(cfg.ReaperDeleteBatchSize),
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
