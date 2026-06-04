package billing

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
)

// TestWebhookHandler_SubscriptionCreated_BusinessTier_ByLookupKey exercises the
// lookup-key path end-to-end: a Stripe subscription whose first item price has
// lookup_key=strait_business_monthly resolves to the Business tier without any
// per-account price ID being registered in StripeMapping.
func TestWebhookHandler_SubscriptionCreated_BusinessTier_ByLookupKey(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	// Empty mapping -- resolution must come from the catalog resolver.
	mapping := NewStripeMappingFromOptions()
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_business",
			ProductID:  "price_unmapped_per_account_id",
			LookupKey:  "strait_business_monthly",
			CustomerID: "cust_business",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000010"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if store.lastUpserted == nil {
		t.Fatal("expected subscription to be upserted")
	}
	if store.lastUpserted.PlanTier != string(domain.PlanBusiness) {
		t.Errorf("plan_tier = %q, want %q", store.lastUpserted.PlanTier, domain.PlanBusiness)
	}
	if store.lastUpserted.StripeLookupKey == nil {
		t.Fatal("expected stripe_lookup_key to be set on upsert")
	}
	if got := *store.lastUpserted.StripeLookupKey; got != "strait_business_monthly" {
		t.Errorf("stripe_lookup_key = %q, want strait_business_monthly", got)
	}
}

// TestWebhookHandler_SubscriptionCreated_FallbackPriceID confirms that when a
// subscription has no lookup_key, resolution falls back to the per-account
// price ID mapping (legacy path).
func TestWebhookHandler_SubscriptionCreated_FallbackPriceID(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMappingFromOptions(WithProPrices("legacy-pro-price", ""))
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_legacy",
			ProductID:  "legacy-pro-price",
			CustomerID: "cust_legacy",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000011"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if store.lastUpserted == nil {
		t.Fatal("expected subscription to be upserted")
	}
	if store.lastUpserted.PlanTier != string(domain.PlanPro) {
		t.Errorf("plan_tier = %q, want pro", store.lastUpserted.PlanTier)
	}
	if store.lastUpserted.StripeLookupKey != nil {
		t.Errorf("stripe_lookup_key should be nil on fallback, got %q", *store.lastUpserted.StripeLookupKey)
	}
}

// TestWebhookHandler_AddonByLookupKey resolves an addon subscription via the
// catalog resolver instead of per-account price ID.
func TestWebhookHandler_AddonByLookupKey(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000020": {
				OrgID:    "00000000-0000-0000-0000-000000000020",
				PlanTier: string(domain.PlanPro),
				Status:   "active",
			},
		},
	}
	mapping := NewStripeMappingFromOptions()
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_addon_lk",
			ProductID:  "price_unmapped",
			LookupKey:  "strait_addon_concurrency_100",
			CustomerID: "cust_addon",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000020"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if store.lastAddonCreated == nil {
		t.Fatal("expected addon to be created via lookup-key resolution")
	}
	if store.lastAddonCreated.AddonType != AddonConcurrency100 {
		t.Errorf("addon_type = %q, want %q", store.lastAddonCreated.AddonType, AddonConcurrency100)
	}
	if store.lastAddonCreated.StripeLookupKey == nil {
		t.Fatal("expected stripe_lookup_key to be set on addon")
	}
	if got := *store.lastAddonCreated.StripeLookupKey; got != "strait_addon_concurrency_100" {
		t.Errorf("addon stripe_lookup_key = %q, want strait_addon_concurrency_100", got)
	}
}

func TestWebhookHandler_RoadmapAddonLookupKeyRejected(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000021": {
				OrgID:    "00000000-0000-0000-0000-000000000021",
				PlanTier: string(domain.PlanPro),
				Status:   "active",
			},
		},
	}
	handler := NewWebhookHandler(store, NewStripeMappingFromOptions(), "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_roadmap_addon_lk",
			ProductID:  "price_unmapped_roadmap",
			LookupKey:  "strait_addon_compliance_archive",
			CustomerID: "cust_roadmap_addon",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000021"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 so roadmap add-on Stripe misconfiguration retries", rr.Code)
	}
	if store.lastAddonCreated != nil {
		t.Fatalf("roadmap lookup key created addon row: %#v", store.lastAddonCreated)
	}
	if store.lastUpserted != nil {
		t.Fatalf("roadmap add-on lookup key upserted plan subscription: %#v", store.lastUpserted)
	}
}

func TestWebhookHandler_LegacyRoadmapAddonPriceDoesNotCreateEntitlement(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000022": {
				OrgID:    "00000000-0000-0000-0000-000000000022",
				PlanTier: string(domain.PlanBusiness),
				Status:   "active",
			},
		},
	}
	mapping := NewStripeMappingFromOptions(
		WithAddonPrice("legacy-compliance-archive-price", AddonComplianceArchive),
	)
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_legacy_roadmap_addon",
			ProductID:  "legacy-compliance-archive-price",
			CustomerID: "cust_legacy_roadmap_addon",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000022"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for ignored legacy roadmap add-on", rr.Code)
	}
	if store.lastAddonCreated != nil {
		t.Fatalf("legacy roadmap add-on price created addon row: %#v", store.lastAddonCreated)
	}
	if store.lastUpserted != nil {
		t.Fatalf("legacy roadmap add-on price upserted plan subscription: %#v", store.lastUpserted)
	}
}
