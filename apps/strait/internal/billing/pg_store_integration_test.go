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

	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}

	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()

	// Clean users table separately since it's not in the standard CleanTables.
	if _, err := testDB.Pool.Exec(ctx, "DELETE FROM users"); err != nil {
		t.Fatalf("clean users: %v", err)
	}

	if err := testDB.CleanTables(ctx); err != nil {
		t.Fatalf("clean tables: %v", err)
	}
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
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
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
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
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
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
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
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	member := &domain.ProjectMemberRole{
		ID:        newID(),
		ProjectID: projectID,
		UserID:    userID,
		RoleID:    role.ID,
		GrantedBy: "tester",
	}
	if err := q.AssignMemberRole(ctx, member); err != nil {
		t.Fatalf("AssignMemberRole() error = %v", err)
	}
}

func createAdminMember(t *testing.T, ctx context.Context, q *store.Queries, projectID, userID, email string) {
	t.Helper()
	_, err := testDB.Pool.Exec(ctx,
		"INSERT INTO users (id, name, email, email_verified) VALUES ($1, $2, $3, true) ON CONFLICT (id) DO NOTHING",
		userID, "Test User "+userID, email)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

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
		if err := q.CreateProjectRole(ctx, role); err != nil {
			t.Fatalf("CreateProjectRole: %v", err)
		}
		roleID = role.ID
	}

	member := &domain.ProjectMemberRole{
		ID:        newID(),
		ProjectID: projectID,
		UserID:    userID,
		RoleID:    roleID,
		GrantedBy: "test",
	}
	if err := q.AssignMemberRole(ctx, member); err != nil {
		t.Fatalf("AssignMemberRole: %v", err)
	}
}

func createWebhookSub(t *testing.T, ctx context.Context, projectID, url string) string {
	t.Helper()
	id := newID()
	_, err := testDB.Pool.Exec(ctx,
		"INSERT INTO webhook_subscriptions (id, project_id, webhook_url, event_types, secret, active) VALUES ($1, $2, $3, $4, $5, true)",
		id, projectID, url, "{run.completed}", "secret")
	if err != nil {
		t.Fatalf("create webhook sub: %v", err)
	}
	return id
}

func createEnvironment(t *testing.T, ctx context.Context, projectID, name string) string {
	t.Helper()
	id := newID()
	slug := "slug-" + id
	_, err := testDB.Pool.Exec(ctx,
		"INSERT INTO environments (id, project_id, name, slug) VALUES ($1, $2, $3, $4)",
		id, projectID, name, slug)
	if err != nil {
		t.Fatalf("create environment: %v", err)
	}
	return id
}

func ensureSub(t *testing.T, ctx context.Context, pgStore *billing.PgStore, orgID string) {
	t.Helper()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure sub: %v", err)
	}
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

	// First call creates a free subscription.
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("first EnsureOrgSubscription error = %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if sub.PlanTier != "free" {
		t.Errorf("plan_tier = %q, want %q", sub.PlanTier, "free")
	}
	if sub.Status != "active" {
		t.Errorf("status = %q, want %q", sub.Status, "active")
	}

	// Second call is idempotent.
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("second EnsureOrgSubscription error = %v", err)
	}
	sub2, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription second error = %v", err)
	}
	if sub2.ID != sub.ID {
		t.Errorf("ID changed after second ensure: %q vs %q", sub2.ID, sub.ID)
	}
}

func TestPgStore_OrgSubscriptionCacheVersionRoundTripAndBump(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-cache-version-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription() error = %v", err)
	}
	if sub.CacheVersion <= 0 {
		t.Fatalf("initial CacheVersion = %d, want positive", sub.CacheVersion)
	}

	initial := sub.CacheVersion
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan() error = %v", err)
	}

	updated, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription(after update) error = %v", err)
	}
	if updated.CacheVersion <= initial {
		t.Fatalf("updated CacheVersion = %d, want > %d", updated.CacheVersion, initial)
	}
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
	if err != nil {
		t.Fatalf("update subscription: %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}

	// Field 1: ID
	if sub.ID == "" {
		t.Errorf("ID is empty")
	}
	// Field 2: OrgID
	if sub.OrgID != orgID {
		t.Errorf("OrgID = %q, want %q", sub.OrgID, orgID)
	}
	// Field 3: PlanTier
	if sub.PlanTier != "pro" {
		t.Errorf("PlanTier = %q, want %q", sub.PlanTier, "pro")
	}
	// Field 4: StripeSubscriptionID
	if sub.StripeSubscriptionID == nil || *sub.StripeSubscriptionID != "stripe-sub-123" {
		t.Errorf("StripeSubscriptionID = %v, want %q", sub.StripeSubscriptionID, "stripe-sub-123")
	}
	// Field 5: StripeCustomerID
	if sub.StripeCustomerID == nil || *sub.StripeCustomerID != "stripe-cust-456" {
		t.Errorf("StripeCustomerID = %v, want %q", sub.StripeCustomerID, "stripe-cust-456")
	}
	// Field 6: Status
	if sub.Status != "active" {
		t.Errorf("Status = %q, want %q", sub.Status, "active")
	}
	// Field 7: CurrentPeriodStart
	if sub.CurrentPeriodStart == nil {
		t.Errorf("CurrentPeriodStart is nil")
	} else if !timeClose(*sub.CurrentPeriodStart, periodStart, 5*time.Second) {
		t.Errorf("CurrentPeriodStart = %v, want close to %v", *sub.CurrentPeriodStart, periodStart)
	}
	// Field 8: CurrentPeriodEnd
	if sub.CurrentPeriodEnd == nil {
		t.Errorf("CurrentPeriodEnd is nil")
	} else if !timeClose(*sub.CurrentPeriodEnd, periodEnd, 5*time.Second) {
		t.Errorf("CurrentPeriodEnd = %v, want close to %v", *sub.CurrentPeriodEnd, periodEnd)
	}
	// Field 9: SpendingLimitMicrousd
	if sub.SpendingLimitMicrousd != 50000000 {
		t.Errorf("SpendingLimitMicrousd = %d, want %d", sub.SpendingLimitMicrousd, 50000000)
	}
	// Field 10: LimitAction
	if sub.LimitAction != "suspend" {
		t.Errorf("LimitAction = %q, want %q", sub.LimitAction, "suspend")
	}
	// Field 11: PendingPlanTier
	if sub.PendingPlanTier == nil || *sub.PendingPlanTier != "free" {
		t.Errorf("PendingPlanTier = %v, want %q", sub.PendingPlanTier, "free")
	}
	// Field 12: CanceledAt
	if sub.CanceledAt == nil {
		t.Errorf("CanceledAt is nil")
	} else if !timeClose(*sub.CanceledAt, canceledAt, 5*time.Second) {
		t.Errorf("CanceledAt = %v, want close to %v", *sub.CanceledAt, canceledAt)
	}
	// Field 13: AnomalyThresholdWarning
	if math.Abs(sub.AnomalyThresholdWarning-2.5) > 0.01 {
		t.Errorf("AnomalyThresholdWarning = %f, want 2.5", sub.AnomalyThresholdWarning)
	}
	// Field 14: AnomalyThresholdCritical
	if math.Abs(sub.AnomalyThresholdCritical-8.0) > 0.01 {
		t.Errorf("AnomalyThresholdCritical = %f, want 8.0", sub.AnomalyThresholdCritical)
	}
	// Field 15: GracePeriodEnd
	if sub.GracePeriodEnd == nil {
		t.Errorf("GracePeriodEnd is nil")
	} else if !timeClose(*sub.GracePeriodEnd, graceEnd, 5*time.Second) {
		t.Errorf("GracePeriodEnd = %v, want close to %v", *sub.GracePeriodEnd, graceEnd)
	}
	// Field 16: PaymentStatus
	if sub.PaymentStatus != "grace" {
		t.Errorf("PaymentStatus = %q, want %q", sub.PaymentStatus, "grace")
	}
	// Field 17: OverrideDailyRunLimit
	if sub.OverrideDailyRunLimit == nil || *sub.OverrideDailyRunLimit != 1000 {
		t.Errorf("OverrideDailyRunLimit = %v, want 1000", sub.OverrideDailyRunLimit)
	}
	// Field 18: OverrideConcurrentRunLimit
	if sub.OverrideConcurrentRunLimit == nil || *sub.OverrideConcurrentRunLimit != 50 {
		t.Errorf("OverrideConcurrentRunLimit = %v, want 50", sub.OverrideConcurrentRunLimit)
	}
	// Field 19: EnforcementMode
	if sub.EnforcementMode != "warn" {
		t.Errorf("EnforcementMode = %q, want %q", sub.EnforcementMode, "warn")
	}
	// Field 20: MonthlyUsageEmail
	if sub.MonthlyUsageEmail != true {
		t.Errorf("MonthlyUsageEmail = %v, want true", sub.MonthlyUsageEmail)
	}
	// Field 21a: CreatedAt
	if sub.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero")
	}
	// Field 21b: UpdatedAt
	if sub.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt is zero")
	}
}

func TestPgStore_GetOrgSubscription_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	_, err := pgStore.GetOrgSubscription(ctx, "org-nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
	if !errors.Is(err, billing.ErrSubscriptionNotFound) {
		if !contains(err.Error(), "not found") {
			t.Errorf("error = %v, want ErrSubscriptionNotFound", err)
		}
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
	if err := pgStore.UpsertOrgSubscription(ctx, sub); err != nil {
		t.Fatalf("UpsertOrgSubscription error = %v", err)
	}

	got, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if got.PlanTier != "pro" {
		t.Errorf("PlanTier = %q, want %q", got.PlanTier, "pro")
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q", got.Status, "active")
	}

	// Upsert again with different plan.
	sub.PlanTier = "enterprise"
	if err := pgStore.UpsertOrgSubscription(ctx, sub); err != nil {
		t.Fatalf("second UpsertOrgSubscription error = %v", err)
	}
	got2, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription second error = %v", err)
	}
	if got2.PlanTier != "enterprise" {
		t.Errorf("PlanTier after upsert = %q, want %q", got2.PlanTier, "enterprise")
	}
}

