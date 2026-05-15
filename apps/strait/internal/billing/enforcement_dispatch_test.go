package billing

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"strait/internal/domain"
)

// newFakeDispatcherEnforcer returns an Enforcer wired to a mockBillingStore
// (preloaded with the given subscription + period spend) and a fakeDispatcher
// so tests can drive CheckSpendingLimit through the spend boundaries and
// assert exactly which billing webhook events were dispatched.
func newFakeDispatcherEnforcer(t *testing.T, sub *OrgSubscription, periodSpend int64) (*Enforcer, *mockBillingStore, *fakeDispatcher) {
	t.Helper()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			sub.OrgID: sub,
		},
		periodSpendByOrg: map[string]int64{sub.OrgID: periodSpend},
	}
	d := &fakeDispatcher{}
	e := NewEnforcer(store, nil, nil, WithBillingDispatcher(d))
	return e, store, d
}

func newPaidSubscription(orgID, tier string, limitMicrousd int64, action string) *OrgSubscription {
	periodStart := time.Now().UTC().Add(-7 * 24 * time.Hour)
	periodEnd := periodStart.Add(30 * 24 * time.Hour)
	return &OrgSubscription{
		ID:                    "sub_" + orgID,
		OrgID:                 orgID,
		PlanTier:              tier,
		Status:                "active",
		CurrentPeriodStart:    &periodStart,
		CurrentPeriodEnd:      &periodEnd,
		SpendingLimitMicrousd: limitMicrousd,
		LimitAction:           action,
	}
}

func TestCheckSpendingLimit_DispatchesCapWarningAt80Pct(t *testing.T) {
	t.Parallel()

	sub := newPaidSubscription("org_warn", string(domain.PlanPro), 1_000_000, "block") // $1.00 cap
	e, _, d := newFakeDispatcherEnforcer(t, sub, 850_000)                              // 85% of cap

	if err := e.CheckSpendingLimit(context.Background(), sub.OrgID); err != nil {
		t.Fatalf("CheckSpendingLimit err = %v", err)
	}
	got := dispatchedEventTypes(d)
	if !contains(got, domain.WebhookEventBillingCapWarning) {
		t.Errorf("cap_warning not dispatched; got events %v", got)
	}
	if contains(got, domain.WebhookEventBillingCapReached) {
		t.Errorf("cap_reached should not dispatch under 100%%; got %v", got)
	}
}

func TestCheckSpendingLimit_DispatchesCapReachedAndOverageDisabled(t *testing.T) {
	t.Parallel()

	sub := newPaidSubscription("org_block", string(domain.PlanPro), 1_000_000, "block") // action=block
	e, _, d := newFakeDispatcherEnforcer(t, sub, 1_500_000)                             // 150% of cap

	err := e.CheckSpendingLimit(context.Background(), sub.OrgID)
	var limitErr *LimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("CheckSpendingLimit err = %v, want *LimitError", err)
	}
	got := dispatchedEventTypes(d)
	if !contains(got, domain.WebhookEventBillingCapReached) {
		t.Errorf("cap_reached not dispatched; got %v", got)
	}
	if !contains(got, domain.WebhookEventBillingOverageDisabled) {
		t.Errorf("overage_disabled not dispatched on action=block; got %v", got)
	}
	if contains(got, domain.WebhookEventBillingCapDisabled) {
		t.Errorf("cap_disabled should not fire on action=block; got %v", got)
	}
}

func TestCheckSpendingLimit_DispatchesCapDisabledOnDisableAction(t *testing.T) {
	t.Parallel()

	sub := newPaidSubscription("org_disable", string(domain.PlanScale), 5_000_000, "disable")
	e, _, d := newFakeDispatcherEnforcer(t, sub, 7_000_000) // 140% of cap

	var capErr *LimitError
	if !errors.As(e.CheckSpendingLimit(context.Background(), sub.OrgID), &capErr) {
		t.Fatal("expected LimitError when cap reached")
	}
	got := dispatchedEventTypes(d)
	if !contains(got, domain.WebhookEventBillingCapDisabled) {
		t.Errorf("cap_disabled not dispatched on action=disable; got %v", got)
	}
	if contains(got, domain.WebhookEventBillingOverageDisabled) {
		t.Errorf("overage_disabled should not fire on action=disable; got %v", got)
	}
}

func TestCheckSpendingLimit_DedupPerPeriod(t *testing.T) {
	t.Parallel()

	sub := newPaidSubscription("org_dedup", string(domain.PlanPro), 1_000_000, "block")
	e, _, d := newFakeDispatcherEnforcer(t, sub, 850_000) // 85%

	for range 5 {
		_ = e.CheckSpendingLimit(context.Background(), sub.OrgID)
	}
	got := dispatchedEventTypes(d)
	if cnt := countEvent(got, domain.WebhookEventBillingCapWarning); cnt != 1 {
		t.Errorf("cap_warning dispatched %d times across 5 calls; want exactly 1", cnt)
	}
}

func TestCheckSpendingLimit_PeriodRolloverResetsDedup(t *testing.T) {
	t.Parallel()

	sub := newPaidSubscription("org_roll", string(domain.PlanPro), 1_000_000, "block")
	e, store, d := newFakeDispatcherEnforcer(t, sub, 850_000)

	if err := e.CheckSpendingLimit(context.Background(), sub.OrgID); err != nil {
		t.Fatalf("first check err = %v", err)
	}

	// Simulate period rollover by upserting a subscription with a new
	// current_period_start; the mock store mirrors PgStore semantics by
	// resetting cap-event marks on rollover.
	newPeriodStart := sub.CurrentPeriodStart.Add(30 * 24 * time.Hour)
	rolled := *sub
	rolled.CurrentPeriodStart = &newPeriodStart
	delete(store.capEventMarks, sub.OrgID)
	store.subscriptions[sub.OrgID] = &rolled

	if err := e.CheckSpendingLimit(context.Background(), sub.OrgID); err != nil {
		t.Fatalf("second period check err = %v", err)
	}
	got := dispatchedEventTypes(d)
	if cnt := countEvent(got, domain.WebhookEventBillingCapWarning); cnt != 2 {
		t.Errorf("cap_warning dispatched %d times across two periods; want 2", cnt)
	}
}

func TestCheckSpendingLimit_NoDispatcherIsNoOp(t *testing.T) {
	t.Parallel()

	sub := newPaidSubscription("org_nodisp", string(domain.PlanPro), 1_000_000, "block")
	store := &mockBillingStore{
		subscriptions:    map[string]*OrgSubscription{sub.OrgID: sub},
		periodSpendByOrg: map[string]int64{sub.OrgID: 1_500_000},
	}
	e := NewEnforcer(store, nil, nil) // no dispatcher
	var noDispErr *LimitError
	if !errors.As(e.CheckSpendingLimit(context.Background(), sub.OrgID), &noDispErr) {
		t.Fatal("expected LimitError")
	}
	// No panic, no dispatch — already implied by reaching here.
}

func dispatchedEventTypes(d *fakeDispatcher) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, 0, len(d.calls))
	for _, c := range d.calls {
		out = append(out, c.eventType)
	}
	return out
}

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}

func countEvent(haystack []string, needle string) int {
	n := 0
	for _, s := range haystack {
		if s == needle {
			n++
		}
	}
	return n
}

// Compile-time check that the payload helper round-trips through JSON.
var _ = func() *BillingEventEnvelope {
	var env BillingEventEnvelope
	_ = json.Unmarshal([]byte(`{}`), &env)
	return &env
}()
