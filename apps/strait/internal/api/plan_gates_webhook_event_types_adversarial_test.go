package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckWebhookEventTypes_InternalSecretBypass_Blocked is the canonical
// internal-secret bypass test. It confirms that the X-Internal-Secret header -
// which lets schedulers and workers skip the per-request project context check -
// does NOT bypass the plan-tier event-type gate.
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
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
	require.False(t, createCalled)
	assert.Contains(t,
		w.Body.String(), "not available")
}

// TestCheckWebhookEventTypes_PremiumEvent_BypassAttempt simulates the smuggle
// vector: an internal-secret caller targets a Starter-tier project and asks
// for a Pro-tier event type. The gate must reject and name the offending
// event type so audit logs can attribute the attempt.
func TestCheckWebhookEventTypes_PremiumEvent_BypassAttempt(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, _ *domain.WebhookSubscription) error {
			require.Fail(t,

				"CreateWebhookSubscription must not be reached")
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
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
	assert.Contains(t,
		w.Body.String(), "run.timed_out")
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
	require.Error(t, err)
	assert.Contains(t,
		err.Error(), "run.timed_out")
}
