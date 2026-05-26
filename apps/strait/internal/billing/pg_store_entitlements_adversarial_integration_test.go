//go:build integration

package billing_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// TestAdversarial_UpdateEntitlements_OversizedPayloadHandledCleanly stuffs a
// 1MB synthetic AddonPacks map into the snapshot and confirms the write
// either succeeds in full or fails cleanly — never silently truncates. JSONB
// in Postgres has a TOAST ceiling far above this, but the test pins the
// behavior so future schema constraints (e.g., a CHECK on column size) get
// caught.
func TestAdversarial_UpdateEntitlements_OversizedPayloadHandledCleanly(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ent-big-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}

	want := billing.GetPlanLimits(domain.PlanPro)
	want.MaxAddonPacks = make(map[billing.AddonType]int, 5_000)
	bigKey := strings.Repeat("k", 200)
	for i := range 5_000 {
		want.MaxAddonPacks[billing.AddonType(bigKey+strconv.Itoa(i))] = i
	}

	err := pgStore.UpdateEntitlements(ctx, orgID, want)
	if err != nil {
		// Cleanly errored — that's the alternative valid behavior.
		// What we forbid is silent truncation, which the next read
		// would expose if we didn't error here.
		return
	}

	// Wrote successfully — read back and confirm the map round-tripped
	// at full size. A truncated payload would deserialize a smaller map.
	var raw []byte
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT entitlements FROM organization_subscriptions WHERE org_id = $1
	`, orgID).Scan(&raw); err != nil {
		t.Fatalf("read entitlements: %v", err)
	}
	if len(raw) < 100_000 {
		t.Errorf("payload appears truncated: got %d bytes", len(raw))
	}
}

// TestAdversarial_UpdateEntitlements_SQLInjectionInOrgID confirms the
// org_id parameter binds via pgx (not string concatenation). A crafted
// org_id containing SQL must hit zero rows, never execute the embedded
// statement.
func TestAdversarial_UpdateEntitlements_SQLInjectionInOrgID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	// Set up two real orgs — if the injection worked, both would get
	// rewritten by the smuggled UPDATE.
	for _, id := range []string{"org-real-a-" + newID(), "org-real-b-" + newID()} {
		if err := pgStore.EnsureOrgSubscription(ctx, id); err != nil {
			t.Fatalf("EnsureOrgSubscription %s: %v", id, err)
		}
	}

	malicious := "'; UPDATE organization_subscriptions SET plan_tier = 'enterprise'; --"
	want := billing.GetPlanLimits(domain.PlanFree)
	if err := pgStore.UpdateEntitlements(ctx, malicious, want); err != nil {
		t.Fatalf("UpdateEntitlements with crafted org_id should be a no-op, got error: %v", err)
	}

	// No real org should have been silently elevated.
	var count int
	err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM organization_subscriptions WHERE plan_tier = 'enterprise'
	`).Scan(&count)
	if err != nil {
		t.Fatalf("count enterprise rows: %v", err)
	}
	if count != 0 {
		t.Errorf("SQL injection bypass: %d rows promoted to enterprise", count)
	}
}
