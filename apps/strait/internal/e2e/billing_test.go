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

	"strait/internal/api"
	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"
)

// TestE2E_Billing_SubscribeConsumeExceedUpgrade walks the full billing surface
// against the real HTTP layer:
//
//  1. subscribe: an org starts on the Starter plan (3 projects max).
//  2. consume:   creates 3 projects through POST /v1/projects/ — all succeed.
//  3. exceed:    a 4th project create returns 402 with the canonical
//                `quota_exceeded` body (kind=project_limit_reached, plan,
//                limit, current, upgrade_url).
//  4. upgrade:   the org plan flips to Pro (10 projects max) and the cache is
//                invalidated; the same request that was rejected before now
//                succeeds.
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
