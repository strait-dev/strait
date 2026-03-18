package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sourcegraph/conc"

	"time"

	"strait/internal/config"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/telemetry"
)

// SchedulerStore combines the store interfaces required by all scheduler components.
type SchedulerStore interface {
	CronStore
	PollerStore
	ReaperStore
	IndexMaintenanceStore
	StatsAggregatorStore
	CostEstimateRefresherStore
	MemoryCleanupStore
	store.DebounceStore
	store.BatchStore
	store.RunComputeUsageStore
}

type Scheduler struct {
	cron                  *CronScheduler
	poller                *DelayedPoller
	reaper                *Reaper
	indexMaintainer       *IndexMaintainer
	debouncePoller        *DebouncePoller
	batchFlusher          *BatchFlusher
	statsAggregator       *StatsAggregator
	budgetMonitor         *BudgetMonitor
	costEstimateRefresher *CostEstimateRefresher
	memoryCleanup         *MemoryCleanup
	wg                    conc.WaitGroup
}

// New creates a new scheduler that runs the cron, poller, and reaper.
func New(ctx context.Context, cfg *config.Config, s SchedulerStore, q queue.Queue, wfCallback WorkflowCallback, wfTrigger WorkflowTrigger, opts ...SchedulerOption) *Scheduler {
	sched := &Scheduler{
		cron:   NewCronScheduler(ctx, s, q, wfTrigger).WithDefaultRunTTLSecs(cfg.DefaultRunTTLSecs),
		poller: NewDelayedPoller(s, slog.Default(), cfg.PollerInterval),
		reaper: NewReaper(s, cfg.ReaperInterval, cfg.StaleThreshold, cfg.RunRetentionShort, cfg.RunRetentionLong, true, wfCallback).
			WithWorkflowRetention(cfg.WorkflowRetention).
			WithEventTriggerRetention(cfg.EventTriggerRetention).
			WithDeleteBatchSize(cfg.ReaperDeleteBatchSize).
			WithStalledThreshold(cfg.StalledWorkflowThreshold).
			WithStalledAction(cfg.StalledWorkflowAction),
		indexMaintainer:       NewIndexMaintainer(s, cfg.IndexMaintenanceInterval),
		debouncePoller:        NewDebouncePoller(s, q, cfg.DebouncePollerInterval),
		batchFlusher:          NewBatchFlusher(s, q, cfg.BatchFlushInterval),
		statsAggregator:       NewStatsAggregator(s),
		budgetMonitor:         NewBudgetMonitor(s, nil, 5*time.Minute),
		costEstimateRefresher: NewCostEstimateRefresher(s, time.Hour),
		memoryCleanup:         NewMemoryCleanup(s, 5*time.Minute),
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

// WithBudgetWebhookEnqueuer sets the webhook enqueuer for the budget monitor.
func WithBudgetWebhookEnqueuer(enqueuer BudgetMonitorWebhookEnqueuer) SchedulerOption {
	return func(s *Scheduler) {
		s.budgetMonitor.enqueuer = enqueuer
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.cron.LoadJobs(ctx); err != nil {
		return fmt.Errorf("load cron jobs: %w", err)
	}

	s.cron.Start()
	s.wg.Go(func() { s.poller.Run(ctx) })
	s.wg.Go(func() { s.reaper.Run(ctx) })
	s.wg.Go(func() { s.indexMaintainer.Run(ctx) })
	s.wg.Go(func() { s.debouncePoller.Run(ctx) })
	s.wg.Go(func() { s.batchFlusher.Run(ctx) })
	s.wg.Go(func() { s.statsAggregator.Run(ctx) })
	s.wg.Go(func() { s.budgetMonitor.Run(ctx) })
	s.wg.Go(func() { s.costEstimateRefresher.Run(ctx) })
	s.wg.Go(func() { s.memoryCleanup.Run(ctx) })

	slog.Info("scheduler started")
	return nil
}

func (s *Scheduler) Stop() {
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	s.wg.Wait()
	slog.Info("scheduler stopped")
}
