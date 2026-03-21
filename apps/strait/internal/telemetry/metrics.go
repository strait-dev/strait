package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

type Metrics struct {
	RunTransitions           metric.Int64Counter
	DequeueDuration          metric.Float64Histogram
	DispatchDuration         metric.Float64Histogram
	DispatchErrors           metric.Int64Counter
	ReaperOperations         metric.Int64Counter
	ReaperRecordsDeleted     metric.Int64Counter
	CronTriggers             metric.Int64Counter
	PollerRunsQueued         metric.Int64Counter
	WorkflowTriggers         metric.Int64Counter
	WorkflowStepProgressions metric.Int64Counter
	QueueDepth               metric.Int64ObservableGauge
	ExecutionTraceDispatch   metric.Float64Histogram
	ExecutionTraceQueueWait  metric.Float64Histogram
	WebhookDeliveriesTotal   metric.Int64Counter
	WebhookDeliveryDuration  metric.Float64Histogram
	WebhookDeliveryAttempts  metric.Int64Counter
	WebhookRetryAttempts     metric.Int64Counter
	WebhookCircuitBreaker    metric.Int64Gauge
	EndpointHealthScore      metric.Float64Gauge
	WebhookPayloadBytes      metric.Int64Histogram
	AnalyticsQueryDuration   metric.Float64Histogram
	BulkOperationsTotal      metric.Int64Counter
	BulkItemsProcessed       metric.Int64Counter
	ChildCancellationsTotal  metric.Int64Counter
	LatencyAnomalies         metric.Int64Counter
	SnoozeTotal              metric.Int64Counter

	// Event trigger metrics.
	EventTriggersCreated     metric.Int64Counter
	EventTriggersReceived    metric.Int64Counter
	EventTriggersTimedOut    metric.Int64Counter
	EventTriggerWaitDuration metric.Float64Histogram

	WorkflowDependencyWaits  metric.Int64Counter
	WorkflowStepWaitDuration metric.Float64Histogram
	WorkflowStalledRuns      metric.Int64Counter

	// Worker pool gauges (reported via ObservePool callback).
	PoolRunningWorkers metric.Int64ObservableGauge
	PoolWaitingTasks   metric.Int64ObservableGauge

	// Worker pool lifetime counters (reported via ObservePool callback).
	PoolSubmittedTasks  metric.Int64ObservableCounter
	PoolCompletedTasks  metric.Int64ObservableCounter
	PoolSuccessfulTasks metric.Int64ObservableCounter
	PoolFailedTasks     metric.Int64ObservableCounter
	PoolDroppedTasks    metric.Int64ObservableCounter
	ShutdownTotal       metric.Int64Counter
	DLQDepth            metric.Int64Gauge
	QueueDepthPerJob    metric.Int64Gauge

	// Managed dispatch metrics.
	ManagedDispatchTotal    metric.Int64Counter
	ManagedDispatchDuration metric.Float64Histogram
	ManagedMachinesActive   metric.Int64UpDownCounter

	// DB connection pool metrics.
	DBPoolTotalConns    metric.Int64ObservableGauge
	DBPoolIdleConns     metric.Int64ObservableGauge
	DBPoolAcquiredConns metric.Int64ObservableGauge
	DBPoolMaxConns      metric.Int64ObservableGauge

	// HTTP request metrics (otelchi only generates traces, not metrics).
	HTTPRequestDuration  metric.Float64Histogram
	HTTPInflightRequests metric.Int64UpDownCounter

	// Operational depth gauges.
	WebhookBacklogDepth       metric.Int64Gauge
	ClickHouseExporterPending metric.Int64Gauge

	// Run lifecycle metrics.
	RunDuration metric.Float64Histogram

	// Scheduler drift metrics.
	CronDrift metric.Float64Histogram

	// ClickHouse exporter failure metrics.
	ClickHouseDroppedRecords metric.Int64Counter
	ClickHouseFlushFailures  metric.Int64Counter

	// Notification delivery metrics.
	NotificationDeliveriesTotal metric.Int64Counter

	// Log drain metrics.
	LogDrainEventsTotal metric.Int64Counter

	// Pub/sub metrics.
	PubSubPublishErrors metric.Int64Counter
}

