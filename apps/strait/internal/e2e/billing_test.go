//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/api"
	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/webhook"
)

// TestE2E_Billing_SubscribeConsumeExceedUpgrade walks the full billing surface
// against the real HTTP layer:
//
//  1. subscribe: an org starts on the Starter plan (3 projects max).
//  2. consume:   creates 3 projects through POST /v1/projects/ — all succeed.
//  3. exceed:    a 4th project create returns 402 with the canonical
//     `quota_exceeded` body (kind=project_limit_reached, plan,
//     limit, current, upgrade_url).
//  4. upgrade:   the org plan flips to Pro (10 projects max) and the cache is
//     invalidated; the same request that was rejected before now
//     succeeds.
//
// This exercises the bridge end-to-end: handler returns a bare
// *billing.LimitError, the Huma bridge converts it to a structured 402 via
// writeTypedError/newQuotaExceeded, and the standard ErrorResponse envelope
// is bypassed because the SDK contract uses the raw quota_exceeded body.
func TestE2E_Billing_SubscribeConsumeExceedUpgrade(t *testing.T) {
	mustClean(t)

	ctx := context.Background()

	// Build a real billing enforcer rooted on the same DB/Redis the rest of
	// the e2e suite uses. We deliberately do NOT mutate the package-level
	// testServer because other tests rely on the no-enforcer path.
	pgStore := billing.NewPgStore(testEnv.DB.Pool)
	enforcer := billing.NewEnforcer(pgStore, testEnv.Redis.Client, slog.Default())

	billingServer := api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:       "test-secret-value",
			JWTSigningKey:        testJWTSigningKey,
			SecretEncryptionKey:  "test-encryption-key-32bytes!!!!",
			CORSAllowedOrigins:   []string{"*"},
			CORSAllowCredentials: false,
			MaxBulkTriggerItems:  500,
		},
		Store:           testStore,
		Queue:           testQueue,
		TxPool:          testEnv.DB.Pool,
		BillingEnforcer: enforcer,
	})

	orgID := "00000000-0000-0000-0000-0000000000e2"

	// (1) subscribe: lazy-create the org's subscription row and pin it to Starter.
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure subscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanStarter), "active"); err != nil {
		t.Fatalf("upgrade to starter: %v", err)
	}
	enforcer.InvalidateOrgCache(orgID)

	createProject := func(t *testing.T, projectID string) *httptest.ResponseRecorder {
		t.Helper()
		body := fmt.Sprintf(`{"id":%q,"org_id":%q,"name":%q}`, projectID, orgID, projectID)
		req := httptest.NewRequest(http.MethodPost, "/v1/projects/", strings.NewReader(body))
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		billingServer.ServeHTTP(w, req)
		return w
	}

	// (2) consume: three creates fill the Starter cap exactly.
	for i := range billing.MaxProjectsStarter {
		pid := fmt.Sprintf("proj-bill-%d-%s", i, newID())
		w := createProject(t, pid)
		if w.Code != http.StatusCreated {
			t.Fatalf("create project %d: status = %d, body = %s", i, w.Code, w.Body.String())
		}
	}

	// (3) exceed: the next create must surface as 402 with the canonical
	// quota_exceeded body, not the ErrorResponse envelope.
	overPID := "proj-bill-over-" + newID()
	w := createProject(t, overPID)
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("over-cap create: status = %d, want 402, body = %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode 402 body: %v", err)
	}
	if got["code"] != "quota_exceeded" {
		t.Errorf("code = %v, want quota_exceeded", got["code"])
	}
	if got["kind"] != "project_limit_reached" {
		t.Errorf("kind = %v, want project_limit_reached", got["kind"])
	}
	if got["limit"].(float64) != float64(billing.MaxProjectsStarter) {
		t.Errorf("limit = %v, want %d", got["limit"], billing.MaxProjectsStarter)
	}
	if got["plan"] != string(domain.PlanStarter) {
		t.Errorf("plan = %v, want %q", got["plan"], domain.PlanStarter)
	}
	if got["upgrade_url"] == "" || got["upgrade_url"] == nil {
		t.Errorf("upgrade_url is empty; clients rely on it to deep-link to checkout")
	}
	if _, leaked := got["error"]; leaked {
		t.Errorf("ErrorResponse envelope leaked into 402 body; SDKs expect the raw quota_exceeded shape")
	}

	// (4) upgrade: bump the plan, invalidate the cached limits, and confirm
	// the same request that was rejected one beat ago now succeeds.
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("upgrade to pro: %v", err)
	}
	enforcer.InvalidateOrgCache(orgID)

	w = createProject(t, overPID)
	if w.Code != http.StatusCreated {
		t.Fatalf("post-upgrade create: status = %d, want 201, body = %s", w.Code, w.Body.String())
	}
}

