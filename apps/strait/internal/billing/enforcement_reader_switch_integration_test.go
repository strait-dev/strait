//go:build integration

package billing_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// writeRawEntitlements sets the entitlements column directly, bypassing
// PgStore. Used to seed stale or tampered snapshots for the reader switch
// behavior under test.
func writeRawEntitlements(t *testing.T, ctx context.Context, orgID string, raw []byte) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE organization_subscriptions SET entitlements = $2::jsonb WHERE org_id = $1`,
		orgID, raw); err != nil {
		t.Fatalf("write raw entitlements for %s: %v", orgID, err)
	}
}

func TestReaderSwitch_IntegrationStaleSnapshotIsAuthoritative(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-rs-stale-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanFree), "active"); err != nil {
		t.Fatalf("plan free: %v", err)
	}

	// Hand-write Enterprise-tier limits to a Free org's row. This is the
	// documented trust boundary: the DB row IS authoritative once the
	// reader switch is on. Anyone with direct DB access can override
	// what the catalog says.
	stale := billing.GetPlanLimits(domain.PlanEnterprise)
	raw, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	writeRawEntitlements(t, ctx, orgID, raw)

	enforcer := billing.NewEnforcer(pgStore, nil, nil,
		billing.WithEntitlementsAuthoritative(true))

	got, err := enforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgPlanLimits: %v", err)
	}
	want := billing.GetPlanLimits(domain.PlanEnterprise)
	if got.MaxConcurrentRuns != want.MaxConcurrentRuns {
		t.Errorf("expected snapshot to be authoritative: got %d, want %d (Enterprise)",
			got.MaxConcurrentRuns, want.MaxConcurrentRuns)
	}
}

func TestReaderSwitch_IntegrationEmptySnapshotPopulatesOpportunistically(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-rs-empty-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanScale), "active"); err != nil {
		t.Fatalf("plan: %v", err)
	}
	// Nuke the snapshot so the reader has to recompute and write back.
	writeRawEntitlements(t, ctx, orgID, []byte("{}"))

	enforcer := billing.NewEnforcer(pgStore, nil, nil,
		billing.WithEntitlementsAuthoritative(true))

	if _, err := enforcer.GetOrgPlanLimits(ctx, orgID); err != nil {
		t.Fatalf("first GetOrgPlanLimits: %v", err)
	}

	// Direct read: the column is now populated.
	var raw []byte
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT entitlements FROM organization_subscriptions WHERE org_id = $1`,
		orgID).Scan(&raw); err != nil {
		t.Fatalf("read column: %v", err)
	}
	if string(raw) == "{}" || len(raw) <= 2 {
		t.Errorf("opportunistic write did not populate column: got %q", string(raw))
	}
	var snap billing.OrgPlanLimits
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("unmarshal column: %v", err)
	}
	want := billing.GetPlanLimits(domain.PlanScale)
	if snap.MaxConcurrentRuns != want.MaxConcurrentRuns {
		t.Errorf("populated snapshot does not match Scale: got %d, want %d",
			snap.MaxConcurrentRuns, want.MaxConcurrentRuns)
	}
}

// TestAdversarialReaderSwitch_MalformedJSONFallsBackAndOverwrites exercises
// the malformed-column path. The reader must NOT panic; it must log, fall
// back to recompute, and overwrite the column with a valid snapshot so
// subsequent reads hit the fast path.
func TestAdversarialReaderSwitch_MalformedJSONFallsBackAndOverwrites(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-rs-bad-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("plan: %v", err)
	}
	// Postgres jsonb won't store invalid JSON, so simulate a malformed
	// snapshot by writing a JSON object with values of the wrong type.
	// json.Unmarshal into OrgPlanLimits will fail because PlanTier is
	// expected to be a string but we send a number.
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE organization_subscriptions SET entitlements = '{"PlanTier": 12345}'::jsonb WHERE org_id = $1`,
		orgID); err != nil {
		t.Fatalf("write malformed: %v", err)
	}

	enforcer := billing.NewEnforcer(pgStore, nil, nil,
		billing.WithEntitlementsAuthoritative(true))

	got, err := enforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgPlanLimits panic-free fallback failed: %v", err)
	}
	want := billing.GetPlanLimits(domain.PlanPro)
	if got.MaxConcurrentRuns != want.MaxConcurrentRuns {
		t.Errorf("fallback returned %d, want Pro=%d", got.MaxConcurrentRuns, want.MaxConcurrentRuns)
	}

	// Column should now be a valid snapshot (overwrite happened).
	var raw []byte
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT entitlements FROM organization_subscriptions WHERE org_id = $1`,
		orgID).Scan(&raw); err != nil {
		t.Fatalf("re-read: %v", err)
	}
	var fixed billing.OrgPlanLimits
	if err := json.Unmarshal(raw, &fixed); err != nil {
		t.Fatalf("column not repaired (still malformed): %s: %v", string(raw), err)
	}
	if fixed.MaxConcurrentRuns != want.MaxConcurrentRuns {
		t.Errorf("repaired snapshot wrong: got %d, want %d", fixed.MaxConcurrentRuns, want.MaxConcurrentRuns)
	}
}

// TestAdversarialReaderSwitch_ConcurrentReadersAndWriters fires N goroutines
// reading limits while M goroutines flip plan tiers. With -race this
// catches any data corruption in the read path; the assertion is just
// that every read returns a coherent snapshot for some valid tier.
func TestAdversarialReaderSwitch_ConcurrentReadersAndWriters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-rs-race-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("plan: %v", err)
	}

	enforcer := billing.NewEnforcer(pgStore, nil, nil,
		billing.WithEntitlementsAuthoritative(true))

	tiers := []domain.PlanTier{
		domain.PlanStarter, domain.PlanPro, domain.PlanScale, domain.PlanBusiness,
	}
	validTier := func(p domain.PlanTier) bool {
		for _, t := range tiers {
			if t == p {
				return true
			}
		}
		return p == domain.PlanFree // Free is also valid as catalog default.
	}

	const writers = 4
	const readers = 16
	const reads = 25

	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tier := tiers[i%len(tiers)]
			_ = pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(tier), "active")
		}(i)
	}
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range reads {
				got, err := enforcer.GetOrgPlanLimits(ctx, orgID)
				if err != nil {
					t.Errorf("reader err: %v", err)
					return
				}
				if !validTier(got.PlanTier) {
					t.Errorf("reader saw invalid tier %q", got.PlanTier)
					return
				}
			}
		}()
	}
	wg.Wait()
}
