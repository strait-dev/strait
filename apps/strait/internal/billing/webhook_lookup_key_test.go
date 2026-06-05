package billing

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.NotNil(
		t, store.lastUpserted,
	)
	assert.Equal(t,
		string(domain.PlanBusiness),
		store.lastUpserted.
			PlanTier)
	require.NotNil(
		t, store.lastUpserted.
			StripeLookupKey,
	)
	assert.Equal(t,
		"strait_business_monthly",
		*store.lastUpserted.
			StripeLookupKey,
	)
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
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.NotNil(
		t, store.lastUpserted,
	)
	assert.Equal(t,
		string(domain.PlanPro), store.
			lastUpserted.
			PlanTier)
	assert.Nil(t, store.lastUpserted.
		StripeLookupKey,
	)
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
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.NotNil(
		t, store.lastAddonCreated,
	)
	assert.Equal(t,
		AddonConcurrency100,
		store.lastAddonCreated.
			AddonType)
	require.NotNil(
		t, store.lastAddonCreated.
			StripeLookupKey,
	)
	assert.Equal(t,
		"strait_addon_concurrency_100",

		*store.lastAddonCreated.
			StripeLookupKey,
	)
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
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusInternalServerError,

		rr.Code,
	)
	require.Nil(t, store.lastAddonCreated)
	require.Nil(t, store.lastUpserted)
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
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.Nil(t, store.lastAddonCreated)
	require.Nil(t, store.lastUpserted)
}