// TestE2E_Billing_CapDispatchToSubscriber closes the gap noted in
// orchestration_e2e_impl_test.go:278 — the cap-dispatch path was only
// covered by unit + integration tests, never by the e2e harness with a
// real *webhook.DeliveryWorker writing rows to webhook_deliveries. The
// test wires the production dispatcher chain end to end and asserts that
// crossing the spending-limit thresholds enqueues real delivery rows
// addressed to the project's subscription, with the canonical billing
// event envelope in the payload column.
//
//  1. Pro plan, $1.00 spending limit, action=disable.
//  2. Project subscribed to billing.cap_warning + billing.cap_reached +
//     billing.cap_disabled.
//  3. Seed usage_records at $0.85, call CheckSpendingLimit → expect one
//     pending delivery whose envelope.event_type == billing.cap_warning.
//  4. Bump usage past $1.00, call again → expect two more pending
//     deliveries (cap_reached + cap_disabled). Per-period dedup keeps
//     cap_warning from re-firing.
func TestE2E_Billing_CapDispatchToSubscriber(t *testing.T) {
	mustClean(t)
	ctx := context.Background()

	orgID := "org-cap-e2e-" + newID()
	projectID := "proj-cap-e2e-" + newID()
	subscriptionID := "sub-cap-e2e-" + newID()

	pgStore := billing.NewPgStore(testEnv.DB.Pool)
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure subscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("set plan: %v", err)
	}
	const spendingLimit = int64(1_000_000) // $1.00
	if err := pgStore.UpdateSpendingLimit(ctx, orgID, spendingLimit, "disable"); err != nil {
		t.Fatalf("set spending limit: %v", err)
	}

	// Seed project + project→org mapping. The enforcer's dispatch path
	// fans events out through ListProjectsByOrg, so the mapping row is
	// what guarantees the dispatcher actually finds our subscription.
	if _, err := testEnv.DB.Pool.Exec(ctx,
		`INSERT INTO projects (id, org_id, name, created_at, updated_at)
		 VALUES ($1, $2, 'cap-dispatch-e2e', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET org_id = EXCLUDED.org_id`,
		projectID, orgID,
	); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := pgStore.SetProjectOrgID(ctx, projectID, orgID); err != nil {
		t.Fatalf("set project org: %v", err)
	}

	sub := &domain.WebhookSubscription{
		ID:         subscriptionID,
		ProjectID:  projectID,
		WebhookURL: "https://example.com/cap-dispatch-e2e",
		EventTypes: []string{
			domain.WebhookEventBillingCapWarning,
			domain.WebhookEventBillingCapReached,
			domain.WebhookEventBillingCapDisabled,
		},
		Secret: "cap-e2e-secret",
		Active: true,
	}
	if err := testStore.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("create webhook subscription: %v", err)
	}

	// Real DeliveryWorker + BillingDispatcher — the same wiring main.go
	// builds. We do NOT start the delivery loop; the test only cares
	// about pending rows being enqueued, not delivered.
	worker := webhook.NewDeliveryWorker(testStore, slog.Default())
	dispatcher := webhook.NewBillingDispatcher(worker, pgStore, testStore, slog.Default())
	enforcer := billing.NewEnforcer(pgStore, testEnv.Redis.Client, slog.Default(),
		billing.WithBillingDispatcher(dispatcher))

	monthStart := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(), 1, 0, 0, 0, 0, time.UTC)

	// (3) Push spend to 85% — fires billing.cap_warning exactly once.
	if err := pgStore.UpsertUsageRecord(ctx, &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        projectID,
		PeriodDate:       monthStart,
		RunsCount:        1,
		ComputeCostMicro: 850_000,
	}); err != nil {
		t.Fatalf("seed usage record (warning): %v", err)
	}
	if err := enforcer.CheckSpendingLimit(ctx, orgID); err != nil {
		t.Fatalf("warning-pass CheckSpendingLimit returned %v, want nil", err)
	}

	warnings := readSubscriptionDeliveries(t, ctx, subscriptionID, domain.WebhookEventBillingCapWarning)
	if got := len(warnings); got != 1 {
		t.Fatalf("cap_warning deliveries = %d, want 1", got)
	}
	if warnings[0].PlanTier != string(domain.PlanPro) {
		t.Errorf("cap_warning envelope plan_tier = %q, want pro", warnings[0].PlanTier)
	}
	if warnings[0].OrgID != orgID {
		t.Errorf("cap_warning envelope org_id = %q, want %q", warnings[0].OrgID, orgID)
	}

	// (4) Push spend past the limit — fires cap_reached and (because
	// action=disable) cap_disabled. cap_warning is dedup-suppressed.
	if err := pgStore.UpsertUsageRecord(ctx, &billing.UsageRecord{
		ID:               newID(),
		OrgID:            orgID,
		ProjectID:        projectID,
		PeriodDate:       monthStart.Add(24 * time.Hour),
		RunsCount:        1,
		ComputeCostMicro: 700_000, // total 1_550_000 > $1.00 cap
	}); err != nil {
		t.Fatalf("seed usage record (over-cap): %v", err)
	}
	if err := enforcer.CheckSpendingLimit(ctx, orgID); err == nil {
		t.Fatalf("over-cap CheckSpendingLimit returned nil, want *LimitError")
	}

	reached := readSubscriptionDeliveries(t, ctx, subscriptionID, domain.WebhookEventBillingCapReached)
	if got := len(reached); got != 1 {
		t.Fatalf("cap_reached deliveries = %d, want 1", got)
	}
	disabled := readSubscriptionDeliveries(t, ctx, subscriptionID, domain.WebhookEventBillingCapDisabled)
	if got := len(disabled); got != 1 {
		t.Fatalf("cap_disabled deliveries = %d, want 1", got)
	}

	// Per-period dedup: a second CheckSpendingLimit must not re-enqueue
	// any of the three cap events.
	if err := enforcer.CheckSpendingLimit(ctx, orgID); err == nil {
		t.Fatalf("second over-cap CheckSpendingLimit returned nil, want *LimitError")
	}
	for _, evType := range []string{
		domain.WebhookEventBillingCapWarning,
		domain.WebhookEventBillingCapReached,
		domain.WebhookEventBillingCapDisabled,
	} {
		all := readSubscriptionDeliveries(t, ctx, subscriptionID, evType)
		if want := 1; len(all) != want {
			t.Errorf("%s deliveries after dedup pass = %d, want %d", evType, len(all), want)
		}
	}
}

