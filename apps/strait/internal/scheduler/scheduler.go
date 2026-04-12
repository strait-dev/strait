package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sourcegraph/conc"

	"time"

	"strait/internal/clickhouse"
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
	cron                     *CronScheduler
	poller                   *DelayedPoller
	reaper                   *Reaper
	indexMaintainer          *IndexMaintainer
	debouncePoller           *DebouncePoller
	batchFlusher             *BatchFlusher
	statsAggregator          *StatsAggregator
	budgetMonitor            *BudgetMonitor
	anomalyMonitor           *AnomalyMonitor
	usageFlusher             *UsageFlusher
	concurrentReconciler     *ConcurrentReconciler
	downgradeApplier         *DowngradeApplier
	costEstimateRefresher    *CostEstimateRefresher
	memoryCleanup            *MemoryCleanup
	gracePeriodEnforcer      *GracePeriodEnforcer
	staleSubscriptionChecker *StaleSubscriptionChecker
	webhookMessageCleanup    *WebhookMessageCleanup
	contractExpiryChecker    *ContractExpiryChecker
	wg                       conc.WaitGroup
}

// New creates a new scheduler that runs the cron, poller, and reaper.
func New(ctx context.Context, cfg *config.Config, s SchedulerStore, q queue.Queue, wfCallback WorkflowCallback, wfTrigger WorkflowTrigger, opts ...SchedulerOption) *Scheduler {
	sched := &Scheduler{
		cron: NewCronScheduler(ctx, s, q, wfTrigger).
			WithDefaultRunTTLSecs(cfg.DefaultRunTTLSecs).
			WithWorkflowCallback(wfCallback),
		poller: NewDelayedPoller(s, slog.Default(), cfg.PollerInterval),
		reaper: NewReaper(s, cfg.ReaperInterval, cfg.StaleThreshold, cfg.RunRetentionShort, cfg.RunRetentionLong, true, wfCallback).
			WithWorkflowRetention(cfg.WorkflowRetention).
			WithEventTriggerRetention(cfg.EventTriggerRetention).
			WithDeleteBatchSize(cfg.ReaperDeleteBatchSize).
			WithStalledThreshold(cfg.StalledWorkflowThreshold).
			WithStalledAction(cfg.StalledWorkflowAction).
			WithAuditRetention(cfg.AuditRetentionDefaultDays),
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

// WithMachineStopper attaches a container runtime to the cron scheduler
// for stopping managed containers on cancel_running overlap policy.
func WithMachineStopper(ms MachineStopper) SchedulerOption {
	return func(s *Scheduler) {
		s.cron.machineStopper = ms
	}
}

// WithChExporter attaches the ClickHouse exporter to the reaper for event trigger analytics.
func WithChExporter(e *clickhouse.Exporter) SchedulerOption {
	return func(s *Scheduler) {
		s.reaper.WithChExporter(e)
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

// WithGracePeriodEnforcer enables periodic enforcement of expired payment grace periods.
func WithGracePeriodEnforcer(enforcer *GracePeriodEnforcer) SchedulerOption {
	return func(s *Scheduler) {
		s.gracePeriodEnforcer = enforcer
	}
}

// WithStaleSubscriptionChecker enables periodic detection of stale subscriptions.
func WithStaleSubscriptionChecker(checker *StaleSubscriptionChecker) SchedulerOption {
	return func(s *Scheduler) {
		s.staleSubscriptionChecker = checker
	}
}

// WithWebhookMessageCleanup enables periodic cleanup of old processed webhook messages.
func WithWebhookMessageCleanup(cleanup *WebhookMessageCleanup) SchedulerOption {
	return func(s *Scheduler) {
		s.webhookMessageCleanup = cleanup
	}
}

// WithOrgRetentionResolver enables per-org plan-based data retention in the reaper.
func WithOrgRetentionResolver(resolver OrgRetentionResolver) SchedulerOption {
	return func(s *Scheduler) {
		s.reaper = s.reaper.WithOrgRetention(resolver)
	}
}

// WithContractExpiryChecker enables periodic enterprise contract expiry reminders.
func WithContractExpiryChecker(checker *ContractExpiryChecker) SchedulerOption {
	return func(s *Scheduler) {
		s.contractExpiryChecker = checker
	}
}

// WithIndexMaintainerAdvisoryLocker enables single-leader execution of the
// periodic REINDEX loop across multiple worker instances sharing a database.
// Without this, every worker runs REINDEX independently, which is safe (the
// underlying REINDEX INDEX CONCURRENTLY takes its own heavy lock) but wastes
// work.
func WithIndexMaintainerAdvisoryLocker(locker AdvisoryLocker) SchedulerOption {
	return func(s *Scheduler) {
		s.indexMaintainer.WithAdvisoryLocker(locker)
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.cron.LoadJobs(ctx); err != nil {
		return fmt.Errorf("load cron jobs: %w", err)
	}

	s.cron.Start()
	safeGo(&s.wg, "poller", func() { s.poller.Run(ctx) })
	safeGo(&s.wg, "reaper", func() { s.reaper.Run(ctx) })
	safeGo(&s.wg, "index_maintainer", func() { s.indexMaintainer.Run(ctx) })
	safeGo(&s.wg, "debounce_poller", func() { s.debouncePoller.Run(ctx) })
	safeGo(&s.wg, "batch_flusher", func() { s.batchFlusher.Run(ctx) })
	safeGo(&s.wg, "stats_aggregator", func() { s.statsAggregator.Run(ctx) })
	safeGo(&s.wg, "budget_monitor", func() { s.budgetMonitor.Run(ctx) })
	safeGo(&s.wg, "cost_estimate_refresher", func() { s.costEstimateRefresher.Run(ctx) })
	safeGo(&s.wg, "memory_cleanup", func() { s.memoryCleanup.Run(ctx) })
	if s.usageFlusher != nil {
		safeGo(&s.wg, "usage_flusher", func() { s.usageFlusher.Run(ctx) })
	}
	if s.concurrentReconciler != nil {
		safeGo(&s.wg, "concurrent_reconciler", func() { s.concurrentReconciler.Run(ctx) })
	}
	if s.downgradeApplier != nil {
		safeGo(&s.wg, "downgrade_applier", func() { s.downgradeApplier.Run(ctx) })
	}
	if s.anomalyMonitor != nil {
		safeGo(&s.wg, "anomaly_monitor", func() { s.anomalyMonitor.Run(ctx) })
	}
	if s.gracePeriodEnforcer != nil {
		safeGo(&s.wg, "grace_period_enforcer", func() { s.gracePeriodEnforcer.Run(ctx) })
	}
	if s.staleSubscriptionChecker != nil {
		safeGo(&s.wg, "stale_subscription_checker", func() { s.staleSubscriptionChecker.Run(ctx) })
	}
	if s.webhookMessageCleanup != nil {
		safeGo(&s.wg, "webhook_message_cleanup", func() { s.webhookMessageCleanup.Run(ctx) })
	}
	if s.contractExpiryChecker != nil {
		safeGo(&s.wg, "contract_expiry_checker", func() { s.contractExpiryChecker.Run(ctx) })
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
