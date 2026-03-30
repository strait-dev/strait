package billing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

// signStandardWebhook creates Standard Webhooks headers for a test request.
func signStandardWebhook(t *testing.T, secret string, body []byte) (msgID, timestamp, signature string) {
	t.Helper()
	msgID = "msg_test123"
	ts := time.Now().Unix()
	timestamp = fmt.Sprintf("%d", ts)

	// Decode secret (strip whsec_ prefix, base64-decode).
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		t.Fatalf("failed to decode secret: %v", err)
	}

	signedContent := fmt.Sprintf("%s.%s.%s", msgID, timestamp, string(body))
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signedContent))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	signature = "v1," + sig
	return msgID, timestamp, signature
}

// testSecret is a base64-encoded HMAC key with whsec_ prefix for tests.
var testSecret = "whsec_" + base64.StdEncoding.EncodeToString([]byte("test-webhook-secret-key-1234567"))

func TestWebhookHandler_VerifySignature(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	body := []byte(`{"type":"subscription.created","data":{}}`)

	t.Run("valid_signature", func(t *testing.T) {
		t.Parallel()
		msgID, ts, sig := signStandardWebhook(t, base64.StdEncoding.EncodeToString([]byte("test-webhook-secret-key-1234567")), body)
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
		req.Header.Set("webhook-id", msgID)
		req.Header.Set("webhook-timestamp", ts)
		req.Header.Set("webhook-signature", sig)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code == http.StatusUnauthorized {
			t.Error("expected valid signature to pass")
		}
	})

	t.Run("invalid_signature", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
		req.Header.Set("webhook-id", "msg_test")
		req.Header.Set("webhook-timestamp", fmt.Sprintf("%d", time.Now().Unix()))
		req.Header.Set("webhook-signature", "v1,invalidsig")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("missing_headers", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 with missing headers, got %d", rr.Code)
		}
	})

	t.Run("expired_timestamp", func(t *testing.T) {
		t.Parallel()
		oldTS := fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix())
		key, _ := base64.StdEncoding.DecodeString(base64.StdEncoding.EncodeToString([]byte("test-webhook-secret-key-1234567")))
		signedContent := fmt.Sprintf("msg_old.%s.%s", oldTS, string(body))
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(signedContent))
		sig := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
		req.Header.Set("webhook-id", "msg_old")
		req.Header.Set("webhook-timestamp", oldTS)
		req.Header.Set("webhook-signature", sig)
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
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.created",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_123",
			ProductID:  "pro-id",
			CustomerID: "cust_456",
			Metadata:   map[string]string{"org_id": "org_test"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	if store.lastUpserted == nil {
		t.Fatal("expected subscription to be upserted")
	}
	if store.lastUpserted.OrgID != "org_test" {
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
			"org_revoke": {
				OrgID:           "org_revoke",
				PlanTier:        "pro",
				Status:          "active",
				PendingPlanTier: &pendingTier,
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.revoked",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_123",
			ProductID:  "pro-id",
			CustomerID: "cust_456",
			Metadata:   map[string]string{"org_id": "org_revoke"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
	if store.lastClearedPending != "org_revoke" {
		t.Errorf("cleared pending org = %q, want org_revoke", store.lastClearedPending)
	}
	if store.subscriptions["org_revoke"].PendingPlanTier != nil {
		t.Fatal("expected pending plan tier to be cleared on revoke")
	}
}

func TestWebhookHandler_UnknownEventType(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewPolarMapping("", "", "", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "some.unknown.event",
		Data: json.RawMessage(`{}`),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for unknown events, got %d", rr.Code)
	}
}

func TestWebhookHandler_IdempotentUpsert(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewPolarMapping("starter-id", "", "", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.created",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_idem",
			ProductID:  "starter-id",
			CustomerID: "cust_idem",
			Metadata:   map[string]string{"org_id": "org_idem"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	// Send twice
	for i := range 2 {
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
			"org_limit": {
				OrgID:                 "org_limit",
				PlanTier:              "starter",
				Status:                "active",
				SpendingLimitMicrousd: 50000000, // $50
				LimitAction:           "notify",
				PendingPlanTier:       &pendingTier,
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	// Re-deliver the same subscription.created webhook.
	payload := PolarWebhookPayload{
		Type: "subscription.created",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_dup",
			ProductID:  "starter-id",
			CustomerID: "cust_dup",
			Metadata:   map[string]string{"org_id": "org_limit"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// The spending limit should be preserved (not reset to -1).
	sub := store.subscriptions["org_limit"]
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
			"org_period": {
				OrgID:              "org_period",
				PlanTier:           "starter",
				Status:             "active",
				CurrentPeriodStart: &oldStart,
				CurrentPeriodEnd:   &oldEnd,
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	newStart := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	newEnd := time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC)

	payload := PolarWebhookPayload{
		Type: "subscription.updated",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:                 "sub_period",
			ProductID:          "starter-id",
			CustomerID:         "cust_period",
			CurrentPeriodStart: &newStart,
			CurrentPeriodEnd:   &newEnd,
			Metadata:           map[string]string{"org_id": "org_period"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
			"org_down": {
				OrgID:    "org_down",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	// Update from pro to starter (downgrade).
	payload := PolarWebhookPayload{
		Type: "subscription.updated",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_down",
			ProductID:  "starter-id",
			CustomerID: "cust_down",
			Metadata:   map[string]string{"org_id": "org_down"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Plan should still be "pro" (not immediately downgraded).
	sub := store.subscriptions["org_down"]
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
			"org_up": {
				OrgID:           "org_up",
				PlanTier:        "starter",
				Status:          "active",
				PendingPlanTier: &pendingTier,
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	// Update from starter to pro (upgrade).
	payload := PolarWebhookPayload{
		Type: "subscription.updated",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_up",
			ProductID:  "pro-id",
			CustomerID: "cust_up",
			Metadata:   map[string]string{"org_id": "org_up"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
	if store.lastClearedPending != "org_up" {
		t.Errorf("expected pending tier to be cleared for org_up, got %q", store.lastClearedPending)
	}
	if store.subscriptions["org_up"].PendingPlanTier != nil {
		t.Fatal("expected pending tier to be cleared on immediate upgrade")
	}
}

func TestWebhook_CancellationThenUpgradeClearsPendingFreeTier(t *testing.T) {
	t.Parallel()

	pendingTier := "free"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_reactivate": {
				OrgID:           "org_reactivate",
				PlanTier:        "starter",
				Status:          "canceled",
				PendingPlanTier: &pendingTier,
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.updated",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_reactivate",
			ProductID:  "pro-id",
			CustomerID: "cust_reactivate",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "org_reactivate"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if store.subscriptions["org_reactivate"].PendingPlanTier != nil {
		t.Fatal("expected stale pending free tier to be cleared")
	}
	if store.subscriptions["org_reactivate"].PlanTier != "pro" {
		t.Fatalf("plan tier = %q, want pro", store.subscriptions["org_reactivate"].PlanTier)
	}
}

func TestWebhook_CanceledSetsPendingFreeTier(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_cancel": {
				OrgID:    "org_cancel",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	now := time.Now()
	payload := PolarWebhookPayload{
		Type: "subscription.canceled",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_cancel",
			ProductID:  "pro-id",
			CustomerID: "cust_cancel",
			CanceledAt: &now,
			Metadata:   map[string]string{"org_id": "org_cancel"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
	sub := store.subscriptions["org_cancel"]
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
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.canceled",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_noexist",
			ProductID:  "pro-id",
			CustomerID: "cust_noexist",
			Metadata:   map[string]string{"org_id": "org_noexist"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
		mapping := NewPolarMapping("starter-id", "", "pro-id", "")
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

		payload := PolarWebhookPayload{
			Type: "subscription.created",
			Data: mustJSON(t, PolarSubscriptionData{
				ID:         "sub_starter",
				ProductID:  "starter-id",
				CustomerID: "cust_starter",
				Metadata:   map[string]string{"org_id": "org_starter"},
			}),
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
		mapping := NewPolarMapping("starter-id", "", "pro-id", "")
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

		payload := PolarWebhookPayload{
			Type: "subscription.created",
			Data: mustJSON(t, PolarSubscriptionData{
				ID:         "sub_pro",
				ProductID:  "pro-id",
				CustomerID: "cust_pro",
				Metadata:   map[string]string{"org_id": "org_pro"},
			}),
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
		mapping := NewPolarMapping("starter-id", "", "pro-id", "")

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
			WithWelcomeEmail(welcomeFn))

		payload := PolarWebhookPayload{
			Type: "subscription.created",
			Data: mustJSON(t, PolarSubscriptionData{
				ID:         "sub_welcome",
				ProductID:  "starter-id",
				CustomerID: "cust_welcome",
				Customer: &PolarCustomerData{
					ID:    "cust_welcome",
					Email: "user@example.com",
				},
				Metadata: map[string]string{"org_id": "org_welcome"},
			}),
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
		if calls[0].orgID != "org_welcome" {
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
		mapping := NewPolarMapping("starter-id", "", "pro-id", "")

		called := false
		welcomeFn := func(_ context.Context, _ string, _ domain.PlanTier, _ string) error {
			called = true
			return nil
		}

		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
			WithWelcomeEmail(welcomeFn))

		payload := PolarWebhookPayload{
			Type: "subscription.created",
			Data: mustJSON(t, PolarSubscriptionData{
				ID:         "sub_noemail",
				ProductID:  "starter-id",
				CustomerID: "cust_noemail",
				Metadata:   map[string]string{"org_id": "org_noemail"},
			}),
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
		mapping := NewPolarMapping("starter-id", "", "pro-id", "")

		// No WithWelcomeEmail option -- should not panic.
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

		payload := PolarWebhookPayload{
			Type: "subscription.created",
			Data: mustJSON(t, PolarSubscriptionData{
				ID:         "sub_nofn",
				ProductID:  "starter-id",
				CustomerID: "cust_nofn",
				Customer: &PolarCustomerData{
					ID:    "cust_nofn",
					Email: "user@example.com",
				},
				Metadata: map[string]string{"org_id": "org_nofn"},
			}),
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit)

	payload := PolarWebhookPayload{
		Type: "subscription.created",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_audit",
			ProductID:  "pro-id",
			CustomerID: "cust_audit",
			Metadata:   map[string]string{"org_id": "org_audit"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit)

	payload := PolarWebhookPayload{
		Type: "subscription.created",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_details",
			ProductID:  "pro-id",
			CustomerID: "cust_details",
			Metadata:   map[string]string{"org_id": "org_details"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
	if details["polar_subscription_id"] != "sub_details" {
		t.Errorf("polar_subscription_id = %q, want sub_details", details["polar_subscription_id"])
	}
}

func TestWebhook_SubscriptionUpdated_Upgrade_AuditHasPreviousTier(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_upgrade_audit": {
				OrgID:    "org_upgrade_audit",
				PlanTier: "starter",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit)

	payload := PolarWebhookPayload{
		Type: "subscription.updated",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_upgrade_audit",
			ProductID:  "pro-id",
			CustomerID: "cust_upgrade_audit",
			Metadata:   map[string]string{"org_id": "org_upgrade_audit"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
			"org_down_audit": {
				OrgID:    "org_down_audit",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit)

	payload := PolarWebhookPayload{
		Type: "subscription.updated",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_down_audit",
			ProductID:  "starter-id",
			CustomerID: "cust_down_audit",
			Metadata:   map[string]string{"org_id": "org_down_audit"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
			"org_cancel_audit": {
				OrgID:    "org_cancel_audit",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit)

	now := time.Now()
	payload := PolarWebhookPayload{
		Type: "subscription.canceled",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_cancel_audit",
			ProductID:  "pro-id",
			CustomerID: "cust_cancel_audit",
			CanceledAt: &now,
			Metadata:   map[string]string{"org_id": "org_cancel_audit"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
			"org_revoke_audit": {
				OrgID:    "org_revoke_audit",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit)

	payload := PolarWebhookPayload{
		Type: "subscription.revoked",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_revoke_audit",
			ProductID:  "pro-id",
			CustomerID: "cust_revoke_audit",
			Metadata:   map[string]string{"org_id": "org_revoke_audit"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	// Pass nil for auditStore - should not panic.
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.created",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_nil_audit",
			ProductID:  "pro-id",
			CustomerID: "cust_nil_audit",
			Metadata:   map[string]string{"org_id": "org_nil_audit"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit)

	payload := PolarWebhookPayload{
		Type: "subscription.created",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_restype",
			ProductID:  "pro-id",
			CustomerID: "cust_restype",
			Metadata:   map[string]string{"org_id": "org_restype"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
	if audit.events[0].ResourceID != "org_restype" {
		t.Errorf("resource_id = %q, want org_restype", audit.events[0].ResourceID)
	}
	if audit.events[0].ActorType != "system" {
		t.Errorf("actor_type = %q, want system", audit.events[0].ActorType)
	}
	if audit.events[0].ActorID != "polar-webhook" {
		t.Errorf("actor_id = %q, want polar-webhook", audit.events[0].ActorID)
	}
}

// Grace period webhook tests.

func TestWebhook_PaymentFailed_SetsGracePeriod72h(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_pastdue": {
				OrgID:         "org_pastdue",
				PlanTier:      "pro",
				Status:        "active",
				PaymentStatus: "ok",
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.updated",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_pastdue",
			ProductID:  "pro-id",
			CustomerID: "cust_pastdue",
			Status:     "past_due",
			Metadata:   map[string]string{"org_id": "org_pastdue"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	sub := store.subscriptions["org_pastdue"]
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
			"org_grace_status": {
				OrgID:         "org_grace_status",
				PlanTier:      "starter",
				Status:        "active",
				PaymentStatus: "ok",
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.updated",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_grace",
			ProductID:  "starter-id",
			CustomerID: "cust_grace",
			Status:     "past_due",
			Metadata:   map[string]string{"org_id": "org_grace_status"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
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
			"org_recover": {
				OrgID:          "org_recover",
				PlanTier:       "pro",
				Status:         "active",
				PaymentStatus:  "grace",
				GracePeriodEnd: &graceEnd,
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.active",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_recover",
			ProductID:  "pro-id",
			CustomerID: "cust_recover",
			Metadata:   map[string]string{"org_id": "org_recover"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	sub := store.subscriptions["org_recover"]
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
			"org_extend": {
				OrgID:          "org_extend",
				PlanTier:       "pro",
				Status:         "active",
				PaymentStatus:  "grace",
				GracePeriodEnd: &oldGrace,
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.updated",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_extend",
			ProductID:  "pro-id",
			CustomerID: "cust_extend",
			Status:     "past_due",
			Metadata:   map[string]string{"org_id": "org_extend"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	sub := store.subscriptions["org_extend"]
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
			"org_free_pay": {
				OrgID:         "org_free_pay",
				PlanTier:      "free",
				Status:        "active",
				PaymentStatus: "ok",
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.updated",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_free_pay",
			ProductID:  "starter-id",
			CustomerID: "cust_free_pay",
			Status:     "past_due",
			Metadata:   map[string]string{"org_id": "org_free_pay"},
		}),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Free org should not have grace period set.
	sub := store.subscriptions["org_free_pay"]
	if sub.PaymentStatus != "ok" {
		t.Errorf("payment_status = %q, want ok (no grace for free orgs)", sub.PaymentStatus)
	}
}

func TestWebhook_EmptySecretCloudMode_Rejects(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithEdition("cloud"))

	payload := PolarWebhookPayload{
		Type: "subscription.created",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "org-1"},
		}),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/polar", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 in cloud mode with empty secret, got %d", rec.Code)
	}
}

func TestWebhook_EmptySecretCommunityMode_Allows(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithEdition("community"))

	payload := PolarWebhookPayload{
		Type: "subscription.created",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "org-1"},
		}),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/polar", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("community mode should not reject unsigned webhooks, got %d", rec.Code)
	}
}

func TestWebhook_EmptySecretDefaultEdition_Allows(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")

	// No WithEdition option -- defaults to empty string (non-cloud)
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := PolarWebhookPayload{
		Type: "subscription.created",
		Data: mustJSON(t, PolarSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "org-1"},
		}),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/polar", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("default edition should not reject unsigned webhooks, got %d", rec.Code)
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
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("test-secret-key-32bytes-long!!!!"))
	handler := NewWebhookHandler(store, mapping, secret, slog.Default(), nil, nil)

	f.Fuzz(func(t *testing.T, sigHeader string) {
		payload := `{"type":"subscription.created","data":{"id":"sub_1","product_id":"starter-id","customer_id":"cust_1","status":"active","metadata":{"org_id":"org-1"}}}`
		req := httptest.NewRequest(http.MethodPost, "/webhooks/polar", strings.NewReader(payload))
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
