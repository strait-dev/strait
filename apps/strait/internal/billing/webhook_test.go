package billing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

// signStripeWebhook creates a Stripe-Signature header value for a test request.
func signStripeWebhook(t *testing.T, secret string, body []byte) string {
	t.Helper()
	ts := fmt.Sprintf("%d", time.Now().Unix())
	signedContent := ts + "." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedContent))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%s,v1=%s", ts, sig)
}

// testSecret is the webhook signing secret used in tests.
var testSecret = "whsec_test_secret_for_unit_tests_only"

func TestWebhookHandler_VerifySignature(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	body := []byte(`{"type":"customer.subscription.created","data":{"object":{"id":"sub_sig","status":"active","items":{"data":[{"price":{"id":"starter-id"}}]},"customer":{"id":"cust_sig","metadata":{"org_id":"550e8400-e29b-41d4-a716-446655440000"}}}}}`)

	t.Run("valid_signature", func(t *testing.T) {
		t.Parallel()
		sig := signStripeWebhook(t, testSecret, body)
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", sig)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code == http.StatusUnauthorized {
			t.Error("expected valid signature to pass")
		}
	})

	t.Run("invalid_signature", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", "t=1234567890,v1=invalidsig")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("missing_headers", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 with missing headers, got %d", rr.Code)
		}
	})

	t.Run("expired_timestamp", func(t *testing.T) {
		t.Parallel()
		oldTS := fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix())
		signedContent := oldTS + "." + string(body)
		mac := hmac.New(sha256.New, []byte(testSecret))
		mac.Write([]byte(signedContent))
		sig := fmt.Sprintf("t=%s,v1=%s", oldTS, hex.EncodeToString(mac.Sum(nil)))

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", sig)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for expired timestamp, got %d", rr.Code)
		}
	})
}

func TestWebhookHandler_SubscriptionCreated(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_123",
			ProductID:  "pro-id",
			CustomerID: "cust_456",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000001"},
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
		t.Errorf("expected 200, got %d", rr.Code)
	}

	if store.lastUpserted == nil {
		t.Fatal("expected subscription to be upserted")
	}
	if store.lastUpserted.OrgID != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("org_id = %q, want org_test", store.lastUpserted.OrgID)
	}
	if store.lastUpserted.PlanTier != "pro" {
		t.Errorf("plan_tier = %q, want pro", store.lastUpserted.PlanTier)
	}
}

