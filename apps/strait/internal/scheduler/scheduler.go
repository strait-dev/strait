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
	anomalyMonitor        *AnomalyMonitor
	usageFlusher          *UsageFlusher
	concurrentReconciler  *ConcurrentReconciler
	downgradeApplier      *DowngradeApplier
	costEstimateRefresher *CostEstimateRefresher
	memoryCleanup         *MemoryCleanup
	referralExpiry        *ReferralExpiry
	gracePeriodEnforcer   *GracePeriodEnforcer
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

// WithConcurrentReconciler enables periodic reconciliation of concurrent run counters.
func WithConcurrentReconciler(reconciler *ConcurrentReconciler) SchedulerOption {
	return func(s *Scheduler) {
		s.concurrentReconciler = reconciler
	}
}

// WithDowngradeApplier enables periodic application of pending plan downgrades.
func WithDowngradeApplier(applier *DowngradeApplier) SchedulerOption {
	return func(s *Scheduler) {
		s.downgradeApplier = applier
	}
}

// WithAnomalyMonitor sets an anomaly monitor for periodic cost anomaly detection.
func WithAnomalyMonitor(monitor *AnomalyMonitor) SchedulerOption {
	return func(s *Scheduler) {
		s.anomalyMonitor = monitor
	}
}

// WithUsageFlusher sets a usage flusher for periodic usage record materialization.
func WithUsageFlusher(flusher *UsageFlusher) SchedulerOption {
	return func(s *Scheduler) {
		s.usageFlusher = flusher
	}
}

// WithReferralExpiry enables periodic expiration of old referral credits.
func WithReferralExpiry(expiry *ReferralExpiry) SchedulerOption {
	return func(s *Scheduler) {
		s.referralExpiry = expiry
	}
}

// WithGracePeriodEnforcer enables periodic enforcement of expired payment grace periods.
func WithGracePeriodEnforcer(enforcer *GracePeriodEnforcer) SchedulerOption {
	return func(s *Scheduler) {
		s.gracePeriodEnforcer = enforcer
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
	if s.usageFlusher != nil {
		s.wg.Go(func() { s.usageFlusher.Run(ctx) })
	}
	if s.concurrentReconciler != nil {
		s.wg.Go(func() { s.concurrentReconciler.Run(ctx) })
	}
	if s.downgradeApplier != nil {
		s.wg.Go(func() { s.downgradeApplier.Run(ctx) })
	}
	if s.anomalyMonitor != nil {
		s.wg.Go(func() { s.anomalyMonitor.Run(ctx) })
	}
	if s.referralExpiry != nil {
		s.wg.Go(func() { s.referralExpiry.Run(ctx) })
	}
	if s.gracePeriodEnforcer != nil {
		s.wg.Go(func() { s.gracePeriodEnforcer.Run(ctx) })
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