func TestPgStore_UpdateOrgSubscriptionPlan(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-upplan-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, "pro", "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan error = %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if sub.PlanTier != "pro" {
		t.Errorf("PlanTier = %q, want %q", sub.PlanTier, "pro")
	}
	if sub.Status != "active" {
		t.Errorf("Status = %q, want %q", sub.Status, "active")
	}
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
		t.Fatalf("update subscription bindings: %v", err)
	}

	bySub, err := pgStore.GetOrgSubscriptionByStripeSubscriptionID(ctx, "sub_lookup_123")
	if err != nil {
		t.Fatalf("GetOrgSubscriptionByStripeSubscriptionID() error = %v", err)
	}
	if bySub.OrgID != orgID {
		t.Fatalf("subscription binding org = %q, want %q", bySub.OrgID, orgID)
	}

	byCustomer, err := pgStore.GetOrgSubscriptionByStripeCustomerID(ctx, "cus_lookup_123")
	if err != nil {
		t.Fatalf("GetOrgSubscriptionByStripeCustomerID() error = %v", err)
	}
	if byCustomer.OrgID != orgID {
		t.Fatalf("customer binding org = %q, want %q", byCustomer.OrgID, orgID)
	}

	if _, err := pgStore.GetOrgSubscriptionByStripeSubscriptionID(ctx, "sub_missing"); !errors.Is(err, billing.ErrSubscriptionNotFound) {
		t.Fatalf("missing subscription binding error = %v, want ErrSubscriptionNotFound", err)
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
		t.Fatalf("seed first binding: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET stripe_subscription_id = 'sub_unique_123'
		WHERE org_id = $1
	`, orgB); err == nil {
		t.Fatal("expected duplicate stripe_subscription_id to fail")
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET stripe_customer_id = 'cus_unique_123'
		WHERE org_id = $1
	`, orgB); err == nil {
		t.Fatal("expected duplicate stripe_customer_id to fail")
	}
}

func TestPgStore_UpdateOrgSubscriptionPlan_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.UpdateOrgSubscriptionPlan(ctx, "org-nonexistent", "pro", "active")
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
}

func TestPgStore_UpdateOrgSubscriptionFull(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-upfull-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	ps := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	pe := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	if err := pgStore.UpdateOrgSubscriptionFull(ctx, orgID, "pro", "active", &ps, &pe); err != nil {
		t.Fatalf("UpdateOrgSubscriptionFull error = %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if sub.PlanTier != "pro" {
		t.Errorf("PlanTier = %q, want %q", sub.PlanTier, "pro")
	}
	if sub.CurrentPeriodStart == nil || !timeClose(*sub.CurrentPeriodStart, ps, 5*time.Second) {
		t.Errorf("CurrentPeriodStart = %v, want %v", sub.CurrentPeriodStart, ps)
	}
	if sub.CurrentPeriodEnd == nil || !timeClose(*sub.CurrentPeriodEnd, pe, 5*time.Second) {
		t.Errorf("CurrentPeriodEnd = %v, want %v", sub.CurrentPeriodEnd, pe)
	}
}

func TestPgStore_UpdateOrgSubscriptionFull_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	ps := time.Now().UTC()
	pe := ps.Add(30 * 24 * time.Hour)
	err := pgStore.UpdateOrgSubscriptionFull(ctx, "org-nonexistent", "pro", "active", &ps, &pe)
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
}

func TestPgStore_SetPendingPlanTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-pending-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	if err := pgStore.SetPendingPlanTier(ctx, orgID, "free"); err != nil {
		t.Fatalf("SetPendingPlanTier error = %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if sub.PendingPlanTier == nil || *sub.PendingPlanTier != "free" {
		t.Errorf("PendingPlanTier = %v, want %q", sub.PendingPlanTier, "free")
	}
}

func TestPgStore_SetPendingPlanTier_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.SetPendingPlanTier(ctx, "org-nonexistent", "free")
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
}

func TestPgStore_SetPendingDowngrade(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-penddown-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	ps := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	pe := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	if err := pgStore.SetPendingDowngrade(ctx, orgID, "free", &ps, &pe); err != nil {
		t.Fatalf("SetPendingDowngrade error = %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if sub.PendingPlanTier == nil || *sub.PendingPlanTier != "free" {
		t.Errorf("PendingPlanTier = %v, want %q", sub.PendingPlanTier, "free")
	}
	if sub.CurrentPeriodStart == nil || !timeClose(*sub.CurrentPeriodStart, ps, 5*time.Second) {
		t.Errorf("CurrentPeriodStart = %v, want %v", sub.CurrentPeriodStart, ps)
	}
	if sub.CurrentPeriodEnd == nil || !timeClose(*sub.CurrentPeriodEnd, pe, 5*time.Second) {
		t.Errorf("CurrentPeriodEnd = %v, want %v", sub.CurrentPeriodEnd, pe)
	}
}

func TestPgStore_ClearPendingPlanTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-clrpend-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	if err := pgStore.SetPendingPlanTier(ctx, orgID, "free"); err != nil {
		t.Fatalf("SetPendingPlanTier error = %v", err)
	}

	if err := pgStore.ClearPendingPlanTier(ctx, orgID); err != nil {
		t.Fatalf("ClearPendingPlanTier error = %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if sub.PendingPlanTier != nil {
		t.Errorf("PendingPlanTier = %v, want nil", sub.PendingPlanTier)
	}
}

func TestPgStore_ClearPendingPlanTier_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.ClearPendingPlanTier(ctx, "org-nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
}

func TestPgStore_ApplyPendingDowngrade(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-applydown-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	// Upgrade to pro first.
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, "pro", "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan error = %v", err)
	}
	if err := pgStore.SetPendingPlanTier(ctx, orgID, "free"); err != nil {
		t.Fatalf("SetPendingPlanTier error = %v", err)
	}
	if err := pgStore.ApplyPendingDowngrade(ctx, orgID); err != nil {
		t.Fatalf("ApplyPendingDowngrade error = %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if sub.PlanTier != "free" {
		t.Errorf("PlanTier = %q, want %q", sub.PlanTier, "free")
	}
	if sub.PendingPlanTier != nil {
		t.Errorf("PendingPlanTier = %v, want nil", sub.PendingPlanTier)
	}
}

func TestPgStore_ApplyPendingDowngradeIfTier_RequiresSamePendingTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-applydown-if-tier-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, "pro", "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan error = %v", err)
	}
	if err := pgStore.SetPendingPlanTier(ctx, orgID, "starter"); err != nil {
		t.Fatalf("SetPendingPlanTier error = %v", err)
	}

	applied, err := pgStore.ApplyPendingDowngradeIfTier(ctx, orgID, "free")
	if err != nil {
		t.Fatalf("ApplyPendingDowngradeIfTier wrong tier error = %v", err)
	}
	if applied {
		t.Fatal("expected wrong pending tier to skip conditional apply")
	}
	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if sub.PlanTier != "pro" || sub.PendingPlanTier == nil || *sub.PendingPlanTier != "starter" {
		t.Fatalf("subscription changed after wrong-tier apply: plan=%q pending=%v", sub.PlanTier, sub.PendingPlanTier)
	}

	applied, err = pgStore.ApplyPendingDowngradeIfTier(ctx, orgID, "starter")
	if err != nil {
		t.Fatalf("ApplyPendingDowngradeIfTier correct tier error = %v", err)
	}
	if !applied {
		t.Fatal("expected matching pending tier to apply")
	}
	sub, err = pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription after apply error = %v", err)
	}
	if sub.PlanTier != "starter" || sub.PendingPlanTier != nil {
		t.Fatalf("subscription not conditionally applied: plan=%q pending=%v", sub.PlanTier, sub.PendingPlanTier)
	}
}

func TestPgStore_ApplyPendingDowngradeTierIfPending_RetainsPendingUntilConditionalClear(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-applydown-retain-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, "pro", "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan error = %v", err)
	}
	if err := pgStore.SetPendingPlanTier(ctx, orgID, "starter"); err != nil {
		t.Fatalf("SetPendingPlanTier error = %v", err)
	}

	applied, err := pgStore.ApplyPendingDowngradeTierIfPending(ctx, orgID, "free")
	if err != nil {
		t.Fatalf("ApplyPendingDowngradeTierIfPending wrong tier error = %v", err)
	}
	if applied {
		t.Fatal("expected wrong pending tier to skip retained apply")
	}

	applied, err = pgStore.ApplyPendingDowngradeTierIfPending(ctx, orgID, "starter")
	if err != nil {
		t.Fatalf("ApplyPendingDowngradeTierIfPending error = %v", err)
	}
	if !applied {
		t.Fatal("expected matching pending tier to apply")
	}
	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription after retained apply error = %v", err)
	}
	if sub.PlanTier != "starter" || sub.PendingPlanTier == nil || *sub.PendingPlanTier != "starter" {
		t.Fatalf("retained apply should set plan and keep pending tier: plan=%q pending=%v", sub.PlanTier, sub.PendingPlanTier)
	}

	cleared, err := pgStore.ClearPendingPlanTierIfTier(ctx, orgID, "free")
	if err != nil {
		t.Fatalf("ClearPendingPlanTierIfTier wrong tier error = %v", err)
	}
	if cleared {
		t.Fatal("expected wrong pending tier to skip conditional clear")
	}
	sub, err = pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription after skipped clear error = %v", err)
	}
	if sub.PendingPlanTier == nil || *sub.PendingPlanTier != "starter" {
		t.Fatalf("wrong-tier clear should retain pending tier, got %v", sub.PendingPlanTier)
	}

	cleared, err = pgStore.ClearPendingPlanTierIfTier(ctx, orgID, "starter")
	if err != nil {
		t.Fatalf("ClearPendingPlanTierIfTier error = %v", err)
	}
	if !cleared {
		t.Fatal("expected matching pending tier to clear")
	}
	sub, err = pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription after clear error = %v", err)
	}
	if sub.PlanTier != "starter" || sub.PendingPlanTier != nil {
		t.Fatalf("conditional clear should only remove pending tier: plan=%q pending=%v", sub.PlanTier, sub.PendingPlanTier)
	}
}

func TestPgStore_ApplyPendingDowngrade_NoPending(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-nopend-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	err := pgStore.ApplyPendingDowngrade(ctx, orgID)
	if err == nil {
		t.Fatal("expected error when no pending downgrade, got nil")
	}
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

	// orgA: pending downgrade + period ended (should be listed)
	if err := pgStore.SetPendingDowngrade(ctx, orgA, "free", &past, &past); err != nil {
		t.Fatalf("SetPendingDowngrade orgA: %v", err)
	}
	// orgB: pending downgrade + period not ended yet (should NOT be listed)
	if err := pgStore.SetPendingDowngrade(ctx, orgB, "free", &past, &future); err != nil {
		t.Fatalf("SetPendingDowngrade orgB: %v", err)
	}
	// orgC: no pending downgrade (should NOT be listed)

	subs, err := pgStore.ListOrgsWithPendingDowngrade(ctx)
	if err != nil {
		t.Fatalf("ListOrgsWithPendingDowngrade error = %v", err)
	}

	found := false
	for _, s := range subs {
		if s.OrgID == orgA {
			found = true
		}
		if s.OrgID == orgB || s.OrgID == orgC {
			t.Errorf("unexpected org in result: %q", s.OrgID)
		}
	}
	if !found {
		t.Errorf("orgA not found in pending downgrade list")
	}
}

