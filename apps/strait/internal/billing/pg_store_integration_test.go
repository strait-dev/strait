//go:build integration

package billing_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.SetupSharedTestDB(ctx, "../../migrations", "billing")
	if err != nil {
		log.Fatalf("setup test db: %v", err)
	}

	// Create the users table if it doesn't exist (not in migrations but needed by ListOrgAdminEmails).
	_, err = testDB.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			name TEXT,
			email TEXT,
			email_verified BOOLEAN DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Fatalf("create users table: %v", err)
	}

	code := m.Run()
	testDB.Cleanup(ctx)
	os.Exit(code)
}

func mustQueries(t *testing.T) *store.Queries {
	t.Helper()
	require.False(t, testDB ==

		nil || testDB.
		Pool ==
		nil)

	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()

	// Clean users table separately since it's not in the standard CleanTables.
	if _, err := testDB.Pool.Exec(ctx, "DELETE FROM users"); err != nil {
		require.Failf(t, "test failure",

			"clean users: %v", err)
	}
	require.NoError(t, testDB.
		CleanTables(ctx))

}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func createProject(t *testing.T, ctx context.Context, q *store.Queries, orgID, name string) *domain.Project {
	t.Helper()

	project := &domain.Project{
		ID:    newID(),
		OrgID: orgID,
		Name:  name,
	}
	require.NoError(t, q.CreateProject(
		ctx, project,
	))

	return project
}

func createJob(t *testing.T, ctx context.Context, q *store.Queries, projectID string) *domain.Job {
	t.Helper()

	job := &domain.Job{
		ID:            newID(),
		ProjectID:     projectID,
		Name:          "job-" + newID(),
		Slug:          "slug-" + newID(),
		Description:   "job description",
		Cron:          "*/5 * * * *",
		PayloadSchema: []byte(`{"type":"object"}`),
		EndpointURL:   "https://example.com/webhook",
		MaxAttempts:   5,
		TimeoutSecs:   120,
		Enabled:       true,
		WebhookURL:    "https://example.com/callback",
		WebhookSecret: "secret",
	}
	require.NoError(t, q.CreateJob(ctx,
		job))

	return job
}

func createRun(t *testing.T, ctx context.Context, q *store.Queries, job *domain.Job, status domain.RunStatus) *domain.JobRun {
	t.Helper()

	run := &domain.JobRun{
		ID:            newID(),
		JobID:         job.ID,
		ProjectID:     job.ProjectID,
		Status:        status,
		Attempt:       1,
		Payload:       []byte(`{"hello":"world"}`),
		TriggeredBy:   domain.TriggerManual,
		Priority:      0,
		ExecutionMode: domain.ExecutionModeHTTP,
	}
	require.NoError(t, q.CreateRun(ctx,
		run))

	return run
}

func createMember(t *testing.T, ctx context.Context, q *store.Queries, projectID, userID string) {
	t.Helper()

	role := &domain.ProjectRole{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "member-" + newID(),
		Permissions: []string{"jobs:read"},
	}
	require.NoError(t, q.CreateProjectRole(ctx, role))

	member := &domain.ProjectMemberRole{
		ID:        newID(),
		ProjectID: projectID,
		UserID:    userID,
		RoleID:    role.ID,
		GrantedBy: "tester",
	}
	require.NoError(t, q.AssignMemberRole(ctx, member))

}

func createAdminMember(t *testing.T, ctx context.Context, q *store.Queries, projectID, userID, email string) {
	t.Helper()
	_, err := testDB.Pool.Exec(ctx,
		"INSERT INTO users (id, name, email, email_verified) VALUES ($1, $2, $3, true) ON CONFLICT (id) DO NOTHING",
		userID, "Test User "+userID, email)
	require.NoError(t, err)

	// Try to find an existing admin role for this project first.
	var roleID string
	err = testDB.Pool.QueryRow(ctx,
		"SELECT id FROM project_roles WHERE project_id = $1 AND name = 'admin' LIMIT 1",
		projectID).Scan(&roleID)
	if err != nil {
		// No admin role exists yet; create one.
		role := &domain.ProjectRole{
			ID:          newID(),
			ProjectID:   projectID,
			Name:        "admin",
			Permissions: []string{"*"},
		}
		require.NoError(t, q.CreateProjectRole(ctx, role))

		roleID = role.ID
	}

	member := &domain.ProjectMemberRole{
		ID:        newID(),
		ProjectID: projectID,
		UserID:    userID,
		RoleID:    roleID,
		GrantedBy: "test",
	}
	require.NoError(t, q.AssignMemberRole(ctx, member))

}

func createWebhookSub(t *testing.T, ctx context.Context, projectID, url string) string {
	t.Helper()
	id := newID()
	_, err := testDB.Pool.Exec(ctx,
		"INSERT INTO webhook_subscriptions (id, project_id, webhook_url, event_types, secret, active) VALUES ($1, $2, $3, $4, $5, true)",
		id, projectID, url, "{run.completed}", "secret")
	require.NoError(t, err)

	return id
}

func createEnvironment(t *testing.T, ctx context.Context, projectID, name string) string {
	t.Helper()
	id := newID()
	slug := "slug-" + id
	_, err := testDB.Pool.Exec(ctx,
		"INSERT INTO environments (id, project_id, name, slug) VALUES ($1, $2, $3, $4)",
		id, projectID, name, slug)
	require.NoError(t, err)

	return id
}

func ensureSub(t *testing.T, ctx context.Context, pgStore *billing.PgStore, orgID string) {
	t.Helper()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))

}

func ptr[T any](v T) *T { return &v } //nolint:modernize // new(expr) doesn't work for literals

func timeClose(a, b time.Time, d time.Duration) bool {
	diff := a.Sub(b)
	if diff < 0 {
		diff = -diff
	}
	return diff < d
}

func TestPgStore_EnsureOrgSubscription(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ensure-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))

	// First call creates a free subscription.

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, "free", sub.
		PlanTier,
	)
	assert.Equal(t, "active",

		sub.Status,
	)
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))

	// Second call is idempotent.

	sub2, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, sub.ID, sub2.
		ID)

}

func TestPgStore_OrgSubscriptionCacheVersionRoundTripAndBump(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-cache-version-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	require.False(t, sub.CacheVersion <=
		0)

	initial := sub.CacheVersion
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID,
			string(domain.
				PlanPro,
			), "active",
		))

	updated, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	require.False(t, updated.
		CacheVersion <=
		initial,
	)

}

func TestPgStore_GetOrgSubscription(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-get-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	now := time.Now().UTC()
	periodStart := now.Add(-24 * time.Hour)
	periodEnd := now.Add(30 * 24 * time.Hour)
	canceledAt := now.Add(-1 * time.Hour)
	graceEnd := now.Add(7 * 24 * time.Hour)

	// Set up a subscription with all fields populated via raw SQL.
	_, err := testDB.Pool.Exec(ctx, `
		UPDATE organization_subscriptions SET
			plan_tier = 'pro',
			stripe_subscription_id = 'stripe-sub-123',
			stripe_customer_id = 'stripe-cust-456',
			status = 'active',
			current_period_start = $2,
			current_period_end = $3,
			spending_limit_microusd = 50000000,
			limit_action = 'suspend',
			pending_plan_tier = 'free',
			canceled_at = $4,
			anomaly_threshold_warning = 2.5,
			anomaly_threshold_critical = 8.0,
			grace_period_end = $5,
			payment_status = 'grace',
			override_daily_run_limit = 1000,
			override_concurrent_run_limit = 50,
			enforcement_mode = 'warn',
			monthly_usage_email = true,
			updated_at = NOW()
		WHERE org_id = $1
	`, orgID, periodStart, periodEnd, canceledAt, graceEnd)
	require.NoError(t, err)

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.NotEqual(t, "", sub.
		ID)
	assert.Equal(t, orgID, sub.
		OrgID)
	assert.Equal(t, "pro", sub.
		PlanTier,
	)
	assert.False(t, sub.StripeSubscriptionID ==
		nil ||
		*sub.
			StripeSubscriptionID !=
			"stripe-sub-123",
	)
	assert.False(t, sub.StripeCustomerID ==
		nil ||
		*sub.StripeCustomerID !=
			"stripe-cust-456",
	)
	assert.Equal(t, "active",

		sub.Status,
	)

	// Field 1: ID

	// Field 2: OrgID

	// Field 3: PlanTier

	// Field 4: StripeSubscriptionID

	// Field 5: StripeCustomerID

	// Field 6: Status

	// Field 7: CurrentPeriodStart
	if sub.CurrentPeriodStart == nil {
		assert.Failf(t, "test failure",

			"CurrentPeriodStart is nil")
	} else if !timeClose(*sub.CurrentPeriodStart, periodStart, 5*time.Second) {
		assert.Failf(t, "test failure",

			"CurrentPeriodStart = %v, want close to %v", *sub.CurrentPeriodStart, periodStart)
	}
	// Field 8: CurrentPeriodEnd
	if sub.CurrentPeriodEnd == nil {
		assert.Failf(t, "test failure",

			"CurrentPeriodEnd is nil")
	} else if !timeClose(*sub.CurrentPeriodEnd, periodEnd, 5*time.Second) {
		assert.Failf(t, "test failure",

			"CurrentPeriodEnd = %v, want close to %v", *sub.CurrentPeriodEnd, periodEnd)
	}
	assert.EqualValues(t, 50000000,

		sub.SpendingLimitMicrousd,
	)
	assert.Equal(t, "suspend",

		sub.LimitAction,
	)
	assert.False(t, sub.PendingPlanTier ==
		nil ||
		*sub.PendingPlanTier !=
			"free")

	// Field 9: SpendingLimitMicrousd

	// Field 10: LimitAction

	// Field 11: PendingPlanTier

	// Field 12: CanceledAt
	if sub.CanceledAt == nil {
		assert.Failf(t, "test failure",

			"CanceledAt is nil")
	} else if !timeClose(*sub.CanceledAt, canceledAt, 5*time.Second) {
		assert.Failf(t, "test failure",

			"CanceledAt = %v, want close to %v", *sub.CanceledAt, canceledAt)
	}
	assert.LessOrEqual(t, math.
		Abs(sub.
			AnomalyThresholdWarning-
			2.5,
		), 0.01,
	)
	assert.LessOrEqual(t, math.
		Abs(sub.
			AnomalyThresholdCritical-
			8.0,
		), 0.01,
	)

	// Field 13: AnomalyThresholdWarning

	// Field 14: AnomalyThresholdCritical

	// Field 15: GracePeriodEnd
	if sub.GracePeriodEnd == nil {
		assert.Failf(t, "test failure",

			"GracePeriodEnd is nil")
	} else if !timeClose(*sub.GracePeriodEnd, graceEnd, 5*time.Second) {
		assert.Failf(t, "test failure",

			"GracePeriodEnd = %v, want close to %v", *sub.GracePeriodEnd, graceEnd)
	}
	assert.Equal(t, "grace",
		sub.
			PaymentStatus,
	)
	assert.False(t, sub.OverrideDailyRunLimit ==
		nil ||
		*sub.
			OverrideDailyRunLimit !=
			1000,
	)
	assert.False(t, sub.OverrideConcurrentRunLimit ==
		nil ||
		*sub.OverrideConcurrentRunLimit !=
			50)
	assert.Equal(t, "warn", sub.
		EnforcementMode,
	)
	assert.Equal(t, true, sub.
		MonthlyUsageEmail,
	)
	assert.False(t, sub.CreatedAt.
		IsZero())
	assert.False(t, sub.UpdatedAt.
		IsZero())

	// Field 16: PaymentStatus

	// Field 17: OverrideDailyRunLimit

	// Field 18: OverrideConcurrentRunLimit

	// Field 19: EnforcementMode

	// Field 20: MonthlyUsageEmail

	// Field 21a: CreatedAt

	// Field 21b: UpdatedAt

}

