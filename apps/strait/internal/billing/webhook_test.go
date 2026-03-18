package billing

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookHandler_VerifySignature(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	store := &mockBillingStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, secret, slog.Default())

	body := []byte(`{"type":"subscription.created","data":{}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := hex.EncodeToString(mac.Sum(nil))

	t.Run("valid_signature", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
		req.Header.Set("X-Polar-Signature", validSig)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code == http.StatusUnauthorized {
			t.Error("expected valid signature to pass")
		}
	})

	t.Run("invalid_signature", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/polar", bytes.NewReader(body))
		req.Header.Set("X-Polar-Signature", "invalid")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

func TestWebhookHandler_SubscriptionCreated(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewPolarMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default())

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
	handler := NewWebhookHandler(store, mapping, "", slog.Default())

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
	handler := NewWebhookHandler(store, mapping, "", slog.Default())

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
	handler := NewWebhookHandler(store, mapping, "", slog.Default())

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

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