func TestPgStore_UpdateSpendingLimit(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-splimit-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	if err := pgStore.UpdateSpendingLimit(ctx, orgID, 200_000_000, "suspend"); err != nil {
		t.Fatalf("UpdateSpendingLimit error = %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if sub.SpendingLimitMicrousd != 200_000_000 {
		t.Errorf("SpendingLimitMicrousd = %d, want %d", sub.SpendingLimitMicrousd, 200_000_000)
	}
	if sub.LimitAction != "suspend" {
		t.Errorf("LimitAction = %q, want %q", sub.LimitAction, "suspend")
	}
}

func TestPgStore_UpdateSpendingLimit_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.UpdateSpendingLimit(ctx, "org-nonexistent", 100, "notify")
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
}

func TestPgStore_GetProjectOrgID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-projorg-" + newID()
	p := createProject(t, ctx, q, orgID, "Test Project")

	got, err := pgStore.GetProjectOrgID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProjectOrgID error = %v", err)
	}
	if got != orgID {
		t.Errorf("GetProjectOrgID = %q, want %q", got, orgID)
	}
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
	if err != nil {
		t.Fatalf("soft-delete project: %v", err)
	}

	got, err := pgStore.GetActiveProjectOrgID(ctx, pActive.ID)
	if err != nil {
		t.Fatalf("GetActiveProjectOrgID active error = %v", err)
	}
	if got != orgID {
		t.Errorf("GetActiveProjectOrgID active = %q, want %q", got, orgID)
	}

	// Deleted project should return error (no rows).
	_, err = pgStore.GetActiveProjectOrgID(ctx, pDeleted.ID)
	if err == nil {
		t.Fatal("expected error for deleted project, got nil")
	}
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
	if err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	ids, err := pgStore.ListProjectsByOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("ListProjectsByOrg error = %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("ListProjectsByOrg len = %d, want 2", len(ids))
	}
	idSet := map[string]bool{ids[0]: true, ids[1]: true}
	if !idSet[p1.ID] || !idSet[p2.ID] {
		t.Errorf("ListProjectsByOrg = %v, want %v and %v", ids, p1.ID, p2.ID)
	}
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
	if err != nil {
		t.Fatalf("CountProjectsByOrg error = %v", err)
	}
	if count != 2 {
		t.Errorf("CountProjectsByOrg = %d, want 2", count)
	}
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
	if err != nil {
		t.Fatalf("CountOrgsByUser error = %v", err)
	}
	if count != 2 {
		t.Errorf("CountOrgsByUser = %d, want 2", count)
	}
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
	if err != nil {
		t.Fatalf("BulkCountExecutingRunsByOrg error = %v", err)
	}
	if counts[orgA] != 2 {
		t.Errorf("orgA executing = %d, want 2", counts[orgA])
	}
	if counts[orgB] != 1 {
		t.Errorf("orgB executing = %d, want 1", counts[orgB])
	}
	// orgC should have 0 (absent from map is ok).
	if counts[orgC] != 0 {
		t.Errorf("orgC executing = %d, want 0", counts[orgC])
	}
}

func TestPgStore_BulkCountExecutingRunsByOrg_Empty(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	counts, err := pgStore.BulkCountExecutingRunsByOrg(ctx, []string{})
	if err != nil {
		t.Fatalf("BulkCountExecutingRunsByOrg empty error = %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("BulkCountExecutingRunsByOrg empty len = %d, want 0", len(counts))
	}
}

func TestPgStore_SetProjectOrgID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	p := createProject(t, ctx, q, "org-old", "P")
	newOrg := "org-new-" + newID()

	if err := pgStore.SetProjectOrgID(ctx, p.ID, newOrg); err != nil {
		t.Fatalf("SetProjectOrgID error = %v", err)
	}

	got, err := pgStore.GetProjectOrgID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProjectOrgID error = %v", err)
	}
	if got != newOrg {
		t.Errorf("GetProjectOrgID = %q, want %q", got, newOrg)
	}
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
	if err := pgStore.UpsertUsageRecord(ctx, rec); err != nil {
		t.Fatalf("UpsertUsageRecord error = %v", err)
	}

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
	if err := pgStore.UpsertUsageRecord(ctx, rec2); err != nil {
		t.Fatalf("second UpsertUsageRecord error = %v", err)
	}

	// Read back via raw SQL.
	var runs, compute, tokens, usageCost int64
	err := testDB.Pool.QueryRow(ctx,
		"SELECT runs_count, compute_cost_microusd, usage_tokens_total, usage_cost_microusd FROM usage_records WHERE org_id = $1 AND project_id = $2 AND period_date = $3",
		orgID, p.ID, day).Scan(&runs, &compute, &tokens, &usageCost)
	if err != nil {
		t.Fatalf("query usage_records: %v", err)
	}
	if runs != 15 {
		t.Errorf("runs_count = %d, want 15", runs)
	}
	if compute != 6_000_000 {
		t.Errorf("compute_cost = %d, want 6000000", compute)
	}
	if tokens != 1200 {
		t.Errorf("usage_tokens = %d, want 1200", tokens)
	}
	if usageCost != 600_000 {
		t.Errorf("usage_cost = %d, want 600000", usageCost)
	}
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
	if err := pgStore.ReplaceUsageRecord(ctx, rec); err != nil {
		t.Fatalf("ReplaceUsageRecord error = %v", err)
	}
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
	if err := pgStore.ReplaceUsageRecord(ctx, rec2); err != nil {
		t.Fatalf("second ReplaceUsageRecord error = %v", err)
	}

	var runs, compute int64
	err := testDB.Pool.QueryRow(ctx,
		"SELECT runs_count, compute_cost_microusd FROM usage_records WHERE org_id = $1 AND project_id = $2 AND period_date = $3",
		orgID, p.ID, day).Scan(&runs, &compute)
	if err != nil {
		t.Fatalf("query usage_records: %v", err)
	}
	if runs != 3 {
		t.Errorf("runs_count = %d, want replacement 3", runs)
	}
	if compute != 1_000_000 {
		t.Errorf("compute_cost = %d, want replacement 1000000", compute)
	}
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
	if err := pgStore.UpsertUsageRecord(ctx, rec); err != nil {
		t.Fatalf("UpsertUsageRecord error = %v", err)
	}

	from := day
	to := day
	orgRecords, err := pgStore.GetOrgUsageForPeriod(ctx, orgID, from, to)
	if err != nil {
		t.Fatalf("GetOrgUsageForPeriod error = %v", err)
	}
	assertRecordedComputeUsage(t, orgRecords, p.ID, 2_500_000)

	projectRecords, err := pgStore.GetProjectUsageForPeriod(ctx, p.ID, from, to)
	if err != nil {
		t.Fatalf("GetProjectUsageForPeriod error = %v", err)
	}
	assertRecordedComputeUsage(t, projectRecords, p.ID, 2_500_000)

	dailyRecords, err := pgStore.GetOrgDailyUsage(ctx, orgID, day)
	if err != nil {
		t.Fatalf("GetOrgDailyUsage error = %v", err)
	}
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
	if err != nil {
		t.Fatalf("RecordUsageCost first call error = %v", err)
	}
	if !recorded {
		t.Fatal("RecordUsageCost first call recorded = false, want true")
	}

	recorded, err = pgStore.RecordUsageCost(ctx, rec, "strait:cost_recorded:run-ledger-1", "http")
	if err != nil {
		t.Fatalf("RecordUsageCost duplicate call error = %v", err)
	}
	if recorded {
		t.Fatal("RecordUsageCost duplicate call recorded = true, want false")
	}

	var runs, compute int64
	if err := testDB.Pool.QueryRow(ctx,
		"SELECT runs_count, compute_cost_microusd FROM usage_records WHERE org_id = $1 AND project_id = $2 AND period_date = $3",
		orgID, p.ID, day,
	).Scan(&runs, &compute); err != nil {
		t.Fatalf("query usage_records: %v", err)
	}
	if runs != 1 || compute != 20 {
		t.Fatalf("usage aggregate = runs %d compute %d, want runs 1 compute 20", runs, compute)
	}

	var events int
	if err := testDB.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM billing_cost_events WHERE idempotency_key = $1",
		"strait:cost_recorded:run-ledger-1",
	).Scan(&events); err != nil {
		t.Fatalf("query billing_cost_events: %v", err)
	}
	if events != 1 {
		t.Fatalf("billing_cost_events count = %d, want 1", events)
	}
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
		t.Fatalf("set completed run fields: %v", err)
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
	if err := q.CreateWebhookDelivery(ctx, delivery); err != nil {
		t.Fatalf("CreateWebhookDelivery error = %v", err)
	}

	day := time.Date(finishedAt.Year(), finishedAt.Month(), finishedAt.Day(), 0, 0, 0, 0, time.UTC)
	if err := pgStore.ReconcileFlatUsageCosts(ctx, orgID, day); err != nil {
		t.Fatalf("ReconcileFlatUsageCosts error = %v", err)
	}
	if err := pgStore.ReconcileFlatUsageCosts(ctx, orgID, day); err != nil {
		t.Fatalf("ReconcileFlatUsageCosts duplicate error = %v", err)
	}

	var eventCount int
	if err := testDB.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM billing_cost_events WHERE org_id = $1",
		orgID,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count billing_cost_events: %v", err)
	}
	if eventCount != 2 {
		t.Fatalf("billing_cost_events count = %d, want 2", eventCount)
	}

	var runs, compute int64
	if err := testDB.Pool.QueryRow(ctx,
		"SELECT runs_count, compute_cost_microusd FROM usage_records WHERE org_id = $1 AND project_id = $2 AND period_date = $3",
		orgID, p.ID, day,
	).Scan(&runs, &compute); err != nil {
		t.Fatalf("query reconciled usage_records: %v", err)
	}
	if runs != 2 || compute != billing.HTTPCostPerRunMicrousd+billing.WebhookDeliveryCostPerRunMicrousd {
		t.Fatalf("reconciled usage = runs %d compute %d, want runs 2 compute %d",
			runs, compute, billing.HTTPCostPerRunMicrousd+billing.WebhookDeliveryCostPerRunMicrousd)
	}
}

