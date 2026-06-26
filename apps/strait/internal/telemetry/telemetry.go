package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// Init sets up OpenTelemetry tracing with an OTLP HTTP exporter.
// Returns a shutdown function that should be deferred.
// If endpoint is empty, tracing is disabled (noop).
func Init(ctx context.Context, serviceName, endpoint, environment string) (func(context.Context) error, error) {
	if endpoint == "" {
		slog.Info("otel tracing disabled (no endpoint configured)")
		return func(context.Context) error { return nil }, nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse otel trace endpoint: %w", err)
	}
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(u.Host),
	}
	if u.Scheme == "http" {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
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

	slog.Info("otel tracing enabled", "endpoint", redactOTLPEndpoint(u))

	return tp.Shutdown, nil
}

func redactOTLPEndpoint(u *url.URL) string {
	if u == nil {
		return ""
	}
	redacted := *u
	if redacted.User != nil {
		redacted.User = nil
	}

	query := redacted.Query()
	for key := range query {
		if isCredentialQueryKey(key) {
			query.Set(key, "[redacted]")
		}
	}
	redacted.RawQuery = query.Encode()
	return redacted.String()
}

func isCredentialQueryKey(key string) bool {
	key = strings.ToLower(key)
	switch key {
	case "code",
		"jwt",
		"oauth_verifier",
		"passcode",
		"samlart",
		"samlresponse",
		"session",
		"sessionid",
		"sid",
		"sig",
		"ticket":
		return true
	}
	return strings.Contains(key, "token") ||
		strings.Contains(key, "key") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "password") ||
		strings.Contains(key, "auth") ||
		strings.Contains(key, "credential") ||
		strings.Contains(key, "signature") ||
		strings.Contains(key, "session") ||
		strings.Contains(key, "jwt") ||
		strings.Contains(key, "saml") ||
		strings.Contains(key, "assertion") ||
		strings.Contains(key, "ticket")
}
