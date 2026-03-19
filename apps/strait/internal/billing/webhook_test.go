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

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
