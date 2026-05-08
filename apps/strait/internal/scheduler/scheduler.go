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
	MemoryCleanupStore
	store.DebounceStore
	store.BatchStore
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
	memoryCleanup            *MemoryCleanup
	gracePeriodEnforcer      *GracePeriodEnforcer
	quotaResumeEnforcer      *QuotaResumeEnforcer
	staleSubscriptionChecker *StaleSubscriptionChecker
	webhookMessageCleanup    *WebhookMessageCleanup
	contractExpiryChecker    *ContractExpiryChecker
	priorityPromoter         *PriorityPromoter
	counterReconciler        *CounterReconciler
	claimReconciler          *ClaimReconciler
	partitionEnsurer         *PartitionEnsurer
	partitionTuner           *PartitionTuner
	partitionReclaimer       *PartitionReclaimer
	dlqAgeOut                *DLQAgeOut
	outboxFlusher            *OutboxFlusher
	planDriftMonitor         *PlanDriftMonitor
	backpressureSampler      *BackpressureSampler
	wg                       conc.WaitGroup
	tracker                  componentTracker
	componentShutdownTimeout time.Duration
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
			WithAuditRetention(cfg.AuditRetentionDefaultDays).
			WithAuditDLQReclaimBatch(cfg.AuditDLQReclaimBatch).
			WithAuditDLQMaxAgeDays(cfg.AuditDLQMaxAgeDays).
			WithAuditDLQMaxReclaimAttempts(cfg.AuditDLQMaxReclaimAttempts).
			WithArchiveEnabled(cfg.TerminalArchiveEnabled).
			WithAllowPrivateEndpoints(cfg.AllowPrivateEndpoints),
		indexMaintainer: NewIndexMaintainer(s, cfg.IndexMaintenanceInterval),
		debouncePoller:  NewDebouncePoller(s, q, cfg.DebouncePollerInterval),
		batchFlusher:    NewBatchFlusher(s, q, cfg.BatchFlushInterval),
		statsAggregator: NewStatsAggregator(s),
		budgetMonitor:   NewBudgetMonitor(s, nil, 5*time.Minute),
		memoryCleanup:   NewMemoryCleanup(s, 5*time.Minute),
		tracker: componentTracker{sentry: sentrySchedulerMetadata{
			mode:                 cfg.Mode,
			region:               cfg.DefaultRegion,
			checkInsEnabled:      cfg.SentrySchedulerCheckIns,
			checkInMonitorPrefix: cfg.SentrySchedulerCheckInPrefix,
		}},
		componentShutdownTimeout: cfg.SchedulerComponentShutdownTimeout,
	}
	for _, opt := range opts {
		opt(sched)
	}
	return sched
}

// SchedulerOption configures a Scheduler.
type SchedulerOption func(*Scheduler)

// WithSentryRuntime attaches low-cardinality runtime tags to scheduler panic events.
func WithSentryRuntime(mode, region, version string) SchedulerOption {
	return func(s *Scheduler) {
		s.tracker.sentry.mode = mode
		s.tracker.sentry.region = region
		s.tracker.sentry.version = version
	}
}

// WithSentryCheckIns enables Sentry monitor check-ins for tracked scheduler components.
func WithSentryCheckIns(enabled bool, monitorPrefix string) SchedulerOption {
	return func(s *Scheduler) {
		s.tracker.sentry.checkInsEnabled = enabled
		s.tracker.sentry.checkInMonitorPrefix = monitorPrefix
	}
}

