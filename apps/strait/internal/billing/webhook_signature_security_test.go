package billing

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWebhookSignatureSecurity verifies that webhooks are always rejected when the
// signing secret is not configured, regardless of edition. This prevents attackers
// from forging Stripe events by targeting non-cloud deployments.
func TestWebhookSignatureSecurity(t *testing.T) {
	t.Parallel()

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "550e8400-e29b-41d4-a716-446655440000"},
		}),
	}
	body, _ := json.Marshal(payload)

	editions := []string{"", "community", "cloud", "enterprise", "self-hosted"}
	for _, edition := range editions {
		t.Run("empty_secret_"+edition+"_rejects", func(t *testing.T) {
			t.Parallel()

			store := &mockBillingStore{}
			mapping := NewStripeMapping("starter-id", "", "pro-id", "")

			opts := []WebhookOption{}
			if edition != "" {
				opts = append(opts, WithEdition(edition))
			}
			handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, opts...)

			req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			require.Equal(t,
				http.StatusServiceUnavailable,

				rec.Code,
			)

		})
	}

	t.Run("dev_bypass_with_empty_secret_allows", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
			WithDevBypassSignatureCheck())

		req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.False(t,
			rec.Code == http.
				StatusServiceUnavailable ||
				rec.Code == http.
					StatusUnauthorized,
		)

	})

	t.Run("configured_secret_rejects_unsigned", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
		// No Stripe-Signature header
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t,
			http.StatusUnauthorized,
			rec.
				Code)

	})

	t.Run("configured_secret_accepts_valid_signature", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

		sig := signStripeWebhook(t, testSecret, body)
		req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", sig)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.False(t,
			rec.Code == http.
				StatusUnauthorized ||
				rec.Code == http.StatusServiceUnavailable,
		)

	})

	t.Run("configured_secret_rejects_wrong_signature", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

		// Sign with wrong secret
		sig := signStripeWebhook(t, "whsec_wrong_secret_for_test_purposes", body)
		req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", sig)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t,
			http.StatusUnauthorized,
			rec.
				Code)

	})
}