func assertRecordedComputeUsage(t *testing.T, records []billing.UsageRecord, projectID string, want int64) {
	t.Helper()
	for _, rec := range records {
		if rec.ProjectID != projectID {
			continue
		}
		if rec.ComputeCostMicro != want {
			t.Fatalf("ComputeCostMicro = %d, want %d in records %#v", rec.ComputeCostMicro, want, records)
		}
		return
	}
	t.Fatalf("usage records missing project %s: %#v", projectID, records)
}

func TestPgStore_GetProjectBudget_NoRow(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	p := createProject(t, ctx, q, "org-budget", "P")

	budget, action, err := pgStore.GetProjectBudget(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProjectBudget error = %v", err)
	}
	if budget != -1 {
		t.Errorf("budget = %d, want -1", budget)
	}
	if action != "notify" {
		t.Errorf("action = %q, want %q", action, "notify")
	}
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
	if err != nil {
		t.Fatalf("insert project_quotas: %v", err)
	}

	if err := pgStore.SetProjectBudget(ctx, p.ID, 50_000_000, "suspend"); err != nil {
		t.Fatalf("SetProjectBudget error = %v", err)
	}

	budget, action, err := pgStore.GetProjectBudget(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProjectBudget error = %v", err)
	}
	if budget != 50_000_000 {
		t.Errorf("budget = %d, want 50000000", budget)
	}
	if action != "suspend" {
		t.Errorf("action = %q, want %q", action, "suspend")
	}
}
func TestPgStore_UpdateAnomalyThresholds(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-anomaly-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	if err := pgStore.UpdateAnomalyThresholds(ctx, orgID, 5.0, 15.0); err != nil {
		t.Fatalf("UpdateAnomalyThresholds error = %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if math.Abs(sub.AnomalyThresholdWarning-5.0) > 0.01 {
		t.Errorf("AnomalyThresholdWarning = %f, want 5.0", sub.AnomalyThresholdWarning)
	}
	if math.Abs(sub.AnomalyThresholdCritical-15.0) > 0.01 {
		t.Errorf("AnomalyThresholdCritical = %f, want 15.0", sub.AnomalyThresholdCritical)
	}
}

func TestPgStore_UpdateAnomalyThresholds_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.UpdateAnomalyThresholds(ctx, "org-nonexistent", 5.0, 15.0)
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
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

	// Cancel one.
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgC, "free", "canceled"); err != nil {
		t.Fatalf("cancel orgC: %v", err)
	}

	ids, err := pgStore.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		t.Fatalf("ListAllSubscribedOrgIDs error = %v", err)
	}

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	if !idSet[orgA] || !idSet[orgB] {
		t.Errorf("missing orgA or orgB in subscribed list")
	}
	if idSet[orgC] {
		t.Errorf("canceled orgC should not be in subscribed list")
	}
}

func TestPlanRetentionResolver_MissingSubscriptionDoesNotFallbackToFreeRetention(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	resolver := billing.NewPlanRetentionResolver(billing.NewPgStore(testDB.Pool))

	days, err := resolver.GetOrgRetentionDays(ctx, "org-retention-missing-"+newID())
	if !errors.Is(err, billing.ErrSubscriptionNotFound) {
		t.Fatalf("GetOrgRetentionDays error = %v, want ErrSubscriptionNotFound", err)
	}
	if days != 0 {
		t.Fatalf("days = %d, want 0 so retention reaper skips uncertain orgs", days)
	}
}

func TestPgStore_UpdatePaymentStatus(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-paystatus-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	graceEnd := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	if err := pgStore.UpdatePaymentStatus(ctx, orgID, "grace", &graceEnd); err != nil {
		t.Fatalf("UpdatePaymentStatus error = %v", err)
	}

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if sub.PaymentStatus != "grace" {
		t.Errorf("PaymentStatus = %q, want %q", sub.PaymentStatus, "grace")
	}
	if sub.GracePeriodEnd == nil {
		t.Errorf("GracePeriodEnd is nil")
	} else if !timeClose(*sub.GracePeriodEnd, graceEnd, 5*time.Second) {
		t.Errorf("GracePeriodEnd = %v, want %v", *sub.GracePeriodEnd, graceEnd)
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

	if err := pgStore.UpdatePaymentStatus(ctx, orgExpired, "grace", &past); err != nil {
		t.Fatalf("UpdatePaymentStatus expired: %v", err)
	}
	if err := pgStore.UpdatePaymentStatus(ctx, orgFuture, "grace", &future); err != nil {
		t.Fatalf("UpdatePaymentStatus future: %v", err)
	}
	// orgOk stays with payment_status = 'ok'

	subs, err := pgStore.ListOrgsInGracePeriod(ctx)
	if err != nil {
		t.Fatalf("ListOrgsInGracePeriod error = %v", err)
	}

	found := false
	for _, s := range subs {
		if s.OrgID == orgExpired {
			found = true
		}
		if s.OrgID == orgFuture || s.OrgID == orgOk {
			t.Errorf("unexpected org in grace period list: %q", s.OrgID)
		}
	}
	if !found {
		t.Errorf("orgExpired not found in grace period list")
	}
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
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgExpired, "pro", "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan expired: %v", err)
	}
	if err := pgStore.UpdatePaymentStatus(ctx, orgExpired, "grace", &past); err != nil {
		t.Fatalf("UpdatePaymentStatus expired: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgRecovered, "pro", "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan recovered: %v", err)
	}
	if err := pgStore.UpdatePaymentStatus(ctx, orgRecovered, "ok", nil); err != nil {
		t.Fatalf("UpdatePaymentStatus recovered: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgFuture, "pro", "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan future: %v", err)
	}
	if err := pgStore.UpdatePaymentStatus(ctx, orgFuture, "grace", &future); err != nil {
		t.Fatalf("UpdatePaymentStatus future: %v", err)
	}

	restricted, err := pgStore.RestrictExpiredGracePeriod(ctx, orgExpired, &past)
	if err != nil {
		t.Fatalf("RestrictExpiredGracePeriod expired: %v", err)
	}
	if !restricted {
		t.Fatal("expected expired grace period to be restricted")
	}

	expiredSub, err := pgStore.GetOrgSubscription(ctx, orgExpired)
	if err != nil {
		t.Fatalf("GetOrgSubscription expired: %v", err)
	}
	if expiredSub.PlanTier != "free" || expiredSub.Status != "restricted" || expiredSub.PaymentStatus != "restricted" {
		t.Fatalf("expected atomic free/restricted state, got plan=%q status=%q payment=%q", expiredSub.PlanTier, expiredSub.Status, expiredSub.PaymentStatus)
	}

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
			if err != nil {
				t.Fatalf("RestrictExpiredGracePeriod: %v", err)
			}
			if changed {
				t.Fatal("expected conditional update to skip ineligible org")
			}
			sub, err := pgStore.GetOrgSubscription(ctx, tt.orgID)
			if err != nil {
				t.Fatalf("GetOrgSubscription: %v", err)
			}
			if sub.PlanTier != "pro" || sub.Status != "active" {
				t.Fatalf("expected plan/status to remain unchanged, got plan=%q status=%q", sub.PlanTier, sub.Status)
			}
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

	// orgStale: active, period ended > 1 day ago, no pending downgrade
	if err := pgStore.UpdateOrgSubscriptionFull(ctx, orgStale, "pro", "active", &staleEnd, &staleEnd); err != nil {
		t.Fatalf("UpdateOrgSubscriptionFull stale: %v", err)
	}
	// orgFresh: active, period not ended
	if err := pgStore.UpdateOrgSubscriptionFull(ctx, orgFresh, "pro", "active", &staleEnd, &freshEnd); err != nil {
		t.Fatalf("UpdateOrgSubscriptionFull fresh: %v", err)
	}
	// orgPending: active, period ended, but HAS pending downgrade (should not be stale)
	if err := pgStore.UpdateOrgSubscriptionFull(ctx, orgPending, "pro", "active", &staleEnd, &staleEnd); err != nil {
		t.Fatalf("UpdateOrgSubscriptionFull pending: %v", err)
	}
	if err := pgStore.SetPendingPlanTier(ctx, orgPending, "free"); err != nil {
		t.Fatalf("SetPendingPlanTier pending: %v", err)
	}

	subs, err := pgStore.ListStaleSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListStaleSubscriptions error = %v", err)
	}

	found := false
	for _, s := range subs {
		if s.OrgID == orgStale {
			found = true
		}
		if s.OrgID == orgFresh || s.OrgID == orgPending {
			t.Errorf("unexpected org in stale list: %q", s.OrgID)
		}
	}
	if !found {
		t.Errorf("orgStale not found in stale subscriptions list")
	}
}

func TestPgStore_IsProjectSuspended(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	p := createProject(t, ctx, q, "org-susp", "P")

	suspended, err := pgStore.IsProjectSuspended(ctx, p.ID)
	if err != nil {
		t.Fatalf("IsProjectSuspended error = %v", err)
	}
	if suspended {
		t.Errorf("IsProjectSuspended = true, want false")
	}

	_, err = testDB.Pool.Exec(ctx, "UPDATE projects SET suspended = true WHERE id = $1", p.ID)
	if err != nil {
		t.Fatalf("suspend project: %v", err)
	}

	suspended, err = pgStore.IsProjectSuspended(ctx, p.ID)
	if err != nil {
		t.Fatalf("IsProjectSuspended after suspend error = %v", err)
	}
	if !suspended {
		t.Errorf("IsProjectSuspended = false, want true")
	}
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
	if err != nil {
		t.Fatalf("SuspendExcessProjects error = %v", err)
	}
	if count != 2 {
		t.Errorf("SuspendExcessProjects = %d, want 2", count)
	}

	// The oldest project (p1) should NOT be suspended.
	s1, _ := pgStore.IsProjectSuspended(ctx, p1.ID)
	s2, _ := pgStore.IsProjectSuspended(ctx, p2.ID)
	s3, _ := pgStore.IsProjectSuspended(ctx, p3.ID)
	if s1 {
		t.Errorf("p1 (oldest) should not be suspended")
	}
	if !s2 {
		t.Errorf("p2 should be suspended")
	}
	if !s3 {
		t.Errorf("p3 should be suspended")
	}
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
	if err != nil {
		t.Fatalf("ListOrgAdminEmails error = %v", err)
	}

	sort.Strings(emails)
	if len(emails) != 2 {
		t.Fatalf("ListOrgAdminEmails len = %d, want 2", len(emails))
	}
	if emails[0] != "admin1@example.com" {
		t.Errorf("emails[0] = %q, want %q", emails[0], "admin1@example.com")
	}
	if emails[1] != "admin2@example.com" {
		t.Errorf("emails[1] = %q, want %q", emails[1], "admin2@example.com")
	}
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
		t.Fatalf("mark unverified admin: %v", err)
	}

	emails, err := pgStore.ListOrgAdminEmails(ctx, orgID)
	if err != nil {
		t.Fatalf("ListOrgAdminEmails error = %v", err)
	}
	if len(emails) != 1 || emails[0] != "verified@example.com" {
		t.Fatalf("ListOrgAdminEmails = %v, want only verified@example.com", emails)
	}
}

