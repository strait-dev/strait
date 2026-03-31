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
	testDB, err = testutil.SetupTestDB(ctx, "../../migrations")
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

// --------------------------------------------------------------------------.
// Test 1: EnsureOrgSubscription
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 2: GetOrgSubscription - assert all 21 fields
// --------------------------------------------------------------------------.

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
			polar_subscription_id = 'polar-sub-123',
			polar_customer_id = 'polar-cust-456',
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
	// Field 4: PolarSubscriptionID
	if sub.PolarSubscriptionID == nil || *sub.PolarSubscriptionID != "polar-sub-123" {
		t.Errorf("PolarSubscriptionID = %v, want %q", sub.PolarSubscriptionID, "polar-sub-123")
	}
	// Field 5: PolarCustomerID
	if sub.PolarCustomerID == nil || *sub.PolarCustomerID != "polar-cust-456" {
		t.Errorf("PolarCustomerID = %v, want %q", sub.PolarCustomerID, "polar-cust-456")
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

// --------------------------------------------------------------------------.
// Test 3: GetOrgSubscription not found
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 4: UpsertOrgSubscription
// --------------------------------------------------------------------------.

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
		PolarSubscriptionID:   ptr("polar-sub"),  //nolint:modernize
		PolarCustomerID:       ptr("polar-cust"), //nolint:modernize
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

// --------------------------------------------------------------------------.
// Test 5: UpdateOrgSubscriptionPlan
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 6: UpdateOrgSubscriptionPlan not found
// --------------------------------------------------------------------------.

func TestPgStore_UpdateOrgSubscriptionPlan_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.UpdateOrgSubscriptionPlan(ctx, "org-nonexistent", "pro", "active")
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
}

// --------------------------------------------------------------------------.
// Test 7: UpdateOrgSubscriptionFull
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 8: UpdateOrgSubscriptionFull not found
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 9: SetPendingPlanTier
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 10: SetPendingPlanTier not found
// --------------------------------------------------------------------------.

func TestPgStore_SetPendingPlanTier_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.SetPendingPlanTier(ctx, "org-nonexistent", "free")
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
}

// --------------------------------------------------------------------------.
// Test 11: SetPendingDowngrade
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 12: ClearPendingPlanTier
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 13: ClearPendingPlanTier not found
// --------------------------------------------------------------------------.

func TestPgStore_ClearPendingPlanTier_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.ClearPendingPlanTier(ctx, "org-nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
}

// --------------------------------------------------------------------------.
// Test 14: ApplyPendingDowngrade
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 15: ApplyPendingDowngrade no pending
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 16: ListOrgsWithPendingDowngrade
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 17: UpdateSpendingLimit
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 18: UpdateSpendingLimit not found
// --------------------------------------------------------------------------.

func TestPgStore_UpdateSpendingLimit_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.UpdateSpendingLimit(ctx, "org-nonexistent", 100, "notify")
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
}

// --------------------------------------------------------------------------.
// Test 19: GetProjectOrgID
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 20: GetActiveProjectOrgID
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 21: ListProjectsByOrg
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 22: CountProjectsByOrg
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 23: CountOrgsByUser
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 24: BulkCountExecutingRunsByOrg
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 25: BulkCountExecutingRunsByOrg empty
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 26: SetProjectOrgID
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 27: UpsertUsageRecord
// --------------------------------------------------------------------------.

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
		AITokensTotal:    1000,
		AICostMicro:      500_000,
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
		AITokensTotal:    200,
		AICostMicro:      100_000,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := pgStore.UpsertUsageRecord(ctx, rec2); err != nil {
		t.Fatalf("second UpsertUsageRecord error = %v", err)
	}

	// Read back via raw SQL.
	var runs, compute, tokens, ai int64
	err := testDB.Pool.QueryRow(ctx,
		"SELECT runs_count, compute_cost_microusd, ai_tokens_total, ai_cost_microusd FROM usage_records WHERE org_id = $1 AND project_id = $2 AND period_date = $3",
		orgID, p.ID, day).Scan(&runs, &compute, &tokens, &ai)
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
		t.Errorf("ai_tokens = %d, want 1200", tokens)
	}
	if ai != 600_000 {
		t.Errorf("ai_cost = %d, want 600000", ai)
	}
}

// --------------------------------------------------------------------------.
// Test 28: SumOrgPeriodSpend
// --------------------------------------------------------------------------.

