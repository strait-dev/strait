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

	TriggerAdmissionGuard    metric.Int64Counter
	TriggerDependencyGate    metric.Int64Counter
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

	NotificationDeliveryFailures metric.Int64Counter

	// DB connection pool metrics.
	DBPoolTotalConns    metric.Int64ObservableGauge
	DBPoolIdleConns     metric.Int64ObservableGauge
	DBPoolAcquiredConns metric.Int64ObservableGauge
	DBPoolMaxConns      metric.Int64ObservableGauge

	// HTTP request metrics (otelchi only generates traces, not metrics).
	HTTPRequestDuration  metric.Float64Histogram
	HTTPInflightRequests metric.Int64UpDownCounter

	// Pprof access metrics.
	PprofRequests metric.Int64Counter

	// Operational depth gauges.
	WebhookBacklogDepth       metric.Int64Gauge
	ClickHouseExporterPending metric.Int64Gauge

	// Run lifecycle metrics.
	RunDuration metric.Float64Histogram
	RunEndToEnd metric.Float64Histogram

	// Per-tenant run metrics (project_id label). Used for per-tenant capacity
	// governance, cost attribution, and the Grafana multi-tenant dashboard panels.
	//
	// JobDuration records execution wall-clock time by project, execution-mode
	// tier, and terminal status. Buckets are coarser than RunDuration to limit
	// series count.
	//
	// QueueLag records the time each run spent queued before execution began,
	// by project. Useful for per-tenant SLO monitoring (e.g. p99 queue lag < 5s).
	JobDuration metric.Float64Histogram
	QueueLag    metric.Float64Histogram

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

	// Audit event metrics.
	AuditEventsEmitted      metric.Int64Counter
	AuditEventsDropped      metric.Int64Counter
	AuditEventsTruncated    metric.Int64Counter
	AuditDetailsRedacted    metric.Int64Counter
	AuditEventsDeadlettered metric.Int64Counter
	AuditReclaimerSuccess   metric.Int64Counter
	AuditReclaimerFailed    metric.Int64Counter
	AuditReclaimerAbandoned metric.Int64Counter
	AuditDeadletterAged     metric.Int64Counter
	AuditRetentionDeleted   metric.Int64Counter
	AuditSIEMDropped        metric.Int64Counter
	AuditSIEMForwarded      metric.Int64Counter
	AuditSIEMFailed         metric.Int64Counter
	AuditSIEMCircuitOpen    metric.Int64Counter
	AuditSIEMBatchSize      metric.Int64Histogram
	// AuditSIEMBreakerState is a per-scrape observable gauge reporting
	// the SIEM circuit-breaker state (0=closed, 1=half_open, 2=open).
	// Wired via ObserveSIEMBreakerState against an
	// AuditSIEMBreakerStateProvider so operators can alert on a stuck
	// "open" state rather than the cumulative AuditSIEMCircuitOpen count.
	AuditSIEMBreakerState metric.Int64ObservableGauge
	// AuditEventsSyncFallback counts emit-time backpressure fallbacks to a
	// synchronous DB write, labeled by outcome ("success"|"failure"). Pairs
	// with AuditEventsDropped{reason="backpressure_degraded"} which counts
	// the trigger event regardless of outcome.
	AuditEventsSyncFallback metric.Int64Counter

	// Audit async drainer queue saturation gauges (reported via
	// ObserveAuditDrainer callback against an AuditDrainerStatsProvider).
	AuditDrainerQueueDepth    metric.Int64ObservableGauge
	AuditDrainerQueueCapacity metric.Int64ObservableGauge

	// Audit export row-cap metric. Incremented whenever an export stream
	// terminates because it reached the configured row cap, labeled by
	// project_id.
	AuditEventsExportCapped metric.Int64Counter

	// Audit chain verification counters. ChainVerifyTotal increments on
	// every verification attempt (pass or fail); ChainVerifyFailed only
	// on failures, labeled by reason. Together they power a failure rate
	// alert via failed/total.
	AuditChainVerifyTotal  metric.Int64Counter
	AuditChainVerifyFailed metric.Int64Counter

	// AuditDMLRestrictionStatus is a startup counter incremented exactly
	// once per process boot with the current migration 000187 enforcement
	// posture. status=enforced means UPDATE on audit_events is restricted
	// to the signature column for the database role; status=degraded
	// means the migration is a no-op (strait_app role missing or the
	// current role has full UPDATE). Alerts fire when degraded > 0 on
	// any deployment that is supposed to be SOC 2 evidence-gating.
	AuditRetryAttempts        metric.Int64Counter
	AuditDMLRestrictionStatus metric.Int64Counter
}