func TestPgStore_UsageReportDedup(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-usagerpt-" + newID()
	periodEnd := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)

	sent, err := pgStore.HasSentUsageReport(ctx, orgID, periodEnd)
	if err != nil {
		t.Fatalf("HasSentUsageReport error = %v", err)
	}
	if sent {
		t.Errorf("HasSentUsageReport = true before recording, want false")
	}

	if err := pgStore.RecordSentUsageReport(ctx, orgID, periodEnd); err != nil {
		t.Fatalf("RecordSentUsageReport error = %v", err)
	}

	sent, err = pgStore.HasSentUsageReport(ctx, orgID, periodEnd)
	if err != nil {
		t.Fatalf("HasSentUsageReport after record error = %v", err)
	}
	if !sent {
		t.Errorf("HasSentUsageReport = false after recording, want true")
	}

	// Idempotent second recording.
	if err := pgStore.RecordSentUsageReport(ctx, orgID, periodEnd); err != nil {
		t.Fatalf("RecordSentUsageReport idempotent error = %v", err)
	}
}

func TestPgStore_ClaimContractReminderSend_DeduplicatesByOrgDateWindow(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-contract-claim-" + newID()
	contractEnd := time.Date(2026, 6, 30, 18, 30, 0, 0, time.UTC)

	claimed, err := pgStore.ClaimContractReminderSend(ctx, orgID, contractEnd, 30)
	if err != nil {
		t.Fatalf("ClaimContractReminderSend first: %v", err)
	}
	if !claimed {
		t.Fatal("expected first contract reminder claim to succeed")
	}
	claimed, err = pgStore.ClaimContractReminderSend(ctx, orgID, contractEnd.Add(5*time.Hour), 30)
	if err != nil {
		t.Fatalf("ClaimContractReminderSend duplicate: %v", err)
	}
	if claimed {
		t.Fatal("expected duplicate same-day reminder claim to be skipped")
	}
	claimed, err = pgStore.ClaimContractReminderSend(ctx, orgID, contractEnd, 7)
	if err != nil {
		t.Fatalf("ClaimContractReminderSend different window: %v", err)
	}
	if !claimed {
		t.Fatal("expected different reminder window to claim separately")
	}
}

func TestPgStore_UpdateMonthlyUsageEmail(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-monthlyemail-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	// Default should be false.
	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription error = %v", err)
	}
	if sub.MonthlyUsageEmail {
		t.Errorf("MonthlyUsageEmail default = true, want false")
	}

	// Enable.
	if err := pgStore.UpdateMonthlyUsageEmail(ctx, orgID, true); err != nil {
		t.Fatalf("UpdateMonthlyUsageEmail(true) error = %v", err)
	}
	sub, _ = pgStore.GetOrgSubscription(ctx, orgID)
	if !sub.MonthlyUsageEmail {
		t.Errorf("MonthlyUsageEmail after enable = false, want true")
	}

	// Disable.
	if err := pgStore.UpdateMonthlyUsageEmail(ctx, orgID, false); err != nil {
		t.Fatalf("UpdateMonthlyUsageEmail(false) error = %v", err)
	}
	sub, _ = pgStore.GetOrgSubscription(ctx, orgID)
	if sub.MonthlyUsageEmail {
		t.Errorf("MonthlyUsageEmail after disable = true, want false")
	}
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
		if err := pgStore.CreateAddon(ctx, a); err != nil {
			t.Fatalf("CreateAddon %s error = %v", a.ID, err)
		}
	}

	addons, err := pgStore.ListActiveAddons(ctx, orgID)
	if err != nil {
		t.Fatalf("ListActiveAddons error = %v", err)
	}
	if len(addons) != 2 {
		t.Fatalf("ListActiveAddons len = %d, want 2", len(addons))
	}
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
	if err := pgStore.CreateAddon(ctx, a); err != nil {
		t.Fatalf("CreateAddon error = %v", err)
	}

	if err := pgStore.DeactivateAddon(ctx, a.ID); err != nil {
		t.Fatalf("DeactivateAddon error = %v", err)
	}

	addons, err := pgStore.ListActiveAddons(ctx, orgID)
	if err != nil {
		t.Fatalf("ListActiveAddons error = %v", err)
	}
	if len(addons) != 0 {
		t.Errorf("ListActiveAddons after deactivation len = %d, want 0", len(addons))
	}
}

func TestPgStore_CountActiveAddonsByType(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-cntaddons-" + newID()
	for i := range 3 {
		a := &billing.Addon{
			ID:        newID(),
			OrgID:     orgID,
			AddonType: billing.AddonConcurrency100,
			Quantity:  1,
			Active:    true,
		}
		if err := pgStore.CreateAddon(ctx, a); err != nil {
			t.Fatalf("CreateAddon %d error = %v", i, err)
		}
	}
	// One inactive.
	aInact := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonConcurrency100,
		Quantity:  1,
		Active:    false,
	}
	if err := pgStore.CreateAddon(ctx, aInact); err != nil {
		t.Fatalf("CreateAddon inactive error = %v", err)
	}

	count, err := pgStore.CountActiveAddonsByType(ctx, orgID, billing.AddonConcurrency100)
	if err != nil {
		t.Fatalf("CountActiveAddonsByType error = %v", err)
	}
	if count != 3 {
		t.Errorf("CountActiveAddonsByType = %d, want 3", count)
	}

	// Different type should be 0.
	count2, err := pgStore.CountActiveAddonsByType(ctx, orgID, billing.AddonEnvironments5)
	if err != nil {
		t.Fatalf("CountActiveAddonsByType environments_5 = %v", err)
	}
	if count2 != 0 {
		t.Errorf("CountActiveAddonsByType environments_5 = %d, want 0", count2)
	}
}

func TestPgStore_WebhookIdempotency(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	msgID := "msg-" + newID()

	processed, err := pgStore.IsWebhookProcessed(ctx, msgID)
	if err != nil {
		t.Fatalf("IsWebhookProcessed error = %v", err)
	}
	if processed {
		t.Errorf("IsWebhookProcessed = true before recording, want false")
	}

	if err := pgStore.RecordProcessedWebhook(ctx, msgID); err != nil {
		t.Fatalf("RecordProcessedWebhook error = %v", err)
	}

	processed, err = pgStore.IsWebhookProcessed(ctx, msgID)
	if err != nil {
		t.Fatalf("IsWebhookProcessed after record error = %v", err)
	}
	if !processed {
		t.Errorf("IsWebhookProcessed = false after recording, want true")
	}

	// Idempotent second recording.
	if err := pgStore.RecordProcessedWebhook(ctx, msgID); err != nil {
		t.Fatalf("RecordProcessedWebhook idempotent error = %v", err)
	}
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
		t.Fatalf("ClaimWebhookForProcessing concurrent error: %v", err)
	}
	var claimed, skipped int
	for result := range results {
		if result {
			claimed++
		} else {
			skipped++
		}
	}
	if claimed != 1 || skipped != 1 {
		t.Fatalf("claimed/skipped = %d/%d, want 1/1", claimed, skipped)
	}

	processed, err := pgStore.IsWebhookProcessed(ctx, msgID)
	if err != nil {
		t.Fatalf("IsWebhookProcessed(processing) error = %v", err)
	}
	if processed {
		t.Fatal("processing claim must not count as processed")
	}
	status, err := pgStore.GetWebhookProcessingStatus(ctx, msgID)
	if err != nil {
		t.Fatalf("GetWebhookProcessingStatus(processing) error = %v", err)
	}
	if status != "processing" {
		t.Fatalf("GetWebhookProcessingStatus(processing) = %q, want processing", status)
	}

	if err := pgStore.ReleaseWebhookClaim(ctx, msgID); err != nil {
		t.Fatalf("ReleaseWebhookClaim error = %v", err)
	}
	status, err = pgStore.GetWebhookProcessingStatus(ctx, msgID)
	if err != nil {
		t.Fatalf("GetWebhookProcessingStatus(after release) error = %v", err)
	}
	if status != "" {
		t.Fatalf("GetWebhookProcessingStatus(after release) = %q, want empty", status)
	}
	reclaimed, err := pgStore.ClaimWebhookForProcessing(ctx, msgID, 10*time.Minute)
	if err != nil {
		t.Fatalf("ClaimWebhookForProcessing(after release) error = %v", err)
	}
	if !reclaimed {
		t.Fatal("claim after release = false, want true")
	}
	if err := pgStore.MarkWebhookProcessed(ctx, msgID); err != nil {
		t.Fatalf("MarkWebhookProcessed error = %v", err)
	}
	status, err = pgStore.GetWebhookProcessingStatus(ctx, msgID)
	if err != nil {
		t.Fatalf("GetWebhookProcessingStatus(processed) error = %v", err)
	}
	if status != "processed" {
		t.Fatalf("GetWebhookProcessingStatus(processed) = %q, want processed", status)
	}
	again, err := pgStore.ClaimWebhookForProcessing(ctx, msgID, 10*time.Minute)
	if err != nil {
		t.Fatalf("ClaimWebhookForProcessing(after processed) error = %v", err)
	}
	if again {
		t.Fatal("claim after processed = true, want false")
	}
}