func TestPgStore_GetOrgSubscription_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	_, err := pgStore.GetOrgSubscription(ctx, "org-nonexistent")
	require.Error(t, err)

	if !errors.Is(err, billing.ErrSubscriptionNotFound) {
		assert.True(t, contains(err.
			Error(),
			"not found",
		))

	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestPgStore_UpsertOrgSubscription(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-upsert-" + newID()
	now := time.Now().UTC().Truncate(time.Microsecond)
	ps := now.Add(-24 * time.Hour)
	pe := now.Add(30 * 24 * time.Hour)

	sub := &billing.OrgSubscription{
		ID:                    newID(),
		OrgID:                 orgID,
		PlanTier:              "pro",
		StripeSubscriptionID:  ptr("stripe-sub"),  //nolint:modernize
		StripeCustomerID:      ptr("stripe-cust"), //nolint:modernize
		Status:                "active",
		CurrentPeriodStart:    &ps,
		CurrentPeriodEnd:      &pe,
		SpendingLimitMicrousd: 100_000_000,
		LimitAction:           "notify",
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	require.NoError(t, pgStore.
		UpsertOrgSubscription(ctx, sub))

	got, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, "pro", got.
		PlanTier,
	)
	assert.Equal(t, "active",

		got.Status,
	)

	// Upsert again with different plan.
	sub.PlanTier = "enterprise"
	require.NoError(t, pgStore.
		UpsertOrgSubscription(ctx, sub))

	got2, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, "enterprise",

		got2.
			PlanTier)

}

func TestPgStore_UpdateOrgSubscriptionPlan(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-upplan-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID,
			"pro",
			"active",
		))

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, "pro", sub.
		PlanTier,
	)
	assert.Equal(t, "active",

		sub.Status,
	)

}

func TestPgStore_GetOrgSubscriptionByStripeBindings(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-stripe-bindings-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET stripe_subscription_id = 'sub_lookup_123',
		    stripe_customer_id = 'cus_lookup_123'
		WHERE org_id = $1
	`, orgID); err != nil {
		require.Failf(t, "test failure",

			"update subscription bindings: %v", err)
	}

	bySub, err := pgStore.GetOrgSubscriptionByStripeSubscriptionID(ctx, "sub_lookup_123")
	require.NoError(t, err)
	require.Equal(t, orgID, bySub.
		OrgID,
	)

	byCustomer, err := pgStore.GetOrgSubscriptionByStripeCustomerID(ctx, "cus_lookup_123")
	require.NoError(t, err)
	require.Equal(t, orgID, byCustomer.
		OrgID)

	if _, err := pgStore.GetOrgSubscriptionByStripeSubscriptionID(ctx, "sub_missing"); !errors.Is(err, billing.ErrSubscriptionNotFound) {
		require.Failf(t, "test failure",

			"missing subscription binding error = %v, want ErrSubscriptionNotFound", err)
	}
}

func TestPgStore_StripeBindingsAreGloballyUnique(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgA := "org-stripe-unique-a-" + newID()
	orgB := "org-stripe-unique-b-" + newID()
	ensureSub(t, ctx, pgStore, orgA)
	ensureSub(t, ctx, pgStore, orgB)

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET stripe_subscription_id = 'sub_unique_123',
		    stripe_customer_id = 'cus_unique_123'
		WHERE org_id = $1
	`, orgA); err != nil {
		require.Failf(t, "test failure",

			"seed first binding: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET stripe_subscription_id = 'sub_unique_123'
		WHERE org_id = $1
	`, orgB); err == nil {
		require.Fail(t,

			"expected duplicate stripe_subscription_id to fail")
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET stripe_customer_id = 'cus_unique_123'
		WHERE org_id = $1
	`, orgB); err == nil {
		require.Fail(t,

			"expected duplicate stripe_customer_id to fail")
	}
}

func TestPgStore_UpdateOrgSubscriptionPlan_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.UpdateOrgSubscriptionPlan(ctx, "org-nonexistent", "pro", "active")
	require.Error(t, err)

}

func TestPgStore_UpdateOrgSubscriptionFull(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-upfull-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	ps := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	pe := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionFull(ctx,
			orgID,
			"pro",
			"active",
			&ps,
			&pe))

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, "pro", sub.
		PlanTier,
	)
	assert.False(t, sub.CurrentPeriodStart ==
		nil ||
		!timeClose(*sub.
			CurrentPeriodStart,

			ps, 5*time.
				Second,
		))
	assert.False(t, sub.CurrentPeriodEnd ==
		nil ||
		!timeClose(*sub.
			CurrentPeriodEnd,

			pe,
			5*time.
				Second))

}

func TestPgStore_UpdateOrgSubscriptionFull_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	ps := time.Now().UTC()
	pe := ps.Add(30 * 24 * time.Hour)
	err := pgStore.UpdateOrgSubscriptionFull(ctx, "org-nonexistent", "pro", "active", &ps, &pe)
	require.Error(t, err)

}

func TestPgStore_SetPendingPlanTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-pending-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		SetPendingPlanTier(
			ctx, orgID,
			"free",
		))

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.False(t, sub.PendingPlanTier ==
		nil ||
		*sub.PendingPlanTier !=
			"free")

}

func TestPgStore_SetPendingPlanTier_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.SetPendingPlanTier(ctx, "org-nonexistent", "free")
	require.Error(t, err)

}

func TestPgStore_SetPendingDowngrade(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-penddown-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	ps := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	pe := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, pgStore.
		SetPendingDowngrade(ctx, orgID,
			"free",
			&ps,
			&pe))

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.False(t, sub.PendingPlanTier ==
		nil ||
		*sub.PendingPlanTier !=
			"free")
	assert.False(t, sub.CurrentPeriodStart ==
		nil ||
		!timeClose(*sub.
			CurrentPeriodStart,

			ps, 5*time.
				Second,
		))
	assert.False(t, sub.CurrentPeriodEnd ==
		nil ||
		!timeClose(*sub.
			CurrentPeriodEnd,

			pe,
			5*time.
				Second))

}

func TestPgStore_ClearPendingPlanTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-clrpend-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		SetPendingPlanTier(
			ctx, orgID,
			"free",
		))
	require.NoError(t, pgStore.
		ClearPendingPlanTier(ctx, orgID))

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Nil(t, sub.
		PendingPlanTier,
	)

}

func TestPgStore_ClearPendingPlanTier_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.ClearPendingPlanTier(ctx, "org-nonexistent")
	require.Error(t, err)

}

func TestPgStore_ApplyPendingDowngrade(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-applydown-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID,
			"pro",
			"active",
		))
	require.NoError(t, pgStore.
		SetPendingPlanTier(
			ctx, orgID,
			"free",
		))
	require.NoError(t, pgStore.
		ApplyPendingDowngrade(ctx, orgID))

	// Upgrade to pro first.

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, "free", sub.
		PlanTier,
	)
	assert.Nil(t, sub.
		PendingPlanTier,
	)

}

func TestPgStore_ApplyPendingDowngradeIfTier_RequiresSamePendingTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-applydown-if-tier-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID,
			"pro",
			"active",
		))
	require.NoError(t, pgStore.
		SetPendingPlanTier(
			ctx, orgID,
			"starter",
		))

	applied, err := pgStore.ApplyPendingDowngradeIfTier(ctx, orgID, "free")
	require.NoError(t, err)
	require.False(t, applied)

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	require.False(t, sub.PlanTier !=
		"pro" ||
		sub.
			PendingPlanTier ==
			nil ||
		*sub.PendingPlanTier !=
			"starter",
	)

	applied, err = pgStore.ApplyPendingDowngradeIfTier(ctx, orgID, "starter")
	require.NoError(t, err)
	require.True(t, applied)

	sub, err = pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	require.False(t, sub.PlanTier !=
		"starter" ||
		sub.PendingPlanTier !=
			nil,
	)

}

func TestPgStore_ApplyPendingDowngradeTierIfPending_RetainsPendingUntilConditionalClear(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-applydown-retain-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID,
			"pro",
			"active",
		))
	require.NoError(t, pgStore.
		SetPendingPlanTier(
			ctx, orgID,
			"starter",
		))

	applied, err := pgStore.ApplyPendingDowngradeTierIfPending(ctx, orgID, "free")
	require.NoError(t, err)
	require.False(t, applied)

	applied, err = pgStore.ApplyPendingDowngradeTierIfPending(ctx, orgID, "starter")
	require.NoError(t, err)
	require.True(t, applied)

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	require.False(t, sub.PlanTier !=
		"starter" ||
		sub.PendingPlanTier ==
			nil ||
		*sub.
			PendingPlanTier !=
			"starter")

	cleared, err := pgStore.ClearPendingPlanTierIfTier(ctx, orgID, "free")
	require.NoError(t, err)
	require.False(t, cleared)

	sub, err = pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	require.False(t, sub.PendingPlanTier ==
		nil ||
		*sub.PendingPlanTier !=
			"starter",
	)

	cleared, err = pgStore.ClearPendingPlanTierIfTier(ctx, orgID, "starter")
	require.NoError(t, err)
	require.True(t, cleared)

	sub, err = pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	require.False(t, sub.PlanTier !=
		"starter" ||
		sub.PendingPlanTier !=
			nil,
	)

}

func TestPgStore_ApplyPendingDowngrade_NoPending(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-nopend-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	err := pgStore.ApplyPendingDowngrade(ctx, orgID)
	require.Error(t, err)

}