// InitMetrics registers Prometheus metrics and returns the HTTP handler.
func InitMetrics(serviceName, environment string) (*Metrics, http.Handler, func(context.Context) error, error) {
	provider, err := newPrometheusMeterProvider(serviceName, environment)
	if err != nil {
		return nil, nil, nil, err
	}
	otel.SetMeterProvider(provider)

	meter := otel.Meter(serviceName)

	m, err := initMetricInstruments(meter)
	if err != nil {
		return nil, nil, nil, err
	}

	slog.Info("prometheus metrics enabled")
	return m, promhttp.Handler(), newMetricsShutdown(provider), nil
}

func newPrometheusMeterProvider(serviceName, environment string) (*sdkmetric.MeterProvider, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("create prometheus exporter: %w", err)
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
		return nil, fmt.Errorf("create resource: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
		sdkmetric.WithResource(res),
	)
	return provider, nil
}

func initMetricInstruments(meter metric.Meter) (*Metrics, error) {
	runTransitions, err := meter.Int64Counter(
		"strait_run_transitions_total",
		metric.WithDescription("Total run status transitions"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create run transitions counter: %w", err)
	}

	dequeueDuration, err := meter.Float64Histogram(
		"strait_worker_dequeue_duration_seconds",
		metric.WithDescription("Duration of dequeue operations"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5),
	)
	if err != nil {
		return nil, fmt.Errorf("create dequeue duration histogram: %w", err)
	}

	dispatchDuration, err := meter.Float64Histogram(
		"strait_dispatch_duration_seconds",
		metric.WithDescription("Duration of HTTP dispatch operations"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create dispatch duration histogram: %w", err)
	}

	dispatchErrors, err := meter.Int64Counter(
		"strait_dispatch_errors_total",
		metric.WithDescription("Total dispatch errors"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create dispatch errors counter: %w", err)
	}

	reaperOperations, err := meter.Int64Counter(
		"strait_reaper_operations_total",
		metric.WithDescription("Total reaper operations by operation and status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create reaper operations counter: %w", err)
	}

	reaperRecordsDeleted, err := meter.Int64Counter(
		"strait_reaper_records_deleted_total",
		metric.WithDescription("Total records deleted by reaper retention operations"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create reaper records deleted counter: %w", err)
	}

	cronTriggers, err := meter.Int64Counter(
		"strait_scheduler_cron_triggers_total",
		metric.WithDescription("Total cron trigger attempts by status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create cron triggers counter: %w", err)
	}

	pollerRunsQueued, err := meter.Int64Counter(
		"strait_scheduler_poller_runs_queued_total",
		metric.WithDescription("Total delayed runs queued by poller"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create poller runs queued counter: %w", err)
	}

	workflowTriggers, err := meter.Int64Counter(
		"strait_workflow_triggers_total",
		metric.WithDescription("Total workflow trigger attempts by status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create workflow triggers counter: %w", err)
	}

	workflowStepProgressions, err := meter.Int64Counter(
		"strait_workflow_step_progressions_total",
		metric.WithDescription("Total workflow step progressions by step type and status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create workflow step progressions counter: %w", err)
	}

	queueDepth, err := meter.Int64ObservableGauge(
		"strait_queue_depth",
		metric.WithDescription("Current queue depth by status"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create queue depth gauge: %w", err)
	}

	executionTraceDispatch, err := meter.Float64Histogram(
		"strait_execution_trace_dispatch_duration_seconds",
		metric.WithDescription("HTTP dispatch roundtrip duration from execution trace"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(10, 50, 100, 250, 500, 1000, 2500, 5000, 10000),
	)
	if err != nil {
		return nil, fmt.Errorf("create execution trace dispatch histogram: %w", err)
	}

	executionTraceQueueWait, err := meter.Float64Histogram(
		"strait_execution_trace_queue_wait_duration_seconds",
		metric.WithDescription("Queue wait duration from execution trace"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(10, 50, 100, 250, 500, 1000, 2500, 5000, 10000),
	)
	if err != nil {
		return nil, fmt.Errorf("create execution trace queue wait histogram: %w", err)
	}

	webhookDeliveriesTotal, err := meter.Int64Counter(
		"strait_webhook_deliveries_total",
		metric.WithDescription("Total webhook deliveries by delivery status and retry policy"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create webhook deliveries counter: %w", err)
	}

	webhookDeliveryDuration, err := meter.Float64Histogram(
		"strait_webhook_delivery_duration_seconds",
		metric.WithDescription("Webhook delivery HTTP request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30),
	)
	if err != nil {
		return nil, fmt.Errorf("create webhook delivery duration histogram: %w", err)
	}

	webhookDeliveryAttempts, err := meter.Int64Counter(
		"strait_webhook_delivery_attempts_total",
		metric.WithDescription("Total webhook delivery attempts"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create webhook delivery attempts counter: %w", err)
	}

	webhookRetryAttempts, err := meter.Int64Counter(
		"strait_webhook_retry_attempts_total",
		metric.WithDescription("Total webhook delivery retry attempts"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create webhook retry attempts counter: %w", err)
	}

	webhookCircuitBreaker, err := meter.Int64Gauge(
		"strait_webhook_circuit_breaker_state",
		metric.WithDescription("Webhook circuit breaker state (1=current state, 0=other states)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create webhook circuit breaker gauge: %w", err)
	}

	endpointHealthScore, err := meter.Float64Gauge(
		"strait_endpoint_health_score",
		metric.WithDescription("Endpoint health score (0-100, lower is worse)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create endpoint health score gauge: %w", err)
	}

	webhookPayloadBytes, err := meter.Int64Histogram(
		"strait_webhook_payload_bytes",
		metric.WithDescription("Webhook payload size in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("create webhook payload bytes histogram: %w", err)
	}

	eventTriggersCreated, err := meter.Int64Counter(
		"strait_event_triggers_created_total",
		metric.WithDescription("Total event triggers created"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create event triggers created counter: %w", err)
	}

	eventTriggersReceived, err := meter.Int64Counter(
		"strait_event_triggers_received_total",
		metric.WithDescription("Total events received (triggers completed)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create event triggers received counter: %w", err)
	}

	eventTriggersTimedOut, err := meter.Int64Counter(
		"strait_event_triggers_timed_out_total",
		metric.WithDescription("Total event triggers that expired"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create event triggers timed out counter: %w", err)
	}

	eventTriggerWaitDuration, err := meter.Float64Histogram(
		"strait_event_triggers_wait_duration_seconds",
		metric.WithDescription("Duration between trigger creation and event receipt"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("create event trigger wait duration histogram: %w", err)
	}

	analyticsQueryDuration, err := meter.Float64Histogram(
		"strait_analytics_query_duration_seconds",
		metric.WithDescription("Duration of analytics query operations"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5),
	)
	if err != nil {
		return nil, fmt.Errorf("create analytics query duration histogram: %w", err)
	}

	bulkOperationsTotal, err := meter.Int64Counter(
		"strait_bulk_operations_total",
		metric.WithDescription("Total bulk operations"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create bulk operations total counter: %w", err)
	}

	bulkItemsProcessed, err := meter.Int64Counter(
		"strait_bulk_items_processed_total",
		metric.WithDescription("Total items processed in bulk operations"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create bulk items processed counter: %w", err)
	}

	childCancellationsTotal, err := meter.Int64Counter(
		"strait_bulk_child_cancellations_total",
		metric.WithDescription("Total cascading child run cancellations"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create child cancellations total counter: %w", err)
	}

	latencyAnomalies, err := meter.Int64Counter(
		"strait_run_latency_anomalies_total",
		metric.WithDescription("Total runs with duration exceeding 2x P95"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create latency anomalies counter: %w", err)
	}

	snoozeTotal, err := meter.Int64Counter(
		"strait_run_snooze_total",
		metric.WithDescription("Total runs snoozed (requeued without incrementing attempt)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create snooze total counter: %w", err)
	}

	workflowDependencyWaits, err := meter.Int64Counter(
		"strait_workflow_dependency_waits_total",
		metric.WithDescription("Total runs created in waiting state due to unsatisfied dependencies"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create workflow dependency waits counter: %w", err)
	}

	triggerDependencyGate, err := meter.Int64Counter(
		"strait_trigger_dependency_gate_total",
		metric.WithDescription("Total trigger dependency gate decisions, labeled by result"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create trigger dependency gate counter: %w", err)
	}

	triggerAdmissionGuard, err := meter.Int64Counter(
		"strait_trigger_admission_guard_total",
		metric.WithDescription("Total trigger admission guard decisions, labeled by path"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create trigger admission guard counter: %w", err)
	}

	workflowStepWaitDuration, err := meter.Float64Histogram(
		"strait_workflow_step_wait_duration_seconds",
		metric.WithDescription("Time a workflow step spent waiting before running"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("create workflow step wait duration histogram: %w", err)
	}

	workflowStalledRuns, err := meter.Int64Counter(
		"strait_workflow_stalled_runs_total",
		metric.WithDescription("Total stalled workflow runs detected by reaper"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create workflow stalled runs counter: %w", err)
	}

	poolRunning, err := meter.Int64ObservableGauge(
		"strait_worker_pool_running",
		metric.WithDescription("Number of goroutines currently executing tasks"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create pool running workers gauge: %w", err)
	}

	poolWaiting, err := meter.Int64ObservableGauge(
		"strait_worker_pool_waiting",
		metric.WithDescription("Number of tasks waiting in the pool queue"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create pool waiting tasks gauge: %w", err)
	}

	poolSubmitted, err := meter.Int64ObservableCounter(
		"strait_worker_pool_submitted_total",
		metric.WithDescription("Total tasks submitted to the pool"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create pool submitted tasks counter: %w", err)
	}

	poolCompleted, err := meter.Int64ObservableCounter(
		"strait_worker_pool_completed_total",
		metric.WithDescription("Total tasks that finished (success or failure)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create pool completed tasks counter: %w", err)
	}

	poolSuccessful, err := meter.Int64ObservableCounter(
		"strait_worker_pool_successful_total",
		metric.WithDescription("Total tasks that completed without error"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create pool successful tasks counter: %w", err)
	}

	poolFailed, err := meter.Int64ObservableCounter(
		"strait_worker_pool_failed_total",
		metric.WithDescription("Total tasks that panicked or returned error"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create pool failed tasks counter: %w", err)
	}

	poolDropped, err := meter.Int64ObservableCounter(
		"strait_worker_pool_dropped_total",
		metric.WithDescription("Total tasks dropped because pool was stopped"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create pool dropped tasks counter: %w", err)
	}

	shutdownTotal, err := meter.Int64Counter(
		"strait_shutdown_total",
		metric.WithDescription("Total worker shutdown attempts by reason"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create shutdown total counter: %w", err)
	}

	dlqDepth, err := meter.Int64Gauge(
		"strait_dlq_depth",
		metric.WithDescription("Current DLQ depth per job"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create dlq depth gauge: %w", err)
	}

	queueDepthPerJob, _ := meter.Int64Gauge("strait_queue_depth_per_job", metric.WithDescription("Queue depth per job"), metric.WithUnit("1"))

	notifDeliveryFailures, err := meter.Int64Counter(
		"strait_notification_delivery_failures_total",
		metric.WithDescription("Total notification delivery creation failures"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create notification delivery failures counter: %w", err)
	}

	dbPoolTotal, _ := meter.Int64ObservableGauge("strait_db_pool_total_conns", metric.WithDescription("Total DB pool connections"))
	dbPoolIdle, _ := meter.Int64ObservableGauge("strait_db_pool_idle_conns", metric.WithDescription("Idle DB pool connections"))
	dbPoolAcquired, _ := meter.Int64ObservableGauge("strait_db_pool_acquired_conns", metric.WithDescription("Acquired DB pool connections"))
	dbPoolMax, _ := meter.Int64ObservableGauge("strait_db_pool_max_conns", metric.WithDescription("Max DB pool connections"))

	httpRequestDuration, _ := meter.Float64Histogram(
		"strait_http_request_duration_seconds",
		metric.WithDescription("HTTP request duration"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	httpInflightRequests, _ := meter.Int64UpDownCounter(
		"strait_http_inflight_requests",
		metric.WithDescription("Number of HTTP requests currently being handled"),
		metric.WithUnit("1"),
	)
	pprofRequests, _ := meter.Int64Counter(
		"strait_pprof_requests_total",
		metric.WithDescription("Total pprof requests by endpoint and HTTP status"),
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
		"strait_run_duration_seconds",
		metric.WithDescription("Total run execution duration from start to finish"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300),
	)
	runEndToEnd, _ := meter.Float64Histogram(
		"strait_run_end_to_end_seconds",
		metric.WithDescription("End-to-end run latency from created_at to finished_at"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 5, 10, 30, 60, 300, 900, 3600),
	)

	// Per-tenant metrics. Coarser buckets than RunDuration to control cardinality.
	jobDuration, _ := meter.Float64Histogram(
		"strait_job_duration_seconds",
		metric.WithDescription("Job execution duration by project, execution-mode tier, and terminal status"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(1, 5, 15, 30, 60, 120, 300, 600, 900),
	)
	queueLag, _ := meter.Float64Histogram(
		"strait_queue_lag_seconds",
		metric.WithDescription("Time each run spent queued before execution began, by project"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 2, 5, 10, 30, 60, 120),
	)
	cronDrift, _ := meter.Float64Histogram(
		"strait_scheduler_cron_drift_seconds",
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

	auditEventsEmitted, _ := meter.Int64Counter(
		"strait_audit_events_emitted_total",
		metric.WithDescription("Total audit events successfully written to the audit log"),
		metric.WithUnit("1"),
	)
	auditEventsDropped, _ := meter.Int64Counter(
		"strait_audit_events_dropped_total",
		metric.WithDescription("Total audit events dropped (async buffer full or write failure)"),
		metric.WithUnit("1"),
	)
	auditEventsTruncated, _ := meter.Int64Counter(
		"strait_audit_events_truncated_total",
		metric.WithDescription("Total audit events whose details were truncated for exceeding the size cap"),
		metric.WithUnit("1"),
	)
	auditDetailsRedacted, _ := meter.Int64Counter(
		"strait_audit_details_redacted_total",
		metric.WithDescription("Total audit events whose details had secret-shaped substrings redacted at emit time, labeled by shape"),
		metric.WithUnit("1"),
	)
	auditEventsDeadlettered, _ := meter.Int64Counter(
		"strait_audit_events_deadlettered_total",
		metric.WithDescription("Total audit events spilled to the deadletter table after retry exhaustion"),
		metric.WithUnit("1"),
	)
	auditReclaimerSuccess, _ := meter.Int64Counter(
		"strait_audit_reclaimer_success_total",
		metric.WithDescription("Total audit deadletter events successfully reclaimed into the primary chain"),
		metric.WithUnit("1"),
	)
	auditReclaimerFailed, _ := meter.Int64Counter(
		"strait_audit_reclaimer_failed_total",
		metric.WithDescription("Total audit deadletter reclaim attempts that failed, labeled by reason"),
		metric.WithUnit("1"),
	)
	auditReclaimerAbandoned, _ := meter.Int64Counter(
		"strait_audit_reclaimer_abandoned_total",
		metric.WithDescription("Total audit deadletter rows whose reclaim attempts hit the configured max-attempts cap and were skipped this tick"),
		metric.WithUnit("1"),
	)
	auditDeadletterAged, _ := meter.Int64Counter(
		"strait_audit_deadletter_aged_total",
		metric.WithDescription("Total audit deadletter rows dropped by the DLQ retention reaper after exceeding AUDIT_DLQ_MAX_AGE_DAYS, labeled by project_id"),
		metric.WithUnit("1"),
	)
	auditRetentionDeleted, _ := meter.Int64Counter(
		"strait_audit_retention_deleted_total",
		metric.WithDescription("Total audit events deleted by the retention reaper, labeled by project_id"),
		metric.WithUnit("1"),
	)
	auditSIEMDropped, _ := meter.Int64Counter(
		"strait_audit_siem_dropped_total",
		metric.WithDescription("Total audit events dropped from the SIEM forwarding queue, labeled by reason"),
		metric.WithUnit("1"),
	)
	auditSIEMForwarded, _ := meter.Int64Counter(
		"strait_audit_siem_forwarded_total",
		metric.WithDescription("Total audit events successfully forwarded to the SIEM endpoint"),
		metric.WithUnit("1"),
	)
	auditSIEMFailed, _ := meter.Int64Counter(
		"strait_audit_siem_failed_total",
		metric.WithDescription("Total audit SIEM forward failures, labeled by reason"),
		metric.WithUnit("1"),
	)
	auditSIEMCircuitOpen, _ := meter.Int64Counter(
		"strait_audit_siem_circuit_open_total",
		metric.WithDescription("Total transitions of the audit SIEM circuit breaker into the open state"),
		metric.WithUnit("1"),
	)
	auditSIEMBatchSize, _ := meter.Int64Histogram(
		"strait_audit_siem_batch_size_items",
		metric.WithDescription("Audit SIEM forwarded batch size in events"),
		metric.WithUnit("1"),
	)
	auditSIEMBreakerState, _ := meter.Int64ObservableGauge(
		"strait_audit_siem_breaker_state",
		metric.WithDescription("Current audit SIEM circuit-breaker state (0=closed, 1=half_open, 2=open)"),
		metric.WithUnit("1"),
	)
	auditEventsSyncFallback, _ := meter.Int64Counter(
		"strait_audit_events_sync_fallback_total",
		metric.WithDescription("Total async-drainer backpressure events that fell back to a synchronous DB write, labeled by outcome (success|failure)"),
		metric.WithUnit("1"),
	)
	auditDrainerQueueDepth, _ := meter.Int64ObservableGauge(
		"strait_audit_drainer_queue_depth",
		metric.WithDescription("Current depth of the async audit-emit drain channel"),
		metric.WithUnit("1"),
	)
	auditDrainerQueueCapacity, _ := meter.Int64ObservableGauge(
		"strait_audit_drainer_queue_capacity",
		metric.WithDescription("Buffer capacity of the async audit-emit drain channel"),
		metric.WithUnit("1"),
	)
	auditEventsExportCapped, _ := meter.Int64Counter(
		"strait_audit_events_export_capped_total",
		metric.WithDescription("Total audit exports that terminated because they hit the configured row cap, labeled by project_id"),
		metric.WithUnit("1"),
	)
	auditChainVerifyTotal, _ := meter.Int64Counter(
		"strait_audit_chain_verify_total",
		metric.WithDescription("Total audit chain verification attempts (pass or fail)"),
		metric.WithUnit("1"),
	)
	auditChainVerifyFailed, _ := meter.Int64Counter(
		"strait_audit_chain_verify_failed_total",
		metric.WithDescription("Total audit chain verification attempts that did not pass, labeled by reason"),
		metric.WithUnit("1"),
	)
	auditRetryAttempts, _ := meter.Int64Counter(
		"strait_audit_retry_attempts_total",
		metric.WithDescription("Total audit event write retry attempts, labeled by attempt number (1-3) and outcome (success|exhausted)"),
		metric.WithUnit("1"),
	)
	auditDMLRestrictionStatus, _ := meter.Int64Counter(
		"strait_audit_dml_restriction_status",
		metric.WithDescription("Startup posture of migration 000187 audit_events DML restrictions, labeled by status (enforced|degraded)"),
		metric.WithUnit("1"),
	)

	m := &Metrics{
		RunTransitions:               runTransitions,
		DequeueDuration:              dequeueDuration,
		DispatchDuration:             dispatchDuration,
		DispatchErrors:               dispatchErrors,
		ReaperOperations:             reaperOperations,
		ReaperRecordsDeleted:         reaperRecordsDeleted,
		CronTriggers:                 cronTriggers,
		PollerRunsQueued:             pollerRunsQueued,
		WorkflowTriggers:             workflowTriggers,
		WorkflowStepProgressions:     workflowStepProgressions,
		QueueDepth:                   queueDepth,
		ExecutionTraceDispatch:       executionTraceDispatch,
		ExecutionTraceQueueWait:      executionTraceQueueWait,
		WebhookDeliveriesTotal:       webhookDeliveriesTotal,
		WebhookDeliveryDuration:      webhookDeliveryDuration,
		WebhookDeliveryAttempts:      webhookDeliveryAttempts,
		WebhookRetryAttempts:         webhookRetryAttempts,
		WebhookCircuitBreaker:        webhookCircuitBreaker,
		EndpointHealthScore:          endpointHealthScore,
		WebhookPayloadBytes:          webhookPayloadBytes,
		EventTriggersCreated:         eventTriggersCreated,
		EventTriggersReceived:        eventTriggersReceived,
		EventTriggersTimedOut:        eventTriggersTimedOut,
		EventTriggerWaitDuration:     eventTriggerWaitDuration,
		AnalyticsQueryDuration:       analyticsQueryDuration,
		BulkOperationsTotal:          bulkOperationsTotal,
		BulkItemsProcessed:           bulkItemsProcessed,
		ChildCancellationsTotal:      childCancellationsTotal,
		LatencyAnomalies:             latencyAnomalies,
		SnoozeTotal:                  snoozeTotal,
		TriggerAdmissionGuard:        triggerAdmissionGuard,
		TriggerDependencyGate:        triggerDependencyGate,
		WorkflowDependencyWaits:      workflowDependencyWaits,
		WorkflowStepWaitDuration:     workflowStepWaitDuration,
		WorkflowStalledRuns:          workflowStalledRuns,
		PoolRunningWorkers:           poolRunning,
		PoolWaitingTasks:             poolWaiting,
		PoolSubmittedTasks:           poolSubmitted,
		PoolCompletedTasks:           poolCompleted,
		PoolSuccessfulTasks:          poolSuccessful,
		PoolFailedTasks:              poolFailed,
		PoolDroppedTasks:             poolDropped,
		ShutdownTotal:                shutdownTotal,
		DLQDepth:                     dlqDepth,
		QueueDepthPerJob:             queueDepthPerJob,
		NotificationDeliveryFailures: notifDeliveryFailures,
		DBPoolTotalConns:             dbPoolTotal,
		DBPoolIdleConns:              dbPoolIdle,
		DBPoolAcquiredConns:          dbPoolAcquired,
		DBPoolMaxConns:               dbPoolMax,
		HTTPRequestDuration:          httpRequestDuration,
		HTTPInflightRequests:         httpInflightRequests,
		PprofRequests:                pprofRequests,
		WebhookBacklogDepth:          webhookBacklogDepth,
		ClickHouseExporterPending:    clickhouseExporterPending,
		RunDuration:                  runDuration,
		RunEndToEnd:                  runEndToEnd,
		JobDuration:                  jobDuration,
		QueueLag:                     queueLag,
		CronDrift:                    cronDrift,
		ClickHouseDroppedRecords:     clickhouseDroppedRecords,
		ClickHouseFlushFailures:      clickhouseFlushFailures,
		NotificationDeliveriesTotal:  notificationDeliveriesTotal,
		LogDrainEventsTotal:          logDrainEventsTotal,
		PubSubPublishErrors:          pubsubPublishErrors,
		AuditEventsEmitted:           auditEventsEmitted,
		AuditEventsDropped:           auditEventsDropped,
		AuditEventsTruncated:         auditEventsTruncated,
		AuditDetailsRedacted:         auditDetailsRedacted,
		AuditEventsDeadlettered:      auditEventsDeadlettered,
		AuditReclaimerSuccess:        auditReclaimerSuccess,
		AuditReclaimerFailed:         auditReclaimerFailed,
		AuditReclaimerAbandoned:      auditReclaimerAbandoned,
		AuditDeadletterAged:          auditDeadletterAged,
		AuditRetentionDeleted:        auditRetentionDeleted,
		AuditSIEMDropped:             auditSIEMDropped,
		AuditSIEMForwarded:           auditSIEMForwarded,
		AuditSIEMFailed:              auditSIEMFailed,
		AuditSIEMCircuitOpen:         auditSIEMCircuitOpen,
		AuditSIEMBatchSize:           auditSIEMBatchSize,
		AuditSIEMBreakerState:        auditSIEMBreakerState,
		AuditEventsSyncFallback:      auditEventsSyncFallback,
		AuditDrainerQueueDepth:       auditDrainerQueueDepth,
		AuditDrainerQueueCapacity:    auditDrainerQueueCapacity,
		AuditEventsExportCapped:      auditEventsExportCapped,
		AuditChainVerifyTotal:        auditChainVerifyTotal,
		AuditChainVerifyFailed:       auditChainVerifyFailed,
		AuditRetryAttempts:           auditRetryAttempts,
		AuditDMLRestrictionStatus:    auditDMLRestrictionStatus,
	}
	return m, nil
}

func newMetricsShutdown(provider *sdkmetric.MeterProvider) func(context.Context) error {
	var shutdownOnce sync.Once
	var shutdownErr error
	return func(ctx context.Context) error {
		shutdownOnce.Do(func() {
			shutdownErr = provider.Shutdown(ctx)
		})
		return shutdownErr
	}
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

// AuditDrainerStatsProvider exposes the async audit-emit channel's
// instantaneous depth and static capacity for the ObserveAuditDrainer
// asynchronous gauge callback.
type AuditDrainerStatsProvider interface {
	AuditDrainerQueueDepth() int64
	AuditDrainerQueueCapacity() int64
}

// AuditSIEMBreakerStateProvider exposes the current SIEM circuit-breaker
// state as an int64 (0=closed, 1=half_open, 2=open). Implemented by
// *logdrain.AuditSIEMDrain via its BreakerState() method.
type AuditSIEMBreakerStateProvider interface {
	BreakerState() int64
}

// ObserveSIEMBreakerState wires an asynchronous callback that reports the
// current SIEM circuit-breaker state on every Prometheus scrape. Returns
// nil when the provider is nil or the gauge is not configured (callable
// safely from the worker process where SIEM may be disabled).
func (m *Metrics) ObserveSIEMBreakerState(meter metric.Meter, provider AuditSIEMBreakerStateProvider) error {
	if provider == nil || m.AuditSIEMBreakerState == nil {
		return nil
	}
	_, err := meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			o.ObserveInt64(m.AuditSIEMBreakerState, provider.BreakerState())
			return nil
		},
		m.AuditSIEMBreakerState,
	)
	return err
}

// ObserveAuditDrainer registers an asynchronous callback that reports
// the audit async drainer's queue depth and capacity on every Prometheus
// scrape. Call after both Metrics and the Server (which implements the
// provider) are created.
func (m *Metrics) ObserveAuditDrainer(meter metric.Meter, provider AuditDrainerStatsProvider) error {
	if m.AuditDrainerQueueDepth == nil || m.AuditDrainerQueueCapacity == nil {
		return nil
	}
	_, err := meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			o.ObserveInt64(m.AuditDrainerQueueDepth, provider.AuditDrainerQueueDepth())
			o.ObserveInt64(m.AuditDrainerQueueCapacity, provider.AuditDrainerQueueCapacity())
			return nil
		},
		m.AuditDrainerQueueDepth,
		m.AuditDrainerQueueCapacity,
	)
	return err
}