func TestPgStore_DeleteOldWebhookMessages(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	// Insert some messages with backdated timestamps.
	for i := range 5 {
		msgID := fmt.Sprintf("msg-old-%d-%s", i, newID())
		if err := pgStore.RecordProcessedWebhook(ctx, msgID); err != nil {
			t.Fatalf("RecordProcessedWebhook %d error = %v", i, err)
		}
		// Backdate to 10 days ago.
		_, err := testDB.Pool.Exec(ctx,
			"UPDATE processed_webhook_messages SET processed_at = $2 WHERE msg_id = $1",
			msgID, time.Now().UTC().Add(-10*24*time.Hour))
		if err != nil {
			t.Fatalf("backdate msg %d: %v", i, err)
		}
	}
	// One recent message.
	recentMsg := "msg-recent-" + newID()
	if err := pgStore.RecordProcessedWebhook(ctx, recentMsg); err != nil {
		t.Fatalf("RecordProcessedWebhook recent error = %v", err)
	}

	deleted, err := pgStore.DeleteOldWebhookMessages(ctx, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteOldWebhookMessages error = %v", err)
	}
	if deleted != 5 {
		t.Errorf("DeleteOldWebhookMessages = %d, want 5", deleted)
	}

	// Recent message should still exist.
	still, err := pgStore.IsWebhookProcessed(ctx, recentMsg)
	if err != nil {
		t.Fatalf("IsWebhookProcessed recent error = %v", err)
	}
	if !still {
		t.Errorf("recent message was deleted, should have been kept")
	}
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
	if err != nil {
		t.Fatalf("DeactivateExcessWebhookSubscriptions error = %v", err)
	}
	if deactivated != 3 {
		t.Errorf("DeactivateExcessWebhookSubscriptions = %d, want 3", deactivated)
	}

	// Verify only 2 active remain.
	var activeCount int
	err = testDB.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM webhook_subscriptions WHERE project_id = $1 AND active = true", p.ID).Scan(&activeCount)
	if err != nil {
		t.Fatalf("count active webhooks: %v", err)
	}
	if activeCount != 2 {
		t.Errorf("active webhook count = %d, want 2", activeCount)
	}
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
	if err != nil {
		t.Fatalf("DeactivateExcessEnvironments error = %v", err)
	}
	if deactivated != 2 {
		t.Errorf("DeactivateExcessEnvironments = %d, want 2", deactivated)
	}

	// Verify only 2 remain.
	var remaining int
	if err := testDB.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM environments WHERE project_id = $1", p.ID).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 2 {
		t.Errorf("remaining environments = %d, want 2", remaining)
	}
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
	if err != nil {
		t.Fatalf("DeactivateExcessCronJobs error = %v", err)
	}
	if len(deactivated) != 3 {
		t.Errorf("DeactivateExcessCronJobs returned %d ids, want 3", len(deactivated))
	}
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
	if err := q.CreateWorkflow(ctx, oldWorkflow); err != nil {
		t.Fatalf("CreateWorkflow(old) error = %v", err)
	}
	newWorkflow := &domain.Workflow{
		ID:        newID(),
		ProjectID: p.ID,
		Name:      "new scheduled workflow",
		Slug:      "new-scheduled-workflow-" + newID(),
		Enabled:   true,
		Cron:      "15 * * * *",
		Version:   1,
	}
	if err := q.CreateWorkflow(ctx, newWorkflow); err != nil {
		t.Fatalf("CreateWorkflow(new) error = %v", err)
	}

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
			t.Fatalf("set %s updated_at for %s: %v", update.table, update.id, err)
		}
	}

	deactivated, err := pgStore.DeactivateExcessCronJobs(ctx, orgID, 2)
	if err != nil {
		t.Fatalf("DeactivateExcessCronJobs error = %v", err)
	}
	if len(deactivated) != 2 {
		t.Fatalf("DeactivateExcessCronJobs returned %d ids, want 2", len(deactivated))
	}

	assertCronCleared := func(table, id string, wantCleared bool) {
		t.Helper()
		var cron string
		if err := testDB.Pool.QueryRow(ctx, "SELECT COALESCE(cron, '') FROM "+table+" WHERE id = $1", id).Scan(&cron); err != nil {
			t.Fatalf("query %s cron for %s: %v", table, id, err)
		}
		if gotCleared := cron == ""; gotCleared != wantCleared {
			t.Fatalf("%s %s cron cleared = %v, want %v (cron=%q)", table, id, gotCleared, wantCleared, cron)
		}
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
	if err != nil {
		t.Fatalf("CountMembersByOrg() error = %v", err)
	}
	if members != 2 {
		t.Fatalf("CountMembersByOrg() = %d, want 2", members)
	}

	jobA := createJob(t, ctx, q, projectA.ID)
	jobB := createJob(t, ctx, q, projectB.ID)
	jobOther := createJob(t, ctx, q, projectOther.ID)

	_ = createRun(t, ctx, q, jobA, domain.StatusExecuting)
	_ = createRun(t, ctx, q, jobA, domain.StatusCompleted)
	_ = createRun(t, ctx, q, jobB, domain.StatusExecuting)
	_ = createRun(t, ctx, q, jobOther, domain.StatusExecuting)

	executing, err := pgStore.CountExecutingRunsByOrg(ctx, "org-members")
	if err != nil {
		t.Fatalf("CountExecutingRunsByOrg() error = %v", err)
	}
	if executing != 2 {
		t.Fatalf("CountExecutingRunsByOrg() = %d, want 2", executing)
	}
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

	if err := pgStore.UpsertEnterpriseContract(ctx, c); err != nil {
		t.Fatalf("UpsertEnterpriseContract() error = %v", err)
	}

	got, err := pgStore.GetEnterpriseContract(ctx, orgID)
	if err != nil {
		t.Fatalf("GetEnterpriseContract() error = %v", err)
	}

	if got.OrgID != orgID {
		t.Errorf("OrgID = %q, want %q", got.OrgID, orgID)
	}
	if got.EnterpriseTier != billing.EnterpriseTierStarter {
		t.Errorf("EnterpriseTier = %q, want %q", got.EnterpriseTier, billing.EnterpriseTierStarter)
	}
	if got.AnnualCommitmentCents != 1800000 {
		t.Errorf("AnnualCommitmentCents = %d, want 1800000", got.AnnualCommitmentCents)
	}
	if got.OverageDiscountPct != 10 {
		t.Errorf("OverageDiscountPct = %d, want 10", got.OverageDiscountPct)
	}
	if !got.AutoRenew {
		t.Error("AutoRenew = false, want true")
	}
	if got.BillingCadence != "annual" {
		t.Errorf("BillingCadence = %q, want annual", got.BillingCadence)
	}
	if got.StripeSubscriptionID == nil || *got.StripeSubscriptionID != "sub_"+orgID {
		t.Errorf("StripeSubscriptionID mismatch")
	}
	if got.Notes != "test contract" {
		t.Errorf("Notes = %q, want test contract", got.Notes)
	}
}

func TestPgStore_GetEnterpriseContract_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	_, err := pgStore.GetEnterpriseContract(ctx, "org-nonexistent-"+newID())
	if !errors.Is(err, billing.ErrContractNotFound) {
		t.Fatalf("expected ErrContractNotFound, got %v", err)
	}
}

func TestPgStore_UpsertEnterpriseContract_UpdatesExisting(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-upsert-" + newID()
	c1 := makeContract(orgID, billing.EnterpriseTierStarter, time.Now().Add(180*24*time.Hour))

	if err := pgStore.UpsertEnterpriseContract(ctx, c1); err != nil {
		t.Fatalf("first upsert error = %v", err)
	}

	// Upsert again with different tier and discount.
	c2 := makeContract(orgID, billing.EnterpriseTierGrowth, time.Now().Add(365*24*time.Hour))
	c2.OverageDiscountPct = 15
	c2.AnnualCommitmentCents = 4800000
	c2.Notes = "upgraded"

	if err := pgStore.UpsertEnterpriseContract(ctx, c2); err != nil {
		t.Fatalf("second upsert error = %v", err)
	}

	got, err := pgStore.GetEnterpriseContract(ctx, orgID)
	if err != nil {
		t.Fatalf("get after upsert error = %v", err)
	}
	if got.EnterpriseTier != billing.EnterpriseTierGrowth {
		t.Errorf("EnterpriseTier = %q, want %q", got.EnterpriseTier, billing.EnterpriseTierGrowth)
	}
	if got.OverageDiscountPct != 15 {
		t.Errorf("OverageDiscountPct = %d, want 15", got.OverageDiscountPct)
	}
	if got.AnnualCommitmentCents != 4800000 {
		t.Errorf("AnnualCommitmentCents = %d, want 4800000", got.AnnualCommitmentCents)
	}
	if got.Notes != "upgraded" {
		t.Errorf("Notes = %q, want upgraded", got.Notes)
	}
}

func TestPgStore_ListExpiringContracts(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	// Contract expiring in 5 days.
	org5d := "org-5d-" + newID()
	c5 := makeContract(org5d, billing.EnterpriseTierStarter, time.Now().Add(5*24*time.Hour))
	if err := pgStore.UpsertEnterpriseContract(ctx, c5); err != nil {
		t.Fatal(err)
	}

	// Contract expiring in 25 days.
	org25d := "org-25d-" + newID()
	c25 := makeContract(org25d, billing.EnterpriseTierGrowth, time.Now().Add(25*24*time.Hour))
	if err := pgStore.UpsertEnterpriseContract(ctx, c25); err != nil {
		t.Fatal(err)
	}

	// Contract expiring in 60 days (should not be in 30-day list).
	org60d := "org-60d-" + newID()
	c60 := makeContract(org60d, billing.EnterpriseTierLarge, time.Now().Add(60*24*time.Hour))
	if err := pgStore.UpsertEnterpriseContract(ctx, c60); err != nil {
		t.Fatal(err)
	}

	// Already expired (should not appear).
	orgExpired := "org-expired-" + newID()
	cExpired := makeContract(orgExpired, billing.EnterpriseTierStarter, time.Now().Add(-1*24*time.Hour))
	if err := pgStore.UpsertEnterpriseContract(ctx, cExpired); err != nil {
		t.Fatal(err)
	}

	// List contracts expiring within 7 days.
	within7, err := pgStore.ListExpiringContracts(ctx, 7)
	if err != nil {
		t.Fatalf("ListExpiringContracts(7) error = %v", err)
	}
	if len(within7) != 1 {
		t.Fatalf("expected 1 contract expiring within 7 days, got %d", len(within7))
	}
	if within7[0].OrgID != org5d {
		t.Errorf("expected org %q, got %q", org5d, within7[0].OrgID)
	}

	// List contracts expiring within 30 days.
	within30, err := pgStore.ListExpiringContracts(ctx, 30)
	if err != nil {
		t.Fatalf("ListExpiringContracts(30) error = %v", err)
	}
	if len(within30) != 2 {
		t.Fatalf("expected 2 contracts expiring within 30 days, got %d", len(within30))
	}
	// Should be ordered by end date ASC.
	if within30[0].OrgID != org5d {
		t.Errorf("first contract should be 5-day, got %q", within30[0].OrgID)
	}
	if within30[1].OrgID != org25d {
		t.Errorf("second contract should be 25-day, got %q", within30[1].OrgID)
	}

	// List within 0 days (edge: nothing expiring today or before since already-expired excluded).
	within0, err := pgStore.ListExpiringContracts(ctx, 0)
	if err != nil {
		t.Fatalf("ListExpiringContracts(0) error = %v", err)
	}
	if len(within0) != 0 {
		t.Fatalf("expected 0 contracts expiring within 0 days, got %d", len(within0))
	}
}

