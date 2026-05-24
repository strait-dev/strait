package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// TestCheckWebhookEventTypes_InternalSecretBypass_Blocked is the canonical
// internal-secret bypass test for Phase 4.9. It confirms that the X-Internal-
// Secret header — which lets schedulers and workers skip the per-request
// project context check — does NOT bypass the plan-tier event-type gate.
// The gate fires on the project_id supplied in the request body, regardless
// of caller identity.
//
// This is intentional: the plan tier reflects the customer's contract, not
// the caller's identity. Internal callers operating on a customer project
// must respect the customer's tier — otherwise a leaked internal secret
// would let the leaker subscribe a Free-tier project to premium events.
func TestCheckWebhookEventTypes_InternalSecretBypass_Blocked(t *testing.T) {
	t.Parallel()

	createCalled := false
	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, _ *domain.WebhookSubscription) error {
			createCalled = true
			return nil
		},
	}
	// MaxWebhookEndpoints>0 lets the request reach the event-type gate; the
	// "none" level then rejects regardless of caller identity.
	limits := billing.OrgPlanLimits{DisplayName: "Free", WebhookEventLevel: "none", MaxWebhookEndpoints: 5}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"],"secret":"sekret"}`
	w := httptest.NewRecorder()
	// authedRequest already sets X-Internal-Secret — this is the leak scenario.
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from event-type gate; got %d: %s", w.Code, w.Body.String())
	}
	if createCalled {
		t.Fatal("CreateWebhookSubscription must not run when the gate rejects")
	}
	if !strings.Contains(w.Body.String(), "not available") {
		t.Errorf("error must mention plan unavailability, got: %s", w.Body.String())
	}
}

// TestCheckWebhookEventTypes_PremiumEvent_BypassAttempt simulates the smuggle
// vector: an internal-secret caller targets a Starter-tier project and asks
// for a Pro-tier event type. The gate must reject and name the offending
// event type so audit logs can attribute the attempt.
func TestCheckWebhookEventTypes_PremiumEvent_BypassAttempt(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, _ *domain.WebhookSubscription) error {
			t.Fatal("CreateWebhookSubscription must not be reached")
			return nil
		},
	}
	limits := billing.OrgPlanLimits{DisplayName: "Starter", WebhookEventLevel: "basic", MaxWebhookEndpoints: 5}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	// run.timed_out is not on the basic allowlist.
	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed","run.timed_out"],"secret":"sekret"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "run.timed_out") {
		t.Errorf("response must name the offending event type, got: %s", w.Body.String())
	}
}

// TestCheckWebhookEventTypes_LeadingValidEvent_StillRejectsTrailingPremium
// catches a sloppy fast-path implementation that might short-circuit on the
// first allowed event without iterating the rest.
func TestCheckWebhookEventTypes_LeadingValidEvent_StillRejectsTrailingPremium(t *testing.T) {
	t.Parallel()

	limits := billing.OrgPlanLimits{DisplayName: "Starter", WebhookEventLevel: "basic"}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	// First event passes; second must trip the gate.
	err := srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{"run.completed", "run.timed_out"})
	if err == nil {
		t.Fatal("gate must inspect every event in the slice, not short-circuit on the first allowed one")
	}
	if !strings.Contains(err.Error(), "run.timed_out") {
		t.Errorf("error must name the trailing offender, got: %v", err)
	}
}
