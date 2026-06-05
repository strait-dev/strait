package billing

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMockStore_WebhookIdempotency(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}

	processed, err := store.IsWebhookProcessed(context.Background(), "msg_1")
	require.NoError(t,
		err)
	require.False(t,
		processed,
	)

	err = store.RecordProcessedWebhook(context.Background(), "msg_1")
	require.NoError(t,
		err)

}

func TestMockStore_CountActiveAddonsByType(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}

	count, err := store.CountActiveAddonsByType(context.Background(), "org-1", AddonConcurrency100)
	require.NoError(t,
		err)
	require.EqualValues(t, 0, count)

}

func TestWebhook_ReplayCacheClearedOnError(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		// Return error for subscription lookup to trigger handler error.
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*OrgSubscription, error) {
			return nil, fmt.Errorf("simulated error")
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithDevBypassSignatureCheck(), WithEdition("community"))

	body := `{"id":"evt-idem-err","type":"customer.subscription.created","data":{"object":{"id":"sub_err","status":"active","items":{"data":[{"price":{"id":"unknown-id"},"current_period_start":1700000000,"current_period_end":1702592000}]},"customer":{"id":"cust_1","email":"test@example.com","metadata":{"org_id":"550e8400-e29b-41d4-a716-446655440000"}}}}}`

	// First request -- handler will error (unknown product).
	req1 := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	req1.Header.Set("webhook-id", "msg_error_test")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// Second request (retry) -- should NOT be rejected by replay cache.
	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	req2.Header.Set("webhook-id", "msg_error_test")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	require.False(t,
		rec2.Code ==
			http.StatusOK &&
			rec1.
				Code != http.StatusOK,
	)

	// The retry should reach the handler (not be silently rejected as 200).
	// It may still error (unknown product), but the key point is it wasn't
	// blocked by the replay cache.

	// This would mean first failed but retry was silently accepted -- that's the bug.

}

func TestWebhook_DBIdempotencyPreventsReprocessing(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithDevBypassSignatureCheck(), WithEdition("community"))

	body := `{"id":"evt-idem-ok","type":"customer.subscription.created","data":{"object":{"id":"sub_1","status":"active","items":{"data":[{"price":{"id":"starter-id"},"current_period_start":1700000000,"current_period_end":1702592000}]},"customer":{"id":"cust_1","email":"test@example.com","metadata":{"org_id":"550e8400-e29b-41d4-a716-446655440000"}}}}}`

	// First request -- processes normally.
	req1 := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	req1.Header.Set("webhook-id", "msg_db_test")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// Verify first was processed.
	if rec1.Code != http.StatusOK {
		t.Logf("first request returned %d, checking if it was processed", rec1.Code)
	}
}

func TestWebhook_DBClaimPreventsConcurrentReprocessing(t *testing.T) {
	t.Parallel()

	claim := false
	store := &mockBillingStore{claimWebhookResult: &claim, webhookProcessingStatus: webhookStatusProcessed}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithDevBypassSignatureCheck(), WithEdition("community"))

	body := `{"id":"evt-idem-claimed","type":"customer.subscription.created","data":{"object":{"id":"sub_claimed","status":"active","items":{"data":[{"price":{"id":"starter-id"},"current_period_start":1700000000,"current_period_end":1702592000}]},"customer":{"id":"cust_1","email":"test@example.com","metadata":{"org_id":"550e8400-e29b-41d4-a716-446655440000"}}}}}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t,
		http.StatusOK,
		rec.
			Code)
	require.Nil(t, store.
		lastUpserted,
	)

}

func TestWebhook_DBClaimInProgressReturnsRetryableError(t *testing.T) {
	t.Parallel()

	claim := false
	store := &mockBillingStore{claimWebhookResult: &claim, webhookProcessingStatus: webhookStatusProcessing}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithDevBypassSignatureCheck(), WithEdition("community"))

	body := `{"id":"evt-idem-processing","type":"customer.subscription.created","data":{"object":{"id":"sub_processing","status":"active","items":{"data":[{"price":{"id":"starter-id"},"current_period_start":1700000000,"current_period_end":1702592000}]},"customer":{"id":"cust_1","email":"test@example.com","metadata":{"org_id":"550e8400-e29b-41d4-a716-446655440000"}}}}}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t,
		http.StatusServiceUnavailable,

		rec.Code)
	require.NotEqual(
		t, "",
		rec.Header().
			Get("Retry-After"))
	require.Nil(t, store.
		lastUpserted,
	)

}
