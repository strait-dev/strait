package billing

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"
)

// makeReaderSwitchEnforcer builds an Enforcer wired to a fresh mockBillingStore.
// The store is returned so tests can inspect post-hoc state (e.g. opportunistic
// snapshot writes). A nil rdb is passed because none of the reader-switch tests
// exercise the Redis paths.
func makeReaderSwitchEnforcer(t *testing.T, authoritative bool) (*Enforcer, *mockBillingStore) {
	t.Helper()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{},
	}
	e := NewEnforcer(store, nil, nil, WithEntitlementsAuthoritative(authoritative))
	return e, store
}

func TestReaderSwitch_SnapshotPresent_ReadsDirectly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, store := makeReaderSwitchEnforcer(t, true)

	// Hand-craft a snapshot whose values would never come out of the
	// catalog pipeline so we can prove the reader returned the snapshot
	// verbatim, not a recomputation.
	snap := GetPlanLimits(domain.PlanPro)
	snap.MaxConcurrentRuns = 99999
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	store.subscriptions["org-1"] = &OrgSubscription{
		ID: "sub", OrgID: "org-1", PlanTier: string(domain.PlanFree),
		Status: "active", EnforcementMode: "enforce",
		Entitlements: raw,
	}

	got, err := e.GetOrgPlanLimits(ctx, "org-1")
	if err != nil {
		t.Fatalf("GetOrgPlanLimits: %v", err)
	}
	if got.MaxConcurrentRuns != 99999 {
		t.Errorf("MaxConcurrentRuns = %d, want 99999 (proves snapshot was read, not recomputed)",
			got.MaxConcurrentRuns)
	}
}

func TestReaderSwitch_EmptySnapshot_FallsBackAndOpportunisticallyWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, store := makeReaderSwitchEnforcer(t, true)

	store.subscriptions["org-2"] = &OrgSubscription{
		ID: "sub", OrgID: "org-2", PlanTier: string(domain.PlanPro),
		Status: "active", EnforcementMode: "enforce",
		Entitlements: []byte("{}"),
	}

	got, err := e.GetOrgPlanLimits(ctx, "org-2")
	if err != nil {
		t.Fatalf("GetOrgPlanLimits: %v", err)
	}
	want := GetPlanLimits(domain.PlanPro)
	if got.MaxConcurrentRuns != want.MaxConcurrentRuns {
		t.Errorf("recompute mismatch: got %d, want %d",
			got.MaxConcurrentRuns, want.MaxConcurrentRuns)
	}
	// Opportunistic write must have fired.
	if store.lastEntitlementsUpdates == nil || store.lastEntitlementsUpdates["org-2"].MaxConcurrentRuns != want.MaxConcurrentRuns {
		t.Errorf("opportunistic UpdateEntitlements not called for org-2: %+v",
			store.lastEntitlementsUpdates)
	}
}

func TestReaderSwitch_NilSnapshot_FallsBackAndOpportunisticallyWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, store := makeReaderSwitchEnforcer(t, true)

	// Entitlements explicitly nil — older rows that predate migration 259
	// or rows produced by a partial backfill resume.
	store.subscriptions["org-3"] = &OrgSubscription{
		ID: "sub", OrgID: "org-3", PlanTier: string(domain.PlanScale),
		Status: "active", EnforcementMode: "enforce",
	}

	if _, err := e.GetOrgPlanLimits(ctx, "org-3"); err != nil {
		t.Fatalf("GetOrgPlanLimits: %v", err)
	}
	if _, ok := store.lastEntitlementsUpdates["org-3"]; !ok {
		t.Errorf("opportunistic write missing for org-3: %+v", store.lastEntitlementsUpdates)
	}
}

func TestReaderSwitch_AuthoritativeFalse_AlwaysRecomputes_NeverWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, store := makeReaderSwitchEnforcer(t, false)

	// Even with a present and divergent snapshot, with the flag off the
	// reader must ignore it and recompute. And it must NOT opportunistically
	// write — the operator escape hatch is supposed to be silent.
	snap := GetPlanLimits(domain.PlanPro)
	snap.MaxConcurrentRuns = 99999
	raw, _ := json.Marshal(snap)
	store.subscriptions["org-4"] = &OrgSubscription{
		ID: "sub", OrgID: "org-4", PlanTier: string(domain.PlanPro),
		Status: "active", EnforcementMode: "enforce",
		Entitlements: raw,
	}

	got, err := e.GetOrgPlanLimits(ctx, "org-4")
	if err != nil {
		t.Fatalf("GetOrgPlanLimits: %v", err)
	}
	want := GetPlanLimits(domain.PlanPro)
	if got.MaxConcurrentRuns != want.MaxConcurrentRuns {
		t.Errorf("expected recompute (catalog Pro = %d), got %d",
			want.MaxConcurrentRuns, got.MaxConcurrentRuns)
	}
	if len(store.lastEntitlementsUpdates) != 0 {
		t.Errorf("authoritative=false must not opportunistically write, got: %+v",
			store.lastEntitlementsUpdates)
	}
}

func TestReaderSwitch_ConcurrentOverrideAppliesOnTopOfSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, store := makeReaderSwitchEnforcer(t, true)

	// Snapshot says Pro-tier concurrency; the per-org override must win.
	snap := GetPlanLimits(domain.PlanPro)
	raw, _ := json.Marshal(snap)
	override := 7
	store.subscriptions["org-5"] = &OrgSubscription{
		ID: "sub", OrgID: "org-5", PlanTier: string(domain.PlanPro),
		Status:                     "active",
		EnforcementMode:            "enforce",
		Entitlements:               raw,
		OverrideConcurrentRunLimit: &override,
	}

	got, err := e.GetOrgPlanLimits(ctx, "org-5")
	if err != nil {
		t.Fatalf("GetOrgPlanLimits: %v", err)
	}
	if got.MaxConcurrentRuns != override {
		t.Errorf("override not applied: got %d, want %d", got.MaxConcurrentRuns, override)
	}
}

func TestReaderSwitch_LegacyDailyOverrideIgnoredForLaunch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, store := makeReaderSwitchEnforcer(t, true)

	snap := GetPlanLimits(domain.PlanFree)
	raw, _ := json.Marshal(snap)
	override := 7
	store.subscriptions["org-legacy-daily"] = &OrgSubscription{
		ID: "sub", OrgID: "org-legacy-daily", PlanTier: string(domain.PlanFree),
		Status:                "active",
		EnforcementMode:       "enforce",
		Entitlements:          raw,
		OverrideDailyRunLimit: &override,
	}

	got, err := e.GetOrgPlanLimits(ctx, "org-legacy-daily")
	if err != nil {
		t.Fatalf("GetOrgPlanLimits: %v", err)
	}
	if got.MaxRunsPerDay != -1 {
		t.Errorf("legacy daily override changed launch limit: got %d, want -1", got.MaxRunsPerDay)
	}
}

func TestHasPersistedEntitlements_BoundaryCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  []byte
		want bool
	}{
		{"nil", nil, false},
		{"empty", []byte{}, false},
		{"one byte", []byte("x"), false},
		{"empty object", []byte("{}"), false},
		{"empty object with whitespace", []byte("  {}  "), false},
		{"populated", []byte(`{"PlanTier":"pro"}`), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := hasPersistedEntitlements(tc.raw); got != tc.want {
				t.Errorf("hasPersistedEntitlements(%q) = %v, want %v", string(tc.raw), got, tc.want)
			}
		})
	}
}
