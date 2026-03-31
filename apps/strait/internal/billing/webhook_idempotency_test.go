package billing

import (
	"context"
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