// WithSchedulerMetrics attaches telemetry metrics to the reaper.
func WithSchedulerMetrics(m *telemetry.Metrics) SchedulerOption {
	return func(s *Scheduler) {
		s.reaper.WithMetrics(m)
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

// WithPriorityPromoter enables priority aging via a dedicated
// scheduler goroutine instead of a mutable dequeue ORDER BY.
func WithPriorityPromoter(p *PriorityPromoter) SchedulerOption {
	return func(s *Scheduler) {
		s.priorityPromoter = p
	}
}

// WithCounterReconciler enables counter drift reconciliation.
func WithCounterReconciler(r *CounterReconciler) SchedulerOption {
	return func(s *Scheduler) {
		s.counterReconciler = r
	}
}

// WithClaimReconciler enables periodic claim table drift reconciliation.
func WithClaimReconciler(r *ClaimReconciler) SchedulerOption {
	return func(s *Scheduler) {
		s.claimReconciler = r
	}
}

// WithPartitionEnsurer enables partition self-heal.
func WithPartitionEnsurer(p *PartitionEnsurer) SchedulerOption {
	return func(s *Scheduler) {
		s.partitionEnsurer = p
	}
}

// WithPartitionTuner enables per-partition autovacuum tuning.
func WithPartitionTuner(p *PartitionTuner) SchedulerOption {
	return func(s *Scheduler) {
		s.partitionTuner = p
	}
}

func WithPartitionReclaimer(p *PartitionReclaimer) SchedulerOption {
	return func(s *Scheduler) {
		s.partitionReclaimer = p
	}
}

// WithDLQAgeOut enables DLQ archival.
func WithDLQAgeOut(a *DLQAgeOut) SchedulerOption {
	return func(s *Scheduler) {
		s.dlqAgeOut = a
	}
}

// WithOutboxFlusher enables outbox promotion.
func WithOutboxFlusher(f *OutboxFlusher) SchedulerOption {
	return func(s *Scheduler) {
		s.outboxFlusher = f
	}
}

// WithPlanDriftMonitor enables daily plan drift detection.
func WithPlanDriftMonitor(m *PlanDriftMonitor) SchedulerOption {
	return func(s *Scheduler) {
		s.planDriftMonitor = m
	}
}

// WithBackpressureSampler enables the periodic sampler that populates
// the strait.queue.backpressure_tokens_available gauge from the
// project_rate_limits table. Pass nil (or a nil sampler built by
// NewBackpressureSampler when disabled) to skip registration.
func WithBackpressureSampler(s *BackpressureSampler) SchedulerOption {
	return func(sched *Scheduler) {
		sched.backpressureSampler = s
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

// WithQuotaResumeEnforcer enables periodic resumption of jobs paused due to quota
// exhaustion at the billing-period boundary.
func WithQuotaResumeEnforcer(enforcer *QuotaResumeEnforcer) SchedulerOption {
	return func(s *Scheduler) {
		s.quotaResumeEnforcer = enforcer
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

// WithReaperAdvisoryLocker enables single-leader execution of side-effectful
// reaper maintenance across scheduler replicas.
func WithReaperAdvisoryLocker(locker AdvisoryLocker) SchedulerOption {
	return func(s *Scheduler) {
		s.reaper.WithAdvisoryLocker(locker)
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.cron.LoadJobs(ctx); err != nil {
		return fmt.Errorf("load cron jobs: %w", err)
	}

	s.cron.Start()
	s.tracker.track(ctx, &s.wg, "poller", func(componentCtx context.Context) { s.poller.Run(componentCtx) })
	s.tracker.track(ctx, &s.wg, "reaper", func(componentCtx context.Context) { s.reaper.Run(componentCtx) })
	s.tracker.track(ctx, &s.wg, "index_maintainer", func(componentCtx context.Context) { s.indexMaintainer.Run(componentCtx) })
	s.tracker.track(ctx, &s.wg, "debounce_poller", func(componentCtx context.Context) { s.debouncePoller.Run(componentCtx) })
	s.tracker.track(ctx, &s.wg, "batch_flusher", func(componentCtx context.Context) { s.batchFlusher.Run(componentCtx) })
	s.tracker.track(ctx, &s.wg, "stats_aggregator", func(componentCtx context.Context) { s.statsAggregator.Run(componentCtx) })
	s.tracker.track(ctx, &s.wg, "budget_monitor", func(componentCtx context.Context) { s.budgetMonitor.Run(componentCtx) })
	s.tracker.track(ctx, &s.wg, "memory_cleanup", func(componentCtx context.Context) { s.memoryCleanup.Run(componentCtx) })
	if s.usageFlusher != nil {
		s.tracker.track(ctx, &s.wg, "usage_flusher", func(componentCtx context.Context) { s.usageFlusher.Run(componentCtx) })
	}
	if s.concurrentReconciler != nil {
		s.tracker.track(ctx, &s.wg, "concurrent_reconciler", func(componentCtx context.Context) { s.concurrentReconciler.Run(componentCtx) })
	}
	if s.downgradeApplier != nil {
		s.tracker.track(ctx, &s.wg, "downgrade_applier", func(componentCtx context.Context) { s.downgradeApplier.Run(componentCtx) })
	}
	if s.anomalyMonitor != nil {
		s.tracker.track(ctx, &s.wg, "anomaly_monitor", func(componentCtx context.Context) { s.anomalyMonitor.Run(componentCtx) })
	}
	if s.gracePeriodEnforcer != nil {
		s.tracker.track(ctx, &s.wg, "grace_period_enforcer", func(componentCtx context.Context) { s.gracePeriodEnforcer.Run(componentCtx) })
	}
	if s.quotaResumeEnforcer != nil {
		s.tracker.track(ctx, &s.wg, "quota_resume_enforcer", func(componentCtx context.Context) { s.quotaResumeEnforcer.Run(componentCtx) })
	}
	if s.staleSubscriptionChecker != nil {
		s.tracker.track(ctx, &s.wg, "stale_subscription_checker", func(componentCtx context.Context) { s.staleSubscriptionChecker.Run(componentCtx) })
	}
	if s.webhookMessageCleanup != nil {
		s.tracker.track(ctx, &s.wg, "webhook_message_cleanup", func(componentCtx context.Context) { s.webhookMessageCleanup.Run(componentCtx) })
	}
	if s.contractExpiryChecker != nil {
		s.tracker.track(ctx, &s.wg, "contract_expiry_checker", func(componentCtx context.Context) { s.contractExpiryChecker.Run(componentCtx) })
	}
	if s.priorityPromoter != nil {
		s.tracker.track(ctx, &s.wg, "priority_promoter", func(componentCtx context.Context) { s.priorityPromoter.Run(componentCtx) })
	}
	if s.counterReconciler != nil {
		s.tracker.track(ctx, &s.wg, "counter_reconciler", func(componentCtx context.Context) { s.counterReconciler.Run(componentCtx) })
	}
	if s.claimReconciler != nil {
		s.tracker.track(ctx, &s.wg, "claim_reconciler", func(componentCtx context.Context) { s.claimReconciler.Run(componentCtx) })
	}
	if s.partitionEnsurer != nil {
		s.tracker.track(ctx, &s.wg, "partition_ensurer", func(componentCtx context.Context) { s.partitionEnsurer.Run(componentCtx) })
	}
	if s.partitionTuner != nil {
		s.tracker.track(ctx, &s.wg, "partition_tuner", func(componentCtx context.Context) { s.partitionTuner.Run(componentCtx) })
	}
	if s.partitionReclaimer != nil {
		s.tracker.track(ctx, &s.wg, "partition_reclaimer", func(componentCtx context.Context) { s.partitionReclaimer.Run(componentCtx) })
	}
	if s.dlqAgeOut != nil {
		s.tracker.track(ctx, &s.wg, "dlq_age_out", func(componentCtx context.Context) { s.dlqAgeOut.Run(componentCtx) })
	}
	if s.outboxFlusher != nil {
		s.tracker.track(ctx, &s.wg, "outbox_flusher", func(componentCtx context.Context) { s.outboxFlusher.Run(componentCtx) })
	}
	if s.planDriftMonitor != nil {
		s.tracker.track(ctx, &s.wg, "plan_drift_monitor", func(componentCtx context.Context) { s.planDriftMonitor.Run(componentCtx) })
	}
	if s.backpressureSampler != nil {
		s.tracker.track(ctx, &s.wg, "backpressure_sampler", func(componentCtx context.Context) { s.backpressureSampler.Run(componentCtx) })
	}

	slog.Info("scheduler started")
	return nil
}

func (s *Scheduler) Stop() {
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()

	timeout := s.componentShutdownTimeout
	if timeout <= 0 {
		timeout = defaultComponentShutdownTimeout
	}

	// Wait per-component with a bounded deadline so a single stuck goroutine
	// can no longer pin the entire shutdown path. Components past the
	// deadline are logged and counted on strait.scheduler.shutdown_timeouts_total.
	// Once the deadline is hit we return immediately instead of blocking on
	// the aggregate WaitGroup forever.
	timedOut := s.tracker.waitWithTimeout(context.Background(), timeout)
	if timedOut == 0 {
		s.wg.Wait()
	} else {
		slog.Warn("scheduler stop returning before all components exited",
			"timed_out_components", timedOut,
		)
	}

	slog.Info("scheduler stopped", "timed_out_components", timedOut)
}