// TestE2E_Billing_DunningStateTransitions wires a real *billing.Dunner
// against the production PgStore and asserts that one Tick is enough to
// march a long-stale row from step 1 to step 6, flip payment_status to
// suspended, and enqueue the billing.delinquent + billing.suspended
// outbound events at the project's subscription. This complements the
// unit dunner_test (fake store) and the spending-limit integration tests
// by exercising ProcessDueDunningRows + dispatcher on real Postgres.
func TestE2E_Billing_DunningStateTransitions(t *testing.T) {
	mustClean(t)
	ctx := context.Background()

	orgID := "org-dun-e2e-" + newID()
	projectID := "proj-dun-e2e-" + newID()
	subscriptionID := "sub-dun-e2e-" + newID()

	pgStore := billing.NewPgStore(testEnv.DB.Pool)
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure subscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("set plan: %v", err)
	}

	if _, err := testEnv.DB.Pool.Exec(ctx,
		`INSERT INTO projects (id, org_id, name, created_at, updated_at)
		 VALUES ($1, $2, 'dunning-e2e', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET org_id = EXCLUDED.org_id`,
		projectID, orgID,
	); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := pgStore.SetProjectOrgID(ctx, projectID, orgID); err != nil {
		t.Fatalf("set project org: %v", err)
	}

	sub := &domain.WebhookSubscription{
		ID:         subscriptionID,
		ProjectID:  projectID,
		WebhookURL: "https://example.com/dunning-e2e",
		EventTypes: []string{
			domain.WebhookEventBillingDelinquent,
			domain.WebhookEventBillingSuspended,
		},
		Secret: "dun-e2e-secret",
		Active: true,
	}
	if err := testStore.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("create webhook subscription: %v", err)
	}

	// Seed an active dunning cycle from 75 days ago. One Tick should
	// jump step 1 → step 6 (the schedule is monotone, last-step-wins).
	entered := time.Now().UTC().Add(-75 * 24 * time.Hour)
	if _, err := testEnv.DB.Pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET dunning_step = 1,
		    dunning_entered_at = $2,
		    dunning_last_tick_at = NULL,
		    dunning_resolved_at = NULL,
		    payment_status = 'grace',
		    updated_at = NOW()
		WHERE org_id = $1
	`, orgID, entered); err != nil {
		t.Fatalf("seed dunning row: %v", err)
	}

	worker := webhook.NewDeliveryWorker(testStore, slog.Default())
	dispatcher := webhook.NewBillingDispatcher(worker, pgStore, testStore, slog.Default())
	dunner := billing.NewDunner(pgStore, billing.WithDunnerDispatcher(dispatcher))

	if err := dunner.Tick(ctx); err != nil {
		t.Fatalf("dunner Tick: %v", err)
	}

	var (
		gotStep   int
		gotStatus string
	)
	if err := testEnv.DB.Pool.QueryRow(ctx, `
		SELECT dunning_step, COALESCE(payment_status, '')
		FROM organization_subscriptions
		WHERE org_id = $1
	`, orgID).Scan(&gotStep, &gotStatus); err != nil {
		t.Fatalf("read subscription after tick: %v", err)
	}
	if gotStep != billing.DunningStepDay74 {
		t.Errorf("dunning_step = %d, want %d", gotStep, billing.DunningStepDay74)
	}
	if gotStatus != "suspended" {
		t.Errorf("payment_status = %q, want suspended", gotStatus)
	}

	delinquent := readSubscriptionDeliveries(t, ctx, subscriptionID, domain.WebhookEventBillingDelinquent)
	if got := len(delinquent); got != 1 {
		t.Fatalf("billing.delinquent deliveries = %d, want 1", got)
	}
	suspended := readSubscriptionDeliveries(t, ctx, subscriptionID, domain.WebhookEventBillingSuspended)
	if got := len(suspended); got != 1 {
		t.Fatalf("billing.suspended deliveries = %d, want 1", got)
	}
	if delinquent[0].OrgID != orgID || suspended[0].OrgID != orgID {
		t.Errorf("envelope org_id mismatch: delinquent=%q suspended=%q want=%q",
			delinquent[0].OrgID, suspended[0].OrgID, orgID)
	}
}

// readSubscriptionDeliveries returns every webhook_deliveries row whose
// payload envelope matches the given event_type for the given subscription.
// The billing-event subscription path does not populate the event_type
// column on webhook_deliveries (only the run-webhook path does), so we
// decode the canonical billing envelope from payload and filter on
// envelope.event_type instead.
func readSubscriptionDeliveries(t *testing.T, ctx context.Context, subscriptionID, eventType string) []billing.BillingEventEnvelope {
	t.Helper()
	rows, err := testEnv.DB.Pool.Query(ctx, `
		SELECT payload
		FROM webhook_deliveries
		WHERE subscription_id = $1
		ORDER BY created_at ASC
	`, subscriptionID)
	if err != nil {
		t.Fatalf("query webhook_deliveries: %v", err)
	}
	defer rows.Close()

	out := make([]billing.BillingEventEnvelope, 0, 4)
	for rows.Next() {
		var payload json.RawMessage
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan delivery row: %v", err)
		}
		var env billing.BillingEventEnvelope
		if err := json.Unmarshal(payload, &env); err != nil {
			t.Fatalf("decode billing envelope: %v (raw=%s)", err, string(payload))
		}
		if env.EventType == eventType {
			out = append(out, env)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iter: %v", err)
	}
	return out
}
