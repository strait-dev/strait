package billing

import (
	"testing"

	"strait/internal/domain"
)

// Simulates the Stripe webhook path: when a subscription.created event
// arrives with a Business price ID, the mapping must resolve to
// PlanBusiness. This guards against silently falling through to PlanFree
// when Business prices are configured through WithBusinessPrices.
func TestWithBusinessPrices_NoSilentFallthroughToFree(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithStarterPrices("s-m", "s-y"),
		WithProPrices("p-m", "p-y"),
		WithScalePrices("sc-m", "sc-y"),
		WithBusinessPrices("biz-m", "biz-y"),
	)
	tier, ok := m.TierForPrice("biz-m")
	if !ok || tier != domain.PlanBusiness {
		t.Fatalf("Business monthly resolved to (%q, %v), want Business", tier, ok)
	}
}

// When the Business env vars are empty but a Business price ID arrives
// (e.g. someone configures the Stripe product but forgets to set the
// env), the lookup must return ok=false, tier=Free, so the caller can
// log + escalate rather than silently classify the org as Free.
func TestWithBusinessPrices_EmptyEnvUnresolved(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithBusinessPrices("", ""),
	)
	tier, ok := m.TierForPrice("price_live_business_id")
	if ok {
		t.Errorf("ok = true for unresolved Business price; mapping must signal unknown")
	}
	if tier != domain.PlanFree {
		t.Errorf("tier = %q, want %q (fail-closed default)", tier, domain.PlanFree)
	}
}

// Sandbox price IDs in a live-configured mapping (or vice versa) must
// not resolve. Stripe environment isolation is the security boundary;
// the mapping must respect it.
func TestWithBusinessPrices_EnvironmentMismatchDoesNotResolve(t *testing.T) {
	t.Parallel()

	live := NewStripeMappingFromOptions(
		WithBusinessPrices("price_live_biz_m", "price_live_biz_y"),
	)
	for _, sandboxID := range []string{"price_test_biz_m", "price_test_biz_y", "price_1TUlbKCY4bMQR1xeozU9kimD"} {
		tier, ok := live.TierForPrice(sandboxID)
		if ok || tier != domain.PlanFree {
			t.Errorf("sandbox ID %q resolved against live mapping: (%q, %v)", sandboxID, tier, ok)
		}
	}
}
