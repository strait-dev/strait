package queue

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// QueueMetrics bundles the queue-health observability gauges and
// counters. They live in a dedicated meter ("strait/queue_health") so adding
// new signals does not require modifying the monolithic telemetry.Metrics
// struct. All instruments are lazily initialised and safe for concurrent use.
type QueueMetrics struct {
	OldestQueuedAge               metric.Float64Histogram
	DequeueScanRows               metric.Float64Histogram
	DeadTupleRatio                metric.Float64Gauge
	LiveTuples                    metric.Int64Gauge
	HotUpdateRatio                metric.Float64Gauge
	NotifyDropped                 metric.Int64Counter
	NotifyReconnects              metric.Int64Counter
	NotifyWakeDelivered           metric.Int64Counter
	HeartbeatReclaims             metric.Int64Counter
	RetryScheduleLag              metric.Float64Histogram
	MaskedRowsPending             metric.Int64Gauge
	CounterDrift                  metric.Int64Gauge
	PartitionDequeueLag           metric.Float64Histogram
	ClaimToStart                  metric.Float64Histogram
	CircuitStateTransitions       metric.Int64Counter
	OutboxLag                     metric.Float64Histogram
	OutboxQuarantinedTotal        metric.Int64Counter
	BackpressureTokensAvailable   metric.Int64Gauge
	EventChannelDropped           metric.Int64Counter
	RetryAttempts                 metric.Float64Histogram
	DLQOldestUnmaskedAge          metric.Float64Gauge
	HistoryRowsArchivedTotal      metric.Int64Counter
	HistoryLiveTuples             metric.Int64Gauge
	HistoryRetentionDeletedTotal  metric.Int64Counter
	ArchiveStrandedTerminal       metric.Int64Gauge
	QueueDepthByStatus            metric.Int64Gauge
	NotifyDegradedDurationSeconds metric.Float64Histogram
	EventChannelSaturationRatio   metric.Float64Gauge
	SchedulerShutdownTimeouts     metric.Int64Counter
	IndexDeadItems                metric.Int64Gauge
	ClaimDuration                 metric.Float64Histogram
	LockSkipped                   metric.Int64Counter
	VisibilityTimeoutExpirations  metric.Int64Counter
	ConcurrencyUtilization        metric.Float64Gauge
	OutboxClaimDepth              metric.Int64Gauge
	OutboxOldestReadyAge          metric.Float64Gauge
	OutboxExpiredLeases           metric.Int64Gauge
	OutboxClaimTableDeadTuples    metric.Int64Gauge
	OutboxClaimTableLiveTuples    metric.Int64Gauge
	PgQueBackgroundErrors         metric.Int64Counter
	PgQueConsumerLag              metric.Int64Gauge
	ExecutorDrainWakeRequested    metric.Int64Counter
	ExecutorDrainWakeDelivered    metric.Int64Counter
	ExecutorDrainWakeCoalesced    metric.Int64Counter
	ExecutorPolls                 metric.Int64Counter
}

var (
	queueMetricsOnce sync.Once
	queueMetricsInst *QueueMetrics
	errQueueMetrics  error
)

// Metrics returns the process-wide QueueMetrics singleton, initialising it
// on first use. Callers that need a nop fallback (tests without an OTEL
// provider installed) can pass the returned value directly; the underlying
// OTEL API is nil-safe and records to a noop meter when no SDK is registered.
func Metrics() (*QueueMetrics, error) {
	queueMetricsOnce.Do(func() {
		queueMetricsInst, errQueueMetrics = newQueueMetrics()
	})
	return queueMetricsInst, errQueueMetrics
}

// ResetMetricsForTest clears the singleton so tests can re-initialise with a
// fresh meter provider. Not safe for concurrent use with production code.
func ResetMetricsForTest() {
	queueMetricsOnce = sync.Once{}
	queueMetricsInst = nil
	errQueueMetrics = nil
}

