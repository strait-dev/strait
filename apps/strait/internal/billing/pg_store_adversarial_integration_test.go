//go:build integration

package billing_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

// --------------------------------------------------------------------------.
// A1: Cross-org data isolation -- usage from org B must not appear in org A
// --------------------------------------------------------------------------.

func TestAdversarial_CrossOrgIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgA := "org-iso-a-" + newID()
	orgB := "org-iso-b-" + newID()

	pA := createProject(t, ctx, q, orgA, "PA")
	pB := createProject(t, ctx, q, orgB, "PB")

	jobA := createJob(t, ctx, q, pA.ID)
	jobB := createJob(t, ctx, q, pB.ID)

	runA := createRun(t, ctx, q, jobA, domain.StatusCompleted)
	runB := createRun(t, ctx, q, jobB, domain.StatusCompleted)

	aiA := &domain.RunUsage{
		ID: newID(), RunID: runA.ID,
		Provider: "openai", Model: "gpt-4",
		PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
		CostMicrousd: 500_000,
	}
	aiB := &domain.RunUsage{
		ID: newID(), RunID: runB.ID,
		Provider: "openai", Model: "gpt-4",
		PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300,
		CostMicrousd: 1_000_000,
	}
	if err := q.CreateRunUsage(ctx, aiA); err != nil {
		t.Fatalf("create ai A: %v", err)
	}
	if err := q.CreateRunUsage(ctx, aiB); err != nil {
		t.Fatalf("create ai B: %v", err)
	}

	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC().Add(1 * time.Hour)

	recsA, err := pgStore.GetOrgUsageForPeriod(ctx, orgA, from, to)
	if err != nil {
		t.Fatalf("GetOrgUsageForPeriod A: %v", err)
	}
	for _, r := range recsA {
		if r.OrgID != orgA {
			t.Errorf("org A usage contains record with org_id = %q", r.OrgID)
		}
		if r.ProjectID == pB.ID {
			t.Errorf("org A usage contains project from org B")
		}
	}

	countA, err := pgStore.CountAIModelCallsByOrg(ctx, orgA, from, to)
	if err != nil {
		t.Fatalf("CountAIModelCallsByOrg A: %v", err)
	}
	if countA != 1 {
		t.Errorf("CountAIModelCallsByOrg A = %d, want 1", countA)
	}
}

// --------------------------------------------------------------------------.
// A2: Deleted projects ignored in suspension
// --------------------------------------------------------------------------.

func TestAdversarial_DeletedProjectsIgnoredInSuspension(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-delsus-" + newID()
	p1 := createProject(t, ctx, q, orgID, "P1")
	p2 := createProject(t, ctx, q, orgID, "P2")
	p3 := createProject(t, ctx, q, orgID, "P3")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, _ = testDB.Pool.Exec(ctx, "UPDATE projects SET created_at = $2 WHERE id = $1", p1.ID, base)
	_, _ = testDB.Pool.Exec(ctx, "UPDATE projects SET created_at = $2 WHERE id = $1", p2.ID, base.Add(time.Hour))
	_, _ = testDB.Pool.Exec(ctx, "UPDATE projects SET created_at = $2 WHERE id = $1", p3.ID, base.Add(2*time.Hour))

	// Soft-delete p3.
	_, _ = testDB.Pool.Exec(ctx, "UPDATE projects SET deleted_at = NOW() WHERE id = $1", p3.ID)

	// Allow 1 project: only p2 should be suspended (p3 is deleted, so not counted).
	count, err := pgStore.SuspendExcessProjects(ctx, orgID, 1)
	if err != nil {
		t.Fatalf("SuspendExcessProjects: %v", err)
	}
	if count != 1 {
		t.Errorf("suspended = %d, want 1", count)
	}

	s1, _ := pgStore.IsProjectSuspended(ctx, p1.ID)
	s2, _ := pgStore.IsProjectSuspended(ctx, p2.ID)
	if s1 {
		t.Errorf("p1 (oldest) should not be suspended")
	}
	if !s2 {
		t.Errorf("p2 should be suspended")
	}
}

// --------------------------------------------------------------------------.
// A3: Integer overflow in spending limit
// --------------------------------------------------------------------------.

func TestAdversarial_SpendingLimitMaxInt64(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-maxint-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	maxInt64 := int64(9_223_372_036_854_775_807)
	if err := pgStore.UpdateSpendingLimit(ctx, orgID, maxInt64, "notify"); err != nil {
		t.Fatalf("UpdateSpendingLimit max int64: %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription: %v", err)
	}
	if sub.SpendingLimitMicrousd != maxInt64 {
		t.Errorf("SpendingLimitMicrousd = %d, want %d", sub.SpendingLimitMicrousd, maxInt64)
	}
}

// --------------------------------------------------------------------------.
// A4: Concurrent upsert race -- 10 goroutines upserting the same org
// --------------------------------------------------------------------------.

