package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type Metrics struct {
	RunTransitions          metric.Int64Counter
	DequeueDuration         metric.Float64Histogram
	DispatchDuration        metric.Float64Histogram
	DispatchErrors          metric.Int64Counter
	ExecutionTraceDispatch  metric.Float64Histogram
	ExecutionTraceQueueWait metric.Float64Histogram

	// Worker pool gauges (reported via ObservePool callback).
	PoolRunningWorkers metric.Int64ObservableGauge
	PoolWaitingTasks   metric.Int64ObservableGauge

	// Worker pool lifetime counters (reported via ObservePool callback).
	PoolSubmittedTasks  metric.Int64ObservableCounter
	PoolCompletedTasks  metric.Int64ObservableCounter
	PoolSuccessfulTasks metric.Int64ObservableCounter
	PoolFailedTasks     metric.Int64ObservableCounter
	PoolDroppedTasks    metric.Int64ObservableCounter
}

// InitMetrics registers Prometheus metrics and returns the HTTP handler.
func InitMetrics(serviceName string) (*Metrics, http.Handler, func(context.Context) error, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create prometheus exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
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
		"orchestrator.run.transitions",
		metric.WithDescription("Total run status transitions"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create run transitions counter: %w", err)
	}

	dequeueDuration, err := meter.Float64Histogram(
		"orchestrator.dequeue.duration",
		metric.WithDescription("Duration of dequeue operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create dequeue duration histogram: %w", err)
	}

	dispatchDuration, err := meter.Float64Histogram(
		"orchestrator.dispatch.duration",
		metric.WithDescription("Duration of HTTP dispatch operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create dispatch duration histogram: %w", err)
	}

	dispatchErrors, err := meter.Int64Counter(
		"orchestrator.dispatch.errors",
		metric.WithDescription("Total dispatch errors"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create dispatch errors counter: %w", err)
	}

	executionTraceDispatch, err := meter.Float64Histogram(
		"orchestrator.execution.trace.dispatch_duration",
		metric.WithDescription("HTTP dispatch roundtrip duration from execution trace"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create execution trace dispatch histogram: %w", err)
	}

	executionTraceQueueWait, err := meter.Float64Histogram(
		"orchestrator.execution.trace.queue_wait_duration",
		metric.WithDescription("Queue wait duration from execution trace"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create execution trace queue wait histogram: %w", err)
	}

	poolRunning, err := meter.Int64ObservableGauge(
		"orchestrator.pool.running_workers",
		metric.WithDescription("Number of goroutines currently executing tasks"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool running workers gauge: %w", err)
	}

	poolWaiting, err := meter.Int64ObservableGauge(
		"orchestrator.pool.waiting_tasks",
		metric.WithDescription("Number of tasks waiting in the pool queue"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool waiting tasks gauge: %w", err)
	}

	poolSubmitted, err := meter.Int64ObservableCounter(
		"orchestrator.pool.submitted_tasks",
		metric.WithDescription("Total tasks submitted to the pool"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool submitted tasks counter: %w", err)
	}

	poolCompleted, err := meter.Int64ObservableCounter(
		"orchestrator.pool.completed_tasks",
		metric.WithDescription("Total tasks that finished (success or failure)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool completed tasks counter: %w", err)
	}

	poolSuccessful, err := meter.Int64ObservableCounter(
		"orchestrator.pool.successful_tasks",
		metric.WithDescription("Total tasks that completed without error"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool successful tasks counter: %w", err)
	}

	poolFailed, err := meter.Int64ObservableCounter(
		"orchestrator.pool.failed_tasks",
		metric.WithDescription("Total tasks that panicked or returned error"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool failed tasks counter: %w", err)
	}

	poolDropped, err := meter.Int64ObservableCounter(
		"orchestrator.pool.dropped_tasks",
		metric.WithDescription("Total tasks dropped because pool was stopped"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create pool dropped tasks counter: %w", err)
	}

	m := &Metrics{
		RunTransitions:          runTransitions,
		DequeueDuration:         dequeueDuration,
		DispatchDuration:        dispatchDuration,
		DispatchErrors:          dispatchErrors,
		ExecutionTraceDispatch:  executionTraceDispatch,
		ExecutionTraceQueueWait: executionTraceQueueWait,
		PoolRunningWorkers:      poolRunning,
		PoolWaitingTasks:        poolWaiting,
		PoolSubmittedTasks:      poolSubmitted,
		PoolCompletedTasks:      poolCompleted,
		PoolSuccessfulTasks:     poolSuccessful,
		PoolFailedTasks:         poolFailed,
		PoolDroppedTasks:        poolDropped,
	}

	slog.Info("prometheus metrics enabled")
	return m, promhttp.Handler(), provider.Shutdown, nil
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