func newQueueMetrics() (*QueueMetrics, error) {
	meter := otel.Meter("strait/queue_health")

	oldestAge, err := meter.Float64Histogram(
		"strait_queue_oldest_queued_age_seconds",
		metric.WithDescription("Age in seconds of the oldest queued run observed at dequeue time"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 5, 10, 30, 60, 300, 900, 3600),
	)
	if err != nil {
		return nil, fmt.Errorf("oldest queued age histogram: %w", err)
	}
	scanRows, err := meter.Float64Histogram(
		"strait_queue_dequeue_scan_rows",
		metric.WithDescription("Approximate rows examined per dequeue claim (from pg_stat_statements where available)"),
		metric.WithUnit("1"),
		metric.WithExplicitBucketBoundaries(1, 10, 100, 1000, 10000, 100000),
	)
	if err != nil {
		return nil, fmt.Errorf("dequeue scan rows histogram: %w", err)
	}
	deadRatio, err := meter.Float64Gauge(
		"strait_queue_dead_tuple_ratio",
		metric.WithDescription("Ratio of dead tuples to live tuples per job_runs partition"),
	)
	if err != nil {
		return nil, fmt.Errorf("dead tuple ratio gauge: %w", err)
	}
	liveTuples, err := meter.Int64Gauge(
		"strait_queue_live_tuples",
		metric.WithDescription("Live tuple count per job_runs partition"),
	)
	if err != nil {
		return nil, fmt.Errorf("live tuples gauge: %w", err)
	}
	hotRatio, err := meter.Float64Gauge(
		"strait_queue_hot_update_ratio",
		metric.WithDescription("HOT updates / total updates per job_runs partition"),
	)
	if err != nil {
		return nil, fmt.Errorf("hot update ratio gauge: %w", err)
	}
	notifyDropped, err := meter.Int64Counter(
		"strait_queue_notify_dropped_total",
		metric.WithDescription("Number of queue wake notifications dropped because the wake channel was full"),
	)
	if err != nil {
		return nil, fmt.Errorf("notify dropped counter: %w", err)
	}
	notifyReconnects, err := meter.Int64Counter(
		"strait_queue_notify_reconnects_total",
		metric.WithDescription("Number of times the LISTEN connection had to reconnect"),
	)
	if err != nil {
		return nil, fmt.Errorf("notify reconnects counter: %w", err)
	}
	notifyWakeDelivered, err := meter.Int64Counter(
		"strait_queue_notify_wake_delivered_total",
		metric.WithDescription("Number of queue wake notifications successfully delivered to the wake channel"),
	)
	if err != nil {
		return nil, fmt.Errorf("notify wake delivered counter: %w", err)
	}
	heartbeatReclaims, err := meter.Int64Counter(
		"strait_queue_heartbeat_reclaims_total",
		metric.WithDescription("Number of stuck runs reclaimed after a heartbeat went stale"),
	)
	if err != nil {
		return nil, fmt.Errorf("heartbeat reclaims counter: %w", err)
	}
	retryLag, err := meter.Float64Histogram(
		"strait_queue_retry_schedule_lag_seconds",
		metric.WithDescription("Delta between intended next_retry_at and observed dequeue time"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30),
	)
	if err != nil {
		return nil, fmt.Errorf("retry schedule lag histogram: %w", err)
	}
	masked, err := meter.Int64Gauge(
		"strait_queue_masked_rows_pending",
		metric.WithDescription("Number of rows marked visible_until but not yet physically dropped"),
	)
	if err != nil {
		return nil, fmt.Errorf("masked rows pending gauge: %w", err)
	}
	counterDrift, err := meter.Int64Gauge(
		"strait_queue_counter_drift",
		metric.WithDescription("Absolute drift observed between trigger-maintained counters and ground truth (job_active_counts + dlq_counts combined)"),
	)
	if err != nil {
		return nil, fmt.Errorf("counter drift gauge: %w", err)
	}

	partitionDequeueLag, err := meter.Float64Histogram(
		"strait_queue_partition_dequeue_lag_seconds",
		metric.WithDescription("Wall-clock duration of a per-project dequeue call"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5),
	)
	if err != nil {
		return nil, fmt.Errorf("partition dequeue lag histogram: %w", err)
	}
	claimToStart, err := meter.Float64Histogram(
		"strait_queue_claim_to_start_seconds",
		metric.WithDescription("Time between a run being claimed by the executor and the start of user work"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5),
	)
	if err != nil {
		return nil, fmt.Errorf("claim to start histogram: %w", err)
	}
	circuitTransitions, err := meter.Int64Counter(
		"strait_queue_circuit_state_transitions_total",
		metric.WithDescription("DB circuit breaker state transitions, labelled by from/to"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("circuit state transitions counter: %w", err)
	}
	outboxLag, err := meter.Float64Histogram(
		"strait_queue_outbox_lag_seconds",
		metric.WithDescription("Age of an outbox row at the time the flusher promoted it"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 5, 10, 30, 60, 300, 900),
	)
	if err != nil {
		return nil, fmt.Errorf("outbox lag histogram: %w", err)
	}
	outboxQuarantinedTotal, err := meter.Int64Counter(
		"strait_queue_outbox_quarantined_total",
		metric.WithDescription("Terminal outbox rows quarantined after enqueue promotion failed"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("outbox quarantined counter: %w", err)
	}
	backpressureTokens, err := meter.Int64Gauge(
		"strait_queue_backpressure_tokens_available",
		metric.WithDescription("Available backpressure tokens per project (sampled)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("backpressure tokens gauge: %w", err)
	}
	eventChannelDropped, err := meter.Int64Counter(
		"strait_worker_event_channel_dropped_total",
		metric.WithDescription("Executor lifecycle events dropped because the event channel was full or closed"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("event channel dropped counter: %w", err)
	}
	retryAttempts, err := meter.Float64Histogram(
		"strait_worker_retry_attempt_number",
		metric.WithDescription("Attempt number observed when a run was re-enqueued for retry"),
		metric.WithUnit("1"),
		metric.WithExplicitBucketBoundaries(1, 2, 3, 4, 5, 7, 10, 15, 20),
	)
	if err != nil {
		return nil, fmt.Errorf("retry attempts histogram: %w", err)
	}
	dlqOldestAge, err := meter.Float64Gauge(
		"strait_queue_dlq_oldest_unmasked_age_seconds",
		metric.WithDescription("Age in seconds of the oldest visible dead-letter row"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("dlq oldest unmasked age gauge: %w", err)
	}
	schedulerShutdownTimeouts, err := meter.Int64Counter(
		"strait_scheduler_shutdown_timeouts_total",
		metric.WithDescription("Scheduler background components that exceeded the configured shutdown deadline"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("scheduler shutdown timeouts counter: %w", err)
	}
	eventChannelSaturation, err := meter.Float64Gauge(
		"strait_worker_event_channel_saturation_ratio",
		metric.WithDescription("Fraction of the executor event channel buffer in use (0.0-1.0)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("event channel saturation gauge: %w", err)
	}
	pgqueBackgroundErrors, err := meter.Int64Counter(
		"strait_queue_pgque_background_errors_total",
		metric.WithDescription("PgQue background operation failures grouped by bounded operation name"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("pgque background errors counter: %w", err)
	}
	pgqueConsumerLag, err := meter.Int64Gauge(
		"strait_queue_pgque_consumer_lag_ticks",
		metric.WithDescription("PgQue tick lag between the latest queue tick and this consumer's last acknowledged tick"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("pgque consumer lag gauge: %w", err)
	}
	executorDrainWakeRequested, err := meter.Int64Counter(
		"strait_worker_drain_wake_requested_total",
		metric.WithDescription("Number of internal executor drain wakes requested after a full dequeue batch"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("executor drain wake requested counter: %w", err)
	}
	executorDrainWakeDelivered, err := meter.Int64Counter(
		"strait_worker_drain_wake_delivered_total",
		metric.WithDescription("Number of internal executor drain wakes delivered to the run loop"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("executor drain wake delivered counter: %w", err)
	}
	executorDrainWakeCoalesced, err := meter.Int64Counter(
		"strait_worker_drain_wake_coalesced_total",
		metric.WithDescription("Number of internal executor drain wakes coalesced because one was already pending"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("executor drain wake coalesced counter: %w", err)
	}
	executorPolls, err := meter.Int64Counter(
		"strait_worker_polls_total",
		metric.WithDescription("Executor poll loop invocations grouped by bounded trigger"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("executor polls counter: %w", err)
	}
	m := &QueueMetrics{
		OldestQueuedAge:             oldestAge,
		DequeueScanRows:             scanRows,
		DeadTupleRatio:              deadRatio,
		LiveTuples:                  liveTuples,
		HotUpdateRatio:              hotRatio,
		NotifyDropped:               notifyDropped,
		NotifyReconnects:            notifyReconnects,
		NotifyWakeDelivered:         notifyWakeDelivered,
		HeartbeatReclaims:           heartbeatReclaims,
		RetryScheduleLag:            retryLag,
		MaskedRowsPending:           masked,
		CounterDrift:                counterDrift,
		PartitionDequeueLag:         partitionDequeueLag,
		ClaimToStart:                claimToStart,
		CircuitStateTransitions:     circuitTransitions,
		OutboxLag:                   outboxLag,
		OutboxQuarantinedTotal:      outboxQuarantinedTotal,
		BackpressureTokensAvailable: backpressureTokens,
		EventChannelDropped:         eventChannelDropped,
		RetryAttempts:               retryAttempts,
		DLQOldestUnmaskedAge:        dlqOldestAge,
		EventChannelSaturationRatio: eventChannelSaturation,
		SchedulerShutdownTimeouts:   schedulerShutdownTimeouts,
		PgQueBackgroundErrors:       pgqueBackgroundErrors,
		PgQueConsumerLag:            pgqueConsumerLag,
		ExecutorDrainWakeRequested:  executorDrainWakeRequested,
		ExecutorDrainWakeDelivered:  executorDrainWakeDelivered,
		ExecutorDrainWakeCoalesced:  executorDrainWakeCoalesced,
		ExecutorPolls:               executorPolls,
	}
	if err := initArchiveMetrics(meter, m); err != nil {
		return nil, err
	}
	return m, nil
}

func initArchiveMetrics(meter metric.Meter, m *QueueMetrics) error {
	var err error
	m.HistoryRowsArchivedTotal, err = meter.Int64Counter(
		"strait_queue_history_rows_archived_total",
		metric.WithDescription("Terminal runs archived from hot storage to history"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("history rows archived counter: %w", err)
	}
	m.HistoryLiveTuples, err = meter.Int64Gauge(
		"strait_queue_history_live_tuples",
		metric.WithDescription("Live tuple count in job_runs_history"),
	)
	if err != nil {
		return fmt.Errorf("history live tuples gauge: %w", err)
	}
	m.HistoryRetentionDeletedTotal, err = meter.Int64Counter(
		"strait_queue_history_retention_deleted_total",
		metric.WithDescription("History rows deleted by retention reaper"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("history retention deleted counter: %w", err)
	}
	m.ArchiveStrandedTerminal, err = meter.Int64Gauge(
		"strait_queue_archive_stranded_terminal",
		metric.WithDescription("Terminal runs still in hot table past retention cutoff"),
	)
	if err != nil {
		return fmt.Errorf("archive stranded terminal gauge: %w", err)
	}
	m.QueueDepthByStatus, err = meter.Int64Gauge(
		"strait_queue_depth_by_status",
		metric.WithDescription("Queue depth grouped by run status"),
	)
	if err != nil {
		return fmt.Errorf("queue depth by status gauge: %w", err)
	}
	m.NotifyDegradedDurationSeconds, err = meter.Float64Histogram(
		"strait_queue_notify_degraded_duration_seconds",
		metric.WithDescription("Duration of notify degraded mode episodes"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 30, 60, 300, 600, 1800),
	)
	if err != nil {
		return fmt.Errorf("notify degraded duration histogram: %w", err)
	}
	m.IndexDeadItems, err = meter.Int64Gauge(
		"strait_queue_index_dead_items",
		metric.WithDescription("Dead index entries in the dequeue covering index (from pgstatindex)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("index dead items gauge: %w", err)
	}
	m.ClaimDuration, err = meter.Float64Histogram(
		"strait_queue_claim_duration_seconds",
		metric.WithDescription("Duration of queue claim attempts by queue and result"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5),
	)
	if err != nil {
		return fmt.Errorf("claim duration histogram: %w", err)
	}
	m.LockSkipped, err = meter.Int64Counter(
		"strait_queue_lock_skipped_total",
		metric.WithDescription("Queue rows skipped because another worker held a SKIP LOCKED claim"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("lock skipped counter: %w", err)
	}
	m.VisibilityTimeoutExpirations, err = meter.Int64Counter(
		"strait_queue_visibility_timeout_expirations_total",
		metric.WithDescription("Runs reclaimed after visibility timeout or heartbeat expiration"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("visibility timeout expirations counter: %w", err)
	}
	m.ConcurrencyUtilization, err = meter.Float64Gauge(
		"strait_queue_concurrency_utilization",
		metric.WithDescription("Queue concurrency utilization ratio in [0,1]"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("concurrency utilization gauge: %w", err)
	}
	m.OutboxClaimDepth, err = meter.Int64Gauge(
		"strait_outbox_claim_depth",
		metric.WithDescription("Outbox claim-log depth grouped by claim status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("outbox claim depth gauge: %w", err)
	}
	m.OutboxOldestReadyAge, err = meter.Float64Gauge(
		"strait_outbox_oldest_ready_age_seconds",
		metric.WithDescription("Age in seconds of the oldest ready outbox claim-log row"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("outbox oldest ready age gauge: %w", err)
	}
	m.OutboxExpiredLeases, err = meter.Int64Gauge(
		"strait_outbox_expired_leases",
		metric.WithDescription("Outbox claim-log leases past lease_expires_at"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("outbox expired leases gauge: %w", err)
	}
	m.OutboxClaimTableDeadTuples, err = meter.Int64Gauge(
		"strait_outbox_claim_table_dead_tuples",
		metric.WithDescription("Dead tuple count in outbox_claims"),
	)
	if err != nil {
		return fmt.Errorf("outbox claim table dead tuples gauge: %w", err)
	}
	m.OutboxClaimTableLiveTuples, err = meter.Int64Gauge(
		"strait_outbox_claim_table_live_tuples",
		metric.WithDescription("Live tuple count in outbox_claims"),
	)
	if err != nil {
		return fmt.Errorf("outbox claim table live tuples gauge: %w", err)
	}
	return nil
}

var jobRunsPartitionMetricRE = regexp.MustCompile(`^job_runs_p\d{4}_(0[1-9]|1[0-2])$`)

func partitionMetricLabel(partition string) string {
	switch {
	case partition == "job_runs":
		return "job_runs"
	case jobRunsPartitionMetricRE.MatchString(partition):
		return "job_runs_partition"
	case partition == "":
		return "unknown"
	default:
		return "other"
	}
}

// RecordPartitionStats records gauge values for a single partition. The
// partition dimension is collapsed to a bounded label set before recording so
// tenant-controlled or date-derived relnames cannot create unbounded series.
func (m *QueueMetrics) RecordPartitionStats(ctx context.Context, partition string, stats PartitionStats) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(attribute.String("partition", partitionMetricLabel(partition)))
	m.DeadTupleRatio.Record(ctx, stats.DeadTupleRatio, attrs)
	m.LiveTuples.Record(ctx, stats.LiveTuples, attrs)
	if stats.TotalUpdates > 0 {
		ratio := float64(stats.HotUpdates) / float64(stats.TotalUpdates)
		m.HotUpdateRatio.Record(ctx, ratio, attrs)
	}
}

// PartitionStats is a plain row from pg_stat_user_tables for a single table
// (typically a job_runs partition).
type PartitionStats struct {
	Relname        string
	LiveTuples     int64
	DeadTuples     int64
	TotalUpdates   int64
	HotUpdates     int64
	DeadTupleRatio float64
}
