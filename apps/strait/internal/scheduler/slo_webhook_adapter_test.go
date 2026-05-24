package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

type fakeSLOWebhookStore struct {
	subs       []domain.WebhookSubscription
	deliveries []domain.WebhookDelivery
}

func (f *fakeSLOWebhookStore) ListWebhookSubscriptions(context.Context, string) ([]domain.WebhookSubscription, error) {
	return f.subs, nil
}

func (f *fakeSLOWebhookStore) CreateWebhookDelivery(_ context.Context, d *domain.WebhookDelivery) error {
	f.deliveries = append(f.deliveries, *d)
	return nil
}

func TestSLOWebhookAdapter_CreatesClaimableSubscriptionDelivery(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"event":"slo.budget_warning","slo_id":"slo-1"}`)
	store := &fakeSLOWebhookStore{
		subs: []domain.WebhookSubscription{
			{
				ID:         "sub-1",
				ProjectID:  "proj-1",
				WebhookURL: "https://example.com/slo",
				EventTypes: []string{domain.WebhookEventSLOBudgetWarning},
				Active:     true,
			},
		},
	}

	adapter := NewSLOWebhookAdapter(store)
	if err := adapter.NotifySLOBudgetWarning(context.Background(), "proj-1", payload); err != nil {
		t.Fatalf("NotifySLOBudgetWarning: %v", err)
	}

	if len(store.deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(store.deliveries))
	}
	d := store.deliveries[0]
	if d.SubscriptionID != "sub-1" {
		t.Fatalf("SubscriptionID = %q, want sub-1", d.SubscriptionID)
	}
	if d.ProjectID != "proj-1" {
		t.Fatalf("ProjectID = %q, want proj-1", d.ProjectID)
	}
	if string(d.Payload) != string(payload) {
		t.Fatalf("Payload = %s, want %s", d.Payload, payload)
	}
	if d.RetryPolicy != domain.WebhookRetryPolicyExponential {
		t.Fatalf("RetryPolicy = %q, want %q", d.RetryPolicy, domain.WebhookRetryPolicyExponential)
	}
	if d.NextRetryAt == nil || d.NextRetryAt.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("NextRetryAt = %v, want due delivery", d.NextRetryAt)
	}
}