func TestPgStore_ListExpiredContracts(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgExpired := "org-expired-contract-" + newID()
	expired := makeContract(orgExpired, billing.EnterpriseTierStarter, time.Now().Add(-time.Hour))
	if err := pgStore.UpsertEnterpriseContract(ctx, expired); err != nil {
		t.Fatal(err)
	}

	orgFuture := "org-future-contract-" + newID()
	future := makeContract(orgFuture, billing.EnterpriseTierGrowth, time.Now().Add(24*time.Hour))
	if err := pgStore.UpsertEnterpriseContract(ctx, future); err != nil {
		t.Fatal(err)
	}

	got, err := pgStore.ListExpiredContracts(ctx)
	if err != nil {
		t.Fatalf("ListExpiredContracts() error = %v", err)
	}
	if len(got) != 1 || got[0].OrgID != orgExpired {
		t.Fatalf("expired contracts = %+v, want only %s", got, orgExpired)
	}
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
	if err := pgStore.UpsertEnterpriseContract(ctx, contract); err != nil {
		t.Fatalf("UpsertEnterpriseContract expired: %v", err)
	}

	renewed := *contract
	renewed.ContractEndDate = time.Now().UTC().Add(365 * 24 * time.Hour).Truncate(time.Microsecond)
	renewed.AutoRenew = true
	if err := pgStore.UpsertEnterpriseContract(ctx, &renewed); err != nil {
		t.Fatalf("UpsertEnterpriseContract renewed: %v", err)
	}

	restricted, err := pgStore.RestrictExpiredContractIfCurrent(ctx, orgID, observedEnd)
	if err != nil {
		t.Fatalf("RestrictExpiredContractIfCurrent() error = %v", err)
	}
	if restricted {
		t.Fatal("RestrictExpiredContractIfCurrent() restricted renewed contract")
	}
	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription() error = %v", err)
	}
	if sub.PaymentStatus == "restricted" {
		t.Fatal("payment status was restricted for stale expired contract")
	}
}

func TestPgStore_RestrictExpiredContractIfCurrentClearsEnterpriseEntitlements(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-expired-entitlements-" + newID()
	ensureSub(t, ctx, pgStore, orgID)
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanEnterprise), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan enterprise: %v", err)
	}
	mustEqualLimits(t, readEntitlements(t, ctx, orgID), billing.GetPlanLimits(domain.PlanEnterprise), "before expired contract restriction")

	observedEnd := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	contract := makeContract(orgID, billing.EnterpriseTierStarter, observedEnd)
	contract.AutoRenew = false
	if err := pgStore.UpsertEnterpriseContract(ctx, contract); err != nil {
		t.Fatalf("UpsertEnterpriseContract expired: %v", err)
	}

	restricted, err := pgStore.RestrictExpiredContractIfCurrent(ctx, orgID, observedEnd)
	if err != nil {
		t.Fatalf("RestrictExpiredContractIfCurrent() error = %v", err)
	}
	if !restricted {
		t.Fatal("expected expired non-renewing contract to be restricted")
	}
	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription() error = %v", err)
	}
	if sub.PaymentStatus != "restricted" {
		t.Fatalf("payment status = %q, want restricted", sub.PaymentStatus)
	}
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

	if err := pgStore.UpsertEnterpriseContract(ctx, cA); err != nil {
		t.Fatal(err)
	}
	if err := pgStore.UpsertEnterpriseContract(ctx, cB); err != nil {
		t.Fatal(err)
	}

	gotA, err := pgStore.GetEnterpriseContract(ctx, orgA)
	if err != nil {
		t.Fatal(err)
	}
	if gotA.EnterpriseTier != billing.EnterpriseTierStarter {
		t.Errorf("org A tier = %q, want starter", gotA.EnterpriseTier)
	}
	if gotA.Notes != "org A contract" {
		t.Errorf("org A notes leaked org B data: %q", gotA.Notes)
	}

	gotB, err := pgStore.GetEnterpriseContract(ctx, orgB)
	if err != nil {
		t.Fatal(err)
	}
	if gotB.EnterpriseTier != billing.EnterpriseTierLarge {
		t.Errorf("org B tier = %q, want large", gotB.EnterpriseTier)
	}
}

func TestPgStore_UpsertEnterpriseContract_NilStripeSubscriptionID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-nilsub-" + newID()
	c := makeContract(orgID, billing.EnterpriseTierStarter, time.Now().Add(365*24*time.Hour))
	c.StripeSubscriptionID = nil

	if err := pgStore.UpsertEnterpriseContract(ctx, c); err != nil {
		t.Fatalf("upsert with nil StripeSubscriptionID error = %v", err)
	}

	got, err := pgStore.GetEnterpriseContract(ctx, orgID)
	if err != nil {
		t.Fatal(err)
	}
	if got.StripeSubscriptionID != nil {
		t.Errorf("expected nil StripeSubscriptionID, got %v", *got.StripeSubscriptionID)
	}
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
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob(http) error = %v", err)
	}
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
	if err != nil {
		t.Fatalf("PauseHTTPJobsByOrg() error = %v", err)
	}
	if len(paused) != 3 {
		t.Fatalf("expected 3 paused, got %d", len(paused))
	}

	// Count HTTP jobs (should still be 3 -- paused but not deleted).
	count, err := pgStore.CountHTTPJobsByOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("CountHTTPJobsByOrg() error = %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 HTTP jobs, got %d", count)
	}
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

	if err := q.PauseJob(ctx, manualPaused.ID, "user_request"); err != nil {
		t.Fatalf("PauseJob() error = %v", err)
	}

	// Bulk pause should only affect 2 (skip already-paused).
	paused, err := pgStore.PauseHTTPJobsByOrg(ctx, orgID, "plan_downgrade")
	if err != nil {
		t.Fatalf("PauseHTTPJobsByOrg() error = %v", err)
	}
	if len(paused) != 2 {
		t.Fatalf("expected 2 paused (1 already paused), got %d", len(paused))
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if len(paused) != 3 {
		t.Fatalf("expected 3 paused, got %d", len(paused))
	}

	// Manually change one job's pause reason to simulate user-initiated pause.
	if _, err := testDB.Pool.Exec(ctx, "UPDATE jobs SET pause_reason = 'user_request' WHERE id = $1", manualJob.ID); err != nil {
		t.Fatal(err)
	}

	// Unpause only "plan_downgrade" jobs.
	unpaused, err := pgStore.UnpauseJobsByPauseReason(ctx, orgID, "plan_downgrade")
	if err != nil {
		t.Fatalf("UnpauseJobsByPauseReason() error = %v", err)
	}
	if unpaused != 2 {
		t.Fatalf("expected 2 unpaused, got %d", unpaused)
	}

	// The user-paused job should still be paused.
	got, err := q.GetJob(ctx, manualJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Paused {
		t.Error("expected manually-paused job to remain paused")
	}
	if got.PauseReason != "user_request" {
		t.Errorf("expected pause_reason 'user_request', got %q", got.PauseReason)
	}
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

	if countA != 2 {
		t.Errorf("org A count = %d, want 2", countA)
	}
	if countB != 1 {
		t.Errorf("org B count = %d, want 1", countB)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if len(paused) != 3 {
		t.Fatalf("expected 3 paused, got %d", len(paused))
	}

	got, _ := q.GetJob(ctx, h1.ID)
	if !got.Paused {
		t.Error("HTTP job should be paused")
	}
	if got.PauseReason != "plan_downgrade" {
		t.Errorf("pause_reason = %q, want plan_downgrade", got.PauseReason)
	}

	// Simulate upgrade enforcement restoring jobs paused for downgrade.
	unpaused, err := pgStore.UnpauseJobsByPauseReason(ctx, orgID, "plan_downgrade")
	if err != nil {
		t.Fatal(err)
	}
	if unpaused != 3 {
		t.Fatalf("expected 3 unpaused, got %d", unpaused)
	}

	got2, _ := q.GetJob(ctx, h1.ID)
	if got2.Paused {
		t.Error("HTTP job should be unpaused after upgrade")
	}
	if got2.PauseReason != "" {
		t.Errorf("pause_reason should be empty, got %q", got2.PauseReason)
	}
}

// H1 regression: ListOrgsInGracePeriod must include MonthlyUsageEmail

func TestPgStore_ListOrgsInGracePeriod_MonthlyUsageEmail(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-grace-email-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	if err := pgStore.UpdateMonthlyUsageEmail(ctx, orgID, true); err != nil {
		t.Fatalf("UpdateMonthlyUsageEmail: %v", err)
	}

	past := time.Now().UTC().Add(-48 * time.Hour)
	if err := pgStore.UpdatePaymentStatus(ctx, orgID, "grace", &past); err != nil {
		t.Fatalf("UpdatePaymentStatus: %v", err)
	}

	subs, err := pgStore.ListOrgsInGracePeriod(ctx)
	if err != nil {
		t.Fatalf("ListOrgsInGracePeriod: %v", err)
	}

	for _, s := range subs {
		if s.OrgID == orgID {
			if !s.MonthlyUsageEmail {
				t.Fatalf("expected MonthlyUsageEmail=true, got false")
			}
			return
		}
	}
	t.Fatal("org not found in grace period list")
}

// H1 regression: ListStaleSubscriptions must include MonthlyUsageEmail

func TestPgStore_ListStaleSubscriptions_MonthlyUsageEmail(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-stale-email-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	if err := pgStore.UpdateMonthlyUsageEmail(ctx, orgID, true); err != nil {
		t.Fatalf("UpdateMonthlyUsageEmail: %v", err)
	}

	staleEnd := time.Now().UTC().Add(-72 * time.Hour)
	if err := pgStore.UpdateOrgSubscriptionFull(ctx, orgID, "pro", "active", &staleEnd, &staleEnd); err != nil {
		t.Fatalf("UpdateOrgSubscriptionFull: %v", err)
	}

	subs, err := pgStore.ListStaleSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListStaleSubscriptions: %v", err)
	}

	for _, s := range subs {
		if s.OrgID == orgID {
			if !s.MonthlyUsageEmail {
				t.Fatalf("expected MonthlyUsageEmail=true, got false")
			}
			return
		}
	}
	t.Fatal("org not found in stale subscriptions list")
}
func TestPgStore_UpsertOrgSubscription_PreservesPendingPlanTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ppt-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	if err := pgStore.SetPendingPlanTier(ctx, orgID, "free"); err != nil {
		t.Fatalf("SetPendingPlanTier: %v", err)
	}

	sub := &billing.OrgSubscription{
		ID:          newID(),
		OrgID:       orgID,
		PlanTier:    "pro",
		Status:      "active",
		LimitAction: "notify",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := pgStore.UpsertOrgSubscription(ctx, sub); err != nil {
		t.Fatalf("UpsertOrgSubscription: %v", err)
	}

	got, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription: %v", err)
	}
	if got.PendingPlanTier == nil || *got.PendingPlanTier != "free" {
		t.Errorf("PendingPlanTier = %v, want 'free'", got.PendingPlanTier)
	}
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
	if err := pgStore.UpdateOrgSubscriptionFull(ctx, orgID, "pro", "active", &start, &end); err != nil {
		t.Fatalf("set initial period: %v", err)
	}

	if err := pgStore.UpdateOrgSubscriptionFull(ctx, orgID, "enterprise", "active", nil, nil); err != nil {
		t.Fatalf("update with nil periods: %v", err)
	}

	got, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription: %v", err)
	}
	if got.CurrentPeriodStart == nil || !got.CurrentPeriodStart.Equal(start) {
		t.Errorf("CurrentPeriodStart = %v, want %v", got.CurrentPeriodStart, start)
	}
	if got.CurrentPeriodEnd == nil || !got.CurrentPeriodEnd.Equal(end) {
		t.Errorf("CurrentPeriodEnd = %v, want %v", got.CurrentPeriodEnd, end)
	}
}