func TestAdversarial_ConcurrentUpsert(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-race-" + newID()

	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sub := &billing.OrgSubscription{
				ID:          newID(),
				OrgID:       orgID,
				PlanTier:    "pro",
				Status:      "active",
				LimitAction: "notify",
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}
			errs[idx] = pgStore.UpsertOrgSubscription(ctx, sub)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	// Should have exactly one row.
	var count int
	if err := testDB.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM organization_subscriptions WHERE org_id = $1", orgID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("rows = %d, want 1", count)
	}
}

// --------------------------------------------------------------------------.
// A5: Double deactivation idempotency
// --------------------------------------------------------------------------.

func TestAdversarial_DoubleDeactivateAddon(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-deact2-" + newID()
	a := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonConcurrentRuns,
		Quantity:  1,
		Active:    true,
	}
	if err := pgStore.CreateAddon(ctx, a); err != nil {
		t.Fatalf("CreateAddon: %v", err)
	}

	if err := pgStore.DeactivateAddon(ctx, a.ID); err != nil {
		t.Fatalf("first DeactivateAddon: %v", err)
	}
	if err := pgStore.DeactivateAddon(ctx, a.ID); err != nil {
		t.Fatalf("second DeactivateAddon: %v", err)
	}

	addons, err := pgStore.ListActiveAddons(ctx, orgID)
	if err != nil {
		t.Fatalf("ListActiveAddons: %v", err)
	}
	if len(addons) != 0 {
		t.Errorf("active addons = %d, want 0", len(addons))
	}
}

// --------------------------------------------------------------------------.
// A6: Duplicate addon ID handling
// --------------------------------------------------------------------------.

func TestAdversarial_DuplicateAddonID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-dupaddon-" + newID()
	id := newID()
	a := &billing.Addon{
		ID:        id,
		OrgID:     orgID,
		AddonType: billing.AddonConcurrentRuns,
		Quantity:  1,
		Active:    true,
	}
	if err := pgStore.CreateAddon(ctx, a); err != nil {
		t.Fatalf("CreateAddon: %v", err)
	}

	// Second insert with same ID should fail (PK violation).
	err := pgStore.CreateAddon(ctx, a)
	if err == nil {
		t.Error("expected error for duplicate addon ID, got nil")
	}
}

// --------------------------------------------------------------------------.
// A7: Concurrent webhook record -- 50 goroutines
// --------------------------------------------------------------------------.

func TestAdversarial_ConcurrentWebhookRecord(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	msgID := "msg-concurrent-" + newID()

	var wg sync.WaitGroup
	errs := make([]error, 50)
	for i := range 50 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = pgStore.RecordProcessedWebhook(ctx, msgID)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	processed, err := pgStore.IsWebhookProcessed(ctx, msgID)
	if err != nil {
		t.Fatalf("IsWebhookProcessed: %v", err)
	}
	if !processed {
		t.Error("message should be processed after concurrent writes")
	}
}

// --------------------------------------------------------------------------.
// A8: Empty org ID returns nothing (no cross-tenant leak)
// --------------------------------------------------------------------------.

func TestAdversarial_EmptyOrgIDReturnsNothing(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	// Create some data under a real org.
	orgID := "org-real-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	job := createJob(t, ctx, q, p.ID)
	createRun(t, ctx, q, job, domain.StatusExecuting)

	// Querying with empty org should get nothing.
	ids, err := pgStore.ListProjectsByOrg(ctx, "")
	if err != nil {
		t.Fatalf("ListProjectsByOrg empty: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ListProjectsByOrg empty = %d items, want 0", len(ids))
	}

	count, err := pgStore.CountExecutingRunsByOrg(ctx, "")
	if err != nil {
		t.Fatalf("CountExecutingRunsByOrg empty: %v", err)
	}
	if count != 0 {
		t.Errorf("CountExecutingRunsByOrg empty = %d, want 0", count)
	}
}

// --------------------------------------------------------------------------.
// A9: Future period pending downgrade not listed
// --------------------------------------------------------------------------.

func TestAdversarial_FuturePendingDowngradeNotListed(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-futpend-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	future := time.Now().UTC().Add(30 * 24 * time.Hour)
	if err := pgStore.SetPendingDowngrade(ctx, orgID, "free", &future, &future); err != nil {
		t.Fatalf("SetPendingDowngrade: %v", err)
	}

	subs, err := pgStore.ListOrgsWithPendingDowngrade(ctx)
	if err != nil {
		t.Fatalf("ListOrgsWithPendingDowngrade: %v", err)
	}
	for _, s := range subs {
		if s.OrgID == orgID {
			t.Errorf("future pending downgrade should not be listed")
		}
	}
}

// --------------------------------------------------------------------------.
// A10: Member dedup across projects
// --------------------------------------------------------------------------.

func TestAdversarial_MemberDedupAcrossProjects(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-dedup-" + newID()
	userID := "user-shared-" + newID()

	// Same user is a member of 5 projects in the same org.
	for i := range 5 {
		p := createProject(t, ctx, q, orgID, fmt.Sprintf("P%d", i))
		createMember(t, ctx, q, p.ID, userID)
	}

	count, err := pgStore.CountMembersByOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("CountMembersByOrg: %v", err)
	}
	if count != 1 {
		t.Errorf("CountMembersByOrg = %d, want 1 (deduped)", count)
	}
}