func TestPgStore_ListOrgsWithPendingDowngrade(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgA := "org-lspend-a-" + newID()
	orgB := "org-lspend-b-" + newID()
	orgC := "org-lspend-c-" + newID()

	ensureSub(t, ctx, pgStore, orgA)
	ensureSub(t, ctx, pgStore, orgB)
	ensureSub(t, ctx, pgStore, orgC)

	past := time.Now().UTC().Add(-48 * time.Hour)
	future := time.Now().UTC().Add(48 * time.Hour)
	require.NoError(t, pgStore.
		SetPendingDowngrade(ctx, orgA,
			"free",
			&past,
			&past,
		))
	require.NoError(t, pgStore.
		SetPendingDowngrade(ctx, orgB,
			"free",
			&past,
			&future,
		))

	// orgA: pending downgrade + period ended (should be listed)

	// orgB: pending downgrade + period not ended yet (should NOT be listed)

	// orgC: no pending downgrade (should NOT be listed)

	subs, err := pgStore.ListOrgsWithPendingDowngrade(ctx)
	require.NoError(t, err)

	found := false
	for _, s := range subs {
		if s.OrgID == orgA {
			found = true
		}
		assert.False(t, s.OrgID ==

			orgB ||
			s.OrgID ==
				orgC)

	}
	assert.True(t, found)

}

func TestPgStore_UpdateSpendingLimit(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-splimit-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		UpdateSpendingLimit(ctx, orgID,
			200_000_000,

			"suspend",
		))

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.EqualValues(t, 200_000_000,

		sub.SpendingLimitMicrousd,
	)
	assert.Equal(t, "suspend",

		sub.LimitAction,
	)

}

func TestPgStore_UpdateSpendingLimit_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.UpdateSpendingLimit(ctx, "org-nonexistent", 100, "notify")
	require.Error(t, err)

}

func TestPgStore_GetProjectOrgID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-projorg-" + newID()
	p := createProject(t, ctx, q, orgID, "Test Project")

	got, err := pgStore.GetProjectOrgID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, orgID, got)

}

func TestPgStore_GetActiveProjectOrgID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-actprojorg-" + newID()
	pActive := createProject(t, ctx, q, orgID, "Active Project")
	pDeleted := createProject(t, ctx, q, orgID, "Deleted Project")

	// Soft-delete one project.
	_, err := testDB.Pool.Exec(ctx, "UPDATE projects SET deleted_at = NOW() WHERE id = $1", pDeleted.ID)
	require.NoError(t, err)

	got, err := pgStore.GetActiveProjectOrgID(ctx, pActive.ID)
	require.NoError(t, err)
	assert.Equal(t, orgID, got)

	// Deleted project should return error (no rows).
	_, err = pgStore.GetActiveProjectOrgID(ctx, pDeleted.ID)
	require.Error(t, err)

}

func TestPgStore_ListProjectsByOrg(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-listproj-" + newID()
	p1 := createProject(t, ctx, q, orgID, "P1")
	p2 := createProject(t, ctx, q, orgID, "P2")
	pDeleted := createProject(t, ctx, q, orgID, "Deleted")

	_, err := testDB.Pool.Exec(ctx, "UPDATE projects SET deleted_at = NOW() WHERE id = $1", pDeleted.ID)
	require.NoError(t, err)

	ids, err := pgStore.ListProjectsByOrg(ctx, orgID)
	require.NoError(t, err)
	require.Len(t, ids, 2)

	idSet := map[string]bool{ids[0]: true, ids[1]: true}
	assert.False(t, !idSet[p1.
		ID] || !idSet[p2.ID],
	)

}

func TestPgStore_CountProjectsByOrg(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-cntproj-" + newID()
	createProject(t, ctx, q, orgID, "P1")
	createProject(t, ctx, q, orgID, "P2")
	pDel := createProject(t, ctx, q, orgID, "PDel")
	_, _ = testDB.Pool.Exec(ctx, "UPDATE projects SET deleted_at = NOW() WHERE id = $1", pDel.ID)

	count, err := pgStore.CountProjectsByOrg(ctx, orgID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

}

func TestPgStore_CountOrgsByUser(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	userID := "user-cntorgs-" + newID()
	orgA := "org-a-" + newID()
	orgB := "org-b-" + newID()

	pA := createProject(t, ctx, q, orgA, "ProjectA")
	pB := createProject(t, ctx, q, orgB, "ProjectB")
	createMember(t, ctx, q, pA.ID, userID)
	createMember(t, ctx, q, pB.ID, userID)

	count, err := pgStore.CountOrgsByUser(ctx, userID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

}

func TestPgStore_BulkCountExecutingRunsByOrg(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgA := "org-bulk-a-" + newID()
	orgB := "org-bulk-b-" + newID()
	orgC := "org-bulk-c-" + newID()

	pA := createProject(t, ctx, q, orgA, "PA")
	pB := createProject(t, ctx, q, orgB, "PB")
	_ = createProject(t, ctx, q, orgC, "PC") // no runs

	jobA := createJob(t, ctx, q, pA.ID)
	jobB := createJob(t, ctx, q, pB.ID)

	createRun(t, ctx, q, jobA, domain.StatusExecuting)
	createRun(t, ctx, q, jobA, domain.StatusExecuting)
	createRun(t, ctx, q, jobA, domain.StatusCompleted) // not executing
	createRun(t, ctx, q, jobB, domain.StatusExecuting)

	counts, err := pgStore.BulkCountExecutingRunsByOrg(ctx, []string{orgA, orgB, orgC})
	require.NoError(t, err)
	assert.EqualValues(t, 2, counts[orgA])
	assert.EqualValues(t, 1, counts[orgB])
	assert.EqualValues(t, 0, counts[orgC])

	// orgC should have 0 (absent from map is ok).

}

func TestPgStore_BulkCountExecutingRunsByOrg_Empty(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	counts, err := pgStore.BulkCountExecutingRunsByOrg(ctx, []string{})
	require.NoError(t, err)
	assert.Len(t, counts, 0)

}

func TestPgStore_SetProjectOrgID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	p := createProject(t, ctx, q, "org-old", "P")
	newOrg := "org-new-" + newID()
	require.NoError(t, pgStore.
		SetProjectOrgID(ctx,
			p.ID, newOrg,
		))

	got, err := pgStore.GetProjectOrgID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, newOrg, got)

}

func TestPgStore_UpsertUsageRecord(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-usagerec-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()

	rec := &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        p.ID,
		PeriodDate:       day,
		RunsCount:        10,
		ComputeCostMicro: 5_000_000,
		UsageTokensTotal: 1000,
		UsageCostMicro:   500_000,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	require.NoError(t, pgStore.
		UpsertUsageRecord(ctx,
			rec))

	// Upsert again to accumulate.
	rec2 := &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        p.ID,
		PeriodDate:       day,
		RunsCount:        5,
		ComputeCostMicro: 1_000_000,
		UsageTokensTotal: 200,
		UsageCostMicro:   100_000,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	require.NoError(t, pgStore.
		UpsertUsageRecord(ctx,
			rec2))

	// Read back via raw SQL.
	var runs, compute, tokens, usageCost int64
	err := testDB.Pool.QueryRow(ctx,
		"SELECT runs_count, compute_cost_microusd, usage_tokens_total, usage_cost_microusd FROM usage_records WHERE org_id = $1 AND project_id = $2 AND period_date = $3",
		orgID, p.ID, day).Scan(&runs, &compute, &tokens, &usageCost)
	require.NoError(t, err)
	assert.EqualValues(t, 15, runs)
	assert.EqualValues(t, 6_000_000,

		compute)
	assert.EqualValues(t, 1200, tokens)
	assert.EqualValues(t, 600_000,
		usageCost,
	)

}

func TestPgStore_ReplaceUsageRecord_ReplacesSnapshot(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-usagesnap-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()

	rec := &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        p.ID,
		PeriodDate:       day,
		RunsCount:        10,
		ComputeCostMicro: 5_000_000,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	require.NoError(t, pgStore.
		ReplaceUsageRecord(
			ctx, rec))

	rec2 := &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        p.ID,
		PeriodDate:       day,
		RunsCount:        3,
		ComputeCostMicro: 1_000_000,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	require.NoError(t, pgStore.
		ReplaceUsageRecord(
			ctx, rec2),
	)

	var runs, compute int64
	err := testDB.Pool.QueryRow(ctx,
		"SELECT runs_count, compute_cost_microusd FROM usage_records WHERE org_id = $1 AND project_id = $2 AND period_date = $3",
		orgID, p.ID, day).Scan(&runs, &compute)
	require.NoError(t, err)
	assert.EqualValues(t, 3, runs)
	assert.EqualValues(t, 1_000_000,

		compute)

}

func TestPgStore_GetUsageForPeriod_IncludesRecordedComputeCosts(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-usage-compute-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	now := day.Add(12 * time.Hour)

	rec := &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        p.ID,
		PeriodDate:       day,
		RunsCount:        1,
		ComputeCostMicro: 2_500_000,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	require.NoError(t, pgStore.
		UpsertUsageRecord(ctx,
			rec))

	from := day
	to := day
	orgRecords, err := pgStore.GetOrgUsageForPeriod(ctx, orgID, from, to)
	require.NoError(t, err)

	assertRecordedComputeUsage(t, orgRecords, p.ID, 2_500_000)

	projectRecords, err := pgStore.GetProjectUsageForPeriod(ctx, p.ID, from, to)
	require.NoError(t, err)

	assertRecordedComputeUsage(t, projectRecords, p.ID, 2_500_000)

	dailyRecords, err := pgStore.GetOrgDailyUsage(ctx, orgID, day)
	require.NoError(t, err)

	assertRecordedComputeUsage(t, dailyRecords, p.ID, 2_500_000)
}

func TestPgStore_RecordUsageCost_DedupesLedgerAndUsage(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-cost-ledger-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	day := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	now := day.Add(10 * time.Hour)
	rec := &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        p.ID,
		PeriodDate:       day,
		RunsCount:        1,
		ComputeCostMicro: 20,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	recorded, err := pgStore.RecordUsageCost(ctx, rec, "strait:cost_recorded:run-ledger-1", "http")
	require.NoError(t, err)
	require.True(t, recorded)

	recorded, err = pgStore.RecordUsageCost(ctx, rec, "strait:cost_recorded:run-ledger-1", "http")
	require.NoError(t, err)
	require.False(t, recorded)

	var runs, compute int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx, "SELECT runs_count, compute_cost_microusd FROM usage_records WHERE org_id = $1 AND project_id = $2 AND period_date = $3",

		orgID,
		p.ID, day).Scan(&runs, &compute))
	require.False(t, runs !=
		1 ||
		compute !=
			20)

	var events int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx, "SELECT COUNT(*) FROM billing_cost_events WHERE idempotency_key = $1",

		"strait:cost_recorded:run-ledger-1",
	).Scan(&events))
	require.EqualValues(t, 1, events)

}

