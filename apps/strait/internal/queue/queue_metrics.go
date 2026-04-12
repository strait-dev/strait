package queue

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// QueueMetrics bundles the Phase 2 queue-health observability gauges and
// counters. They live in a dedicated meter ("strait/queue_health") so adding
// new signals does not require modifying the monolithic telemetry.Metrics
// struct. All instruments are lazily initialised and safe for concurrent use.
type QueueMetrics struct {
	OldestQueuedAge   metric.Float64Histogram
	DequeueScanRows   metric.Float64Histogram
	DeadTupleRatio    metric.Float64Gauge
	LiveTuples        metric.Int64Gauge
	HotUpdateRatio    metric.Float64Gauge
	NotifyDropped     metric.Int64Counter
	NotifyReconnects  metric.Int64Counter
	HeartbeatReclaims metric.Int64Counter
	RetryScheduleLag  metric.Float64Histogram
	MaskedRowsPending metric.Int64Gauge
	// Round 2 Phase 3: absolute drift observed by the counter reconciler.
	CounterDrift metric.Int64Gauge

	// Phase 1 hot-path instruments.
	PartitionDequeueLag         metric.Float64Histogram
	ClaimToStart                metric.Float64Histogram
	CircuitStateTransitions     metric.Int64Counter
	OutboxLag                   metric.Float64Histogram
	BackpressureTokensAvailable metric.Int64Gauge
	EventChannelDropped         metric.Int64Counter
	RetryAttempts               metric.Float64Histogram
	DLQOldestUnmaskedAge        metric.Float64Gauge
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
		"strait.queue.oldest_queued_age_seconds",
		metric.WithDescription("Age in seconds of the oldest queued run observed at dequeue time"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 5, 10, 30, 60, 300, 900, 3600),
	)
	if err != nil {
		return nil, fmt.Errorf("oldest queued age histogram: %w", err)
	}
	scanRows, err := meter.Float64Histogram(
		"strait.queue.dequeue_scan_rows",
		metric.WithDescription("Approximate rows examined per dequeue claim (from pg_stat_statements where available)"),
		metric.WithUnit("1"),
		metric.WithExplicitBucketBoundaries(1, 10, 100, 1000, 10000, 100000),
	)
	if err != nil {
		return nil, fmt.Errorf("dequeue scan rows histogram: %w", err)
	}
	deadRatio, err := meter.Float64Gauge(
		"strait.queue.dead_tuple_ratio",
		metric.WithDescription("Ratio of dead tuples to live tuples per job_runs partition"),
	)
	if err != nil {
		return nil, fmt.Errorf("dead tuple ratio gauge: %w", err)
	}
	liveTuples, err := meter.Int64Gauge(
		"strait.queue.live_tuples",
		metric.WithDescription("Live tuple count per job_runs partition"),
	)
	if err != nil {
		return nil, fmt.Errorf("live tuples gauge: %w", err)
	}
	hotRatio, err := meter.Float64Gauge(
		"strait.queue.hot_update_ratio",
		metric.WithDescription("HOT updates / total updates per job_runs partition"),
	)
	if err != nil {
		return nil, fmt.Errorf("hot update ratio gauge: %w", err)
	}
	notifyDropped, err := meter.Int64Counter(
		"strait.queue.notify_dropped_total",
		metric.WithDescription("Number of queue wake notifications dropped because the wake channel was full"),
	)
	if err != nil {
		return nil, fmt.Errorf("notify dropped counter: %w", err)
	}
	notifyReconnects, err := meter.Int64Counter(
		"strait.queue.notify_reconnects_total",
		metric.WithDescription("Number of times the LISTEN connection had to reconnect"),
	)
	if err != nil {
		return nil, fmt.Errorf("notify reconnects counter: %w", err)
	}
	heartbeatReclaims, err := meter.Int64Counter(
		"strait.queue.heartbeat_reclaims_total",
		metric.WithDescription("Number of stuck runs reclaimed after a heartbeat went stale"),
	)
	if err != nil {
		return nil, fmt.Errorf("heartbeat reclaims counter: %w", err)
	}
	retryLag, err := meter.Float64Histogram(
		"strait.queue.retry_schedule_lag_seconds",
		metric.WithDescription("Delta between intended next_retry_at and observed dequeue time"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30),
	)
	if err != nil {
		return nil, fmt.Errorf("retry schedule lag histogram: %w", err)
	}
	masked, err := meter.Int64Gauge(
		"strait.queue.masked_rows_pending",
		metric.WithDescription("Number of rows marked visible_until but not yet physically dropped"),
	)
	if err != nil {
		return nil, fmt.Errorf("masked rows pending gauge: %w", err)
	}
	counterDrift, err := meter.Int64Gauge(
		"strait.queue.counter_drift",
		metric.WithDescription("Absolute drift observed between trigger-maintained counters and ground truth (job_active_counts + dlq_counts combined)"),
	)
	if err != nil {
		return nil, fmt.Errorf("counter drift gauge: %w", err)
	}

	partitionDequeueLag, err := meter.Float64Histogram(
		"strait.queue.partition_dequeue_lag_seconds",
		metric.WithDescription("Wall-clock duration of a DequeueNPartitioned call per partition"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5),
	)
	if err != nil {
		return nil, fmt.Errorf("partition dequeue lag histogram: %w", err)
	}
	claimToStart, err := meter.Float64Histogram(
		"strait.queue.claim_to_start_seconds",
		metric.WithDescription("Time between a run being claimed by the executor and the start of user work"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5),
	)
	if err != nil {
		return nil, fmt.Errorf("claim to start histogram: %w", err)
	}
	circuitTransitions, err := meter.Int64Counter(
		"strait.queue.circuit_state_transitions_total",
		metric.WithDescription("DB circuit breaker state transitions, labelled by from/to"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("circuit state transitions counter: %w", err)
	}
	outboxLag, err := meter.Float64Histogram(
		"strait.queue.outbox_lag_seconds",
		metric.WithDescription("Age of an outbox row at the time the flusher promoted it"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 5, 10, 30, 60, 300, 900),
	)
	if err != nil {
		return nil, fmt.Errorf("outbox lag histogram: %w", err)
	}
	backpressureTokens, err := meter.Int64Gauge(
		"strait.queue.backpressure_tokens_available",
		metric.WithDescription("Available backpressure tokens per project (sampled)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("backpressure tokens gauge: %w", err)
	}
	eventChannelDropped, err := meter.Int64Counter(
		"strait.worker.event_channel_dropped_total",
		metric.WithDescription("Executor lifecycle events dropped because the event channel was full or closed"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("event channel dropped counter: %w", err)
	}
	retryAttempts, err := meter.Float64Histogram(
		"strait.worker.retry_attempts",
		metric.WithDescription("Attempt number observed when a run was re-enqueued for retry"),
		metric.WithUnit("1"),
		metric.WithExplicitBucketBoundaries(1, 2, 3, 4, 5, 7, 10, 15, 20),
	)
	if err != nil {
		return nil, fmt.Errorf("retry attempts histogram: %w", err)
	}
	dlqOldestAge, err := meter.Float64Gauge(
		"strait.queue.dlq_oldest_unmasked_age_seconds",
		metric.WithDescription("Age in seconds of the oldest visible dead-letter row"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("dlq oldest unmasked age gauge: %w", err)
	}

	return &QueueMetrics{
		OldestQueuedAge:   oldestAge,
		DequeueScanRows:   scanRows,
		DeadTupleRatio:    deadRatio,
		LiveTuples:        liveTuples,
		HotUpdateRatio:    hotRatio,
		NotifyDropped:     notifyDropped,
		NotifyReconnects:  notifyReconnects,
		HeartbeatReclaims: heartbeatReclaims,
		RetryScheduleLag:  retryLag,
		MaskedRowsPending: masked,
		CounterDrift:      counterDrift,

		PartitionDequeueLag:         partitionDequeueLag,
		ClaimToStart:                claimToStart,
		CircuitStateTransitions:     circuitTransitions,
		OutboxLag:                   outboxLag,
		BackpressureTokensAvailable: backpressureTokens,
		EventChannelDropped:         eventChannelDropped,
		RetryAttempts:               retryAttempts,
		DLQOldestUnmaskedAge:        dlqOldestAge,
	}, nil
}

// RecordPartitionStats records gauge values for a single partition. The
// partition label is passed through as a dimension on every emitted point.
func (m *QueueMetrics) RecordPartitionStats(ctx context.Context, partition string, stats PartitionStats) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(attribute.String("partition", partition))
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
