package billing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
)

func TestWebhookHandler_AgentSubscriptionCreated(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-agent-1": {OrgID: "org-agent-1", PlanTier: "free", AgentPlanTier: "agent_free"},
		},
	}
	mapping := NewStripeMappingFromOptions(
		WithAgentMakerPrices("agent-maker-monthly", "agent-maker-yearly"),
		WithAgentGrowthPrices("agent-growth-monthly", "agent-growth-yearly"),
		WithStarterPrices("starter-monthly", ""),
	)
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	body := buildAgentSubscriptionEvent(t, "customer.subscription.created", "agent-maker-monthly", "org-agent-1")
	sig := signStripeWebhook(t, testSecret, body)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sig)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
}

func TestWebhookHandler_AgentTierRouting_NotJobsTier(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-route-1": {OrgID: "org-route-1", PlanTier: "starter", AgentPlanTier: "agent_free"},
		},
	}
	mapping := NewStripeMappingFromOptions(
		WithAgentMakerPrices("agent-maker-price", ""),
		WithStarterPrices("starter-price", ""),
	)
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	// Agent price should route to agent handler, not jobs handler.
	body := buildAgentSubscriptionEvent(t, "customer.subscription.created", "agent-maker-price", "org-route-1")
	sig := signStripeWebhook(t, testSecret, body)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sig)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	// Verify the Jobs PlanTier was NOT changed (should still be "starter").
	sub := store.subscriptions["org-route-1"]
	if sub != nil && sub.PlanTier != "starter" {
		t.Errorf("Jobs PlanTier = %q, want starter (should not be changed by agent subscription)", sub.PlanTier)
	}
}

func TestWebhookHandler_AgentSubscriptionUpdated(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-update-1": {OrgID: "org-update-1", PlanTier: "starter", AgentPlanTier: "agent_maker"},
		},
	}
	mapping := NewStripeMappingFromOptions(
		WithAgentGrowthPrices("agent-growth-price", ""),
		WithStarterPrices("starter-price", ""),
	)
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	body := buildAgentSubscriptionEvent(t, "customer.subscription.updated", "agent-growth-price", "org-update-1")
	sig := signStripeWebhook(t, testSecret, body)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sig)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
}

func TestWebhookHandler_IsAgentPlan_Routing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tier    domain.PlanTier
		isAgent bool
	}{
		{"agent_free", domain.AgentPlanFree, true},
		{"agent_maker", domain.AgentPlanMaker, true},
		{"agent_growth", domain.AgentPlanGrowth, true},
		{"agent_enterprise", domain.AgentPlanEnterprise, true},
		{"jobs_free", domain.PlanFree, false},
		{"jobs_starter", domain.PlanStarter, false},
		{"jobs_pro", domain.PlanPro, false},
		{"jobs_scale", domain.PlanScale, false},
		{"jobs_enterprise", domain.PlanEnterprise, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.tier.IsAgentPlan() != tt.isAgent {
				t.Errorf("IsAgentPlan(%q) = %v, want %v", tt.tier, tt.tier.IsAgentPlan(), tt.isAgent)
			}
		})
	}
}

func TestStripeMappingAgentPrices(t *testing.T) {
	t.Parallel()

	mapping := NewStripeMappingFromOptions(
		WithAgentMakerPrices("maker-mo", "maker-yr"),
		WithAgentGrowthPrices("growth-mo", "growth-yr"),
		WithStarterPrices("starter-mo", ""),
	)

	tests := []struct {
		priceID  string
		wantTier domain.PlanTier
		wantOk   bool
	}{
		{"maker-mo", domain.AgentPlanMaker, true},
		{"maker-yr", domain.AgentPlanMaker, true},
		{"growth-mo", domain.AgentPlanGrowth, true},
		{"growth-yr", domain.AgentPlanGrowth, true},
		{"starter-mo", domain.PlanStarter, true},
		{"unknown-price", domain.PlanFree, false},
	}

	for _, tt := range tests {
		t.Run(tt.priceID, func(t *testing.T) {
			t.Parallel()
			tier, ok := mapping.TierForPrice(tt.priceID)
			if ok != tt.wantOk {
				t.Errorf("TierForPrice(%q) ok = %v, want %v", tt.priceID, ok, tt.wantOk)
			}
			if tier != tt.wantTier {
				t.Errorf("TierForPrice(%q) tier = %q, want %q", tt.priceID, tier, tt.wantTier)
			}
		})
	}
}

// buildAgentSubscriptionEvent creates a minimal Stripe webhook event payload for testing.
func buildAgentSubscriptionEvent(t *testing.T, eventType, priceID, orgID string) []byte {
	t.Helper()

	event := map[string]any{
		"type": eventType,
		"data": map[string]any{
			"object": map[string]any{
				"id":     "sub_agent_test_" + orgID,
				"status": "active",
				"items": map[string]any{
					"data": []map[string]any{
						{
							"price": map[string]any{
								"id": priceID,
							},
							"current_period_start": 1700000000,
							"current_period_end":   1702592000,
						},
					},
				},
				"customer": map[string]any{
					"id": fmt.Sprintf("cus_%s", orgID),
					"metadata": map[string]any{
						"org_id": orgID,
					},
				},
			},
		},
	}

	body, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal webhook event: %v", err)
	}
	return body
}