func TestPgStore_ReconcileFlatUsageCosts_RepairsMissingLedgerAndUsage(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-cost-reconcile-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	job := createJob(t, ctx, q, p.ID)
	finishedAt := time.Now().UTC()
	run := createRun(t, ctx, q, job, domain.StatusCompleted)
	if _, err := testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET finished_at = $1, execution_mode = $2 WHERE id = $3",
		finishedAt, domain.ExecutionModeHTTP, run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"set completed run fields: %v", err)
	}

	statusCode := 200
	delivery := &domain.WebhookDelivery{
		ID:             newID(),
		RunID:          run.ID,
		JobID:          job.ID,
		WebhookURL:     "https://example.com/callback",
		Status:         domain.WebhookStatusDelivered,
		Attempts:       1,
		MaxAttempts:    3,
		LastStatusCode: &statusCode,
		DeliveredAt:    &finishedAt,
	}
	require.NoError(t, q.CreateWebhookDelivery(ctx,
		delivery,
	))

	day := time.Date(finishedAt.Year(), finishedAt.Month(), finishedAt.Day(), 0, 0, 0, 0, time.UTC)
	require.NoError(t, pgStore.
		ReconcileFlatUsageCosts(ctx,
			orgID,
			day))
	require.NoError(t, pgStore.
		ReconcileFlatUsageCosts(ctx,
			orgID,
			day))

	var eventCount int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx, "SELECT COUNT(*) FROM billing_cost_events WHERE org_id = $1",

		orgID,
	).Scan(
		&eventCount))
	require.EqualValues(t, 2, eventCount)

	var runs, compute int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx, "SELECT runs_count, compute_cost_microusd FROM usage_records WHERE org_id = $1 AND project_id = $2 AND period_date = $3",

		orgID,
		p.ID, day).Scan(&runs, &compute))
	require.False(t, runs !=
		2 ||
		compute !=
			billing.
				HTTPCostPerRunMicrousd+
				billing.
					WebhookDeliveryCostPerRunMicrousd,
	)

}

func assertRecordedComputeUsage(t *testing.T, records []billing.UsageRecord, projectID string, want int64) {
	t.Helper()
	for _, rec := range records {
		if rec.ProjectID != projectID {
			continue
		}
		require.Equal(t, want, rec.
			ComputeCostMicro,
		)

		return
	}
	require.Failf(t, "test failure",

		"usage records missing project %s: %#v", projectID, records)
}

func TestPgStore_GetProjectBudget_NoRow(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	p := createProject(t, ctx, q, "org-budget", "P")

	budget, action, err := pgStore.GetProjectBudget(ctx, p.ID)
	require.NoError(t, err)
	assert.EqualValues(t, -1, budget)
	assert.Equal(t, "notify",

		action)

}

func TestPgStore_SetProjectBudget(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	p := createProject(t, ctx, q, "org-setbudget", "P")

	// Insert a project_quotas row first (required by SetProjectBudget which does UPDATE).
	_, err := testDB.Pool.Exec(ctx,
		"INSERT INTO project_quotas (project_id) VALUES ($1) ON CONFLICT DO NOTHING", p.ID)
	require.NoError(t, err)
	require.NoError(t, pgStore.
		SetProjectBudget(ctx,
			p.ID, 50_000_000,

			"suspend",
		))

	budget, action, err := pgStore.GetProjectBudget(ctx, p.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 50_000_000,

		budget)
	assert.Equal(t, "suspend",

		action)

}
func TestPgStore_UpdateAnomalyThresholds(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-anomaly-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		UpdateAnomalyThresholds(ctx,
			orgID,
			5.0, 15.0,
		))

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.LessOrEqual(t, math.
		Abs(sub.
			AnomalyThresholdWarning-
			5.0,
		), 0.01,
	)
	assert.LessOrEqual(t, math.
		Abs(sub.
			AnomalyThresholdCritical-
			15.0,
		), 0.01,
	)

}

func TestPgStore_UpdateAnomalyThresholds_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.UpdateAnomalyThresholds(ctx, "org-nonexistent", 5.0, 15.0)
	require.Error(t, err)

}

func TestPgStore_ListAllSubscribedOrgIDs(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgA := "org-listall-a-" + newID()
	orgB := "org-listall-b-" + newID()
	orgC := "org-listall-c-" + newID()

	ensureSub(t, ctx, pgStore, orgA)
	ensureSub(t, ctx, pgStore, orgB)
	ensureSub(t, ctx, pgStore, orgC)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgC,
			"free",
			"canceled",
		))

	// Cancel one.

	ids, err := pgStore.ListAllSubscribedOrgIDs(ctx)
	require.NoError(t, err)

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	assert.False(t, !idSet[orgA] || !idSet[orgB])
	assert.False(t, idSet[orgC])

}

func TestPlanRetentionResolver_MissingSubscriptionDoesNotFallbackToFreeRetention(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	resolver := billing.NewPlanRetentionResolver(billing.NewPgStore(testDB.Pool))

	days, err := resolver.GetOrgRetentionDays(ctx, "org-retention-missing-"+newID())
	require.True(t, errors.Is(
		err, billing.
			ErrSubscriptionNotFound,
	),
	)
	require.EqualValues(t, 0, days)

}

func TestPgStore_UpdatePaymentStatus(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-paystatus-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	graceEnd := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, pgStore.
		UpdatePaymentStatus(ctx, orgID,
			"grace",
			&graceEnd,
		),
	)

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, "grace",
		sub.
			PaymentStatus,
	)

	if sub.GracePeriodEnd == nil {
		assert.Failf(t, "test failure",

			"GracePeriodEnd is nil")
	} else if !timeClose(*sub.GracePeriodEnd, graceEnd, 5*time.Second) {
		assert.Failf(t, "test failure",

			"GracePeriodEnd = %v, want %v", *sub.GracePeriodEnd, graceEnd)
	}
}

func TestPgStore_ListOrgsInGracePeriod(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgExpired := "org-grace-exp-" + newID()
	orgFuture := "org-grace-fut-" + newID()
	orgOk := "org-grace-ok-" + newID()

	ensureSub(t, ctx, pgStore, orgExpired)
	ensureSub(t, ctx, pgStore, orgFuture)
	ensureSub(t, ctx, pgStore, orgOk)

	past := time.Now().UTC().Add(-48 * time.Hour)
	future := time.Now().UTC().Add(48 * time.Hour)
	require.NoError(t, pgStore.
		UpdatePaymentStatus(ctx, orgExpired,

			"grace",
			&past,
		))
	require.NoError(t, pgStore.
		UpdatePaymentStatus(ctx, orgFuture,

			"grace",
			&future,
		))

	// orgOk stays with payment_status = 'ok'

	subs, err := pgStore.ListOrgsInGracePeriod(ctx)
	require.NoError(t, err)

	found := false
	for _, s := range subs {
		if s.OrgID == orgExpired {
			found = true
		}
		assert.False(t, s.OrgID ==

			orgFuture ||
			s.OrgID ==
				orgOk,
		)

	}
	assert.True(t, found)

}

func TestPgStore_RestrictExpiredGracePeriod_AtomicConditionalUpdate(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgExpired := "org-grace-restrict-" + newID()
	orgRecovered := "org-grace-recovered-" + newID()
	orgFuture := "org-grace-future-" + newID()
	ensureSub(t, ctx, pgStore, orgExpired)
	ensureSub(t, ctx, pgStore, orgRecovered)
	ensureSub(t, ctx, pgStore, orgFuture)

	past := time.Now().UTC().Add(-48 * time.Hour)
	future := time.Now().UTC().Add(48 * time.Hour)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgExpired,

			"pro", "active",
		))
	require.NoError(t, pgStore.
		UpdatePaymentStatus(ctx, orgExpired,

			"grace",
			&past,
		))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgRecovered,

			"pro",
			"active",
		))
	require.NoError(t, pgStore.
		UpdatePaymentStatus(ctx, orgRecovered,

			"ok",
			nil))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgFuture,
			"pro",
			"active",
		))
	require.NoError(t, pgStore.
		UpdatePaymentStatus(ctx, orgFuture,

			"grace",
			&future,
		))

	restricted, err := pgStore.RestrictExpiredGracePeriod(ctx, orgExpired, &past)
	require.NoError(t, err)
	require.True(t, restricted)

	expiredSub, err := pgStore.GetOrgSubscription(ctx, orgExpired)
	require.NoError(t, err)
	require.False(t, expiredSub.
		PlanTier !=
		"free" ||
		expiredSub.
			Status !=
			"restricted" ||
		expiredSub.
			PaymentStatus !=
			"restricted",
	)

	for _, tt := range []struct {
		name     string
		orgID    string
		graceEnd *time.Time
	}{
		{name: "recovered payment", orgID: orgRecovered, graceEnd: &past},
		{name: "future grace", orgID: orgFuture, graceEnd: &future},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changed, err := pgStore.RestrictExpiredGracePeriod(ctx, tt.orgID, tt.graceEnd)
			require.NoError(t, err)
			require.False(t, changed)

			sub, err := pgStore.GetOrgSubscription(ctx, tt.orgID)
			require.NoError(t, err)
			require.False(t, sub.PlanTier !=
				"pro" ||
				sub.
					Status !=
					"active",
			)

		})
	}
}

func TestPgStore_ListStaleSubscriptions(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgStale := "org-stale-" + newID()
	orgFresh := "org-fresh-" + newID()
	orgPending := "org-pending-stale-" + newID()

	ensureSub(t, ctx, pgStore, orgStale)
	ensureSub(t, ctx, pgStore, orgFresh)
	ensureSub(t, ctx, pgStore, orgPending)

	staleEnd := time.Now().UTC().Add(-72 * time.Hour) // > 1 day ago
	freshEnd := time.Now().UTC().Add(24 * time.Hour)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionFull(ctx,
			orgStale,
			"pro",
			"active",

			&staleEnd,
			&staleEnd,
		))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionFull(ctx,
			orgFresh,
			"pro",
			"active",

			&staleEnd,
			&freshEnd,
		))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionFull(ctx,
			orgPending,

			"pro", "active",

			&staleEnd,
			&staleEnd,
		))
	require.NoError(t, pgStore.
		SetPendingPlanTier(
			ctx, orgPending,

			"free"),
	)

	// orgStale: active, period ended > 1 day ago, no pending downgrade

	// orgFresh: active, period not ended

	// orgPending: active, period ended, but HAS pending downgrade (should not be stale)

	subs, err := pgStore.ListStaleSubscriptions(ctx)
	require.NoError(t, err)

	found := false
	for _, s := range subs {
		if s.OrgID == orgStale {
			found = true
		}
		assert.False(t, s.OrgID ==

			orgFresh ||
			s.OrgID ==
				orgPending,
		)

	}
	assert.True(t, found)

}

