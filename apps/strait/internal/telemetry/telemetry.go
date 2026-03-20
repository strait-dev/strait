package telemetry

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// Init sets up OpenTelemetry tracing with an OTLP HTTP exporter.
// Returns a shutdown function that should be deferred.
// If endpoint is empty, tracing is disabled (noop).
func Init(ctx context.Context, serviceName, endpoint, environment string) (func(context.Context) error, error) {
	if endpoint == "" {
		slog.Info("otel tracing disabled (no endpoint configured)")
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
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

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("otel tracing enabled", "endpoint", endpoint)

	return tp.Shutdown, nil
}
