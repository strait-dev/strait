package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"orchestrator/internal/config"
	"orchestrator/internal/queue"
	"orchestrator/internal/store"
)

type Scheduler struct {
	cron   *CronScheduler
	poller *DelayedPoller
	reaper *Reaper
	wg     sync.WaitGroup
}

func New(cfg *config.Config, s store.Store, q queue.Queue) *Scheduler {
	return &Scheduler{
		cron:   NewCronScheduler(s, q),
		poller: NewDelayedPoller(s, cfg.PollerInterval),
		reaper: NewReaper(s, cfg.ReaperInterval, cfg.StaleThreshold),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.cron.LoadJobs(ctx); err != nil {
		return fmt.Errorf("load cron jobs: %w", err)
	}

	s.cron.Start()
	s.wg.Add(2)
	go func() { defer s.wg.Done(); s.poller.Run(ctx) }()
	go func() { defer s.wg.Done(); s.reaper.Run(ctx) }()

	slog.Info("scheduler started")
	return nil
}

func (s *Scheduler) Stop() {
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	s.wg.Wait()
	slog.Info("scheduler stopped")
}