func TestPgStore_IsProjectSuspended(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	p := createProject(t, ctx, q, "org-susp", "P")

	suspended, err := pgStore.IsProjectSuspended(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, suspended)

	_, err = testDB.Pool.Exec(ctx, "UPDATE projects SET suspended = true WHERE id = $1", p.ID)
	require.NoError(t, err)

	suspended, err = pgStore.IsProjectSuspended(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, suspended)

}

func TestPgStore_SuspendExcessProjects(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-suspexc-" + newID()
	p1 := createProject(t, ctx, q, orgID, "P1")
	p2 := createProject(t, ctx, q, orgID, "P2")
	p3 := createProject(t, ctx, q, orgID, "P3")

	// Stagger created_at so ordering is deterministic.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, _ = testDB.Pool.Exec(ctx, "UPDATE projects SET created_at = $2 WHERE id = $1", p1.ID, base)
	_, _ = testDB.Pool.Exec(ctx, "UPDATE projects SET created_at = $2 WHERE id = $1", p2.ID, base.Add(time.Hour))
	_, _ = testDB.Pool.Exec(ctx, "UPDATE projects SET created_at = $2 WHERE id = $1", p3.ID, base.Add(2*time.Hour))

	// Allow only 1 project.
	count, err := pgStore.SuspendExcessProjects(ctx, orgID, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	// The oldest project (p1) should NOT be suspended.
	s1, _ := pgStore.IsProjectSuspended(ctx, p1.ID)
	s2, _ := pgStore.IsProjectSuspended(ctx, p2.ID)
	s3, _ := pgStore.IsProjectSuspended(ctx, p3.ID)
	assert.False(t, s1)
	assert.True(t, s2)
	assert.True(t, s3)

}

func TestPgStore_ListOrgAdminEmails(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-adminemails-" + newID()
	p := createProject(t, ctx, q, orgID, "P")

	createAdminMember(t, ctx, q, p.ID, "admin-user-1", "admin1@example.com")
	createAdminMember(t, ctx, q, p.ID, "admin-user-2", "admin2@example.com")
	// Create a non-admin member.
	createMember(t, ctx, q, p.ID, "regular-user")

	emails, err := pgStore.ListOrgAdminEmails(ctx, orgID)
	require.NoError(t, err)

	sort.Strings(emails)
	require.Len(t, emails, 2)
	assert.Equal(t, "admin1@example.com",

		emails[0])
	assert.Equal(t, "admin2@example.com",

		emails[1])

}

func TestPgStore_ListOrgAdminEmails_ExcludesUnverifiedEmails(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-adminemails-verified-" + newID()
	p := createProject(t, ctx, q, orgID, "P")

	createAdminMember(t, ctx, q, p.ID, "verified-admin", "verified@example.com")
	createAdminMember(t, ctx, q, p.ID, "unverified-admin", "unverified@example.com")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE users SET email_verified = false WHERE id = $1`, "unverified-admin"); err != nil {
		require.Failf(t, "test failure",

			"mark unverified admin: %v", err)
	}

	emails, err := pgStore.ListOrgAdminEmails(ctx, orgID)
	require.NoError(t, err)
	require.False(t, len(emails) != 1 ||
		emails[0] !=
			"verified@example.com",
	)

}

func TestPgStore_UsageReportDedup(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-usagerpt-" + newID()
	periodEnd := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)

	sent, err := pgStore.HasSentUsageReport(ctx, orgID, periodEnd)
	require.NoError(t, err)
	assert.False(t, sent)
	require.NoError(t, pgStore.
		RecordSentUsageReport(ctx, orgID,
			periodEnd,
		))

	sent, err = pgStore.HasSentUsageReport(ctx, orgID, periodEnd)
	require.NoError(t, err)
	assert.True(t, sent)
	require.NoError(t, pgStore.
		RecordSentUsageReport(ctx, orgID,
			periodEnd,
		))

	// Idempotent second recording.

}

func TestPgStore_ClaimContractReminderSend_DeduplicatesByOrgDateWindow(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-contract-claim-" + newID()
	contractEnd := time.Date(2026, 6, 30, 18, 30, 0, 0, time.UTC)

	claimed, err := pgStore.ClaimContractReminderSend(ctx, orgID, contractEnd, 30)
	require.NoError(t, err)
	require.True(t, claimed)

	claimed, err = pgStore.ClaimContractReminderSend(ctx, orgID, contractEnd.Add(5*time.Hour), 30)
	require.NoError(t, err)
	require.False(t, claimed)

	claimed, err = pgStore.ClaimContractReminderSend(ctx, orgID, contractEnd, 7)
	require.NoError(t, err)
	require.True(t, claimed)

}

func TestPgStore_UpdateMonthlyUsageEmail(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-monthlyemail-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	// Default should be false.
	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.False(t, sub.MonthlyUsageEmail)
	require.NoError(t, pgStore.
		UpdateMonthlyUsageEmail(ctx,
			orgID,
			true))

	// Enable.

	sub, _ = pgStore.GetOrgSubscription(ctx, orgID)
	assert.True(t, sub.MonthlyUsageEmail)
	require.NoError(t, pgStore.
		UpdateMonthlyUsageEmail(ctx,
			orgID,
			false))

	// Disable.

	sub, _ = pgStore.GetOrgSubscription(ctx, orgID)
	assert.False(t, sub.MonthlyUsageEmail)

}

func TestPgStore_ListActiveAddons(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-addons-" + newID()

	a1 := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonConcurrency100,
		Quantity:  5,
		Active:    true,
	}
	a2 := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonEnvironments5,
		Quantity:  10,
		Active:    true,
	}
	aInactive := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonHistory30d,
		Quantity:  1,
		Active:    false,
	}

	for _, a := range []*billing.Addon{a1, a2, aInactive} {
		require.NoError(t, pgStore.
			CreateAddon(ctx, a),
		)

	}

	addons, err := pgStore.ListActiveAddons(ctx, orgID)
	require.NoError(t, err)
	require.Len(t, addons, 2)

}

func TestPgStore_DeactivateAddon(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-deactaddon-" + newID()
	a := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonConcurrency100,
		Quantity:  5,
		Active:    true,
	}
	require.NoError(t, pgStore.
		CreateAddon(ctx, a),
	)
	require.NoError(t, pgStore.
		DeactivateAddon(ctx,
			a.ID))

	addons, err := pgStore.ListActiveAddons(ctx, orgID)
	require.NoError(t, err)
	assert.Len(t, addons, 0)

}

func TestPgStore_CountActiveAddonsByType(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-cntaddons-" + newID()
	for range 3 {
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

	}
	// One inactive.
	aInact := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonConcurrency100,
		Quantity:  1,
		Active:    false,
	}
	require.NoError(t, pgStore.
		CreateAddon(ctx, aInact))

	count, err := pgStore.CountActiveAddonsByType(ctx, orgID, billing.AddonConcurrency100)
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)

	// Different type should be 0.
	count2, err := pgStore.CountActiveAddonsByType(ctx, orgID, billing.AddonEnvironments5)
	require.NoError(t, err)
	assert.EqualValues(t, 0, count2)

}

func TestPgStore_WebhookIdempotency(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	msgID := "msg-" + newID()

	processed, err := pgStore.IsWebhookProcessed(ctx, msgID)
	require.NoError(t, err)
	assert.False(t, processed)
	require.NoError(t, pgStore.
		RecordProcessedWebhook(ctx, msgID))

	processed, err = pgStore.IsWebhookProcessed(ctx, msgID)
	require.NoError(t, err)
	assert.True(t, processed)
	require.NoError(t, pgStore.
		RecordProcessedWebhook(ctx, msgID))

	// Idempotent second recording.

}

func TestPgStore_WebhookProcessingClaimIsAtomic(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	msgID := "msg-claim-" + newID()
	start := make(chan struct{})
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			claimed, err := pgStore.ClaimWebhookForProcessing(ctx, msgID, 10*time.Minute)
			if err != nil {
				errs <- err
				return
			}
			results <- claimed
		})
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.Failf(t, "test failure",

			"ClaimWebhookForProcessing concurrent error: %v", err)
	}
	var claimed, skipped int
	for result := range results {
		if result {
			claimed++
		} else {
			skipped++
		}
	}
	require.False(t, claimed !=
		1 || skipped !=
		1)

	processed, err := pgStore.IsWebhookProcessed(ctx, msgID)
	require.NoError(t, err)
	require.False(t, processed)

	status, err := pgStore.GetWebhookProcessingStatus(ctx, msgID)
	require.NoError(t, err)
	require.Equal(t, "processing",

		status,
	)
	require.NoError(t, pgStore.
		ReleaseWebhookClaim(ctx, msgID))

	status, err = pgStore.GetWebhookProcessingStatus(ctx, msgID)
	require.NoError(t, err)
	require.Equal(t, "", status)

	reclaimed, err := pgStore.ClaimWebhookForProcessing(ctx, msgID, 10*time.Minute)
	require.NoError(t, err)
	require.True(t, reclaimed)
	require.NoError(t, pgStore.
		MarkWebhookProcessed(ctx, msgID))

	status, err = pgStore.GetWebhookProcessingStatus(ctx, msgID)
	require.NoError(t, err)
	require.Equal(t, "processed",

		status,
	)

	again, err := pgStore.ClaimWebhookForProcessing(ctx, msgID, 10*time.Minute)
	require.NoError(t, err)
	require.False(t, again)

}

func TestPgStore_DeleteOldWebhookMessages(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	// Insert some messages with backdated timestamps.
	for i := range 5 {
		msgID := fmt.Sprintf("msg-old-%d-%s", i, newID())
		require.NoError(t, pgStore.
			RecordProcessedWebhook(ctx, msgID))

		// Backdate to 10 days ago.
		_, err := testDB.Pool.Exec(ctx,
			"UPDATE processed_webhook_messages SET processed_at = $2 WHERE msg_id = $1",
			msgID, time.Now().UTC().Add(-10*24*time.Hour))
		require.NoError(t, err)

	}
	// One recent message.
	recentMsg := "msg-recent-" + newID()
	require.NoError(t, pgStore.
		RecordProcessedWebhook(ctx, recentMsg))

	deleted, err := pgStore.DeleteOldWebhookMessages(ctx, time.Now().UTC().Add(-24*time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 5, deleted)

	// Recent message should still exist.
	still, err := pgStore.IsWebhookProcessed(ctx, recentMsg)
	require.NoError(t, err)
	assert.True(t, still)

}

func TestPgStore_DeactivateExcessWebhookSubscriptions(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-deactwh-" + newID()
	p := createProject(t, ctx, q, orgID, "P")

	// Create 5 webhook subscriptions.
	for i := range 5 {
		createWebhookSub(t, ctx, p.ID, fmt.Sprintf("https://example.com/wh%d", i))
	}

	deactivated, err := pgStore.DeactivateExcessWebhookSubscriptions(ctx, orgID, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, deactivated)

	// Verify only 2 active remain.
	var activeCount int
	err = testDB.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM webhook_subscriptions WHERE project_id = $1 AND active = true", p.ID).Scan(&activeCount)
	require.NoError(t, err)
	assert.EqualValues(t, 2, activeCount)

}

func TestPgStore_DeactivateExcessEnvironments(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-deactenv-" + newID()
	p := createProject(t, ctx, q, orgID, "P")

	for i := range 4 {
		createEnvironment(t, ctx, p.ID, fmt.Sprintf("env-%d", i))
	}

	deactivated, err := pgStore.DeactivateExcessEnvironments(ctx, orgID, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 2, deactivated)

	// Verify only 2 remain.
	var remaining int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx, "SELECT COUNT(*) FROM environments WHERE project_id = $1",

		p.ID).Scan(&remaining))
	assert.EqualValues(t, 2, remaining)

}

func TestPgStore_DeactivateExcessCronJobs(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-deactcron-" + newID()
	p := createProject(t, ctx, q, orgID, "P")

	// Create 5 cron jobs (the createJob helper sets cron = "*/5 * * * *").
	for range 5 {
		createJob(t, ctx, q, p.ID)
	}

	deactivated, err := pgStore.DeactivateExcessCronJobs(ctx, orgID, 2)
	require.NoError(t, err)
	assert.Len(t, deactivated,

		3)

}

func TestPgStore_DeactivateExcessCronJobs_IncludesWorkflows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-deactcron-workflows-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	oldJob := createJob(t, ctx, q, p.ID)
	newJob := createJob(t, ctx, q, p.ID)
	oldWorkflow := &domain.Workflow{
		ID:        newID(),
		ProjectID: p.ID,
		Name:      "old scheduled workflow",
		Slug:      "old-scheduled-workflow-" + newID(),
		Enabled:   true,
		Cron:      "0 * * * *",
		Version:   1,
	}
	require.NoError(t, q.CreateWorkflow(ctx, oldWorkflow))

	newWorkflow := &domain.Workflow{
		ID:        newID(),
		ProjectID: p.ID,
		Name:      "new scheduled workflow",
		Slug:      "new-scheduled-workflow-" + newID(),
		Enabled:   true,
		Cron:      "15 * * * *",
		Version:   1,
	}
	require.NoError(t, q.CreateWorkflow(ctx, newWorkflow))

	updates := []struct {
		table string
		id    string
		at    time.Time
	}{
		{table: "jobs", id: oldJob.ID, at: base},
		{table: "workflows", id: oldWorkflow.ID, at: base.Add(time.Hour)},
		{table: "jobs", id: newJob.ID, at: base.Add(2 * time.Hour)},
		{table: "workflows", id: newWorkflow.ID, at: base.Add(3 * time.Hour)},
	}
	for _, update := range updates {
		if _, err := testDB.Pool.Exec(ctx, "UPDATE "+update.table+" SET updated_at = $2 WHERE id = $1", update.id, update.at); err != nil {
			require.Failf(t, "test failure",

				"set %s updated_at for %s: %v", update.table, update.id, err)
		}
	}

	deactivated, err := pgStore.DeactivateExcessCronJobs(ctx, orgID, 2)
	require.NoError(t, err)
	require.Len(t, deactivated,

		2)

	assertCronCleared := func(table, id string, wantCleared bool) {
		t.Helper()
		var cron string
		require.NoError(t, testDB.
			Pool.QueryRow(ctx, "SELECT COALESCE(cron, '') FROM "+
			table+
			" WHERE id = $1",

			id).Scan(&cron))
		require.Equal(t, wantCleared,

			cron ==
				"")

	}
	assertCronCleared("jobs", oldJob.ID, true)
	assertCronCleared("workflows", oldWorkflow.ID, true)
	assertCronCleared("jobs", newJob.ID, false)
	assertCronCleared("workflows", newWorkflow.ID, false)
}

func TestPgStore_CountMembersAndExecutingRunsByOrg(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustQueries(t)
	pgStore := billing.NewPgStore(testDB.Pool)

	projectA := createProject(t, ctx, q, "org-members", "Project A")
	projectB := createProject(t, ctx, q, "org-members", "Project B")
	projectOther := createProject(t, ctx, q, "org-other", "Project Other")

	createMember(t, ctx, q, projectA.ID, "user-1")
	createMember(t, ctx, q, projectB.ID, "user-1")
	createMember(t, ctx, q, projectB.ID, "user-2")
	createMember(t, ctx, q, projectOther.ID, "user-3")

	members, err := pgStore.CountMembersByOrg(ctx, "org-members")
	require.NoError(t, err)
	require.EqualValues(t, 2, members)

	jobA := createJob(t, ctx, q, projectA.ID)
	jobB := createJob(t, ctx, q, projectB.ID)
	jobOther := createJob(t, ctx, q, projectOther.ID)

	_ = createRun(t, ctx, q, jobA, domain.StatusExecuting)
	_ = createRun(t, ctx, q, jobA, domain.StatusCompleted)
	_ = createRun(t, ctx, q, jobB, domain.StatusExecuting)
	_ = createRun(t, ctx, q, jobOther, domain.StatusExecuting)

	executing, err := pgStore.CountExecutingRunsByOrg(ctx, "org-members")
	require.NoError(t, err)
	require.EqualValues(t, 2, executing)

}

// Enterprise contract integration tests

func makeContract(orgID string, tier billing.EnterpriseTier, endDate time.Time) *billing.EnterpriseContract {
	subID := "sub_" + orgID
	return &billing.EnterpriseContract{
		ID:                    "contract_" + orgID,
		OrgID:                 orgID,
		EnterpriseTier:        tier,
		AnnualCommitmentCents: 1800000,
		OverageDiscountPct:    10,
		ContractStartDate:     time.Now().Add(-180 * 24 * time.Hour),
		ContractEndDate:       endDate,
		AutoRenew:             true,
		BillingCadence:        "annual",
		StripeSubscriptionID:  &subID,
		Notes:                 "test contract",
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
}

func TestPgStore_UpsertAndGetEnterpriseContract(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ent-" + newID()
	c := makeContract(orgID, billing.EnterpriseTierStarter, time.Now().Add(180*24*time.Hour))
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			c))

	got, err := pgStore.GetEnterpriseContract(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, orgID, got.
		OrgID)
	assert.Equal(t, billing.EnterpriseTierStarter,

		got.EnterpriseTier,
	)
	assert.EqualValues(t, 1800000,
		got.
			AnnualCommitmentCents,
	)
	assert.EqualValues(t, 10, got.OverageDiscountPct)
	assert.True(t, got.AutoRenew)
	assert.Equal(t, "annual",

		got.BillingCadence,
	)
	assert.False(t, got.StripeSubscriptionID ==
		nil ||
		*got.
			StripeSubscriptionID !=
			"sub_"+
				orgID,
	)
	assert.Equal(t, "test contract",

		got.
			Notes)

}

func TestPgStore_GetEnterpriseContract_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	_, err := pgStore.GetEnterpriseContract(ctx, "org-nonexistent-"+newID())
	require.True(t, errors.Is(
		err, billing.
			ErrContractNotFound,
	))

}

func TestPgStore_UpsertEnterpriseContract_UpdatesExisting(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-upsert-" + newID()
	c1 := makeContract(orgID, billing.EnterpriseTierStarter, time.Now().Add(180*24*time.Hour))
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			c1))

	// Upsert again with different tier and discount.
	c2 := makeContract(orgID, billing.EnterpriseTierGrowth, time.Now().Add(365*24*time.Hour))
	c2.OverageDiscountPct = 15
	c2.AnnualCommitmentCents = 4800000
	c2.Notes = "upgraded"
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			c2))

	got, err := pgStore.GetEnterpriseContract(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, billing.EnterpriseTierGrowth,

		got.EnterpriseTier,
	)
	assert.EqualValues(t, 15, got.OverageDiscountPct)
	assert.EqualValues(t, 4800000,
		got.
			AnnualCommitmentCents,
	)
	assert.Equal(t, "upgraded",

		got.Notes,
	)

}

func TestPgStore_ListExpiringContracts(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	// Contract expiring in 5 days.
	org5d := "org-5d-" + newID()
	c5 := makeContract(org5d, billing.EnterpriseTierStarter, time.Now().Add(5*24*time.Hour))
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			c5))

	// Contract expiring in 25 days.
	org25d := "org-25d-" + newID()
	c25 := makeContract(org25d, billing.EnterpriseTierGrowth, time.Now().Add(25*24*time.Hour))
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			c25))

	// Contract expiring in 60 days (should not be in 30-day list).
	org60d := "org-60d-" + newID()
	c60 := makeContract(org60d, billing.EnterpriseTierLarge, time.Now().Add(60*24*time.Hour))
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			c60))

	// Already expired (should not appear).
	orgExpired := "org-expired-" + newID()
	cExpired := makeContract(orgExpired, billing.EnterpriseTierStarter, time.Now().Add(-1*24*time.Hour))
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			cExpired,
		))

	// List contracts expiring within 7 days.
	within7, err := pgStore.ListExpiringContracts(ctx, 7)
	require.NoError(t, err)
	require.Len(t, within7, 1)
	assert.Equal(t, org5d, within7[0].OrgID)

	// List contracts expiring within 30 days.
	within30, err := pgStore.ListExpiringContracts(ctx, 30)
	require.NoError(t, err)
	require.Len(t, within30,
		2,
	)
	assert.Equal(t, org5d, within30[0].
		OrgID)
	assert.Equal(t, org25d, within30[1].
		OrgID)

	// Should be ordered by end date ASC.

	// List within 0 days (edge: nothing expiring today or before since already-expired excluded).
	within0, err := pgStore.ListExpiringContracts(ctx, 0)
	require.NoError(t, err)
	require.Len(t, within0, 0)

}

func TestPgStore_ListExpiredContracts(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgExpired := "org-expired-contract-" + newID()
	expired := makeContract(orgExpired, billing.EnterpriseTierStarter, time.Now().Add(-time.Hour))
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			expired,
		))

	orgFuture := "org-future-contract-" + newID()
	future := makeContract(orgFuture, billing.EnterpriseTierGrowth, time.Now().Add(24*time.Hour))
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			future,
		))

	got, err := pgStore.ListExpiredContracts(ctx)
	require.NoError(t, err)
	require.False(t, len(got) !=
		1 || got[0].OrgID !=
		orgExpired,
	)

}

func TestPgStore_RestrictExpiredContractIfCurrentSkipsRenewedContract(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-expired-renewed-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	observedEnd := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	contract := makeContract(orgID, billing.EnterpriseTierStarter, observedEnd)
	contract.AutoRenew = false
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			contract,
		))

	renewed := *contract
	renewed.ContractEndDate = time.Now().UTC().Add(365 * 24 * time.Hour).Truncate(time.Microsecond)
	renewed.AutoRenew = true
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			&renewed,
		))

	restricted, err := pgStore.RestrictExpiredContractIfCurrent(ctx, orgID, observedEnd)
	require.NoError(t, err)
	require.False(t, restricted)

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	require.NotEqual(t, "restricted",

		sub.
			PaymentStatus,
	)

}

func TestPgStore_RestrictExpiredContractIfCurrentClearsEnterpriseEntitlements(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-expired-entitlements-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID,
			string(domain.
				PlanEnterprise,
			), "active",
		))

	mustEqualLimits(t, readEntitlements(t, ctx, orgID), billing.GetPlanLimits(domain.PlanEnterprise), "before expired contract restriction")

	observedEnd := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	contract := makeContract(orgID, billing.EnterpriseTierStarter, observedEnd)
	contract.AutoRenew = false
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			contract,
		))

	restricted, err := pgStore.RestrictExpiredContractIfCurrent(ctx, orgID, observedEnd)
	require.NoError(t, err)
	require.True(t, restricted)

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	require.Equal(t, "restricted",

		sub.
			PaymentStatus,
	)

	mustEqualLimits(t, readEntitlements(t, ctx, orgID), billing.GetPlanLimits(domain.PlanFree), "after expired contract restriction")
}

func TestPgStore_EnterpriseContract_CrossOrgIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgA := "org-iso-a-" + newID()
	orgB := "org-iso-b-" + newID()

	cA := makeContract(orgA, billing.EnterpriseTierStarter, time.Now().Add(365*24*time.Hour))
	cA.Notes = "org A contract"
	cB := makeContract(orgB, billing.EnterpriseTierLarge, time.Now().Add(365*24*time.Hour))
	cB.Notes = "org B contract"
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			cA))
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			cB))

	gotA, err := pgStore.GetEnterpriseContract(ctx, orgA)
	require.NoError(t, err)
	assert.Equal(t, billing.EnterpriseTierStarter,

		gotA.EnterpriseTier,
	)
	assert.Equal(t, "org A contract",

		gotA.
			Notes)

	gotB, err := pgStore.GetEnterpriseContract(ctx, orgB)
	require.NoError(t, err)
	assert.Equal(t, billing.EnterpriseTierLarge,

		gotB.
			EnterpriseTier,
	)

}

func TestPgStore_UpsertEnterpriseContract_NilStripeSubscriptionID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-nilsub-" + newID()
	c := makeContract(orgID, billing.EnterpriseTierStarter, time.Now().Add(365*24*time.Hour))
	c.StripeSubscriptionID = nil
	require.NoError(t, pgStore.
		UpsertEnterpriseContract(ctx,
			c))

	got, err := pgStore.GetEnterpriseContract(ctx, orgID)
	require.NoError(t, err)
	assert.Nil(t, got.
		StripeSubscriptionID,
	)

}

// HTTP job downgrade lifecycle integration tests

func createHTTPJob(t *testing.T, ctx context.Context, q *store.Queries, projectID string) *domain.Job {
	t.Helper()
	job := &domain.Job{
		ID:            newID(),
		ProjectID:     projectID,
		Name:          "http-job-" + newID(),
		Slug:          "http-slug-" + newID(),
		EndpointURL:   "https://example.com/http",
		MaxAttempts:   3,
		TimeoutSecs:   60,
		Enabled:       true,
		ExecutionMode: domain.ExecutionModeHTTP,
	}
	require.NoError(t, q.CreateJob(ctx,
		job))

	return job
}

func TestPgStore_PauseHTTPJobsByOrg(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-http-pause-" + newID()
	p := createProject(t, ctx, q, orgID, "P1")

	createHTTPJob(t, ctx, q, p.ID)
	createHTTPJob(t, ctx, q, p.ID)
	createHTTPJob(t, ctx, q, p.ID)

	paused, err := pgStore.PauseHTTPJobsByOrg(ctx, orgID, "plan_downgrade")
	require.NoError(t, err)
	require.Len(t, paused, 3)

	// Count HTTP jobs (should still be 3 -- paused but not deleted).
	count, err := pgStore.CountHTTPJobsByOrg(ctx, orgID)
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)

}

func TestPgStore_PauseHTTPJobsByOrg_AlreadyPaused(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-already-paused-" + newID()
	p := createProject(t, ctx, q, orgID, "P1")

	// Create 3 HTTP jobs, manually pause one.
	createHTTPJob(t, ctx, q, p.ID)
	manualPaused := createHTTPJob(t, ctx, q, p.ID)
	createHTTPJob(t, ctx, q, p.ID)
	require.NoError(t, q.PauseJob(ctx,
		manualPaused.
			ID, "user_request",
	))

	// Bulk pause should only affect 2 (skip already-paused).
	paused, err := pgStore.PauseHTTPJobsByOrg(ctx, orgID, "plan_downgrade")
	require.NoError(t, err)
	require.Len(t, paused, 2)

}

func TestPgStore_UnpauseJobsByPauseReason(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-unpause-" + newID()
	p := createProject(t, ctx, q, orgID, "P1")

	// Create 3 HTTP jobs, pause all with "plan_downgrade".
	createHTTPJob(t, ctx, q, p.ID)
	createHTTPJob(t, ctx, q, p.ID)
	manualJob := createHTTPJob(t, ctx, q, p.ID)

	// Pause 2 with plan_downgrade via bulk.
	paused, err := pgStore.PauseHTTPJobsByOrg(ctx, orgID, "plan_downgrade")
	require.NoError(t, err)
	require.Len(t, paused, 3)

	// Manually change one job's pause reason to simulate user-initiated pause.
	_, err = testDB.Pool.Exec(ctx, "UPDATE jobs SET pause_reason = 'user_request' WHERE id = $1", manualJob.ID)
	require.NoError(t, err)

	// Unpause only "plan_downgrade" jobs.
	unpaused, err := pgStore.UnpauseJobsByPauseReason(ctx, orgID, "plan_downgrade")
	require.NoError(t, err)
	require.EqualValues(t, 2, unpaused)

	// The user-paused job should still be paused.
	got, err := q.GetJob(ctx, manualJob.ID)
	require.NoError(t, err)
	assert.True(t, got.Paused)
	assert.Equal(t, "user_request",

		got.
			PauseReason,
	)

}

func TestPgStore_CountHTTPJobsByOrg_CrossOrgIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgA := "org-iso-a-" + newID()
	orgB := "org-iso-b-" + newID()
	pA := createProject(t, ctx, q, orgA, "PA")
	pB := createProject(t, ctx, q, orgB, "PB")

	createHTTPJob(t, ctx, q, pA.ID)
	createHTTPJob(t, ctx, q, pA.ID)
	createHTTPJob(t, ctx, q, pB.ID)

	countA, _ := pgStore.CountHTTPJobsByOrg(ctx, orgA)
	countB, _ := pgStore.CountHTTPJobsByOrg(ctx, orgB)
	assert.EqualValues(t, 2, countA)
	assert.EqualValues(t, 1, countB)

}

func TestPgStore_HTTPDowngradeLifecycle_FullCycle(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-lifecycle-" + newID()
	p := createProject(t, ctx, q, orgID, "P1")

	h1 := createHTTPJob(t, ctx, q, p.ID)
	createHTTPJob(t, ctx, q, p.ID)
	createHTTPJob(t, ctx, q, p.ID)

	// Simulate downgrade enforcement by pausing HTTP jobs.
	paused, err := pgStore.PauseHTTPJobsByOrg(ctx, orgID, "plan_downgrade")
	require.NoError(t, err)
	require.Len(t, paused, 3)

	got, _ := q.GetJob(ctx, h1.ID)
	assert.True(t, got.Paused)
	assert.Equal(t, "plan_downgrade",

		got.
			PauseReason,
	)

	// Simulate upgrade enforcement restoring jobs paused for downgrade.
	unpaused, err := pgStore.UnpauseJobsByPauseReason(ctx, orgID, "plan_downgrade")
	require.NoError(t, err)
	require.EqualValues(t, 3, unpaused)

	got2, _ := q.GetJob(ctx, h1.ID)
	assert.False(t, got2.Paused)
	assert.Equal(t, "", got2.
		PauseReason,
	)

}

// H1 regression: ListOrgsInGracePeriod must include MonthlyUsageEmail

func TestPgStore_ListOrgsInGracePeriod_MonthlyUsageEmail(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-grace-email-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		UpdateMonthlyUsageEmail(ctx,
			orgID,
			true))

	past := time.Now().UTC().Add(-48 * time.Hour)
	require.NoError(t, pgStore.
		UpdatePaymentStatus(ctx, orgID,
			"grace",
			&past,
		))

	subs, err := pgStore.ListOrgsInGracePeriod(ctx)
	require.NoError(t, err)

	for _, s := range subs {
		if s.OrgID == orgID {
			require.True(t, s.MonthlyUsageEmail)

			return
		}
	}
	require.Fail(t,

		"org not found in grace period list")
}

// H1 regression: ListStaleSubscriptions must include MonthlyUsageEmail

func TestPgStore_ListStaleSubscriptions_MonthlyUsageEmail(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-stale-email-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		UpdateMonthlyUsageEmail(ctx,
			orgID,
			true))

	staleEnd := time.Now().UTC().Add(-72 * time.Hour)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionFull(ctx,
			orgID,
			"pro",
			"active",
			&staleEnd,

			&staleEnd,
		))

	subs, err := pgStore.ListStaleSubscriptions(ctx)
	require.NoError(t, err)

	for _, s := range subs {
		if s.OrgID == orgID {
			require.True(t, s.MonthlyUsageEmail)

			return
		}
	}
	require.Fail(t,

		"org not found in stale subscriptions list")
}
func TestPgStore_UpsertOrgSubscription_PreservesPendingPlanTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ppt-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		SetPendingPlanTier(
			ctx, orgID,
			"free",
		))

	sub := &billing.OrgSubscription{
		ID:          newID(),
		OrgID:       orgID,
		PlanTier:    "pro",
		Status:      "active",
		LimitAction: "notify",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	require.NoError(t, pgStore.
		UpsertOrgSubscription(ctx, sub))

	got, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.False(t, got.PendingPlanTier ==
		nil ||
		*got.PendingPlanTier !=
			"free")

}

// M2: UpdateOrgSubscriptionFull with nil period preserves existing

func TestPgStore_UpdateOrgSubscriptionFull_NilPeriodPreservesExisting(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-nilperiod-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionFull(ctx,
			orgID,
			"pro",
			"active",
			&start,
			&end,
		))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionFull(ctx,
			orgID,
			"enterprise",

			"active",
			nil,
			nil))

	got, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.False(t, got.CurrentPeriodStart ==
		nil ||
		!got.CurrentPeriodStart.
			Equal(start))
	assert.False(t, got.CurrentPeriodEnd ==
		nil ||
		!got.CurrentPeriodEnd.
			Equal(end))

}

// M3: BulkCountExecutingRunsByOrg -- zero-run org absent from map

func TestPgStore_BulkCountExecutingRunsByOrg_ZeroRunOrgAbsent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-zero-runs-" + newID()
	result, err := pgStore.BulkCountExecutingRunsByOrg(ctx, []string{orgID})
	require.NoError(t, err)

	if count, present := result[orgID]; present {
		assert.Failf(t, "test failure",

			"expected org absent from map, got count=%d", count)
	}
}

// M4: ListOrgsWithPendingDowngrade near boundary

func TestPgStore_ListOrgsWithPendingDowngrade_NearBoundary(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgPast := "org-pd-past-" + newID()
	orgFuture := "org-pd-future-" + newID()

	ensureSub(t, ctx, pgStore, orgPast)
	ensureSub(t, ctx, pgStore, orgFuture)

	past := time.Now().UTC().Add(-1 * time.Second)
	future := time.Now().UTC().Add(1 * time.Hour)
	require.NoError(t, pgStore.
		SetPendingDowngrade(ctx, orgPast,
			"free",
			&past, &past,
		))
	require.NoError(t, pgStore.
		SetPendingDowngrade(ctx, orgFuture,

			"free",
			&future,
			&future,
		))

	subs, err := pgStore.ListOrgsWithPendingDowngrade(ctx)
	require.NoError(t, err)

	var foundPast, foundFuture bool
	for _, s := range subs {
		if s.OrgID == orgPast {
			foundPast = true
		}
		if s.OrgID == orgFuture {
			foundFuture = true
		}
	}
	assert.True(t, foundPast)
	assert.False(t, foundFuture)

}

// M5: ListStaleSubscriptions one-day boundary

func TestPgStore_ListStaleSubscriptions_OneDayBoundary(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgNotStale := "org-notstale-" + newID()
	orgStale := "org-stale-" + newID()

	ensureSub(t, ctx, pgStore, orgNotStale)
	ensureSub(t, ctx, pgStore, orgStale)

	notStaleEnd := time.Now().UTC().Add(-23 * time.Hour)
	staleEnd := time.Now().UTC().Add(-25 * time.Hour)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionFull(ctx,
			orgNotStale,

			"pro",
			"active",
			&notStaleEnd,

			&notStaleEnd,
		))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionFull(ctx,
			orgStale,
			"pro",
			"active",

			&staleEnd,
			&staleEnd,
		))

	subs, err := pgStore.ListStaleSubscriptions(ctx)
	require.NoError(t, err)

	var foundNotStale, foundStale bool
	for _, s := range subs {
		if s.OrgID == orgNotStale {
			foundNotStale = true
		}
		if s.OrgID == orgStale {
			foundStale = true
		}
	}
	assert.True(t, foundStale)
	assert.False(t, foundNotStale)

}

// M6: GetProjectOrgID not found

func TestPgStore_GetProjectOrgID_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	_, err := pgStore.GetProjectOrgID(ctx, "nonexistent-project-"+newID())
	assert.Error(t, err)

}
func TestPgStore_ListOrgAdminEmails_DedupsAcrossProjects(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-dedup-email-" + newID()
	userID := "user-admin-" + newID()
	email := fmt.Sprintf("admin-%s@example.com", newID())

	p1 := createProject(t, ctx, q, orgID, "P1")
	p2 := createProject(t, ctx, q, orgID, "P2")

	createAdminMember(t, ctx, q, p1.ID, userID, email)
	createAdminMember(t, ctx, q, p2.ID, userID, email)

	emails, err := pgStore.ListOrgAdminEmails(ctx, orgID)
	require.NoError(t, err)
	assert.Len(t, emails, 1)

}

// M9: GetOrgUsageForPeriod single day (from == to)

func TestPgStore_GetOrgUsageForPeriod_SingleDay(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-singleday-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	job := createJob(t, ctx, q, p.ID)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)

	run1 := createRun(t, ctx, q, job, domain.StatusCompleted)
	_, _ = testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET created_at = $2 WHERE id = $1", run1.ID, today.Add(6*time.Hour))

	run2 := createRun(t, ctx, q, job, domain.StatusCompleted)
	_, _ = testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET created_at = $2 WHERE id = $1", run2.ID, tomorrow.Add(6*time.Hour))

	recs, err := pgStore.GetOrgUsageForPeriod(ctx, orgID, today, today)
	require.NoError(t, err)

	var totalRuns int64
	for _, r := range recs {
		totalRuns += r.RunsCount
	}
	assert.EqualValues(t, 1, totalRuns)

}

// L1: ApplyPendingDowngrade same tier (noop)

func TestPgStore_ApplyPendingDowngrade_SameTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-same-tier-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		SetPendingPlanTier(
			ctx, orgID,
			"free",
		))
	require.NoError(t, pgStore.
		ApplyPendingDowngrade(ctx, orgID))

	got, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, "free", got.
		PlanTier,
	)
	assert.Nil(t, got.
		PendingPlanTier,
	)

}

// L2: Usage report dedup -- time truncation

func TestPgStore_UsageReportDedup_TimeTruncation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-dedup-trunc-" + newID()

	endOfDay := time.Date(2026, 3, 15, 23, 59, 59, 0, time.UTC)
	require.NoError(t, pgStore.
		RecordSentUsageReport(ctx, orgID,
			endOfDay,
		),
	)

	startOfDay := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	sent, err := pgStore.HasSentUsageReport(ctx, orgID, startOfDay)
	require.NoError(t, err)
	assert.True(t, sent)

}

func TestPgStore_UsageReportClaimState_AllowsStaleClaimRetry(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-report-claim-" + newID()
	periodEnd := time.Date(2026, 4, 30, 23, 0, 0, 0, time.UTC)

	claimed, err := pgStore.ClaimUsageReportSend(ctx, orgID, periodEnd)
	require.NoError(t, err)
	require.True(t, claimed)

	sent, err := pgStore.HasSentUsageReport(ctx, orgID, periodEnd)
	require.NoError(t, err)
	require.False(t, sent)

	claimed, err = pgStore.ClaimUsageReportSend(ctx, orgID, periodEnd)
	require.NoError(t, err)
	require.False(t, claimed)

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE sent_usage_reports
		SET claimed_at = NOW() - INTERVAL '2 hours'
		WHERE org_id = $1 AND period_end = $2
	`, orgID, periodEnd.Truncate(24*time.Hour)); err != nil {
		require.Failf(t, "test failure",

			"age claim: %v", err)
	}

	claimed, err = pgStore.ClaimUsageReportSend(ctx, orgID, periodEnd)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, pgStore.
		FinalizeUsageReportSend(ctx,
			orgID,
			periodEnd,
		))

	sent, err = pgStore.HasSentUsageReport(ctx, orgID, periodEnd)
	require.NoError(t, err)
	require.True(t, sent)

	claimed, err = pgStore.ClaimUsageReportSend(ctx, orgID, periodEnd)
	require.NoError(t, err)
	require.False(t, claimed)

}

