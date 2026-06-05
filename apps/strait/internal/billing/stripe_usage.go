//go:build cloud

package billing

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"strait/internal/telemetry"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/billing/meterevent"
)

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
	ensureStripeKey(secretKey)
	r := &StripeUsageReporter{
		secretKey:      secretKey,
		meterEventName: "run_overage",
		logger:         logger,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// IngestRunOverage sends one orchestration-run overage unit to the Stripe
// run_overage meter. The runID is used as the identifier for deduplication.
func (r *StripeUsageReporter) IngestRunOverage(ctx context.Context, stripeCustomerID, runID string) error {
	if r.secretKey == "" || stripeCustomerID == "" {
		return nil
	}
	return r.ingest(ctx, stripeCustomerID, runID)
}

func (r *StripeUsageReporter) ingest(ctx context.Context, customerID, runID string) error {
	ts := time.Now().Unix()
	params := &stripe.BillingMeterEventParams{
		EventName: stripe.String(r.meterEventName),
		Payload: map[string]string{
			"stripe_customer_id": customerID,
			"value":              strconv.FormatInt(1, 10),
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
		recordBillingStripeUsageDropped(ctx, "error")
		return fmt.Errorf("stripe meter event ingestion failed: %w", err)
	}

	recordBillingStripeUsageIngested(ctx, "ok")

	r.logger.Debug("stripe meter event ingested",
		"customer_id", customerID,
		"run_id", runID,
	)
	return nil
}
