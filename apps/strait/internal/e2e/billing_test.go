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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx, orgID,
			string(domain.
				PlanStarter,
			), "active"))

	// (1) subscribe: lazy-create the org's subscription row and pin it to Starter.

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
		require.Equal(t, http.
			StatusCreated,
			w.Code,
		)

	}

	// (3) exceed: the next create must surface as 402 with the canonical
	// quota_exceeded body, not the ErrorResponse envelope.
	overPID := "proj-bill-over-" + newID()
	w := createProject(t, overPID)
	require.Equal(t, http.
		StatusPaymentRequired,

		w.Code)

	var got map[string]any
	require.NoError(t, json.
		NewDecoder(w.Body).
		Decode(&got))
	assert.Equal(t, "quota_exceeded",

		got["code"])
	assert.Equal(t, "project_limit_reached",

		got["kind"])
	assert.Equal(t, float64(billing.
		MaxProjectsStarter,
	), got["limit"].(float64))
	assert.Equal(t, string(domain.PlanStarter),
		got["plan"])
	assert.False(t, got["upgrade_url"] == "" ||
		got["upgrade_url"] ==
			nil)

	if _, leaked := got["error"]; leaked {
		assert.Failf(t, "test failure",

			"ErrorResponse envelope leaked into 402 body; SDKs expect the raw quota_exceeded shape")
	}
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx, orgID,
			string(domain.
				PlanPro), "active",
		))

	// (4) upgrade: bump the plan, invalidate the cached limits, and confirm
	// the same request that was rejected one beat ago now succeeds.

	enforcer.InvalidateOrgCache(orgID)

	w = createProject(t, overPID)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

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
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx, orgID,
			string(domain.
				PlanPro), "active",
		))

	const spendingLimit = int64(1_000_000)
	require.NoError(t, pgStore.
		UpdateSpendingLimit(ctx, orgID,
			spendingLimit,
			"disable",
		))

	// $1.00

	// Seed project + project→org mapping. The enforcer's dispatch path
	// fans events out through ListProjectsByOrg, so the mapping row is
	// what guarantees the dispatcher actually finds our subscription.
	if _, err := testEnv.DB.Pool.Exec(ctx,
		`INSERT INTO projects (id, org_id, name, created_at, updated_at)
		 VALUES ($1, $2, 'cap-dispatch-e2e', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET org_id = EXCLUDED.org_id`,
		projectID, orgID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert project: %v", err)
	}
	require.NoError(t, pgStore.
		SetProjectOrgID(ctx, projectID,
			orgID))

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
	require.NoError(t, testStore.
		CreateWebhookSubscription(ctx,
			sub))

	// Real DeliveryWorker + BillingDispatcher — the same wiring main.go
	// builds. We do NOT start the delivery loop; the test only cares
	// about pending rows being enqueued, not delivered.
	worker := webhook.NewDeliveryWorker(testStore, slog.Default())
	dispatcher := webhook.NewBillingDispatcher(worker, pgStore, testStore, slog.Default())
	enforcer := billing.NewEnforcer(pgStore, testEnv.Redis.Client, slog.Default(),
		billing.WithBillingDispatcher(dispatcher))

	monthStart := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, pgStore.
		UpsertUsageRecord(ctx, &billing.
			UsageRecord{ID: newID(), OrgID: orgID, ProjectID: projectID,
			PeriodDate: monthStart,

			RunsCount: 1, ComputeCostMicro: 850_000}))
	require.NoError(t, enforcer.
		CheckSpendingLimit(ctx, orgID))

	// (3) Push spend to 85% — fires billing.cap_warning exactly once.

	warnings := readSubscriptionDeliveries(t, ctx, subscriptionID, domain.WebhookEventBillingCapWarning)
	require.EqualValues(t, 1, len(warnings))
	assert.Equal(t, string(domain.PlanPro), warnings[0].PlanTier)
	assert.Equal(t, orgID,

		warnings[0].OrgID)
	require.NoError(t, pgStore.
		UpsertUsageRecord(ctx, &billing.
			UsageRecord{ID: newID(), OrgID: orgID, ProjectID: projectID,
			PeriodDate: monthStart.
				Add(24 * time.Hour), RunsCount: 1, ComputeCostMicro: 700_000}))
	require.Error(t, enforcer.
		CheckSpendingLimit(ctx, orgID))

	// (4) Push spend past the limit — fires cap_reached and (because
	// action=disable) cap_disabled. cap_warning is dedup-suppressed.

	// total 1_550_000 > $1.00 cap

	reached := readSubscriptionDeliveries(t, ctx, subscriptionID, domain.WebhookEventBillingCapReached)
	require.EqualValues(t, 1, len(reached))

	disabled := readSubscriptionDeliveries(t, ctx, subscriptionID, domain.WebhookEventBillingCapDisabled)
	require.EqualValues(t, 1, len(disabled))
	require.Error(t, enforcer.
		CheckSpendingLimit(ctx, orgID))

	// Per-period dedup: a second CheckSpendingLimit must not re-enqueue
	// any of the three cap events.

	for _, evType := range []string{
		domain.WebhookEventBillingCapWarning,
		domain.WebhookEventBillingCapReached,
		domain.WebhookEventBillingCapDisabled,
	} {
		all := readSubscriptionDeliveries(t, ctx, subscriptionID, evType)
		assert.Equal(t, len(all), 1)

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
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx, orgID,
			string(domain.
				PlanPro), "active",
		))

	if _, err := testEnv.DB.Pool.Exec(ctx,
		`INSERT INTO projects (id, org_id, name, created_at, updated_at)
		 VALUES ($1, $2, 'dunning-e2e', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET org_id = EXCLUDED.org_id`,
		projectID, orgID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert project: %v", err)
	}
	require.NoError(t, pgStore.
		SetProjectOrgID(ctx, projectID,
			orgID))

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
	require.NoError(t, testStore.
		CreateWebhookSubscription(ctx,
			sub))

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
		require.Failf(t, "test failure",

			"seed dunning row: %v", err)
	}

	worker := webhook.NewDeliveryWorker(testStore, slog.Default())
	dispatcher := webhook.NewBillingDispatcher(worker, pgStore, testStore, slog.Default())
	dunner := billing.NewDunner(pgStore, billing.WithDunnerDispatcher(dispatcher))
	require.NoError(t, dunner.
		Tick(
			ctx))

	var (
		gotStep   int
		gotStatus string
	)
	require.NoError(t, testEnv.
		DB.Pool.
		QueryRow(ctx, `
		SELECT dunning_step, COALESCE(payment_status, '')
		FROM organization_subscriptions
		WHERE org_id = $1
	`,

			orgID,
		).Scan(&gotStep, &gotStatus))
	assert.Equal(t, billing.
		DunningStepDay74,

		gotStep)
	assert.Equal(t, "suspended",

		gotStatus,
	)

	delinquent := readSubscriptionDeliveries(t, ctx, subscriptionID, domain.WebhookEventBillingDelinquent)
	require.EqualValues(t, 1, len(delinquent))

	suspended := readSubscriptionDeliveries(t, ctx, subscriptionID, domain.WebhookEventBillingSuspended)
	require.EqualValues(t, 1, len(suspended))
	assert.False(t, delinquent[0].OrgID !=
		orgID ||
		suspended[0].OrgID !=
			orgID)

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
	require.NoError(t, err)

	defer rows.Close()

	out := make([]billing.BillingEventEnvelope, 0, 4)
	for rows.Next() {
		var payload json.RawMessage
		require.NoError(t, rows.
			Scan(&payload))

		var env billing.BillingEventEnvelope
		require.NoError(t, json.
			Unmarshal(payload,
				&env))

		if env.EventType == eventType {
			out = append(out, env)
		}
	}
	require.NoError(t, rows.
		Err())

	return out
}
