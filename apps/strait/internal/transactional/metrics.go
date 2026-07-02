package transactional

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	transactionalEmailOutcomeSuccess = "success"
	transactionalEmailOutcomeError   = "error"
	transactionalEmailTransportError = "transport_error"
	transactionalEmailClientError    = "client_error"
	transactionalEmailUnknownStatus  = "unknown"
	transactionalEmailMeterName      = "strait/transactional"
	transactionalEmailRequestsMetric = "strait_transactional_email_requests_total"
	transactionalEmailDurationMetric = "strait_transactional_email_request_duration_seconds"
)

var (
	transactionalMetricsOnce sync.Once
	transactionalRequests    metric.Int64Counter
	transactionalDuration    metric.Float64Histogram
)

func initTransactionalMetrics() {
	transactionalMetricsOnce.Do(func() {
		meter := otel.Meter(transactionalEmailMeterName)
		transactionalRequests, _ = meter.Int64Counter(
			transactionalEmailRequestsMetric,
			metric.WithDescription("Total Go-triggered transactional email requests"),
			metric.WithUnit("1"),
		)
		transactionalDuration, _ = meter.Float64Histogram(
			transactionalEmailDurationMetric,
			metric.WithDescription("Go-triggered transactional email request duration"),
			metric.WithUnit("s"),
			metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
		)
	})
}

func recordTransactionalEmailRequest(ctx context.Context, template TemplateID, outcome, statusCode string, started time.Time) {
	initTransactionalMetrics()
	if ctx == nil {
		ctx = context.Background()
	}
	if statusCode == "" {
		statusCode = transactionalEmailUnknownStatus
	}
	attrs := metric.WithAttributes(
		attribute.String("template", string(template)),
		attribute.String("outcome", outcome),
		attribute.String("status_code", statusCode),
	)
	transactionalRequests.Add(ctx, 1, attrs)
	transactionalDuration.Record(ctx, time.Since(started).Seconds(), attrs)
}
