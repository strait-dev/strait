package scheduler

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"
)

// fakeQuotaOrgStore returns a fixed slice of project IDs for ListProjectsByOrg.
type fakeQuotaOrgStore struct {
	projects []string
}

func (f *fakeQuotaOrgStore) ListProjectsByOrg(_ context.Context, _ string) ([]string, error) {
	return f.projects, nil
}

// fakeQuotaDeliveryStore captures every delivery passed to
// CreateWebhookDelivery so the test can assert the wire shape.
type fakeQuotaDeliveryStore struct {
	subs       []domain.WebhookSubscription
	deliveries []*domain.WebhookDelivery
}

func (f *fakeQuotaDeliveryStore) ListWebhookSubscriptions(_ context.Context, _ string) ([]domain.WebhookSubscription, error) {
	return f.subs, nil
}

func (f *fakeQuotaDeliveryStore) CreateWebhookDelivery(_ context.Context, d *domain.WebhookDelivery) error {
	f.deliveries = append(f.deliveries, d)
	return nil
}

// TestQuotaWebhookAdapter_PreservesPayloadAndSubscription guards against the
// regression where the adapter built a WebhookDelivery without Payload,
// SubscriptionID, NextRetryAt, or RetryPolicy — leaving deliveries
// unreachable by the worker poll loop and devoid of an actual body.
func TestQuotaWebhookAdapter_PreservesPayloadAndSubscription(t *testing.T) {
	t.Parallel()

	subs := []domain.WebhookSubscription{
		{
			ID:         "sub-1",
			ProjectID:  "proj-1",
			WebhookURL: "https://example.com/hook",
			Active:     true,
			EventTypes: []string{domain.WebhookEventQuotaExceeded},
		},
	}
	deliveryStore := &fakeQuotaDeliveryStore{subs: subs}
	adapter := NewQuotaWebhookAdapter(
		&fakeQuotaOrgStore{projects: []string{"proj-1"}},
		deliveryStore,
	)

	payload := json.RawMessage(`{"org_id":"org-x","limit":1000,"used":1001}`)
	if err := adapter.NotifyQuotaExceeded(context.Background(), "org-x", payload); err != nil {
		t.Fatalf("notify: %v", err)
	}

	if len(deliveryStore.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveryStore.deliveries))
	}
	d := deliveryStore.deliveries[0]

	if string(d.Payload) != string(payload) {
		t.Fatalf("payload was dropped: got %q, want %q", string(d.Payload), string(payload))
	}
	if d.SubscriptionID != "sub-1" {
		t.Fatalf("subscription_id was dropped: got %q, want %q", d.SubscriptionID, "sub-1")
	}
	if d.ProjectID != "proj-1" {
		t.Fatalf("project_id was dropped: got %q, want %q", d.ProjectID, "proj-1")
	}
	if d.WebhookURL != "https://example.com/hook" {
		t.Fatalf("webhook_url mismatch: got %q", d.WebhookURL)
	}
	if d.RetryPolicy == "" {
		t.Fatal("retry_policy was empty; worker would skip retry scheduling")
	}
	if d.NextRetryAt == nil {
		t.Fatal("next_retry_at was nil; worker poll loop would never claim this delivery")
	}
	if d.Status != domain.WebhookStatusPending {
		t.Fatalf("status = %q, want %q", d.Status, domain.WebhookStatusPending)
	}
	if d.MaxAttempts <= 0 {
		t.Fatalf("max_attempts = %d, want > 0", d.MaxAttempts)
	}
}

// TestQuotaWebhookAdapter_SkipsInactiveAndNonMatching ensures the existing
// filters still hold after the bug fix.
func TestQuotaWebhookAdapter_SkipsInactiveAndNonMatching(t *testing.T) {
	t.Parallel()

	subs := []domain.WebhookSubscription{
		{ID: "active-match", ProjectID: "proj-1", WebhookURL: "https://a.example.com",
			Active: true, EventTypes: []string{domain.WebhookEventQuotaExceeded}},
		{ID: "inactive", ProjectID: "proj-1", WebhookURL: "https://b.example.com",
			Active: false, EventTypes: []string{domain.WebhookEventQuotaExceeded}},
		{ID: "wrong-type", ProjectID: "proj-1", WebhookURL: "https://c.example.com",
			Active: true, EventTypes: []string{"run.completed"}},
	}
	deliveryStore := &fakeQuotaDeliveryStore{subs: subs}
	adapter := NewQuotaWebhookAdapter(
		&fakeQuotaOrgStore{projects: []string{"proj-1"}},
		deliveryStore,
	)

	if err := adapter.NotifyQuotaExceeded(context.Background(), "org-x", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("notify: %v", err)
	}

	if len(deliveryStore.deliveries) != 1 || deliveryStore.deliveries[0].SubscriptionID != "active-match" {
		t.Fatalf("expected 1 delivery for active-match, got %+v", deliveryStore.deliveries)
	}
}
