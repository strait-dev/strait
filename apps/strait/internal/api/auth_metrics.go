package api

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var authMetrics = newAuthRuntimeMetrics(otel.Meter("strait/api_auth"))

type authRuntimeMetrics struct {
	decisions metric.Int64Counter
	tokenAge  metric.Float64Histogram
	throttled metric.Int64Counter
}

func newAuthRuntimeMetrics(meter metric.Meter) authRuntimeMetrics {
	decisions, _ := meter.Int64Counter(
		"strait_auth_decisions_total",
		metric.WithDescription("Total authentication decisions by credential kind and outcome"),
		metric.WithUnit("1"),
	)
	tokenAge, _ := meter.Float64Histogram(
		"strait_auth_token_age_seconds",
		metric.WithDescription("Age of accepted authentication credentials at verification time"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(60, 300, 900, 3600, 21600, 86400, 604800, 2592000, 7776000),
	)
	throttled, _ := meter.Int64Counter(
		"strait_auth_rate_limit_throttled_total",
		metric.WithDescription("Total authentication attempts rejected by brute-force throttling"),
		metric.WithUnit("1"),
	)
	return authRuntimeMetrics{
		decisions: decisions,
		tokenAge:  tokenAge,
		throttled: throttled,
	}
}

func recordAuthDecision(ctx context.Context, kind, outcome string) {
	authMetrics.recordDecision(ctx, kind, outcome)
}

func recordAuthTokenAge(ctx context.Context, kind string, issuedAt time.Time) {
	authMetrics.recordTokenAge(ctx, kind, issuedAt)
}

func recordAuthRateLimitThrottled(ctx context.Context, scope string) {
	authMetrics.recordRateLimitThrottled(ctx, scope)
}

func (m authRuntimeMetrics) recordDecision(ctx context.Context, kind, outcome string) {
	m.decisions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", normalizeAuthKind(kind)),
		attribute.String("outcome", normalizeAuthOutcome(outcome)),
	))
}

func (m authRuntimeMetrics) recordTokenAge(ctx context.Context, kind string, issuedAt time.Time) {
	if issuedAt.IsZero() {
		return
	}
	age := time.Since(issuedAt).Seconds()
	if age < 0 {
		age = 0
	}
	m.tokenAge.Record(ctx, age, metric.WithAttributes(
		attribute.String("kind", normalizeAuthKind(kind)),
	))
}

func (m authRuntimeMetrics) recordRateLimitThrottled(ctx context.Context, scope string) {
	m.throttled.Add(ctx, 1, metric.WithAttributes(
		attribute.String("scope", normalizeAuthThrottleScope(scope)),
	))
}

func normalizeAuthKind(kind string) string {
	switch kind {
	case "api_key", "jwt", "oidc", "internal_secret":
		return kind
	default:
		return "unknown"
	}
}

func normalizeAuthOutcome(outcome string) string {
	switch outcome {
	case "success", "failure", "throttled":
		return outcome
	default:
		return "failure"
	}
}

func normalizeAuthThrottleScope(scope string) string {
	switch scope {
	case "api_key", "jwt", "oidc", "internal_secret", "auth":
		return scope
	default:
		return "auth"
	}
}
