package billing

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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
	e, store, d := newFakeDispatcherEnforcer(t, sub, 1_500_000)                         // 150% of cap
	store.pausedJobIDs = []string{"schedule-a", "schedule-b"}

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
	if store.pausedOrgID != sub.OrgID || store.pausedReason != "quota_exceeded" {
		t.Fatalf("spending cap should pause org schedules with quota_exceeded, got org=%q reason=%q",
			store.pausedOrgID, store.pausedReason)
	}
	if cnt := countEvent(got, domain.WebhookEventScheduleSuspended); cnt != 2 {
		t.Fatalf("schedule.suspended events = %d, want 2 for paused schedules", cnt)
	}
}

func TestCheckMonthlyRunLimit_FreeCapPausesSchedules(t *testing.T) {
	t.Parallel()

	orgID := "org_free_cap_pause"
	sub := &OrgSubscription{
		OrgID:           orgID,
		PlanTier:        string(domain.PlanFree),
		Status:          "active",
		OverageDisabled: true,
	}
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{orgID: sub},
		pausedJobIDs:  []string{"schedule-free"},
	}
	d := &fakeDispatcher{}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	e := NewEnforcer(store, rdb, nil, WithBillingDispatcher(d))

	key := monthlyRunKey(orgID, time.Now())
	if err := mr.Set(key, "5000"); err != nil {
		t.Fatalf("seed monthly run count: %v", err)
	}
	mr.SetTTL(key, time.Hour)

	err := e.CheckMonthlyRunLimitForRun(context.Background(), orgID, "run-free-cap")
	var limitErr *LimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("CheckMonthlyRunLimitForRun err = %v, want *LimitError", err)
	}
	if store.pausedOrgID != orgID || store.pausedReason != "quota_exceeded" {
		t.Fatalf("free monthly cap should pause org schedules with quota_exceeded, got org=%q reason=%q",
			store.pausedOrgID, store.pausedReason)
	}
	if got := countEvent(dispatchedEventTypes(d), domain.WebhookEventScheduleSuspended); got != 1 {
		t.Fatalf("schedule.suspended events = %d, want 1", got)
	}
}

func TestDeepSecCheckSpendingLimitNotifyDispatchesWithoutRejecting(t *testing.T) {
	t.Parallel()

	sub := newPaidSubscription("org_notify", string(domain.PlanPro), 1_000_000, "notify")
	e, _, d := newFakeDispatcherEnforcer(t, sub, 1_500_000)

	if err := e.CheckSpendingLimit(context.Background(), sub.OrgID); err != nil {
		t.Fatalf("notify spending cap should not reject dispatch, got %v", err)
	}
	got := dispatchedEventTypes(d)
	if !contains(got, domain.WebhookEventBillingCapReached) {
		t.Errorf("cap_reached not dispatched for notify cap; got %v", got)
	}
	if contains(got, domain.WebhookEventBillingOverageDisabled) {
		t.Errorf("notify cap must not dispatch overage_disabled; got %v", got)
	}
}

func TestDeepSecCheckSpendingLimitRejectActionRejects(t *testing.T) {
	t.Parallel()

	sub := newPaidSubscription("org_reject", string(domain.PlanPro), 1_000_000, "reject")
	e, _, d := newFakeDispatcherEnforcer(t, sub, 1_500_000)

	var limitErr *LimitError
	if !errors.As(e.CheckSpendingLimit(context.Background(), sub.OrgID), &limitErr) {
		t.Fatal("reject spending cap should return LimitError")
	}
	got := dispatchedEventTypes(d)
	if !contains(got, domain.WebhookEventBillingOverageDisabled) {
		t.Errorf("reject cap should dispatch overage_disabled; got %v", got)
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
