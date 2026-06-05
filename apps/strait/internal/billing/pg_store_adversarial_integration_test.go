//go:build integration

package billing_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sourcegraph/conc"

	"strait/internal/billing"
	"strait/internal/domain"
)

// A1: Cross-org data isolation -- usage from org B must not appear in org A

func TestAdversarial_CrossOrgIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgA := "org-iso-a-" + newID()
	orgB := "org-iso-b-" + newID()

	pA := createProject(t, ctx, q, orgA, "PA")
	pB := createProject(t, ctx, q, orgB, "PB")

	now := time.Now().UTC()
	for _, rec := range []*billing.UsageRecord{
		{
			ID:               newID(),
			OrgID:            orgA,
			ProjectID:        pA.ID,
			PeriodDate:       now,
			RunsCount:        1,
			ComputeCostMicro: 500_000,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		{
			ID:               newID(),
			OrgID:            orgB,
			ProjectID:        pB.ID,
			PeriodDate:       now,
			RunsCount:        1,
			ComputeCostMicro: 1_000_000,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	} {
		if err := pgStore.UpsertUsageRecord(ctx, rec); err != nil {
			t.Fatalf("seed usage record: %v", err)
		}
	}

	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)

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

}

// A2: Deleted projects ignored in suspension

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

// A3: Integer overflow in spending limit

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

// A4: Concurrent upsert race -- 10 goroutines upserting the same org

