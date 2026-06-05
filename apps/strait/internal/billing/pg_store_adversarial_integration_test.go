//go:build integration

package billing_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		require.NoError(t, pgStore.
			UpsertUsageRecord(ctx,
				rec,
			))

	}

	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)

	recsA, err := pgStore.GetOrgUsageForPeriod(ctx, orgA, from, to)
	require.NoError(t, err)

	for _, r := range recsA {
		assert.Equal(t, orgA, r.OrgID)
		assert.NotEqual(t, pB.ID,

			r.ProjectID,
		)

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
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	s1, _ := pgStore.IsProjectSuspended(ctx, p1.ID)
	s2, _ := pgStore.IsProjectSuspended(ctx, p2.ID)
	assert.False(t, s1)
	assert.True(t, s2)

}

// A3: Integer overflow in spending limit

func TestAdversarial_SpendingLimitMaxInt64(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-maxint-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	maxInt64 := int64(9_223_372_036_854_775_807)
	require.NoError(t, pgStore.
		UpdateSpendingLimit(ctx, orgID,
			maxInt64,
			"notify",
		))

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, maxInt64,

		sub.SpendingLimitMicrousd,
	)

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

	for _, err := range errs {
		assert.NoError(t, err)

	}

	// Should have exactly one row.
	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx, "SELECT COUNT(*) FROM organization_subscriptions WHERE org_id = $1",

		orgID).Scan(&count))
	assert.EqualValues(t, 1, count)

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
	require.NoError(t, pgStore.
		CreateAddon(ctx, a),
	)
	require.NoError(t, pgStore.
		DeactivateAddon(ctx,
			a.ID),
	)
	require.NoError(t, pgStore.
		DeactivateAddon(ctx,
			a.ID),
	)

	addons, err := pgStore.ListActiveAddons(ctx, orgID)
	require.NoError(t, err)
	assert.Len(t, addons, 0)

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
	require.NoError(t, pgStore.
		CreateAddon(ctx, a),
	)

	// Second insert with same ID should fail (PK violation).
	err := pgStore.CreateAddon(ctx, a)
	assert.Error(t, err)

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

	for _, err := range errs {
		assert.NoError(t, err)

	}

	processed, err := pgStore.IsWebhookProcessed(ctx, msgID)
	require.NoError(t, err)
	assert.True(t, processed)

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
	require.NoError(t, err)
	assert.Len(t, ids, 0)

	count, err := pgStore.CountExecutingRunsByOrg(ctx, "")
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)

}

// A9: Future period pending downgrade not listed

func TestAdversarial_FuturePendingDowngradeNotListed(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-futpend-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	future := time.Now().UTC().Add(30 * 24 * time.Hour)
	require.NoError(t, pgStore.
		SetPendingDowngrade(ctx, orgID,
			"free",
			&future,

			&future))

	subs, err := pgStore.ListOrgsWithPendingDowngrade(ctx)
	require.NoError(t, err)

	for _, s := range subs {
		assert.NotEqual(t, orgID,

			s.OrgID)

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
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

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
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			base))

	// First insert so conflict path is exercised.

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

	for _, err := range errs {
		assert.NoError(t, err)

	}

	got, err := pgStore.GetEnterpriseContract(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, orgID, got.
		OrgID)
	assert.Equal(t, billing.EnterpriseTierStarter,

		got.EnterpriseTier,
	)

	// One writer must have won; the contract must be valid.

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
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			c1))

	// Second upsert with different ID for same org -- should update, not create second row.
	c2 := *c1
	c2.ID = "contract_u2"
	c2.EnterpriseTier = billing.EnterpriseTierGrowth
	c2.AnnualCommitmentCents = 4800000
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			&c2))

	got, err := pgStore.GetEnterpriseContract(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, billing.EnterpriseTierGrowth,

		got.EnterpriseTier,
	)
	assert.EqualValues(t, 4800000,
		got.
			AnnualCommitmentCents,
	)

	// Should reflect the second upsert's values.

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
		require.NoError(t, pgStore.
			UpsertEnterpriseContract(ctx,
				c))

	}

	mkContract("expired-1h", -1*time.Hour)      // already expired
	mkContract("expired-30d", -30*24*time.Hour) // long expired
	mkContract("expiring-1h", 1*time.Hour)      // about to expire
	mkContract("expiring-6d", 6*24*time.Hour)   // 6 days out
	mkContract("expiring-29d", 29*24*time.Hour) // 29 days out
	mkContract("expiring-31d", 31*24*time.Hour) // 31 days out
	mkContract("future-365d", 365*24*time.Hour) // way out

	within30, err := pgStore.ListExpiringContracts(ctx, 30)
	require.NoError(t, err)

	// Should include: 1h, 6d, 29d (3 contracts). NOT expired or 31d+.
	if len(within30) != 3 {
		assert.Failf(t, "test failure",

			"expected 3 contracts within 30 days, got %d", len(within30))
		for _, c := range within30 {
			t.Logf("  org=%s end=%v", c.OrgID, c.ContractEndDate)
		}
	}

	// Within 1 day should only get the 1h one.
	within1, err := pgStore.ListExpiringContracts(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, within1, 1)

	// Negative days should return nothing.
	withinNeg, err := pgStore.ListExpiringContracts(ctx, -1)
	require.NoError(t, err)
	assert.Len(t, withinNeg,
		0,
	)

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
		require.NoError(t, pgStore.
			UpsertEnterpriseContract(ctx,
				c))

		return orgID
	}

	lapsedMidPeriod := insertContract("lapsed", periodStart.AddDate(0, -1, 0), periodStart.Add(14*24*time.Hour))
	activeThroughPeriod := insertContract("active", periodStart.Add(7*24*time.Hour), periodEnd.Add(24*time.Hour))
	insertContract("ended-at-start", periodStart.AddDate(0, -1, 0), periodStart)
	insertContract("starts-at-end", periodEnd, periodEnd.AddDate(0, 1, 0))

	contracts, err := pgStore.ListEnterpriseContractsOverlappingPeriod(ctx, periodStart, periodEnd)
	require.NoError(t, err)

	seen := make(map[string]bool, len(contracts))
	for _, contract := range contracts {
		seen[contract.OrgID] = true
	}
	require.True(t, seen[lapsedMidPeriod])
	require.True(t, seen[activeThroughPeriod])
	require.Len(t, contracts,

		2)

}