// L3: CountMembersByOrg includes soft-deleted project members

func TestPgStore_CountMembersByOrg_IncludesDeletedProjectMembers(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-delproj-members-" + newID()
	userID := "user-del-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	createMember(t, ctx, q, p.ID, userID)

	_, _ = testDB.Pool.Exec(ctx, "UPDATE projects SET deleted_at = NOW() WHERE id = $1", p.ID)

	count, err := pgStore.CountMembersByOrg(ctx, orgID)
	require.NoError(t, err)

	// Document behavior: members in soft-deleted projects are still counted.
	t.Logf("CountMembersByOrg with deleted project: %d (documents current behavior)", count)
	assert.GreaterOrEqual(t,
		count,
		1)

}

// L4: DeleteOldWebhookMessages with future cutoff

func TestPgStore_DeleteOldWebhookMessages_NoRows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	msgID := "msg-future-" + newID()
	require.NoError(t, pgStore.
		RecordProcessedWebhook(ctx, msgID))

	future := time.Now().UTC().Add(-24 * time.Hour)
	deleted, err := pgStore.DeleteOldWebhookMessages(ctx, future)
	require.NoError(t, err)
	assert.EqualValues(t, 0, deleted)

}

// L6: SuspendExcessProjects with maxProjects=0 suspends all

