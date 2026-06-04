package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sourcegraph/conc"

	"time"

	"strait/internal/billing"
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
	usageReportEmailer       *UsageReportEmailer
	priorityPromoter         *PriorityPromoter
	dunner                   DunnerRunner
	slaCalculator            SLACalculatorRunner
	counterReconciler        *CounterReconciler
	readyRunReconciler       *ReadyRunReconciler
	partitionEnsurer         *PartitionEnsurer
	partitionTuner           *PartitionTuner
	partitionReclaimer       *PartitionReclaimer
	dlqAgeOut                *DLQAgeOut
	outboxFlusher            *OutboxFlusher
	outboxArchiver           *OutboxArchiver
	sloEvaluator             *SLOEvaluator
	planDriftMonitor         *PlanDriftMonitor
	backpressureSampler      *BackpressureSampler
	idempotencyGC            *IdempotencyGC
	heartbeatGC              *HeartbeatGC
	wg                       conc.WaitGroup
	tracker                  componentTracker
	componentShutdownTimeout time.Duration
	cronReloadInterval       time.Duration
}

// New creates a new scheduler that runs the cron, poller, and reaper.
func New(ctx context.Context, cfg *config.Config, s SchedulerStore, q queue.Queue, wfCallback WorkflowCallback, wfTrigger WorkflowTrigger, opts ...SchedulerOption) *Scheduler {
	sched := &Scheduler{
		cron: NewCronScheduler(ctx, s, q, wfTrigger).
			WithDefaultRunTTLSecs(cfg.DefaultRunTTLSecs).
			WithWorkflowCallback(wfCallback),
		poller: delayedPollerForQueue(s, q, cfg.PollerInterval),
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
		cronReloadInterval:       time.Minute,
	}
	for _, opt := range opts {
		opt(sched)
	}
	return sched
}

func delayedPollerForQueue(s SchedulerStore, q queue.Queue, interval time.Duration) *DelayedPoller {
	poller := NewDelayedPoller(s, slog.Default(), interval)
	if promoter, ok := q.(PollerStore); ok {
		poller.WithPromoter(promoter)
	}
	return poller
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

func WithCronReloadInterval(interval time.Duration) SchedulerOption {
	return func(s *Scheduler) {
		s.cronReloadInterval = interval
	}
}

// WithChExporter attaches the ClickHouse exporter to the reaper for event trigger analytics.
func WithChExporter(e *clickhouse.Exporter) SchedulerOption {
	return func(s *Scheduler) {
		s.reaper.WithChExporter(e)
	}
}

// WithRotationSecretDecryptor wires the at-rest secret decryptor used by the
// reaper to recover plaintext HMAC keys for outbound api-key rotation webhooks.
func WithRotationSecretDecryptor(d SecretDecryptor) SchedulerOption {
	return func(s *Scheduler) {
		s.reaper.WithRotationSecretDecryptor(d)
	}
}

// WithBudgetWebhookEnqueuer sets the webhook enqueuer for the budget monitor.
func WithBudgetWebhookEnqueuer(enqueuer BudgetMonitorWebhookEnqueuer) SchedulerOption {
	return func(s *Scheduler) {
		s.budgetMonitor.enqueuer = enqueuer
	}
}

// WithBudgetMonitoringStores wires the concrete billing stores into the
// always-running budget monitor. Without this option the monitor loop runs but
// has no spending/run-limit producers to evaluate.
func WithBudgetMonitoringStores(spending SpendingLimitStore, runLimits RunLimitStore, enforcer *billing.Enforcer) SchedulerOption {
	return func(s *Scheduler) {
		if spending != nil {
			s.budgetMonitor.WithSpendingLimitStore(spending)
		}
		if runLimits != nil && enforcer != nil {
			s.budgetMonitor.WithRunLimitNotifications(runLimits, enforcer)
		}
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

// WithReadyRunReconciler enables periodic ready-event repair.
func WithReadyRunReconciler(r *ReadyRunReconciler) SchedulerOption {
	return func(s *Scheduler) {
		s.readyRunReconciler = r
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

func WithOutboxArchiver(a *OutboxArchiver) SchedulerOption {
	return func(s *Scheduler) {
		s.outboxArchiver = a
	}
}

// WithPlanDriftMonitor enables daily plan drift detection.
func WithPlanDriftMonitor(m *PlanDriftMonitor) SchedulerOption {
	return func(s *Scheduler) {
		s.planDriftMonitor = m
	}
}

// WithIdempotencyGC enables periodic deletion of expired
// job_run_idempotency rows.
func WithIdempotencyGC(g *IdempotencyGC) SchedulerOption {
	return func(s *Scheduler) {
		s.idempotencyGC = g
	}
}

// WithHeartbeatGC enables periodic deletion of orphaned heartbeat rows.
func WithHeartbeatGC(g *HeartbeatGC) SchedulerOption {
	return func(s *Scheduler) {
		s.heartbeatGC = g
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

// DunnerRunner is the narrow interface the scheduler needs from a Dunner
// (defined in internal/billing). Kept here so the scheduler does not have to
// import the billing package just for a concrete type.
type DunnerRunner interface {
	Run(ctx context.Context)
}

// WithDunner registers a dunning-state-machine driver. The dunner is woken
// on its own internal cadence; the scheduler only owns its lifecycle.
func WithDunner(d DunnerRunner) SchedulerOption {
	return func(s *Scheduler) {
		s.dunner = d
	}
}

// SLACalculatorRunner is the narrow interface the scheduler needs from the
// SLA credit calculator (defined in internal/billing). Kept here so the
// scheduler does not have to import the billing package.
type SLACalculatorRunner interface {
	Run(ctx context.Context)
}

// WithSLACalculator registers the SLA-credit calculator. Each tick reads
// the previous month's per-org uptime, issues credit notes for breached
// SLAs, and dispatches sla.credit_issued. Lifecycle owned by the scheduler.
func WithSLACalculator(c SLACalculatorRunner) SchedulerOption {
	return func(s *Scheduler) {
		s.slaCalculator = c
	}
}

// WithUsageFlusher sets a usage flusher for periodic usage record materialization.
func WithUsageFlusher(flusher *UsageFlusher) SchedulerOption {
	return func(s *Scheduler) {
		s.usageFlusher = flusher
	}
}

// WithSLOEvaluator enables periodic SLO evaluation and alert delivery.
func WithSLOEvaluator(evaluator *SLOEvaluator) SchedulerOption {
	return func(s *Scheduler) {
		s.sloEvaluator = evaluator
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

// WithUsageReportEmailer enables monthly usage report emails for opted-in paid orgs.
func WithUsageReportEmailer(emailer *UsageReportEmailer) SchedulerOption {
	return func(s *Scheduler) {
		s.usageReportEmailer = emailer
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
	s.trackComponents(ctx, s.components())

	slog.Info("scheduler started")
	return nil
}

func (s *Scheduler) runCronReloader(ctx context.Context) {
	ticker := time.NewTicker(s.cronReloadInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.cron.LoadJobs(ctx); err != nil {
				slog.Warn("cron reload failed", "error", err)
			}
		}
	}
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