func TestPgStore_SumOrgPeriodSpend(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-sumspend-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	job := createJob(t, ctx, q, p.ID)
	run := createRun(t, ctx, q, job, domain.StatusCompleted)

	cu := &domain.RunComputeUsage{
		ID:            newID(),
		RunID:         run.ID,
		ProjectID:     p.ID,
		JobID:         job.ID,
		MachinePreset: "micro",
		MachineID:     "m1",
		DurationSecs:  60,
		CostMicrousd:  10_000_000,
	}
	if err := q.CreateRunComputeUsage(ctx, cu); err != nil {
		t.Fatalf("CreateRunComputeUsage: %v", err)
	}

	from := time.Now().UTC().Add(-1 * time.Hour)
	total, err := pgStore.SumOrgPeriodSpend(ctx, orgID, from)
	if err != nil {
		t.Fatalf("SumOrgPeriodSpend error = %v", err)
	}
	if total != 10_000_000 {
		t.Errorf("SumOrgPeriodSpend = %d, want 10000000", total)
	}
}

// --------------------------------------------------------------------------.
// Test 29: GetProjectBudget (no row)
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 30: SetProjectBudget and GetProjectBudget
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 31: GetProjectPeriodSpend
// --------------------------------------------------------------------------.

func TestPgStore_GetProjectPeriodSpend(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)

	orgID := "org-projspend-" + newID()
	p := createProject(t, ctx, q, orgID, "P")
	job := createJob(t, ctx, q, p.ID)
	run := createRun(t, ctx, q, job, domain.StatusCompleted)

	cu := &domain.RunComputeUsage{
		ID:            newID(),
		RunID:         run.ID,
		ProjectID:     p.ID,
		JobID:         job.ID,
		MachinePreset: "micro",
		MachineID:     "m1",
		DurationSecs:  30,
		CostMicrousd:  3_000_000,
	}
	if err := q.CreateRunComputeUsage(ctx, cu); err != nil {
		t.Fatalf("CreateRunComputeUsage: %v", err)
	}

	from := time.Now().UTC().Add(-1 * time.Hour)
	total, err := pgStore.GetProjectPeriodSpend(ctx, p.ID, from)
	if err != nil {
		t.Fatalf("GetProjectPeriodSpend error = %v", err)
	}
	if total != 3_000_000 {
		t.Errorf("GetProjectPeriodSpend = %d, want 3000000", total)
	}
}

// --------------------------------------------------------------------------.
// Test 32: UpdateAnomalyThresholds
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 33: UpdateAnomalyThresholds not found
// --------------------------------------------------------------------------.

func TestPgStore_UpdateAnomalyThresholds_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	err := pgStore.UpdateAnomalyThresholds(ctx, "org-nonexistent", 5.0, 15.0)
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
}

// --------------------------------------------------------------------------.
// Test 34: ListAllSubscribedOrgIDs
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 35: UpdatePaymentStatus
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 36: ListOrgsInGracePeriod
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 37: ListStaleSubscriptions
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 38: IsProjectSuspended
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 39: SuspendExcessProjects
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 40: ListOrgAdminEmails
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 41: HasSentUsageReport and RecordSentUsageReport
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 42: UpdateMonthlyUsageEmail
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 43: ListActiveAddons
// --------------------------------------------------------------------------.

func TestPgStore_ListActiveAddons(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-addons-" + newID()

	a1 := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonConcurrentRuns,
		Quantity:  5,
		Active:    true,
	}
	a2 := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonMembers,
		Quantity:  10,
		Active:    true,
	}
	aInactive := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonDataRetention,
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

// --------------------------------------------------------------------------.
// Test 44: DeactivateAddon
// --------------------------------------------------------------------------.