func TestPgStore_SuspendExcessProjects_ZeroMax(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-zero-max-" + newID()
	p1 := createProject(t, ctx, q, orgID, "P1")
	p2 := createProject(t, ctx, q, orgID, "P2")

	count, err := pgStore.SuspendExcessProjects(ctx, orgID, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	s1, _ := pgStore.IsProjectSuspended(ctx, p1.ID)
	s2, _ := pgStore.IsProjectSuspended(ctx, p2.ID)
	assert.False(t, !s1 || !s2)

}

// L7: UpsertOrgSubscription preserves spending_limit

func TestPgStore_UpsertOrgSubscription_PreservesSpendingLimit(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-limit-preserve-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	require.NoError(t, pgStore.
		UpdateSpendingLimit(ctx, orgID,
			5_000_000,

			"enforce",
		))

	sub := &billing.OrgSubscription{
		ID:                    newID(),
		OrgID:                 orgID,
		PlanTier:              "enterprise",
		Status:                "active",
		SpendingLimitMicrousd: 999,
		LimitAction:           "notify",
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}
	require.NoError(t, pgStore.
		UpsertOrgSubscription(ctx, sub))

	got, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.EqualValues(t, 5_000_000,

		got.SpendingLimitMicrousd,
	)

}

// L8: IsProjectSuspended non-existent project

func TestPgStore_IsProjectSuspended_NonExistent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	_, err := pgStore.IsProjectSuspended(ctx, "nonexistent-"+newID())
	assert.Error(t, err)

}
