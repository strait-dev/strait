//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestWebhookSubscriptionCRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	sub := &domain.WebhookSubscription{
		ProjectID:  "project-webhook-subscriptions",
		WebhookURL: "https://example.com/hook",
		EventTypes: []string{"run.completed", "run.failed"},
		Secret:     "secret-1",
		Active:     true,
	}

	if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription() error = %v", err)
	}
	if sub.ID == "" {
		t.Fatal("CreateWebhookSubscription() did not set ID")
	}

	subs, err := q.ListWebhookSubscriptions(ctx, sub.ProjectID)
	if err != nil {
		t.Fatalf("ListWebhookSubscriptions() error = %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("ListWebhookSubscriptions() len = %d, want 1", len(subs))
	}
	if subs[0].ID != sub.ID {
		t.Fatalf("subscription id = %q, want %q", subs[0].ID, sub.ID)
	}

	if err := q.DeleteWebhookSubscription(ctx, sub.ID); err != nil {
		t.Fatalf("DeleteWebhookSubscription() error = %v", err)
	}

	_, err = q.GetWebhookSubscription(ctx, sub.ID)
	if !errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
		t.Fatalf("GetWebhookSubscription() error = %v, want ErrWebhookSubscriptionNotFound", err)
	}
}
