package billing

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"strait/internal/telemetry"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/billing/meterevent"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// stripeKeyOnce ensures the global stripe.Key is set exactly once,
// preventing data races when multiple StripeUsageReporters are created
// concurrently (e.g. in parallel tests).
var stripeKeyOnce sync.Once

// StripeUsageReporter sends usage events to the Stripe Billing Meter.
// Safe for concurrent use from multiple goroutines.
type StripeUsageReporter struct {
	secretKey      string
	meterEventName string
	logger         *slog.Logger
	metrics        *telemetry.Metrics
}

// StripeUsageReporterOption configures a StripeUsageReporter.
type StripeUsageReporterOption func(*StripeUsageReporter)

// WithUsageReporterMetrics attaches Prometheus metrics to the reporter.
func WithUsageReporterMetrics(m *telemetry.Metrics) StripeUsageReporterOption {
	return func(r *StripeUsageReporter) {
		r.metrics = m
	}
}

// NewStripeUsageReporter creates a new reporter. Pass the STRIPE_SECRET_KEY.
// The key is set globally on the stripe package at construction time.
func NewStripeUsageReporter(secretKey string, logger *slog.Logger, opts ...StripeUsageReporterOption) *StripeUsageReporter {
	if logger == nil {
		logger = slog.Default()
	}
	// Set the global Stripe API key exactly once. The stripe-go library uses
	// a package-level global, so concurrent writes race. In production this
	// is called once at startup; sync.Once guards against parallel test init.
	stripeKeyOnce.Do(func() {
		stripe.Key = secretKey //nolint:reassign // stripe-go uses a global key by design
	})
	r := &StripeUsageReporter{
		secretKey:      secretKey,
		meterEventName: "compute_overage",
		logger:         logger,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// IngestComputeUsage sends a single usage event to the Stripe compute_overage meter.
// The costMicroUSD is the cost in micro-USD (1 unit = $0.000001).
// The runID is used as the identifier for deduplication.
func (r *StripeUsageReporter) IngestComputeUsage(ctx context.Context, stripeCustomerID, runID string, costMicroUSD int64) error {
	if r.secretKey == "" || stripeCustomerID == "" {
		return nil // not configured or no customer, skip silently
	}

	return r.ingest(ctx, stripeCustomerID, runID, costMicroUSD)
}

func (r *StripeUsageReporter) ingest(ctx context.Context, customerID, runID string, costMicroUSD int64) error {
	ts := time.Now().Unix()
	params := &stripe.BillingMeterEventParams{
		EventName: stripe.String(r.meterEventName),
		Payload: map[string]string{
			"stripe_customer_id": customerID,
			"value":              strconv.FormatInt(costMicroUSD, 10),
		},
		Timestamp:  &ts,
		Identifier: stripe.String(runID),
	}
	params.Context = ctx

	_, err := meterevent.New(params)
	if err != nil {
		r.logger.Warn("stripe meter event ingestion failed",
			"error", err,
			"customer_id", customerID,
			"run_id", runID,
		)
		if r.metrics != nil && r.metrics.StripeUsageEventsDropped != nil {
			r.metrics.StripeUsageEventsDropped.Add(ctx, 1,
				metric.WithAttributes(attribute.String("status", "error")),
			)
		}
		return fmt.Errorf("stripe meter event ingestion failed: %w", err)
	}

	if r.metrics != nil && r.metrics.StripeUsageEventsIngested != nil {
		r.metrics.StripeUsageEventsIngested.Add(ctx, 1,
			metric.WithAttributes(attribute.String("status", "ok")),
		)
	}

	r.logger.Debug("stripe meter event ingested",
		"customer_id", customerID,
		"run_id", runID,
	)
	return nil
}