func TestAdversarial_ConcurrentUpsert(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-race-" + newID()

	var wg conc.WaitGroup
	errs := make([]error, 10)
	for i := range 10 {
		idx := i
		wg.Go(func() {
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
		})
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

// A5: Double deactivation idempotency

func TestAdversarial_DoubleDeactivateAddon(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-deact2-" + newID()
	a := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonConcurrency100,
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

// A6: Duplicate addon ID handling

func TestAdversarial_DuplicateAddonID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-dupaddon-" + newID()
	id := newID()
	a := &billing.Addon{
		ID:        id,
		OrgID:     orgID,
		AddonType: billing.AddonConcurrency100,
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

// A7: Concurrent webhook record -- 50 goroutines

func TestAdversarial_ConcurrentWebhookRecord(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	msgID := "msg-concurrent-" + newID()

	var wg conc.WaitGroup
	errs := make([]error, 50)
	for i := range 50 {
		idx := i
		wg.Go(func() {
			errs[idx] = pgStore.RecordProcessedWebhook(ctx, msgID)
		})
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

// A8: Empty org ID returns nothing (no cross-tenant leak)

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

// A9: Future period pending downgrade not listed

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

// A10: Member dedup across projects

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

// A11: Concurrent enterprise contract upserts must not lose data

func TestAdversarial_ConcurrentContractUpsert(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-conc-contract-" + newID()
	subID := "sub_conc"
	base := &billing.EnterpriseContract{
		ID:                    "contract_conc",
		OrgID:                 orgID,
		EnterpriseTier:        billing.EnterpriseTierStarter,
		AnnualCommitmentCents: 1800000,
		OverageDiscountPct:    10,
		ContractStartDate:     time.Now().Add(-30 * 24 * time.Hour),
		ContractEndDate:       time.Now().Add(335 * 24 * time.Hour),
		AutoRenew:             true,
		BillingCadence:        "annual",
		StripeSubscriptionID:  &subID,
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	// First insert so conflict path is exercised.
	if err := pgStore.UpsertEnterpriseContract(ctx, base); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}

	var wg conc.WaitGroup
	errs := make([]error, 10)
	for i := range 10 {
		idx := i
		wg.Go(func() {
			c := *base
			c.ID = fmt.Sprintf("contract_conc_%d", idx)
			c.OverageDiscountPct = idx
			c.Notes = fmt.Sprintf("writer_%d", idx)
			errs[idx] = pgStore.UpsertEnterpriseContract(ctx, &c)
		})
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("writer %d error: %v", i, err)
		}
	}

	got, err := pgStore.GetEnterpriseContract(ctx, orgID)
	if err != nil {
		t.Fatalf("get after concurrent upserts: %v", err)
	}
	// One writer must have won; the contract must be valid.
	if got.OrgID != orgID {
		t.Errorf("OrgID = %q, want %q", got.OrgID, orgID)
	}
	if got.EnterpriseTier != billing.EnterpriseTierStarter {
		t.Errorf("tier mutated to %q", got.EnterpriseTier)
	}
}

// A12: Enterprise contract UNIQUE(org_id) enforced -- only one per org

func TestAdversarial_OneContractPerOrg(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-unique-" + newID()
	subID := "sub_u1"
	c1 := &billing.EnterpriseContract{
		ID: "contract_u1", OrgID: orgID,
		EnterpriseTier:        billing.EnterpriseTierStarter,
		AnnualCommitmentCents: 1800000,
		OverageDiscountPct:    10,
		ContractStartDate:     time.Now(), ContractEndDate: time.Now().Add(365 * 24 * time.Hour),
		AutoRenew: true, BillingCadence: "annual",
		StripeSubscriptionID: &subID,
		CreatedAt:            time.Now(), UpdatedAt: time.Now(),
	}

	if err := pgStore.UpsertEnterpriseContract(ctx, c1); err != nil {
		t.Fatal(err)
	}

	// Second upsert with different ID for same org -- should update, not create second row.
	c2 := *c1
	c2.ID = "contract_u2"
	c2.EnterpriseTier = billing.EnterpriseTierGrowth
	c2.AnnualCommitmentCents = 4800000
	if err := pgStore.UpsertEnterpriseContract(ctx, &c2); err != nil {
		t.Fatal(err)
	}

	got, err := pgStore.GetEnterpriseContract(ctx, orgID)
	if err != nil {
		t.Fatal(err)
	}
	// Should reflect the second upsert's values.
	if got.EnterpriseTier != billing.EnterpriseTierGrowth {
		t.Errorf("tier = %q after second upsert, want growth", got.EnterpriseTier)
	}
	if got.AnnualCommitmentCents != 4800000 {
		t.Errorf("commitment = %d, want 4800000", got.AnnualCommitmentCents)
	}
}

// A13: ListExpiringContracts excludes already-expired and far-future

func TestAdversarial_ExpiringContractBoundaries(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	subID := "sub_boundary"

	mkContract := func(orgSuffix string, endOffset time.Duration) {
		orgID := "org-boundary-" + orgSuffix + "-" + newID()
		c := &billing.EnterpriseContract{
			ID: "contract_" + orgSuffix, OrgID: orgID,
			EnterpriseTier:        billing.EnterpriseTierStarter,
			AnnualCommitmentCents: 1800000,
			OverageDiscountPct:    10,
			ContractStartDate:     time.Now().Add(-365 * 24 * time.Hour),
			ContractEndDate:       time.Now().Add(endOffset),
			AutoRenew:             true, BillingCadence: "annual",
			StripeSubscriptionID: &subID,
			CreatedAt:            time.Now(), UpdatedAt: time.Now(),
		}
		if err := pgStore.UpsertEnterpriseContract(ctx, c); err != nil {
			t.Fatalf("create %s: %v", orgSuffix, err)
		}
	}

	mkContract("expired-1h", -1*time.Hour)      // already expired
	mkContract("expired-30d", -30*24*time.Hour) // long expired
	mkContract("expiring-1h", 1*time.Hour)      // about to expire
	mkContract("expiring-6d", 6*24*time.Hour)   // 6 days out
	mkContract("expiring-29d", 29*24*time.Hour) // 29 days out
	mkContract("expiring-31d", 31*24*time.Hour) // 31 days out
	mkContract("future-365d", 365*24*time.Hour) // way out

	within30, err := pgStore.ListExpiringContracts(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}

	// Should include: 1h, 6d, 29d (3 contracts). NOT expired or 31d+.
	if len(within30) != 3 {
		t.Errorf("expected 3 contracts within 30 days, got %d", len(within30))
		for _, c := range within30 {
			t.Logf("  org=%s end=%v", c.OrgID, c.ContractEndDate)
		}
	}

	// Within 1 day should only get the 1h one.
	within1, err := pgStore.ListExpiringContracts(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(within1) != 1 {
		t.Errorf("expected 1 contract within 1 day, got %d", len(within1))
	}

	// Negative days should return nothing.
	withinNeg, err := pgStore.ListExpiringContracts(ctx, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(withinNeg) != 0 {
		t.Errorf("expected 0 contracts for negative days, got %d", len(withinNeg))
	}
}

func TestPgStore_ListEnterpriseContractsOverlappingPeriod_IncludesMidPeriodLapse(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	periodStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	subID := "sub_sla_overlap"

	insertContract := func(suffix string, start, end time.Time) string {
		t.Helper()
		orgID := "org-sla-overlap-" + suffix + "-" + newID()
		c := &billing.EnterpriseContract{
			ID:                    "contract_sla_overlap_" + suffix,
			OrgID:                 orgID,
			EnterpriseTier:        billing.EnterpriseTierStarter,
			AnnualCommitmentCents: 1_800_000,
			OverageDiscountPct:    10,
			ContractStartDate:     start,
			ContractEndDate:       end,
			AutoRenew:             true,
			BillingCadence:        "annual",
			StripeSubscriptionID:  &subID,
			CreatedAt:             time.Now().UTC(),
			UpdatedAt:             time.Now().UTC(),
		}
		if err := pgStore.UpsertEnterpriseContract(ctx, c); err != nil {
			t.Fatalf("UpsertEnterpriseContract(%s): %v", suffix, err)
		}
		return orgID
	}

	lapsedMidPeriod := insertContract("lapsed", periodStart.AddDate(0, -1, 0), periodStart.Add(14*24*time.Hour))
	activeThroughPeriod := insertContract("active", periodStart.Add(7*24*time.Hour), periodEnd.Add(24*time.Hour))
	insertContract("ended-at-start", periodStart.AddDate(0, -1, 0), periodStart)
	insertContract("starts-at-end", periodEnd, periodEnd.AddDate(0, 1, 0))

	contracts, err := pgStore.ListEnterpriseContractsOverlappingPeriod(ctx, periodStart, periodEnd)
	if err != nil {
		t.Fatalf("ListEnterpriseContractsOverlappingPeriod: %v", err)
	}

	seen := make(map[string]bool, len(contracts))
	for _, contract := range contracts {
		seen[contract.OrgID] = true
	}
	if !seen[lapsedMidPeriod] {
		t.Fatal("expected contract that lapsed mid-period to be included")
	}
	if !seen[activeThroughPeriod] {
		t.Fatal("expected contract active through period to be included")
	}
	if len(contracts) != 2 {
		t.Fatalf("expected only the two overlapping contracts, got %d: %+v", len(contracts), contracts)
	}
}

// A14: Empty org ID returns not-found, not a cross-org leak

func TestAdversarial_EmptyOrgIDContract(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	_, err := pgStore.GetEnterpriseContract(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty org ID")
	}
}

// H5: DeactivateExcessCronJobs keeps the newest by updated_at

func TestAdversarial_DeactivateExcessCronJobs_KeepsNewest(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-cron-order-" + newID()
	p := createProject(t, ctx, q, orgID, "P")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var jobIDs []string
	for i := range 5 {
		j := createJob(t, ctx, q, p.ID)
		jobIDs = append(jobIDs, j.ID)
		_, _ = testDB.Pool.Exec(ctx,
			"UPDATE jobs SET updated_at = $2 WHERE id = $1",
			j.ID, base.Add(time.Duration(i)*time.Hour))
	}

	deactivated, err := pgStore.DeactivateExcessCronJobs(ctx, orgID, 2)
	if err != nil {
		t.Fatalf("DeactivateExcessCronJobs: %v", err)
	}
	if len(deactivated) != 3 {
		t.Fatalf("deactivated = %d, want 3", len(deactivated))
	}

	for i, jid := range jobIDs {
		var cron string
		if err := testDB.Pool.QueryRow(ctx,
			"SELECT COALESCE(cron, '') FROM jobs WHERE id = $1", jid).Scan(&cron); err != nil {
			t.Fatalf("query job %d: %v", i, err)
		}
		isNewest := i >= 3
		hasCron := cron != ""
		if isNewest && !hasCron {
			t.Errorf("job %d (newest) should still have cron, got empty", i)
		}
		if !isNewest && hasCron {
			t.Errorf("job %d (oldest) should have cron cleared, got %q", i, cron)
		}
	}
}

// H6: DeactivateExcessEnvironments keeps the newest by created_at

func TestAdversarial_DeactivateExcessEnvironments_KeepsNewest(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-env-order-" + newID()
	p := createProject(t, ctx, q, orgID, "P")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var envIDs []string
	for i := range 4 {
		eid := createEnvironment(t, ctx, p.ID, fmt.Sprintf("env-%d", i))
		envIDs = append(envIDs, eid)
		_, _ = testDB.Pool.Exec(ctx,
			"UPDATE environments SET created_at = $2 WHERE id = $1",
			eid, base.Add(time.Duration(i)*time.Hour))
	}

	deactivated, err := pgStore.DeactivateExcessEnvironments(ctx, orgID, 2)
	if err != nil {
		t.Fatalf("DeactivateExcessEnvironments: %v", err)
	}
	if deactivated != 2 {
		t.Fatalf("deactivated = %d, want 2", deactivated)
	}

	for i, eid := range envIDs {
		var exists bool
		err := testDB.Pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM environments WHERE id = $1)", eid).Scan(&exists)
		if err != nil {
			t.Fatalf("check env %d: %v", i, err)
		}
		isNewest := i >= 2
		if isNewest && !exists {
			t.Errorf("env %d (newest) should still exist", i)
		}
		if !isNewest && exists {
			t.Errorf("env %d (oldest) should be deleted", i)
		}
	}
}

func TestAdversarial_DeactivateExcessEnvironments_PreservesStandardEnvironments(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-env-standard-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	standardID := createEnvironment(t, ctx, p.ID, "standard")
	customOldID := createEnvironment(t, ctx, p.ID, "custom-old")
	customNewID := createEnvironment(t, ctx, p.ID, "custom-new")
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE environments
		SET is_standard = CASE WHEN id = $1 THEN true ELSE false END,
		    created_at = CASE
		      WHEN id = $1 THEN $4::timestamptz
		      WHEN id = $2 THEN $5::timestamptz
		      ELSE $6::timestamptz
		    END
		WHERE id IN ($1, $2, $3)
	`, standardID, customOldID, customNewID,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("seed environment timestamps: %v", err)
	}

	deactivated, err := pgStore.DeactivateExcessEnvironments(ctx, orgID, 1)
	if err != nil {
		t.Fatalf("DeactivateExcessEnvironments: %v", err)
	}
	if deactivated != 1 {
		t.Fatalf("deactivated = %d, want only oldest custom environment", deactivated)
	}
	for _, id := range []string{standardID, customNewID} {
		var exists bool
		if err := testDB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environments WHERE id = $1)`, id).Scan(&exists); err != nil {
			t.Fatalf("check env %s: %v", id, err)
		}
		if !exists {
			t.Fatalf("environment %s should be preserved", id)
		}
	}
	var oldExists bool
	if err := testDB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environments WHERE id = $1)`, customOldID).Scan(&oldExists); err != nil {
		t.Fatalf("check old custom env: %v", err)
	}
	if oldExists {
		t.Fatal("oldest non-standard environment should be deleted")
	}
}

func TestAdversarial_DeactivateExcessLogDrains_KeepsNewest(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-log-order-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var ids []string
	for i := range 4 {
		id := "ld-" + newID()
		ids = append(ids, id)
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO log_drains (id, project_id, name, drain_type, endpoint_url, enabled, created_at, updated_at)
			VALUES ($1, $2, $3, 'http', 'https://example.com/drain', true, $4, $4)
		`, id, p.ID, fmt.Sprintf("drain-%d", i), base.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("insert log drain %d: %v", i, err)
		}
	}

	deactivated, err := pgStore.DeactivateExcessLogDrains(ctx, orgID, 2)
	if err != nil {
		t.Fatalf("DeactivateExcessLogDrains: %v", err)
	}
	if deactivated != 2 {
		t.Fatalf("deactivated = %d, want 2", deactivated)
	}
	for i, id := range ids {
		var enabled bool
		if err := testDB.Pool.QueryRow(ctx, `SELECT enabled FROM log_drains WHERE id = $1`, id).Scan(&enabled); err != nil {
			t.Fatalf("check drain %d: %v", i, err)
		}
		isNewest := i >= 2
		if isNewest && !enabled {
			t.Fatalf("newest drain %d should remain enabled", i)
		}
		if !isNewest && enabled {
			t.Fatalf("oldest drain %d should be disabled", i)
		}
	}
}

func TestAdversarial_DeactivateExcessNotificationChannels_KeepsNewest(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-notif-order-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var ids []string
	for i := range 4 {
		id := "nc-" + newID()
		ids = append(ids, id)
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO notification_channels (id, project_id, channel_type, name, config, enabled, created_at, updated_at)
			VALUES ($1, $2, 'webhook', $3, $5, true, $4, $4)
		`, id, p.ID, fmt.Sprintf("channel-%d", i), base.Add(time.Duration(i)*time.Hour), []byte("{}")); err != nil {
			t.Fatalf("insert notification channel %d: %v", i, err)
		}
	}

	deactivated, err := pgStore.DeactivateExcessNotificationChannelsByProject(ctx, p.ID, 2)
	if err != nil {
		t.Fatalf("DeactivateExcessNotificationChannelsByProject: %v", err)
	}
	if deactivated != 2 {
		t.Fatalf("deactivated = %d, want 2", deactivated)
	}
	for i, id := range ids {
		var enabled bool
		if err := testDB.Pool.QueryRow(ctx, `SELECT enabled FROM notification_channels WHERE id = $1`, id).Scan(&enabled); err != nil {
			t.Fatalf("check channel %d: %v", i, err)
		}
		isNewest := i >= 2
		if isNewest && !enabled {
			t.Fatalf("newest channel %d should remain enabled", i)
		}
		if !isNewest && enabled {
			t.Fatalf("oldest channel %d should be disabled", i)
		}
	}
}

// H7: DeactivateExcessWebhookSubscriptions keeps the newest by created_at

func TestAdversarial_DeactivateExcessWebhookSubscriptions_KeepsNewest(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-wh-order-" + newID()
	p := createProject(t, ctx, q, orgID, "P")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var whIDs []string
	for i := range 5 {
		wid := createWebhookSub(t, ctx, p.ID, fmt.Sprintf("https://example.com/wh%d", i))
		whIDs = append(whIDs, wid)
		_, _ = testDB.Pool.Exec(ctx,
			"UPDATE webhook_subscriptions SET created_at = $2 WHERE id = $1",
			wid, base.Add(time.Duration(i)*time.Hour))
	}

	deactivated, err := pgStore.DeactivateExcessWebhookSubscriptions(ctx, orgID, 2)
	if err != nil {
		t.Fatalf("DeactivateExcessWebhookSubscriptions: %v", err)
	}
	if deactivated != 3 {
		t.Fatalf("deactivated = %d, want 3", deactivated)
	}

	for i, wid := range whIDs {
		var active bool
		if err := testDB.Pool.QueryRow(ctx,
			"SELECT active FROM webhook_subscriptions WHERE id = $1", wid).Scan(&active); err != nil {
			t.Fatalf("check wh %d: %v", i, err)
		}
		isNewest := i >= 3
		if isNewest && !active {
			t.Errorf("webhook %d (newest) should still be active", i)
		}
		if !isNewest && active {
			t.Errorf("webhook %d (oldest) should be deactivated", i)
		}
	}
}

// H4: SuspendExcessProjects determinism with tied created_at

func TestAdversarial_SuspendExcessProjects_TiedCreatedAt(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-tied-" + newID()
	tiedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	var projectIDs []string
	for i := range 3 {
		p := createProject(t, ctx, q, orgID, fmt.Sprintf("P%d", i))
		projectIDs = append(projectIDs, p.ID)
		_, _ = testDB.Pool.Exec(ctx,
			"UPDATE projects SET created_at = $2 WHERE id = $1",
			p.ID, tiedTime)
	}

	count, err := pgStore.SuspendExcessProjects(ctx, orgID, 1)
	if err != nil {
		t.Fatalf("SuspendExcessProjects: %v", err)
	}
	if count != 2 {
		t.Fatalf("suspended = %d, want 2", count)
	}

	var keptID string
	for _, pid := range projectIDs {
		suspended, err := pgStore.IsProjectSuspended(ctx, pid)
		if err != nil {
			t.Fatalf("IsProjectSuspended(%s): %v", pid, err)
		}
		if !suspended {
			keptID = pid
		}
	}
	if keptID == "" {
		t.Fatal("expected exactly one project to be kept")
	}

	// Unsuspend all and repeat -- the same project should be kept.
	_, _ = testDB.Pool.Exec(ctx,
		"UPDATE projects SET suspended = false WHERE org_id = $1", orgID)

	count2, err := pgStore.SuspendExcessProjects(ctx, orgID, 1)
	if err != nil {
		t.Fatalf("second SuspendExcessProjects: %v", err)
	}
	if count2 != 2 {
		t.Fatalf("second suspended = %d, want 2", count2)
	}

	var keptID2 string
	for _, pid := range projectIDs {
		suspended, err := pgStore.IsProjectSuspended(ctx, pid)
		if err != nil {
			t.Fatalf("IsProjectSuspended(%s): %v", pid, err)
		}
		if !suspended {
			keptID2 = pid
		}
	}
	if keptID2 != keptID {
		t.Errorf("non-deterministic: first kept %s, second kept %s", keptID, keptID2)
	}
}
