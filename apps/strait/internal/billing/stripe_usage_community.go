//go:build !cloud

package billing

import (
	"context"
	"log/slog"

	"strait/internal/telemetry"
)

// StripeUsageReporter is a no-op stub in community builds.
// The cloud build replaces this with a real Stripe meter-event sender.
type StripeUsageReporter struct{}

// StripeUsageReporterOption is the option type for the community stub.
// It is identical in shape to the cloud variant so call sites compile in
// either edition.
type StripeUsageReporterOption func(*StripeUsageReporter)

// WithUsageReporterMetrics is accepted for API parity with the cloud build
// but performs no work — the community reporter never emits metrics.
func WithUsageReporterMetrics(_ *telemetry.Metrics) StripeUsageReporterOption {
	return func(*StripeUsageReporter) {}
}

// NewStripeUsageReporter returns a no-op reporter. Community builds have no
// Stripe linkage, so any caller that constructs one gets a stub whose
// IngestRunOverage always returns nil.
func NewStripeUsageReporter(_ string, _ *slog.Logger, _ ...StripeUsageReporterOption) *StripeUsageReporter {
	return &StripeUsageReporter{}
}

// IngestRunOverage is a no-op in community builds.
func (r *StripeUsageReporter) IngestRunOverage(_ context.Context, _, _ string) error {
	return nil
}
