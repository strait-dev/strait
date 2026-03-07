package telemetry

import (
	"context"
	"fmt"
	"log/slog"
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

	m := &Metrics{
		RunTransitions:          runTransitions,
		DequeueDuration:         dequeueDuration,
		DispatchDuration:        dispatchDuration,
		DispatchErrors:          dispatchErrors,
		ExecutionTraceDispatch:  executionTraceDispatch,
		ExecutionTraceQueueWait: executionTraceQueueWait,
	}

	slog.Info("prometheus metrics enabled")
	return m, promhttp.Handler(), provider.Shutdown, nil
}