// M3: BulkCountExecutingRunsByOrg -- zero-run org absent from map

func TestPgStore_BulkCountExecutingRunsByOrg_ZeroRunOrgAbsent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-zero-runs-" + newID()
	result, err := pgStore.BulkCountExecutingRunsByOrg(ctx, []string{orgID})
	if err != nil {
		t.Fatalf("BulkCountExecutingRunsByOrg: %v", err)
	}
	if count, present := result[orgID]; present {
		t.Errorf("expected org absent from map, got count=%d", count)
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

	if err := pgStore.SetPendingDowngrade(ctx, orgPast, "free", &past, &past); err != nil {
		t.Fatalf("SetPendingDowngrade past: %v", err)
	}
	if err := pgStore.SetPendingDowngrade(ctx, orgFuture, "free", &future, &future); err != nil {
		t.Fatalf("SetPendingDowngrade future: %v", err)
	}

	subs, err := pgStore.ListOrgsWithPendingDowngrade(ctx)
	if err != nil {
		t.Fatalf("ListOrgsWithPendingDowngrade: %v", err)
	}

	var foundPast, foundFuture bool
	for _, s := range subs {
		if s.OrgID == orgPast {
			foundPast = true
		}
		if s.OrgID == orgFuture {
			foundFuture = true
		}
	}
	if !foundPast {
		t.Error("expected past-period org to appear")
	}
	if foundFuture {
		t.Error("future-period org should not appear")
	}
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

	if err := pgStore.UpdateOrgSubscriptionFull(ctx, orgNotStale, "pro", "active", &notStaleEnd, &notStaleEnd); err != nil {
		t.Fatalf("update not-stale: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionFull(ctx, orgStale, "pro", "active", &staleEnd, &staleEnd); err != nil {
		t.Fatalf("update stale: %v", err)
	}

	subs, err := pgStore.ListStaleSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListStaleSubscriptions: %v", err)
	}

	var foundNotStale, foundStale bool
	for _, s := range subs {
		if s.OrgID == orgNotStale {
			foundNotStale = true
		}
		if s.OrgID == orgStale {
			foundStale = true
		}
	}
	if !foundStale {
		t.Error("expected stale org (25h past) to appear")
	}
	if foundNotStale {
		t.Error("not-stale org (23h past) should not appear")
	}
}

// M6: GetProjectOrgID not found

func TestPgStore_GetProjectOrgID_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	_, err := pgStore.GetProjectOrgID(ctx, "nonexistent-project-"+newID())
	if err == nil {
		t.Error("expected error for non-existent project")
	}
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
	if err != nil {
		t.Fatalf("ListOrgAdminEmails: %v", err)
	}
	if len(emails) != 1 {
		t.Errorf("expected 1 deduped email, got %d: %v", len(emails), emails)
	}
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
	if err != nil {
		t.Fatalf("GetOrgUsageForPeriod: %v", err)
	}

	var totalRuns int64
	for _, r := range recs {
		totalRuns += r.RunsCount
	}
	if totalRuns != 1 {
		t.Errorf("expected 1 run for single-day, got %d", totalRuns)
	}
}

// L1: ApplyPendingDowngrade same tier (noop)

func TestPgStore_ApplyPendingDowngrade_SameTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-same-tier-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	if err := pgStore.SetPendingPlanTier(ctx, orgID, "free"); err != nil {
		t.Fatalf("SetPendingPlanTier: %v", err)
	}

	if err := pgStore.ApplyPendingDowngrade(ctx, orgID); err != nil {
		t.Fatalf("ApplyPendingDowngrade: %v", err)
	}

	got, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription: %v", err)
	}
	if got.PlanTier != "free" {
		t.Errorf("PlanTier = %q, want 'free'", got.PlanTier)
	}
	if got.PendingPlanTier != nil {
		t.Errorf("PendingPlanTier = %v, want nil", got.PendingPlanTier)
	}
}

// L2: Usage report dedup -- time truncation

func TestPgStore_UsageReportDedup_TimeTruncation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-dedup-trunc-" + newID()

	endOfDay := time.Date(2026, 3, 15, 23, 59, 59, 0, time.UTC)
	if err := pgStore.RecordSentUsageReport(ctx, orgID, endOfDay); err != nil {
		t.Fatalf("RecordSentUsageReport: %v", err)
	}

	startOfDay := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	sent, err := pgStore.HasSentUsageReport(ctx, orgID, startOfDay)
	if err != nil {
		t.Fatalf("HasSentUsageReport: %v", err)
	}
	if !sent {
		t.Error("expected dedup match after truncation (23:59:59 -> 00:00:00)")
	}
}

func TestPgStore_UsageReportClaimState_AllowsStaleClaimRetry(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-report-claim-" + newID()
	periodEnd := time.Date(2026, 4, 30, 23, 0, 0, 0, time.UTC)

	claimed, err := pgStore.ClaimUsageReportSend(ctx, orgID, periodEnd)
	if err != nil {
		t.Fatalf("ClaimUsageReportSend: %v", err)
	}
	if !claimed {
		t.Fatal("first ClaimUsageReportSend returned false, want true")
	}

	sent, err := pgStore.HasSentUsageReport(ctx, orgID, periodEnd)
	if err != nil {
		t.Fatalf("HasSentUsageReport after claim: %v", err)
	}
	if sent {
		t.Fatal("pre-send claim must not be treated as a sent usage report")
	}

	claimed, err = pgStore.ClaimUsageReportSend(ctx, orgID, periodEnd)
	if err != nil {
		t.Fatalf("second ClaimUsageReportSend: %v", err)
	}
	if claimed {
		t.Fatal("fresh claim should block duplicate sender")
	}

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE sent_usage_reports
		SET claimed_at = NOW() - INTERVAL '2 hours'
		WHERE org_id = $1 AND period_end = $2
	`, orgID, periodEnd.Truncate(24*time.Hour)); err != nil {
		t.Fatalf("age claim: %v", err)
	}

	claimed, err = pgStore.ClaimUsageReportSend(ctx, orgID, periodEnd)
	if err != nil {
		t.Fatalf("stale ClaimUsageReportSend: %v", err)
	}
	if !claimed {
		t.Fatal("stale claim should be claimable again")
	}

	if err := pgStore.FinalizeUsageReportSend(ctx, orgID, periodEnd); err != nil {
		t.Fatalf("FinalizeUsageReportSend: %v", err)
	}
	sent, err = pgStore.HasSentUsageReport(ctx, orgID, periodEnd)
	if err != nil {
		t.Fatalf("HasSentUsageReport after finalize: %v", err)
	}
	if !sent {
		t.Fatal("finalized report should be treated as sent")
	}

	claimed, err = pgStore.ClaimUsageReportSend(ctx, orgID, periodEnd)
	if err != nil {
		t.Fatalf("claim after finalized send: %v", err)
	}
	if claimed {
		t.Fatal("sent usage report should block future claims")
	}
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
	if err != nil {
		t.Fatalf("CountMembersByOrg: %v", err)
	}
	// Document behavior: members in soft-deleted projects are still counted.
	t.Logf("CountMembersByOrg with deleted project: %d (documents current behavior)", count)
	if count < 1 {
		t.Error("expected at least 1 member even with deleted project")
	}
}

// L4: DeleteOldWebhookMessages with future cutoff

func TestPgStore_DeleteOldWebhookMessages_NoRows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	msgID := "msg-future-" + newID()
	if err := pgStore.RecordProcessedWebhook(ctx, msgID); err != nil {
		t.Fatalf("RecordProcessedWebhook: %v", err)
	}

	future := time.Now().UTC().Add(-24 * time.Hour)
	deleted, err := pgStore.DeleteOldWebhookMessages(ctx, future)
	if err != nil {
		t.Fatalf("DeleteOldWebhookMessages: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (message is recent)", deleted)
	}
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
	if err != nil {
		t.Fatalf("SuspendExcessProjects: %v", err)
	}
	if count != 2 {
		t.Errorf("suspended = %d, want 2", count)
	}

	s1, _ := pgStore.IsProjectSuspended(ctx, p1.ID)
	s2, _ := pgStore.IsProjectSuspended(ctx, p2.ID)
	if !s1 || !s2 {
		t.Errorf("expected all suspended: p1=%v p2=%v", s1, s2)
	}
}

// L7: UpsertOrgSubscription preserves spending_limit

func TestPgStore_UpsertOrgSubscription_PreservesSpendingLimit(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-limit-preserve-" + newID()
	ensureSub(t, ctx, pgStore, orgID)

	if err := pgStore.UpdateSpendingLimit(ctx, orgID, 5_000_000, "enforce"); err != nil {
		t.Fatalf("UpdateSpendingLimit: %v", err)
	}

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
	if err := pgStore.UpsertOrgSubscription(ctx, sub); err != nil {
		t.Fatalf("UpsertOrgSubscription: %v", err)
	}

	got, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription: %v", err)
	}
	if got.SpendingLimitMicrousd != 5_000_000 {
		t.Errorf("SpendingLimitMicrousd = %d, want 5000000 (preserved)", got.SpendingLimitMicrousd)
	}
}

// L8: IsProjectSuspended non-existent project

func TestPgStore_IsProjectSuspended_NonExistent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	_, err := pgStore.IsProjectSuspended(ctx, "nonexistent-"+newID())
	if err == nil {
		t.Error("expected error for non-existent project")
	}
}