func TestPgStore_DeactivateAddon(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-deactaddon-" + newID()
	a := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonConcurrentRuns,
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

// --------------------------------------------------------------------------.
// Test 45: CountActiveAddonsByType
// --------------------------------------------------------------------------.

func TestPgStore_CountActiveAddonsByType(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-cntaddons-" + newID()
	for i := range 3 {
		a := &billing.Addon{
			ID:        newID(),
			OrgID:     orgID,
			AddonType: billing.AddonConcurrentRuns,
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
		AddonType: billing.AddonConcurrentRuns,
		Quantity:  1,
		Active:    false,
	}
	if err := pgStore.CreateAddon(ctx, aInact); err != nil {
		t.Fatalf("CreateAddon inactive error = %v", err)
	}

	count, err := pgStore.CountActiveAddonsByType(ctx, orgID, billing.AddonConcurrentRuns)
	if err != nil {
		t.Fatalf("CountActiveAddonsByType error = %v", err)
	}
	if count != 3 {
		t.Errorf("CountActiveAddonsByType = %d, want 3", count)
	}

	// Different type should be 0.
	count2, err := pgStore.CountActiveAddonsByType(ctx, orgID, billing.AddonMembers)
	if err != nil {
		t.Fatalf("CountActiveAddonsByType members error = %v", err)
	}
	if count2 != 0 {
		t.Errorf("CountActiveAddonsByType members = %d, want 0", count2)
	}
}

// --------------------------------------------------------------------------.
// Test 46: RecordProcessedWebhook and IsWebhookProcessed
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 47: DeleteOldWebhookMessages
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 48: DeactivateExcessWebhookSubscriptions
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 49: DeactivateExcessEnvironments
// --------------------------------------------------------------------------.

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

// --------------------------------------------------------------------------.
// Test 50: DeactivateExcessCronJobs
// --------------------------------------------------------------------------.

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
	if deactivated != 3 {
		t.Errorf("DeactivateExcessCronJobs = %d, want 3", deactivated)
	}
}

// --------------------------------------------------------------------------.
// Existing tests (kept from original file)
// --------------------------------------------------------------------------.

func TestPgStore_AggregatesComputeAndAIUsage(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustQueries(t)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-usage"
	projectA := createProject(t, ctx, q, orgID, "Project A")
	projectB := createProject(t, ctx, q, orgID, "Project B")
	_ = createProject(t, ctx, q, "org-other", "Project Other")

	jobA := createJob(t, ctx, q, projectA.ID)
	jobB := createJob(t, ctx, q, projectB.ID)
	jobOther := createJob(t, ctx, q, createProject(t, ctx, q, "org-other", "Project Other With Usage").ID)

	day1 := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)

	runA1 := createRun(t, ctx, q, jobA, domain.StatusCompleted)
	runB1 := createRun(t, ctx, q, jobB, domain.StatusCompleted)
	runA2 := createRun(t, ctx, q, jobA, domain.StatusCompleted)
	runOther := createRun(t, ctx, q, jobOther, domain.StatusCompleted)
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $2 WHERE id = $1`, runA1.ID, day1); err != nil {
		t.Fatalf("set runA1 created_at error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $2 WHERE id = $1`, runB1.ID, day1); err != nil {
		t.Fatalf("set runB1 created_at error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $2 WHERE id = $1`, runA2.ID, day2); err != nil {
		t.Fatalf("set runA2 created_at error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $2 WHERE id = $1`, runOther.ID, day1); err != nil {
		t.Fatalf("set runOther created_at error = %v", err)
	}

	computeA1 := &domain.RunComputeUsage{
		ID:            newID(),
		RunID:         runA1.ID,
		ProjectID:     projectA.ID,
		JobID:         jobA.ID,
		MachinePreset: "micro",
		MachineID:     "machine-a1",
		DurationSecs:  30,
		CostMicrousd:  2_000_000,
	}
	if err := q.CreateRunComputeUsage(ctx, computeA1); err != nil {
		t.Fatalf("CreateRunComputeUsage(projectA/day1) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_compute_usage SET created_at = $2 WHERE id = $1`, computeA1.ID, day1); err != nil {
		t.Fatalf("set computeA1 created_at error = %v", err)
	}

	aiA1 := &domain.RunUsage{
		ID:               newID(),
		RunID:            runA1.ID,
		Provider:         "openai",
		Model:            "gpt-5.4-mini",
		PromptTokens:     600,
		CompletionTokens: 400,
		TotalTokens:      1000,
		CostMicrousd:     1_000_000,
	}
	if err := q.CreateRunUsage(ctx, aiA1); err != nil {
		t.Fatalf("CreateRunUsage(projectA/day1) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_usage SET created_at = $2 WHERE id = $1`, aiA1.ID, day1); err != nil {
		t.Fatalf("set aiA1 created_at error = %v", err)
	}

	computeB1 := &domain.RunComputeUsage{
		ID:            newID(),
		RunID:         runB1.ID,
		ProjectID:     projectB.ID,
		JobID:         jobB.ID,
		MachinePreset: "small",
		MachineID:     "machine-b1",
		DurationSecs:  45,
		CostMicrousd:  3_000_000,
	}
	if err := q.CreateRunComputeUsage(ctx, computeB1); err != nil {
		t.Fatalf("CreateRunComputeUsage(projectB/day1) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_compute_usage SET created_at = $2 WHERE id = $1`, computeB1.ID, day1); err != nil {
		t.Fatalf("set computeB1 created_at error = %v", err)
	}

	aiB1 := &domain.RunUsage{
		ID:               newID(),
		RunID:            runB1.ID,
		Provider:         "openai",
		Model:            "gpt-5.4-mini",
		PromptTokens:     300,
		CompletionTokens: 400,
		TotalTokens:      700,
		CostMicrousd:     500_000,
	}
	if err := q.CreateRunUsage(ctx, aiB1); err != nil {
		t.Fatalf("CreateRunUsage(projectB/day1) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_usage SET created_at = $2 WHERE id = $1`, aiB1.ID, day1); err != nil {
		t.Fatalf("set aiB1 created_at error = %v", err)
	}

	aiA2 := &domain.RunUsage{
		ID:               newID(),
		RunID:            runA2.ID,
		Provider:         "openai",
		Model:            "gpt-5.4-mini",
		PromptTokens:     100,
		CompletionTokens: 200,
		TotalTokens:      300,
		CostMicrousd:     250_000,
	}
	if err := q.CreateRunUsage(ctx, aiA2); err != nil {
		t.Fatalf("CreateRunUsage(projectA/day2) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_usage SET created_at = $2 WHERE id = $1`, aiA2.ID, day2); err != nil {
		t.Fatalf("set aiA2 created_at error = %v", err)
	}

	aiOther := &domain.RunUsage{
		ID:               newID(),
		RunID:            runOther.ID,
		Provider:         "openai",
		Model:            "gpt-5.4-mini",
		PromptTokens:     50,
		CompletionTokens: 50,
		TotalTokens:      100,
		CostMicrousd:     75_000,
	}
	if err := q.CreateRunUsage(ctx, aiOther); err != nil {
		t.Fatalf("CreateRunUsage(projectOther/day1) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_usage SET created_at = $2 WHERE id = $1`, aiOther.ID, day1); err != nil {
		t.Fatalf("set aiOther created_at error = %v", err)
	}

	orgRecords, err := pgStore.GetOrgUsageForPeriod(ctx, orgID, day1.Add(-time.Hour), day2.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetOrgUsageForPeriod() error = %v", err)
	}
	if len(orgRecords) != 3 {
		t.Fatalf("GetOrgUsageForPeriod() len = %d, want 3", len(orgRecords))
	}

	recordMap := make(map[string]billing.UsageRecord, len(orgRecords))
	for _, record := range orgRecords {
		key := record.ProjectID + ":" + record.PeriodDate.Format("2006-01-02")
		recordMap[key] = record
	}

	day1A := recordMap[projectA.ID+":2026-03-10"]
	if day1A.RunsCount != 1 || day1A.ComputeCostMicro != 2_000_000 || day1A.AITokensTotal != 1000 || day1A.AICostMicro != 1_000_000 {
		t.Fatalf("unexpected project A day 1 aggregate: %+v", day1A)
	}
	day1B := recordMap[projectB.ID+":2026-03-10"]
	if day1B.RunsCount != 1 || day1B.ComputeCostMicro != 3_000_000 || day1B.AITokensTotal != 700 || day1B.AICostMicro != 500_000 {
		t.Fatalf("unexpected project B day 1 aggregate: %+v", day1B)
	}
	day2A := recordMap[projectA.ID+":2026-03-11"]
	if day2A.RunsCount != 1 || day2A.ComputeCostMicro != 0 || day2A.AITokensTotal != 300 || day2A.AICostMicro != 250_000 {
		t.Fatalf("unexpected project A day 2 aggregate: %+v", day2A)
	}

	projectARecords, err := pgStore.GetProjectUsageForPeriod(ctx, projectA.ID, day1.Add(-time.Hour), day2.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetProjectUsageForPeriod() error = %v", err)
	}
	if len(projectARecords) != 2 {
		t.Fatalf("GetProjectUsageForPeriod() len = %d, want 2", len(projectARecords))
	}

	day1Records, err := pgStore.GetOrgDailyUsage(ctx, orgID, day1)
	if err != nil {
		t.Fatalf("GetOrgDailyUsage() error = %v", err)
	}
	if len(day1Records) != 2 {
		t.Fatalf("GetOrgDailyUsage() len = %d, want 2", len(day1Records))
	}

	day1Start := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	day2Start := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	day3Start := time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)

	day1Count, err := pgStore.CountAIModelCallsByOrg(ctx, orgID, day1Start, day2Start)
	if err != nil {
		t.Fatalf("CountAIModelCallsByOrg(day1) error = %v", err)
	}
	if day1Count != 2 {
		t.Fatalf("CountAIModelCallsByOrg(day1) = %d, want 2", day1Count)
	}

	day2Count, err := pgStore.CountAIModelCallsByOrg(ctx, orgID, day2Start, day3Start)
	if err != nil {
		t.Fatalf("CountAIModelCallsByOrg(day2) error = %v", err)
	}
	if day2Count != 1 {
		t.Fatalf("CountAIModelCallsByOrg(day2) = %d, want 1", day2Count)
	}
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