// InitMetrics registers Prometheus metrics and returns the HTTP handler.
func InitMetrics(serviceName, environment string) (*Metrics, http.Handler, func(context.Context) error, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create prometheus exporter: %w", err)
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName),
	}
	if environment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentName(environment))
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			attrs...,
		),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create resource: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(provider)

	meter := otel.Meter(serviceName)

	runTransitions, err := meter.Int64Counter(
		"strait.run.transitions",
		metric.WithDescription("Total run status transitions"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create run transitions counter: %w", err)
	}

	dequeueDuration, err := meter.Float64Histogram(
		"strait.dequeue.duration",
		metric.WithDescription("Duration of dequeue operations"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create dequeue duration histogram: %w", err)
	}

	dispatchDuration, err := meter.Float64Histogram(
		"strait.dispatch.duration",
		metric.WithDescription("Duration of HTTP dispatch operations"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create dispatch duration histogram: %w", err)
	}

	dispatchErrors, err := meter.Int64Counter(
		"strait.dispatch.errors",
		metric.WithDescription("Total dispatch errors"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create dispatch errors counter: %w", err)
	}

	reaperOperations, err := meter.Int64Counter(
		"strait.reaper.operations",
		metric.WithDescription("Total reaper operations by operation and status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create reaper operations counter: %w", err)
	}

	reaperRecordsDeleted, err := meter.Int64Counter(
		"strait.reaper.records_deleted",
		metric.WithDescription("Total records deleted by reaper retention operations"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create reaper records deleted counter: %w", err)
	}

	cronTriggers, err := meter.Int64Counter(
		"strait.scheduler.cron_triggers",
		metric.WithDescription("Total cron trigger attempts by status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create cron triggers counter: %w", err)
	}

	pollerRunsQueued, err := meter.Int64Counter(
		"strait.scheduler.poller_runs_queued",
		metric.WithDescription("Total delayed runs queued by poller"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create poller runs queued counter: %w", err)
	}

	workflowTriggers, err := meter.Int64Counter(
		"strait.workflow.triggers",
		metric.WithDescription("Total workflow trigger attempts by status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create workflow triggers counter: %w", err)
	}

	workflowStepProgressions, err := meter.Int64Counter(
		"strait.workflow.step_progressions",
		metric.WithDescription("Total workflow step progressions by step type and status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create workflow step progressions counter: %w", err)
	}

	queueDepth, err := meter.Int64ObservableGauge(
		"strait.queue.depth",
		metric.WithDescription("Current queue depth by status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create queue depth gauge: %w", err)
	}

	executionTraceDispatch, err := meter.Float64Histogram(
		"strait.execution.trace.dispatch_duration",
		metric.WithDescription("HTTP dispatch roundtrip duration from execution trace"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(10, 50, 100, 250, 500, 1000, 2500, 5000, 10000),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create execution trace dispatch histogram: %w", err)
	}

	executionTraceQueueWait, err := meter.Float64Histogram(
		"strait.execution.trace.queue_wait_duration",
		metric.WithDescription("Queue wait duration from execution trace"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(10, 50, 100, 250, 500, 1000, 2500, 5000, 10000),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create execution trace queue wait histogram: %w", err)
	}

	webhookDeliveriesTotal, err := meter.Int64Counter(
		"strait_webhook_deliveries_total",
		metric.WithDescription("Total webhook deliveries by delivery status and retry policy"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create webhook deliveries counter: %w", err)
	}

	webhookDeliveryDuration, err := meter.Float64Histogram(
		"strait_webhook_delivery_duration_seconds",
		metric.WithDescription("Webhook delivery HTTP request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create webhook delivery duration histogram: %w", err)
	}

	webhookDeliveryAttempts, err := meter.Int64Counter(
		"strait_webhook_delivery_attempts_total",
		metric.WithDescription("Total webhook delivery attempts"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create webhook delivery attempts counter: %w", err)
	}

	webhookRetryAttempts, err := meter.Int64Counter(
		"strait.webhook.retry_attempts_total",
		metric.WithDescription("Total webhook delivery retry attempts"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create webhook retry attempts counter: %w", err)
	}

	webhookCircuitBreaker, err := meter.Int64Gauge(
		"strait_webhook_circuit_breaker_state",
		metric.WithDescription("Webhook circuit breaker state (1=current state, 0=other states)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create webhook circuit breaker gauge: %w", err)
	}

	endpointHealthScore, err := meter.Float64Gauge(
		"strait_endpoint_health_score",
		metric.WithDescription("Endpoint health score (0-100, lower is worse)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create endpoint health score gauge: %w", err)
	}

	webhookPayloadBytes, err := meter.Int64Histogram(
		"strait_webhook_payload_bytes",
		metric.WithDescription("Webhook payload size in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create webhook payload bytes histogram: %w", err)
	}

	eventTriggersCreated, err := meter.Int64Counter(
		"strait.event_triggers.created",
		metric.WithDescription("Total event triggers created"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create event triggers created counter: %w", err)
	}

	eventTriggersReceived, err := meter.Int64Counter(
		"strait.event_triggers.received",
		metric.WithDescription("Total events received (triggers completed)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create event triggers received counter: %w", err)
	}

	eventTriggersTimedOut, err := meter.Int64Counter(
		"strait.event_triggers.timed_out",
		metric.WithDescription("Total event triggers that expired"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create event triggers timed out counter: %w", err)
	}

	eventTriggerWaitDuration, err := meter.Float64Histogram(
		"strait.event_triggers.wait_duration",
		metric.WithDescription("Duration between trigger creation and event receipt"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create event trigger wait duration histogram: %w", err)
	}

	analyticsQueryDuration, err := meter.Float64Histogram(
		"strait.analytics.query_duration_seconds",
		metric.WithDescription("Duration of analytics query operations"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create analytics query duration histogram: %w", err)
	}

	bulkOperationsTotal, err := meter.Int64Counter(
		"strait.bulk.operations_total",
		metric.WithDescription("Total bulk operations"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create bulk operations total counter: %w", err)
	}

	bulkItemsProcessed, err := meter.Int64Counter(
		"strait.bulk.items_processed_total",
		metric.WithDescription("Total items processed in bulk operations"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create bulk items processed counter: %w", err)
	}

	childCancellationsTotal, err := meter.Int64Counter(
		"strait.bulk.child_cancellations_total",
		metric.WithDescription("Total cascading child run cancellations"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create child cancellations total counter: %w", err)
	}

	latencyAnomalies, err := meter.Int64Counter(
		"strait.run.latency_anomalies",
		metric.WithDescription("Total runs with duration exceeding 2x P95"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create latency anomalies counter: %w", err)
	}

	snoozeTotal, err := meter.Int64Counter(
		"strait.run.snooze_total",
		metric.WithDescription("Total runs snoozed (requeued without incrementing attempt)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create snooze total counter: %w", err)
	}

	workflowDependencyWaits, err := meter.Int64Counter(
		"strait.workflow.dependency_waits",
		metric.WithDescription("Total runs created in waiting state due to unsatisfied dependencies"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create workflow dependency waits counter: %w", err)
	}

	workflowStepWaitDuration, err := meter.Float64Histogram(
		"strait.workflow.step_wait_duration",
		metric.WithDescription("Time a workflow step spent waiting before running"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create workflow step wait duration histogram: %w", err)
	}

	workflowStalledRuns, err := meter.Int64Counter(
		"strait.workflow.stalled_runs",
		metric.WithDescription("Total stalled workflow runs detected by reaper"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create workflow stalled runs counter: %w", err)
	}

	poolRunning, err := meter.Int64ObservableGauge(
		"strait.pool.running_workers",
		metric.WithDescription("Number of goroutines currently executing tasks"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool running workers gauge: %w", err)
	}

	poolWaiting, err := meter.Int64ObservableGauge(
		"strait.pool.waiting_tasks",
		metric.WithDescription("Number of tasks waiting in the pool queue"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool waiting tasks gauge: %w", err)
	}

	poolSubmitted, err := meter.Int64ObservableCounter(
		"strait.pool.submitted_tasks",
		metric.WithDescription("Total tasks submitted to the pool"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool submitted tasks counter: %w", err)
	}

	poolCompleted, err := meter.Int64ObservableCounter(
		"strait.pool.completed_tasks",
		metric.WithDescription("Total tasks that finished (success or failure)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool completed tasks counter: %w", err)
	}

	poolSuccessful, err := meter.Int64ObservableCounter(
		"strait.pool.successful_tasks",
		metric.WithDescription("Total tasks that completed without error"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool successful tasks counter: %w", err)
	}

	poolFailed, err := meter.Int64ObservableCounter(
		"strait.pool.failed_tasks",
		metric.WithDescription("Total tasks that panicked or returned error"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool failed tasks counter: %w", err)
	}

	poolDropped, err := meter.Int64ObservableCounter(
		"strait.pool.dropped_tasks",
		metric.WithDescription("Total tasks dropped because pool was stopped"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool dropped tasks counter: %w", err)
	}

	shutdownTotal, err := meter.Int64Counter(
		"strait_shutdown_total",
		metric.WithDescription("Total worker shutdown attempts by reason"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create shutdown total counter: %w", err)
	}

	dlqDepth, err := meter.Int64Gauge(
		"strait_dlq_depth",
		metric.WithDescription("Current DLQ depth per job"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create dlq depth gauge: %w", err)
	}

	queueDepthPerJob, _ := meter.Int64Gauge("strait_queue_depth_per_job", metric.WithDescription("Queue depth per job"), metric.WithUnit("1"))

	managedDispatchTotal, err := meter.Int64Counter(
		"strait_managed_dispatch_total",
		metric.WithDescription("Total managed container dispatches by status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create managed dispatch total counter: %w", err)
	}

	managedDispatchDuration, err := meter.Float64Histogram(
		"strait_managed_dispatch_duration_seconds",
		metric.WithDescription("Duration of managed container dispatch operations"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 30, 60, 120, 300, 600),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create managed dispatch duration histogram: %w", err)
	}

	managedMachinesActive, err := meter.Int64UpDownCounter(
		"strait_managed_machines_active",
		metric.WithDescription("Number of managed containers currently running"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create managed machines active counter: %w", err)
	}

	dbPoolTotal, _ := meter.Int64ObservableGauge("strait_db_pool_total_conns", metric.WithDescription("Total DB pool connections"))
	dbPoolIdle, _ := meter.Int64ObservableGauge("strait_db_pool_idle_conns", metric.WithDescription("Idle DB pool connections"))
	dbPoolAcquired, _ := meter.Int64ObservableGauge("strait_db_pool_acquired_conns", metric.WithDescription("Acquired DB pool connections"))
	dbPoolMax, _ := meter.Int64ObservableGauge("strait_db_pool_max_conns", metric.WithDescription("Max DB pool connections"))

	httpRequestDuration, _ := meter.Float64Histogram(
		"strait.http.request_duration",
		metric.WithDescription("HTTP request duration"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	httpInflightRequests, _ := meter.Int64UpDownCounter(
		"strait.http.inflight_requests",
		metric.WithDescription("Number of HTTP requests currently being handled"),
		metric.WithUnit("1"),
	)
	webhookBacklogDepth, _ := meter.Int64Gauge(
		"strait_webhook_backlog_depth",
		metric.WithDescription("Number of pending webhook deliveries"),
		metric.WithUnit("1"),
	)
	clickhouseExporterPending, _ := meter.Int64Gauge(
		"strait_clickhouse_exporter_pending",
		metric.WithDescription("Number of records buffered in ClickHouse exporter"),
		metric.WithUnit("1"),
	)
	runDuration, _ := meter.Float64Histogram(
		"strait.run.duration",
		metric.WithDescription("Total run execution duration from start to finish"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300),
	)
	cronDrift, _ := meter.Float64Histogram(
		"strait.scheduler.cron_drift",
		metric.WithDescription("Delta between expected cron fire time and actual fire time"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 2, 5, 10, 30, 60),
	)

	clickhouseDroppedRecords, _ := meter.Int64Counter(
		"strait_clickhouse_dropped_records_total",
		metric.WithDescription("Total records dropped by ClickHouse exporter after max retries"),
		metric.WithUnit("1"),
	)
	clickhouseFlushFailures, _ := meter.Int64Counter(
		"strait_clickhouse_flush_failures_total",
		metric.WithDescription("Total ClickHouse exporter flush failures"),
		metric.WithUnit("1"),
	)
	notificationDeliveriesTotal, _ := meter.Int64Counter(
		"strait_notification_deliveries_total",
		metric.WithDescription("Total notification deliveries by status"),
		metric.WithUnit("1"),
	)
	logDrainEventsTotal, _ := meter.Int64Counter(
		"strait_log_drain_events_total",
		metric.WithDescription("Total log drain events by status"),
		metric.WithUnit("1"),
	)
	pubsubPublishErrors, _ := meter.Int64Counter(
		"strait_pubsub_publish_errors_total",
		metric.WithDescription("Total pub/sub publish failures"),
		metric.WithUnit("1"),
	)

	m := &Metrics{
		RunTransitions:              runTransitions,
		DequeueDuration:             dequeueDuration,
		DispatchDuration:            dispatchDuration,
		DispatchErrors:              dispatchErrors,
		ReaperOperations:            reaperOperations,
		ReaperRecordsDeleted:        reaperRecordsDeleted,
		CronTriggers:                cronTriggers,
		PollerRunsQueued:            pollerRunsQueued,
		WorkflowTriggers:            workflowTriggers,
		WorkflowStepProgressions:    workflowStepProgressions,
		QueueDepth:                  queueDepth,
		ExecutionTraceDispatch:      executionTraceDispatch,
		ExecutionTraceQueueWait:     executionTraceQueueWait,
		WebhookDeliveriesTotal:      webhookDeliveriesTotal,
		WebhookDeliveryDuration:     webhookDeliveryDuration,
		WebhookDeliveryAttempts:     webhookDeliveryAttempts,
		WebhookRetryAttempts:        webhookRetryAttempts,
		WebhookCircuitBreaker:       webhookCircuitBreaker,
		EndpointHealthScore:         endpointHealthScore,
		WebhookPayloadBytes:         webhookPayloadBytes,
		EventTriggersCreated:        eventTriggersCreated,
		EventTriggersReceived:       eventTriggersReceived,
		EventTriggersTimedOut:       eventTriggersTimedOut,
		EventTriggerWaitDuration:    eventTriggerWaitDuration,
		AnalyticsQueryDuration:      analyticsQueryDuration,
		BulkOperationsTotal:         bulkOperationsTotal,
		BulkItemsProcessed:          bulkItemsProcessed,
		ChildCancellationsTotal:     childCancellationsTotal,
		LatencyAnomalies:            latencyAnomalies,
		SnoozeTotal:                 snoozeTotal,
		WorkflowDependencyWaits:     workflowDependencyWaits,
		WorkflowStepWaitDuration:    workflowStepWaitDuration,
		WorkflowStalledRuns:         workflowStalledRuns,
		PoolRunningWorkers:          poolRunning,
		PoolWaitingTasks:            poolWaiting,
		PoolSubmittedTasks:          poolSubmitted,
		PoolCompletedTasks:          poolCompleted,
		PoolSuccessfulTasks:         poolSuccessful,
		PoolFailedTasks:             poolFailed,
		PoolDroppedTasks:            poolDropped,
		ShutdownTotal:               shutdownTotal,
		DLQDepth:                    dlqDepth,
		QueueDepthPerJob:            queueDepthPerJob,
		ManagedDispatchTotal:        managedDispatchTotal,
		ManagedDispatchDuration:     managedDispatchDuration,
		ManagedMachinesActive:       managedMachinesActive,
		DBPoolTotalConns:            dbPoolTotal,
		DBPoolIdleConns:             dbPoolIdle,
		DBPoolAcquiredConns:         dbPoolAcquired,
		DBPoolMaxConns:              dbPoolMax,
		HTTPRequestDuration:         httpRequestDuration,
		HTTPInflightRequests:        httpInflightRequests,
		WebhookBacklogDepth:         webhookBacklogDepth,
		ClickHouseExporterPending:   clickhouseExporterPending,
		RunDuration:                 runDuration,
		CronDrift:                   cronDrift,
		ClickHouseDroppedRecords:    clickhouseDroppedRecords,
		ClickHouseFlushFailures:     clickhouseFlushFailures,
		NotificationDeliveriesTotal: notificationDeliveriesTotal,
		LogDrainEventsTotal:         logDrainEventsTotal,
		PubSubPublishErrors:         pubsubPublishErrors,
	}

	slog.Info("prometheus metrics enabled")
	var shutdownOnce sync.Once
	var shutdownErr error
	shutdown := func(ctx context.Context) error {
		shutdownOnce.Do(func() {
			shutdownErr = provider.Shutdown(ctx)
		})
		return shutdownErr
	}
	return m, promhttp.Handler(), shutdown, nil
}

// PoolStatsProvider exposes pool counters for observable metric callbacks.
type PoolStatsProvider interface {
	RunningWorkers() int64
	WaitingTasks() uint64
	SubmittedTasks() uint64
	CompletedTasks() uint64
	SuccessfulTasks() uint64
	FailedTasks() uint64
	DroppedTasks() uint64
}

// ObservePool registers an asynchronous callback that reports pool stats on
// every Prometheus scrape. Call this after both Metrics and Pool are created.
func (m *Metrics) ObservePool(meter metric.Meter, pool PoolStatsProvider) error {
	saturateInt64 := func(v uint64) int64 {
		if v > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(v)
	}

	_, err := meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			o.ObserveInt64(m.PoolRunningWorkers, pool.RunningWorkers())
			o.ObserveInt64(m.PoolWaitingTasks, saturateInt64(pool.WaitingTasks()))
			o.ObserveInt64(m.PoolSubmittedTasks, saturateInt64(pool.SubmittedTasks()))
			o.ObserveInt64(m.PoolCompletedTasks, saturateInt64(pool.CompletedTasks()))
			o.ObserveInt64(m.PoolSuccessfulTasks, saturateInt64(pool.SuccessfulTasks()))
			o.ObserveInt64(m.PoolFailedTasks, saturateInt64(pool.FailedTasks()))
			o.ObserveInt64(m.PoolDroppedTasks, saturateInt64(pool.DroppedTasks()))
			return nil
		},
		m.PoolRunningWorkers,
		m.PoolWaitingTasks,
		m.PoolSubmittedTasks,
		m.PoolCompletedTasks,
		m.PoolSuccessfulTasks,
		m.PoolFailedTasks,
		m.PoolDroppedTasks,
	)
	return err
}
