package billing

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripeUsageReporter_IngestRunOverage_EmptySecretNoop(t *testing.T) {
	t.Parallel()
	// Empty secret key causes IngestRunOverage to return nil without calling Stripe.
	reporter := NewStripeUsageReporter("", slog.Default())
	err := reporter.IngestRunOverage(context.Background(), "cust-123", "run-456")
	require.NoError(t,
		err)
}

func TestStripeUsageReporter_SkipsWhenNotConfigured(t *testing.T) {
	t.Parallel()

	// Empty secret key should not make any requests.
	reporter := NewStripeUsageReporter("", slog.Default())
	err := reporter.IngestRunOverage(context.Background(), "cust-123", "run-456")
	require.NoError(t,
		err)

	// Empty customer ID should not make any requests.
	reporter2 := NewStripeUsageReporter("sk_test_key", slog.Default())
	err = reporter2.IngestRunOverage(context.Background(), "", "run-456")
	require.NoError(t,
		err)
}

func TestStripeUsageReporter_ConstructorNilLogger(t *testing.T) {
	t.Parallel()
	reporter := NewStripeUsageReporter("sk_test_key", nil)
	require.NotNil(t,
		reporter)

	// Empty customer, so no real API call.
	err := reporter.IngestRunOverage(context.Background(), "", "run-1")
	require.NoError(t,
		err)
}

func TestStripeUsageReporter_ConstructorWithMetrics(t *testing.T) {
	t.Parallel()
	reporter := NewStripeUsageReporter("sk_test_key", slog.Default(), WithUsageReporterMetrics(nil))
	require.NotNil(t,
		reporter)
}
