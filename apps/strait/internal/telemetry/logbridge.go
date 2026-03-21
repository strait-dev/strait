package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
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
		return nil, nil, fmt.Errorf("create log resource: %w", err)
	}

	provider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(exporter)),
		log.WithResource(res),
	)

	handler := otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(provider))
	logger := slog.New(handler)

	slog.Info("otel log bridge enabled", "endpoint", endpoint)

	return logger, provider.Shutdown, nil
}

// TeeHandler fans out log records to multiple slog handlers.
type TeeHandler struct {
	handlers []slog.Handler
}

// NewTeeHandler creates a handler that writes to all provided handlers.
func NewTeeHandler(handlers ...slog.Handler) *TeeHandler {
	return &TeeHandler{handlers: handlers}
}

func (t *TeeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range t.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (t *TeeHandler) Handle(ctx context.Context, record slog.Record) error {
	var firstErr error
	for _, h := range t.handlers {
		if h.Enabled(ctx, record.Level) {
			if err := h.Handle(ctx, record.Clone()); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (t *TeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &TeeHandler{handlers: handlers}
}

func (t *TeeHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &TeeHandler{handlers: handlers}
}
