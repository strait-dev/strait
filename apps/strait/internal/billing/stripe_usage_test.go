package billing

import (
	"context"
	"log/slog"
	"testing"
)

func TestStripeUsageReporter_IngestComputeUsage_EmptySecretNoop(t *testing.T) {
	t.Parallel()
	// Empty secret key causes IngestComputeUsage to return nil without calling Stripe.
	reporter := NewStripeUsageReporter("", slog.Default())
	err := reporter.IngestComputeUsage(context.Background(), "cust-123", "run-456", 1700)
	if err != nil {
		t.Fatalf("expected nil error for empty secret key, got: %v", err)
	}
}

func TestStripeUsageReporter_SkipsWhenNotConfigured(t *testing.T) {
	t.Parallel()

	// Empty secret key should not make any requests.
	reporter := NewStripeUsageReporter("", slog.Default())
	err := reporter.IngestComputeUsage(context.Background(), "cust-123", "run-456", 1700)
	if err != nil {
		t.Fatalf("expected nil error for empty secret, got: %v", err)
	}

	// Empty customer ID should not make any requests.
	reporter2 := NewStripeUsageReporter("sk_test_key", slog.Default())
	err = reporter2.IngestComputeUsage(context.Background(), "", "run-456", 1700)
	if err != nil {
		t.Fatalf("expected nil error for empty customer, got: %v", err)
	}
}

func TestStripeUsageReporter_ConstructorNilLogger(t *testing.T) {
	t.Parallel()
	reporter := NewStripeUsageReporter("sk_test_key", nil)
	if reporter == nil {
		t.Fatal("expected non-nil reporter")
	}
	// Empty customer, so no real API call.
	err := reporter.IngestComputeUsage(context.Background(), "", "run-1", 100)
	if err != nil {
		t.Fatalf("expected nil for empty customer: %v", err)
	}
}

func TestStripeUsageReporter_ConstructorWithMetrics(t *testing.T) {
	t.Parallel()
	reporter := NewStripeUsageReporter("sk_test_key", slog.Default(), WithUsageReporterMetrics(nil))
	if reporter == nil {
		t.Fatal("expected non-nil reporter")
	}
}