// A14: Empty org ID returns not-found, not a cross-org leak

func TestAdversarial_EmptyOrgIDContract(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	_, err := pgStore.GetEnterpriseContract(ctx, "")
	require.Error(t, err)

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
	require.NoError(t, err)
	require.Len(t, deactivated,

		3)

	for i, jid := range jobIDs {
		var cron string
		require.NoError(t, testDB.
			Pool.QueryRow(ctx, "SELECT COALESCE(cron, '') FROM jobs WHERE id = $1",

			jid).Scan(&cron))

		isNewest := i >= 3
		hasCron := cron != ""
		assert.False(t, isNewest &&
			!hasCron,
		)
		assert.False(t, !isNewest &&
			hasCron,
		)

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
	require.NoError(t, err)
	require.EqualValues(t, 2, deactivated)

	for i, eid := range envIDs {
		var exists bool
		err := testDB.Pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM environments WHERE id = $1)", eid).Scan(&exists)
		require.NoError(t, err)

		isNewest := i >= 2
		assert.False(t, isNewest &&
			!exists,
		)
		assert.False(t, !isNewest &&
			exists,
		)

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
		require.Failf(t, "test failure",

			"seed environment timestamps: %v", err)
	}

	deactivated, err := pgStore.DeactivateExcessEnvironments(ctx, orgID, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, deactivated)

	for _, id := range []string{standardID, customNewID} {
		var exists bool
		require.NoError(t, testDB.
			Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environments WHERE id = $1)`,

			id,
		).Scan(&exists))
		require.True(t, exists)

	}
	var oldExists bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environments WHERE id = $1)`,

		customOldID,
	).Scan(&oldExists))
	require.False(t, oldExists)

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
			require.Failf(t, "test failure",

				"insert log drain %d: %v", i, err)
		}
	}

	deactivated, err := pgStore.DeactivateExcessLogDrains(ctx, orgID, 2)
	require.NoError(t, err)
	require.EqualValues(t, 2, deactivated)

	for i, id := range ids {
		var enabled bool
		require.NoError(t, testDB.
			Pool.QueryRow(ctx, `SELECT enabled FROM log_drains WHERE id = $1`,

			id).Scan(&enabled))

		isNewest := i >= 2
		require.False(t, isNewest &&
			!enabled,
		)
		require.False(t, !isNewest &&
			enabled,
		)

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
			require.Failf(t, "test failure",

				"insert notification channel %d: %v", i, err)
		}
	}

	deactivated, err := pgStore.DeactivateExcessNotificationChannelsByProject(ctx, p.ID, 2)
	require.NoError(t, err)
	require.EqualValues(t, 2, deactivated)

	for i, id := range ids {
		var enabled bool
		require.NoError(t, testDB.
			Pool.QueryRow(ctx, `SELECT enabled FROM notification_channels WHERE id = $1`,

			id,
		).Scan(&enabled))

		isNewest := i >= 2
		require.False(t, isNewest &&
			!enabled,
		)
		require.False(t, !isNewest &&
			enabled,
		)

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
	require.NoError(t, err)
	require.EqualValues(t, 3, deactivated)

	for i, wid := range whIDs {
		var active bool
		require.NoError(t, testDB.
			Pool.QueryRow(ctx, "SELECT active FROM webhook_subscriptions WHERE id = $1",

			wid,
		).Scan(&active))

		isNewest := i >= 3
		assert.False(t, isNewest &&
			!active,
		)
		assert.False(t, !isNewest &&
			active,
		)

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
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

	var keptID string
	for _, pid := range projectIDs {
		suspended, err := pgStore.IsProjectSuspended(ctx, pid)
		require.NoError(t, err)

		if !suspended {
			keptID = pid
		}
	}
	require.NotEqual(t, "", keptID)

	// Unsuspend all and repeat -- the same project should be kept.
	_, _ = testDB.Pool.Exec(ctx,
		"UPDATE projects SET suspended = false WHERE org_id = $1", orgID)

	count2, err := pgStore.SuspendExcessProjects(ctx, orgID, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, count2)

	var keptID2 string
	for _, pid := range projectIDs {
		suspended, err := pgStore.IsProjectSuspended(ctx, pid)
		require.NoError(t, err)

		if !suspended {
			keptID2 = pid
		}
	}
	assert.Equal(t, keptID, keptID2)

}
