//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestCreateWebhookSubscription(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	sub := &domain.WebhookSubscription{
		ProjectID:  "proj-create-ws-" + newID(),
		WebhookURL: "https://example.com/hook",
		EventTypes: []string{"run.completed", "run.failed"},
		Secret:     "whsec_test",
		Active:     true,
	}

	if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription() error = %v", err)
	}
	if sub.ID == "" {
		t.Fatal("CreateWebhookSubscription() did not set ID")
	}
	if sub.CreatedAt.IsZero() {
		t.Fatal("CreateWebhookSubscription() did not set CreatedAt")
	}

	// Verify all fields round-trip.
	got, err := q.GetWebhookSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetWebhookSubscription() error = %v", err)
	}
	if got.ID != sub.ID {
		t.Fatalf("ID = %q, want %q", got.ID, sub.ID)
	}
	if got.ProjectID != sub.ProjectID {
		t.Fatalf("ProjectID = %q, want %q", got.ProjectID, sub.ProjectID)
	}
	if got.WebhookURL != sub.WebhookURL {
		t.Fatalf("WebhookURL = %q, want %q", got.WebhookURL, sub.WebhookURL)
	}
	if len(got.EventTypes) != len(sub.EventTypes) {
		t.Fatalf("EventTypes len = %d, want %d", len(got.EventTypes), len(sub.EventTypes))
	}
	for i, et := range got.EventTypes {
		if et != sub.EventTypes[i] {
			t.Fatalf("EventTypes[%d] = %q, want %q", i, et, sub.EventTypes[i])
		}
	}
	if got.Secret != sub.Secret {
		t.Fatalf("Secret = %q, want %q", got.Secret, sub.Secret)
	}
	if got.Active != sub.Active {
		t.Fatalf("Active = %v, want %v", got.Active, sub.Active)
	}
}

func TestCreateWebhookSubscription_CustomID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	customID := newID()
	sub := &domain.WebhookSubscription{
		ID:         customID,
		ProjectID:  "proj-ws-custom-id-" + newID(),
		WebhookURL: "https://example.com/custom",
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}

	if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription() error = %v", err)
	}
	if sub.ID != customID {
		t.Fatalf("ID = %q, want %q", sub.ID, customID)
	}
}

func TestGetWebhookSubscription_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetWebhookSubscription(ctx, newID())
	if !errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
		t.Fatalf("GetWebhookSubscription(missing) error = %v, want ErrWebhookSubscriptionNotFound", err)
	}
}

func TestListWebhookSubscriptions(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-list-ws-" + newID()
	otherProjectID := "proj-list-ws-other-" + newID()

	// Create two active subscriptions.
	for range 2 {
		sub := &domain.WebhookSubscription{
			ProjectID:  projectID,
			WebhookURL: "https://example.com/hook-" + newID(),
			EventTypes: []string{"run.completed"},
			Secret:     "s",
			Active:     true,
		}
		if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
			t.Fatalf("CreateWebhookSubscription() error = %v", err)
		}
	}

	// Create an inactive subscription (excluded from list).
	inactive := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: "https://example.com/inactive",
		EventTypes: []string{"run.failed"},
		Secret:     "s",
		Active:     false,
	}
	if err := q.CreateWebhookSubscription(ctx, inactive); err != nil {
		t.Fatalf("CreateWebhookSubscription(inactive) error = %v", err)
	}

	// Create one in another project.
	other := &domain.WebhookSubscription{
		ProjectID:  otherProjectID,
		WebhookURL: "https://example.com/other",
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	if err := q.CreateWebhookSubscription(ctx, other); err != nil {
		t.Fatalf("CreateWebhookSubscription(other) error = %v", err)
	}

	subs, err := q.ListWebhookSubscriptions(ctx, projectID)
	if err != nil {
		t.Fatalf("ListWebhookSubscriptions() error = %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("len = %d, want 2", len(subs))
	}
	for _, s := range subs {
		if s.ProjectID != projectID {
			t.Fatalf("ProjectID = %q, want %q", s.ProjectID, projectID)
		}
		if !s.Active {
			t.Fatal("listed subscription is inactive, want active only")
		}
	}

	// Empty project.
	empty, err := q.ListWebhookSubscriptions(ctx, "proj-ws-empty-"+newID())
	if err != nil {
		t.Fatalf("ListWebhookSubscriptions(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty len = %d, want 0", len(empty))
	}
}

func TestDeleteWebhookSubscription(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	sub := &domain.WebhookSubscription{
		ProjectID:  "proj-delete-ws-" + newID(),
		WebhookURL: "https://example.com/del",
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription() error = %v", err)
	}

	if err := q.DeleteWebhookSubscription(ctx, sub.ID); err != nil {
		t.Fatalf("DeleteWebhookSubscription() error = %v", err)
	}

	// Should be gone.
	_, err := q.GetWebhookSubscription(ctx, sub.ID)
	if !errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
		t.Fatalf("GetWebhookSubscription(deleted) error = %v, want ErrWebhookSubscriptionNotFound", err)
	}
}

func TestDeleteWebhookSubscription_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteWebhookSubscription(ctx, newID())
	if !errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
		t.Fatalf("DeleteWebhookSubscription(missing) error = %v, want ErrWebhookSubscriptionNotFound", err)
	}
}
