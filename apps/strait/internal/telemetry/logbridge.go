package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	slogmulti "github.com/samber/slog-multi"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// InitLogBridge creates an slog handler that exports log records via OTLP to
// ClickHouse (or any OTel-compatible backend). Returns a shutdown function.
// If endpoint is empty, returns (nil, noop-shutdown, nil).
func InitLogBridge(ctx context.Context, serviceName, endpoint, environment string) (*slog.Logger, func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	if endpoint == "" {
		return nil, noop, nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("parse otel log endpoint: %w", err)
	}
	opts := []otlploghttp.Option{
		otlploghttp.WithEndpoint(u.Host),
	}
	if u.Scheme == "http" {
		opts = append(opts, otlploghttp.WithInsecure())
	}
	exporter, err := otlploghttp.New(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create otlp log exporter: %w", err)
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName),
	}
	if environment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentNameKey.String(environment))
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			attrs...,
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create log resource: %w", err)
	}

	provider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(exporter)),
		log.WithResource(res),
	)

	handler := otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(provider))
	logger := slog.New(handler)

	slog.Info("otel log bridge enabled", "endpoint", redactOTLPEndpoint(u))

	return logger, provider.Shutdown, nil
}

// NewTeeHandler creates a handler that writes to all provided handlers.
func NewTeeHandler(handlers ...slog.Handler) slog.Handler {
	return slogmulti.Fanout(handlers...)
}
