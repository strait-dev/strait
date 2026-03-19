package billing

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
	return
}

// testSecret is a base64-encoded HMAC key with whsec_ prefix for tests.
var testSecret = "whsec_" + base64.StdEncoding.EncodeToString([]byte("test-webhook-secret-key-1234567"))

func TestWebhookHandler_VerifySignature(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil)

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
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil)

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

	store := &mockBillingStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil)

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
}

func TestWebhookHandler_UnknownEventType(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewPolarMapping("", "", "", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil)

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
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil)

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

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_limit": {
				OrgID:                 "org_limit",
				PlanTier:              "starter",
				Status:                "active",
				SpendingLimitMicrousd: 50000000, // $50
				LimitAction:           "notify",
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil)

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
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil)

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
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil)

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

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_up": {
				OrgID:    "org_up",
				PlanTier: "starter",
				Status:   "active",
			},
		},
	}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil)

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
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