func TestWebhookHandler_SubscriptionRevoked(t *testing.T) {
	t.Parallel()

	pendingTier := "starter"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000002": {
				OrgID:           "00000000-0000-0000-0000-000000000002",
				PlanTier:        "pro",
				Status:          "active",
				PendingPlanTier: &pendingTier,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires customer.subscription.deleted with CancelAtPeriodEnd=false for immediate revocation.
	payload := StripeWebhookPayload{
		Type: "customer.subscription.deleted",
		Data: mustJSON(t, testSubscriptionData{
			ID:                "sub_123",
			ProductID:         "pro-id",
			CustomerID:        "cust_456",
			CancelAtPeriodEnd: false,
			Metadata:          map[string]string{"org_id": "00000000-0000-0000-0000-000000000002"},
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
		t.Errorf("expected 200, got %d", rr.Code)
	}

	if store.lastPlanUpdate == nil {
		t.Fatal("expected plan to be updated")
	}
	if store.lastPlanUpdate.tier != "free" {
		t.Errorf("plan_tier = %q, want free", store.lastPlanUpdate.tier)
	}
	if store.lastPlanUpdate.status != "revoked" {
		t.Errorf("status = %q, want revoked", store.lastPlanUpdate.status)
	}
	if store.lastClearedPending != "00000000-0000-0000-0000-000000000002" {
		t.Errorf("cleared pending org = %q, want org_revoke", store.lastClearedPending)
	}
	if store.subscriptions["00000000-0000-0000-0000-000000000002"].PendingPlanTier != nil {
		t.Fatal("expected pending plan tier to be cleared on revoke")
	}
}

func TestWebhookHandler_UnknownEventType(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("", "", "", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "some.unknown.event",
		Data: json.RawMessage(`{}`),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for unknown events, got %d", rr.Code)
	}
}

func TestWebhookHandler_IdempotentUpsert(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_idem",
			ProductID:  "starter-id",
			CustomerID: "cust_idem",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000003"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	// Send twice
	for i := range 2 {
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("attempt %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	if store.upsertCount != 2 {
		t.Errorf("expected 2 upserts (idempotent), got %d", store.upsertCount)
	}
}

func TestWebhook_DuplicateCreatedPreservesSpendingLimit(t *testing.T) {
	t.Parallel()

	pendingTier := "free"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000004": {
				OrgID:                 "00000000-0000-0000-0000-000000000004",
				PlanTier:              "starter",
				Status:                "active",
				SpendingLimitMicrousd: 50000000, // $50
				LimitAction:           "notify",
				PendingPlanTier:       &pendingTier,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Re-deliver the same subscription.created webhook.
	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_dup",
			ProductID:  "starter-id",
			CustomerID: "cust_dup",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000004"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// The spending limit should be preserved (not reset to -1).
	sub := store.subscriptions["00000000-0000-0000-0000-000000000004"]
	if sub.SpendingLimitMicrousd != 50000000 {
		t.Errorf("spending limit was overwritten: got %d, want 50000000", sub.SpendingLimitMicrousd)
	}
	if sub.LimitAction != "notify" {
		t.Errorf("limit action was overwritten: got %q, want notify", sub.LimitAction)
	}
	if sub.PendingPlanTier != nil {
		t.Fatal("expected duplicate create to clear stale pending downgrade")
	}
}

func TestWebhook_UpdatedRefreshesPeriodDates(t *testing.T) {
	t.Parallel()

	oldStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	oldEnd := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000005": {
				OrgID:              "00000000-0000-0000-0000-000000000005",
				PlanTier:           "starter",
				Status:             "active",
				CurrentPeriodStart: &oldStart,
				CurrentPeriodEnd:   &oldEnd,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	newStart := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	newEnd := time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC)

	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:                 "sub_period",
			ProductID:          "starter-id",
			CustomerID:         "cust_period",
			CurrentPeriodStart: &newStart,
			CurrentPeriodEnd:   &newEnd,
			Metadata:           map[string]string{"org_id": "00000000-0000-0000-0000-000000000005"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	if store.lastFullUpdate == nil {
		t.Fatal("expected full update to be called")
	}
	if store.lastFullUpdate.periodStart == nil || !store.lastFullUpdate.periodStart.Equal(newStart) {
		t.Errorf("period start not updated correctly")
	}
	if store.lastFullUpdate.periodEnd == nil || !store.lastFullUpdate.periodEnd.Equal(newEnd) {
		t.Errorf("period end not updated correctly")
	}
}

func TestWebhook_DowngradeDeferred(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000006": {
				OrgID:    "00000000-0000-0000-0000-000000000006",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Update from pro to starter (downgrade).
	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_down",
			ProductID:  "starter-id",
			CustomerID: "cust_down",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000006"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Plan should still be "pro" (not immediately downgraded).
	sub := store.subscriptions["00000000-0000-0000-0000-000000000006"]
	if sub.PlanTier != "pro" {
		t.Errorf("expected plan to remain pro during deferred downgrade, got %q", sub.PlanTier)
	}
	// Pending tier should be set.
	if store.lastPendingTier != "starter" {
		t.Errorf("expected pending tier to be starter, got %q", store.lastPendingTier)
	}
}

func TestWebhook_UpgradeImmediate(t *testing.T) {
	t.Parallel()

	pendingTier := "free"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000007": {
				OrgID:           "00000000-0000-0000-0000-000000000007",
				PlanTier:        "starter",
				Status:          "active",
				PendingPlanTier: &pendingTier,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Update from starter to pro (upgrade).
	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_up",
			ProductID:  "pro-id",
			CustomerID: "cust_up",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000007"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Plan should be immediately updated to "pro".
	if store.lastFullUpdate == nil {
		t.Fatal("expected full update to be called")
	}
	if store.lastFullUpdate.tier != "pro" {
		t.Errorf("expected tier to be pro, got %q", store.lastFullUpdate.tier)
	}
	// No pending tier should be set.
	if store.lastPendingTier != "" {
		t.Errorf("expected no pending tier for upgrade, got %q", store.lastPendingTier)
	}
	if store.lastClearedPending != "00000000-0000-0000-0000-000000000007" {
		t.Errorf("expected pending tier to be cleared for org_up, got %q", store.lastClearedPending)
	}
	if store.subscriptions["00000000-0000-0000-0000-000000000007"].PendingPlanTier != nil {
		t.Fatal("expected pending tier to be cleared on immediate upgrade")
	}
}

func TestWebhook_CancellationThenUpgradeClearsPendingFreeTier(t *testing.T) {
	t.Parallel()

	pendingTier := "free"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000008": {
				OrgID:           "00000000-0000-0000-0000-000000000008",
				PlanTier:        "starter",
				Status:          "canceled",
				PendingPlanTier: &pendingTier,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_reactivate",
			ProductID:  "pro-id",
			CustomerID: "cust_reactivate",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000008"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if store.subscriptions["00000000-0000-0000-0000-000000000008"].PendingPlanTier != nil {
		t.Fatal("expected stale pending free tier to be cleared")
	}
	if store.subscriptions["00000000-0000-0000-0000-000000000008"].PlanTier != "pro" {
		t.Fatalf("plan tier = %q, want pro", store.subscriptions["00000000-0000-0000-0000-000000000008"].PlanTier)
	}
}

func TestWebhook_CanceledSetsPendingFreeTier(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000009": {
				OrgID:    "00000000-0000-0000-0000-000000000009",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	now := time.Now()
	// Stripe fires customer.subscription.deleted with CancelAtPeriodEnd=true for deferred cancellation.
	payload := StripeWebhookPayload{
		Type: "customer.subscription.deleted",
		Data: mustJSON(t, testSubscriptionData{
			ID:                "sub_cancel",
			ProductID:         "pro-id",
			CustomerID:        "cust_cancel",
			CanceledAt:        &now,
			CancelAtPeriodEnd: true,
			Metadata:          map[string]string{"org_id": "00000000-0000-0000-0000-000000000009"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify pending tier was set to "free".
	if store.lastPendingTier != "free" {
		t.Errorf("expected pending tier free, got %q", store.lastPendingTier)
	}

	// Plan should still be "pro" (not immediately changed).
	sub := store.subscriptions["00000000-0000-0000-0000-000000000009"]
	if sub.PlanTier != "pro" {
		t.Errorf("expected plan to remain pro until period end, got %q", sub.PlanTier)
	}
	if sub.Status != "canceled" {
		t.Errorf("expected status canceled, got %q", sub.Status)
	}
}

func TestWebhook_CanceledWithNoPriorSubscription(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.deleted",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_noexist",
			ProductID:  "pro-id",
			CustomerID: "cust_noexist",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000000a"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// No pending tier should be set since no subscription existed.
	if store.lastPendingTier != "" {
		t.Errorf("expected no pending tier, got %q", store.lastPendingTier)
	}
}

func TestWebhookHandler_SubscriptionCreated_SetsMonthlyUsageEmail(t *testing.T) {
	t.Parallel()

	t.Run("starter_plan_enables_monthly_usage_email", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

		payload := StripeWebhookPayload{
			Type: "customer.subscription.created",
			Data: mustJSON(t, testSubscriptionData{
				ID:         "sub_starter",
				ProductID:  "starter-id",
				CustomerID: "cust_starter",
				Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000000b"},
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
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		if store.lastUpserted == nil {
			t.Fatal("expected subscription to be upserted")
		}
		if !store.lastUpserted.MonthlyUsageEmail {
			t.Error("expected MonthlyUsageEmail to be true for starter plan")
		}
	})

	t.Run("pro_plan_enables_monthly_usage_email", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

		payload := StripeWebhookPayload{
			Type: "customer.subscription.created",
			Data: mustJSON(t, testSubscriptionData{
				ID:         "sub_pro",
				ProductID:  "pro-id",
				CustomerID: "cust_pro",
				Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000000c"},
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
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		if store.lastUpserted == nil {
			t.Fatal("expected subscription to be upserted")
		}
		if !store.lastUpserted.MonthlyUsageEmail {
			t.Error("expected MonthlyUsageEmail to be true for pro plan")
		}
	})
}

func TestWebhookHandler_SubscriptionCreated_WelcomeEmail(t *testing.T) {
	t.Parallel()

	t.Run("paid_plan_calls_welcome_email", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")

		type welcomeCall struct {
			orgID         string
			tier          domain.PlanTier
			customerEmail string
		}
		var calls []welcomeCall
		done := make(chan struct{}, 1)

		welcomeFn := func(_ context.Context, orgID string, tier domain.PlanTier, email string) error {
			calls = append(calls, welcomeCall{orgID: orgID, tier: tier, customerEmail: email})
			done <- struct{}{}
			return nil
		}

		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
			WithDevBypassSignatureCheck(), WithWelcomeEmail(welcomeFn))

		payload := StripeWebhookPayload{
			Type: "customer.subscription.created",
			Data: mustJSON(t, testSubscriptionData{
				ID:         "sub_welcome",
				ProductID:  "starter-id",
				CustomerID: "cust_welcome",
				Customer: &testCustomerData{
					ID:    "cust_welcome",
					Email: "user@example.com",
				},
				Metadata: map[string]string{"org_id": "00000000-0000-0000-0000-00000000000d"},
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
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		// Wait for the async goroutine to complete.
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for welcome email callback")
		}

		if len(calls) != 1 {
			t.Fatalf("expected 1 welcome email call, got %d", len(calls))
		}
		if calls[0].orgID != "00000000-0000-0000-0000-00000000000d" {
			t.Errorf("orgID = %q, want org_welcome", calls[0].orgID)
		}
		if calls[0].tier != domain.PlanStarter {
			t.Errorf("tier = %q, want starter", calls[0].tier)
		}
		if calls[0].customerEmail != "user@example.com" {
			t.Errorf("email = %q, want user@example.com", calls[0].customerEmail)
		}
	})

	t.Run("no_customer_email_skips_welcome", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")

		called := false
		welcomeFn := func(_ context.Context, _ string, _ domain.PlanTier, _ string) error {
			called = true
			return nil
		}

		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
			WithDevBypassSignatureCheck(), WithWelcomeEmail(welcomeFn))

		payload := StripeWebhookPayload{
			Type: "customer.subscription.created",
			Data: mustJSON(t, testSubscriptionData{
				ID:         "sub_noemail",
				ProductID:  "starter-id",
				CustomerID: "cust_noemail",
				Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000000e"},
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
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		// Give async goroutine a chance to run (it should not).
		time.Sleep(100 * time.Millisecond)

		if called {
			t.Error("welcome email should not be called when customer email is empty")
		}
	})

	t.Run("no_welcome_fn_configured", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")

		// No WithWelcomeEmail option -- should not panic.
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

		payload := StripeWebhookPayload{
			Type: "customer.subscription.created",
			Data: mustJSON(t, testSubscriptionData{
				ID:         "sub_nofn",
				ProductID:  "starter-id",
				CustomerID: "cust_nofn",
				Customer: &testCustomerData{
					ID:    "cust_nofn",
					Email: "user@example.com",
				},
				Metadata: map[string]string{"org_id": "00000000-0000-0000-0000-00000000000f"},
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
			t.Fatalf("expected 200 when no welcome fn configured, got %d", rr.Code)
		}
	})
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// mockAuditStore records audit events for test assertions.
type mockAuditStore struct {
	events []*domain.AuditEvent
}

func (m *mockAuditStore) CreateAuditEvent(_ context.Context, event *domain.AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func TestWebhook_SubscriptionCreated_CreatesAuditEvent(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_audit",
			ProductID:  "pro-id",
			CustomerID: "cust_audit",
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(audit.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(audit.events))
	}
	if audit.events[0].Action != "subscription.created" {
		t.Errorf("action = %q, want subscription.created", audit.events[0].Action)
	}
}

func TestWebhook_SubscriptionCreated_AuditDetails_ContainsPlanTier(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_details",
			ProductID:  "pro-id",
			CustomerID: "cust_details",
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(audit.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(audit.events))
	}

	var details map[string]string
	if err := json.Unmarshal(audit.events[0].Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["plan_tier"] != "pro" {
		t.Errorf("plan_tier = %q, want pro", details["plan_tier"])
	}
	if details["stripe_subscription_id"] != "sub_details" {
		t.Errorf("stripe_subscription_id = %q, want sub_details", details["stripe_subscription_id"])
	}
}

func TestWebhook_SubscriptionUpdated_Upgrade_AuditHasPreviousTier(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000012": {
				OrgID:    "00000000-0000-0000-0000-000000000012",
				PlanTier: "starter",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_upgrade_audit",
			ProductID:  "pro-id",
			CustomerID: "cust_upgrade_audit",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000012"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(audit.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(audit.events))
	}

	var details map[string]string
	if err := json.Unmarshal(audit.events[0].Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["previous_tier"] != "starter" {
		t.Errorf("previous_tier = %q, want starter", details["previous_tier"])
	}
	if details["plan_tier"] != "pro" {
		t.Errorf("plan_tier = %q, want pro", details["plan_tier"])
	}
}

func TestWebhook_SubscriptionUpdated_Downgrade_AuditHasPendingTier(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000013": {
				OrgID:    "00000000-0000-0000-0000-000000000013",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_down_audit",
			ProductID:  "starter-id",
			CustomerID: "cust_down_audit",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000013"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(audit.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(audit.events))
	}

	var details map[string]string
	if err := json.Unmarshal(audit.events[0].Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["pending_plan_tier"] != "starter" {
		t.Errorf("pending_plan_tier = %q, want starter", details["pending_plan_tier"])
	}
	if details["previous_tier"] != "pro" {
		t.Errorf("previous_tier = %q, want pro", details["previous_tier"])
	}
}

func TestWebhook_SubscriptionCanceled_CreatesAuditEvent(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000014": {
				OrgID:    "00000000-0000-0000-0000-000000000014",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	now := time.Now()
	payload := StripeWebhookPayload{
		Type: "customer.subscription.deleted",
		Data: mustJSON(t, testSubscriptionData{
			ID:                "sub_cancel_audit",
			ProductID:         "pro-id",
			CustomerID:        "cust_cancel_audit",
			CanceledAt:        &now,
			CancelAtPeriodEnd: true,
			Metadata:          map[string]string{"org_id": "00000000-0000-0000-0000-000000000014"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(audit.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(audit.events))
	}
	if audit.events[0].Action != "subscription.canceled" {
		t.Errorf("action = %q, want subscription.canceled", audit.events[0].Action)
	}
}

func TestWebhook_SubscriptionRevoked_CreatesAuditEvent(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000015": {
				OrgID:    "00000000-0000-0000-0000-000000000015",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	// Stripe fires customer.subscription.deleted with CancelAtPeriodEnd=false for immediate revocation.
	payload := StripeWebhookPayload{
		Type: "customer.subscription.deleted",
		Data: mustJSON(t, testSubscriptionData{
			ID:                "sub_revoke_audit",
			ProductID:         "pro-id",
			CustomerID:        "cust_revoke_audit",
			CancelAtPeriodEnd: false,
			Metadata:          map[string]string{"org_id": "00000000-0000-0000-0000-000000000015"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(audit.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(audit.events))
	}
	if audit.events[0].Action != "subscription.revoked" {
		t.Errorf("action = %q, want subscription.revoked", audit.events[0].Action)
	}
}

func TestWebhook_AuditStore_Nil_DoesNotPanic(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	// Pass nil for auditStore - should not panic.
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_nil_audit",
			ProductID:  "pro-id",
			CustomerID: "cust_nil_audit",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000016"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestWebhook_AuditEvent_HasCorrectResourceType(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_restype",
			ProductID:  "pro-id",
			CustomerID: "cust_restype",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000017"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(audit.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(audit.events))
	}
	if audit.events[0].ResourceType != "subscription" {
		t.Errorf("resource_type = %q, want subscription", audit.events[0].ResourceType)
	}
	if audit.events[0].ResourceID != "00000000-0000-0000-0000-000000000017" {
		t.Errorf("resource_id = %q, want org_restype", audit.events[0].ResourceID)
	}
	if audit.events[0].ActorType != "system" {
		t.Errorf("actor_type = %q, want system", audit.events[0].ActorType)
	}
	if audit.events[0].ActorID != "stripe-webhook" {
		t.Errorf("actor_id = %q, want stripe-webhook", audit.events[0].ActorID)
	}
}

// Grace period webhook tests.

func TestWebhook_PaymentFailed_SetsGracePeriod72h(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000018": {
				OrgID:         "00000000-0000-0000-0000-000000000018",
				PlanTier:      "pro",
				Status:        "active",
				PaymentStatus: "ok",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires invoice.payment_failed when a payment attempt fails.
	payload := StripeWebhookPayload{
		Type: "invoice.payment_failed",
		Data: mustJSON(t, testInvoiceData{
			ID:         "inv_pastdue",
			CustomerID: "cust_pastdue",
			SubID:      "sub_pastdue",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000018"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	sub := store.subscriptions["00000000-0000-0000-0000-000000000018"]
	if sub.PaymentStatus != "grace" {
		t.Errorf("payment_status = %q, want grace", sub.PaymentStatus)
	}
	if sub.GracePeriodEnd == nil {
		t.Fatal("expected grace_period_end to be set")
	}
	// Grace period should be roughly 72 hours from now.
	expected := before.Add(72 * time.Hour)
	diff := sub.GracePeriodEnd.Sub(expected)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("grace_period_end off by %v from expected 72h", diff)
	}
}

func TestWebhook_PaymentFailed_StatusBecomesGrace(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000019": {
				OrgID:         "00000000-0000-0000-0000-000000000019",
				PlanTier:      "starter",
				Status:        "active",
				PaymentStatus: "ok",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires invoice.payment_failed when a payment attempt fails.
	payload := StripeWebhookPayload{
		Type: "invoice.payment_failed",
		Data: mustJSON(t, testInvoiceData{
			ID:         "inv_grace",
			CustomerID: "cust_grace",
			SubID:      "sub_grace",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000019"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	if store.lastPaymentStatusUpdate == nil {
		t.Fatal("expected payment status update")
	}
	if store.lastPaymentStatusUpdate.status != "grace" {
		t.Errorf("status = %q, want grace", store.lastPaymentStatusUpdate.status)
	}
}

func TestWebhook_PaymentSucceeded_ClearsGracePeriod(t *testing.T) {
	t.Parallel()

	graceEnd := time.Now().Add(48 * time.Hour)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-00000000001a": {
				OrgID:          "00000000-0000-0000-0000-00000000001a",
				PlanTier:       "pro",
				Status:         "active",
				PaymentStatus:  "grace",
				GracePeriodEnd: &graceEnd,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires customer.subscription.updated with active status when payment recovers.
	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_recover",
			ProductID:  "pro-id",
			CustomerID: "cust_recover",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001a"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	sub := store.subscriptions["00000000-0000-0000-0000-00000000001a"]
	if sub.PaymentStatus != "ok" {
		t.Errorf("payment_status = %q, want ok", sub.PaymentStatus)
	}
	if sub.GracePeriodEnd != nil {
		t.Errorf("expected grace_period_end to be cleared, got %v", sub.GracePeriodEnd)
	}
}

func TestWebhook_PaymentFailed_AlreadyInGrace_Extends(t *testing.T) {
	t.Parallel()

	oldGrace := time.Now().Add(24 * time.Hour) // 24h left
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-00000000001b": {
				OrgID:          "00000000-0000-0000-0000-00000000001b",
				PlanTier:       "pro",
				Status:         "active",
				PaymentStatus:  "grace",
				GracePeriodEnd: &oldGrace,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires invoice.payment_failed for each failed payment attempt.
	payload := StripeWebhookPayload{
		Type: "invoice.payment_failed",
		Data: mustJSON(t, testInvoiceData{
			ID:         "inv_extend",
			CustomerID: "cust_extend",
			SubID:      "sub_extend",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001b"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	sub := store.subscriptions["00000000-0000-0000-0000-00000000001b"]
	if sub.PaymentStatus != "grace" {
		t.Errorf("payment_status = %q, want grace", sub.PaymentStatus)
	}
	// Grace period should be extended to ~72h from now, not the old 24h.
	if sub.GracePeriodEnd == nil {
		t.Fatal("expected grace_period_end to be set")
	}
	if sub.GracePeriodEnd.Before(time.Now().Add(70 * time.Hour)) {
		t.Errorf("expected grace period to be extended to ~72h, got %v", sub.GracePeriodEnd)
	}
}

func TestWebhook_PaymentFailed_FreeOrg_Ignored(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-00000000001c": {
				OrgID:         "00000000-0000-0000-0000-00000000001c",
				PlanTier:      "free",
				Status:        "active",
				PaymentStatus: "ok",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires invoice.payment_failed for each failed payment attempt.
	payload := StripeWebhookPayload{
		Type: "invoice.payment_failed",
		Data: mustJSON(t, testInvoiceData{
			ID:         "inv_free_pay",
			CustomerID: "cust_free_pay",
			SubID:      "sub_free_pay",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001c"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Free org should not have grace period set.
	sub := store.subscriptions["00000000-0000-0000-0000-00000000001c"]
	if sub.PaymentStatus != "ok" {
		t.Errorf("payment_status = %q, want ok (no grace for free orgs)", sub.PaymentStatus)
	}
}

func TestWebhook_EmptySecretCloudMode_Rejects(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithEdition("cloud"))

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001d"},
		}),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 in cloud mode with empty secret, got %d", rec.Code)
	}
}

func TestWebhook_EmptySecretCommunityMode_Rejects(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithEdition("community"))

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001d"},
		}),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 in community mode with empty secret, got %d", rec.Code)
	}
}

func TestWebhook_EmptySecretDefaultEdition_Rejects(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001d"},
		}),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 in default edition with empty secret, got %d", rec.Code)
	}
}

func TestWebhook_EmptySecretWithDevBypass_Allows(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001d"},
		}),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("dev bypass should allow unsigned webhooks, got %d", rec.Code)
	}
}

func TestWebhook_InvoiceUncollectible_SetsRestricted(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000050": {
				OrgID:         "00000000-0000-0000-0000-000000000050",
				PlanTier:      "pro",
				Status:        "active",
				PaymentStatus: "ok",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "invoice.marked_uncollectible",
		Data: mustJSON(t, testInvoiceData{
			ID:         "inv_uncoll",
			CustomerID: "cust_uncoll",
			SubID:      "sub_uncoll",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000050"},
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
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	sub := store.subscriptions["00000000-0000-0000-0000-000000000050"]
	if sub.PaymentStatus != "restricted" {
		t.Errorf("payment_status = %q, want restricted", sub.PaymentStatus)
	}
}

func FuzzWebhookSignatureHeader(f *testing.F) {
	f.Add("v1,abc123")
	f.Add("")
	f.Add("v1,")
	f.Add("v2,something")
	f.Add("v1,dGVzdA== v1,aW52YWxpZA==")
	f.Add(strings.Repeat("v1,x", 1000))

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("test-secret-key-32bytes-long!!!!"))
	handler := NewWebhookHandler(store, mapping, secret, slog.Default(), nil, nil)

	f.Fuzz(func(t *testing.T, sigHeader string) {
		payload := `{"type":"customer.subscription.created","data":{"id":"sub_1","product_id":"starter-id","customer_id":"cust_1","status":"active","metadata":{"org_id":"00000000-0000-0000-0000-00000000001d"}}}`
		req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(payload))
		req.Header.Set("webhook-id", "msg_test")
		req.Header.Set("webhook-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
		req.Header.Set("webhook-signature", sigHeader)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// Must never panic, regardless of signature header content.
		// Valid responses: 200 (if sig matches), 401 (if sig doesn't match),
		// or other codes (e.g., 400 for bad payload). The key assertion is
		// that no panic occurred -- reaching this point means the handler
		// survived the fuzzed input.
		_ = rec.Code
	})
}
