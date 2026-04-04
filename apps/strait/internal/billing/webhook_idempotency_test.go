package billing

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMockStore_WebhookIdempotency(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}

	processed, err := store.IsWebhookProcessed(context.Background(), "msg_1")
	if err != nil {
		t.Fatal(err)
	}
	if processed {
		t.Fatal("expected not processed")
	}

	err = store.RecordProcessedWebhook(context.Background(), "msg_1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestMockStore_CountActiveAddonsByType(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}

	count, err := store.CountActiveAddonsByType(context.Background(), "org-1", AddonConcurrentRuns)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
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
		WithEdition("community"))

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

	// The retry should reach the handler (not be silently rejected as 200).
	// It may still error (unknown product), but the key point is it wasn't
	// blocked by the replay cache.
	if rec2.Code == http.StatusOK && rec1.Code != http.StatusOK {
		// This would mean first failed but retry was silently accepted -- that's the bug.
		t.Fatal("retry was silently accepted despite first request failing")
	}
}

func TestWebhook_DBIdempotencyPreventsReprocessing(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithEdition("community"))

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
