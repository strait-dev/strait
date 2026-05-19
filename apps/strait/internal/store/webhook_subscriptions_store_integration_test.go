//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
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

func TestCreateWebhookSubscriptionWithOrgLimit_ConcurrentCreatesCannotExceedLimit(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-webhook-limit-" + newID()
	projectID := "proj-webhook-limit-" + newID()
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "Webhook Limit"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	const maxEndpoints = 3
	const attempts = 16

	start := make(chan struct{})
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			sub := &domain.WebhookSubscription{
				ID:         "sub-webhook-limit-" + newID(),
				ProjectID:  projectID,
				WebhookURL: "https://example.com/hook/" + newID(),
				EventTypes: []string{"run.completed"},
				Secret:     "whsec_test",
				Active:     true,
			}
			if i%2 == 0 {
				sub.EventTypes = []string{"run.failed"}
			}
			errs <- q.CreateWebhookSubscriptionWithOrgLimit(ctx, sub, orgID, maxEndpoints)
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)

	var created, limited int
	for err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, store.ErrWebhookEndpointLimitExceeded):
			limited++
		default:
			t.Fatalf("unexpected create error: %v", err)
		}
	}
	if created != maxEndpoints {
		t.Fatalf("created = %d, want %d", created, maxEndpoints)
	}
	if limited != attempts-maxEndpoints {
		t.Fatalf("limited = %d, want %d", limited, attempts-maxEndpoints)
	}

	count, err := q.CountWebhookSubscriptionsByOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("CountWebhookSubscriptionsByOrg() error = %v", err)
	}
	if count != maxEndpoints {
		t.Fatalf("stored active webhook endpoints = %d, want %d", count, maxEndpoints)
	}
}

func TestCountWebhookSubscriptionsByOrg_IncludesSiblingProjectsUnderProjectRLS(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-webhook-rls-count-" + newID()
	projectA := "proj-webhook-rls-count-a-" + newID()
	projectB := "proj-webhook-rls-count-b-" + newID()
	for _, projectID := range []string{projectA, projectB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: projectID}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", projectID, err)
		}
	}

	for _, projectID := range []string{projectA, projectB} {
		sub := &domain.WebhookSubscription{
			ID:         "sub-webhook-rls-count-" + newID(),
			ProjectID:  projectID,
			WebhookURL: "https://example.com/hook/" + newID(),
			EventTypes: []string{"run.completed"},
			Secret:     "whsec_test",
			Active:     true,
		}
		if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
			t.Fatalf("CreateWebhookSubscription(%s) error = %v", projectID, err)
		}
	}

	var count int
	var err error
	runAsProject(t, ctx, projectA, false, func(txq *store.Queries) {
		count, err = txq.CountWebhookSubscriptionsByOrg(ctx, orgID)
	})
	if err != nil {
		t.Fatalf("CountWebhookSubscriptionsByOrg() under project RLS error = %v", err)
	}
	if count != 2 {
		t.Fatalf("CountWebhookSubscriptionsByOrg() under project RLS = %d, want 2", count)
	}
}

func TestCreateWebhookSubscriptionWithOrgLimit_CountsSiblingProjectsUnderProjectRLS(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-webhook-rls-limit-" + newID()
	projectA := "proj-webhook-rls-limit-a-" + newID()
	projectB := "proj-webhook-rls-limit-b-" + newID()
	for _, projectID := range []string{projectA, projectB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: projectID}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", projectID, err)
		}
	}

	existing := &domain.WebhookSubscription{
		ID:         "sub-webhook-rls-existing-" + newID(),
		ProjectID:  projectB,
		WebhookURL: "https://example.com/hook/existing",
		EventTypes: []string{"run.completed"},
		Secret:     "whsec_test",
		Active:     true,
	}
	if err := q.CreateWebhookSubscription(ctx, existing); err != nil {
		t.Fatalf("CreateWebhookSubscription(existing) error = %v", err)
	}

	candidate := &domain.WebhookSubscription{
		ID:         "sub-webhook-rls-candidate-" + newID(),
		ProjectID:  projectA,
		WebhookURL: "https://example.com/hook/candidate",
		EventTypes: []string{"run.failed"},
		Secret:     "whsec_test",
		Active:     true,
	}

	var err error
	runAsProject(t, ctx, projectA, false, func(txq *store.Queries) {
		err = txq.CreateWebhookSubscriptionWithOrgLimit(ctx, candidate, orgID, 1)
	})
	if !errors.Is(err, store.ErrWebhookEndpointLimitExceeded) {
		t.Fatalf("CreateWebhookSubscriptionWithOrgLimit() error = %v, want ErrWebhookEndpointLimitExceeded", err)
	}
}

func TestWebhookDeliverySubscriptionPayloadRoundTrip(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	sub := &domain.WebhookSubscription{
		ProjectID:  "proj-wd-sub-" + newID(),
		WebhookURL: "https://example.com/hook",
		EventTypes: []string{"run.completed"},
		Secret:     "whsec_test",
		Active:     true,
	}
	if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription() error = %v", err)
	}

	payload := json.RawMessage(`{"run_id":"run-1","status":"completed"}`)
	delivery := &domain.WebhookDelivery{
		SubscriptionID: sub.ID,
		WebhookURL:     sub.WebhookURL,
		Status:         domain.WebhookStatusPending,
		Attempts:       0,
		MaxAttempts:    3,
		Payload:        payload,
		LastError:      string(payload),
	}
	if err := q.CreateWebhookDelivery(ctx, delivery); err != nil {
		t.Fatalf("CreateWebhookDelivery() error = %v", err)
	}

	got, err := q.GetWebhookDelivery(ctx, delivery.ID)
	if err != nil {
		t.Fatalf("GetWebhookDelivery() error = %v", err)
	}
	if got.SubscriptionID != sub.ID {
		t.Fatalf("SubscriptionID = %q, want %q", got.SubscriptionID, sub.ID)
	}
	if got.ProjectID != sub.ProjectID {
		t.Fatalf("ProjectID = %q, want %q", got.ProjectID, sub.ProjectID)
	}
	if !jsonPayloadEqual(got.Payload, payload) {
		t.Fatalf("Payload = %s, want %s", got.Payload, payload)
	}

	replay, err := q.ReplayWebhookDelivery(ctx, delivery.ID)
	if err != nil {
		t.Fatalf("ReplayWebhookDelivery() error = %v", err)
	}
	if replay.SubscriptionID != sub.ID {
		t.Fatalf("replay SubscriptionID = %q, want %q", replay.SubscriptionID, sub.ID)
	}
	if !jsonPayloadEqual(replay.Payload, payload) {
		t.Fatalf("replay Payload = %s, want %s", replay.Payload, payload)
	}
}

func jsonPayloadEqual(a, b json.RawMessage) bool {
	var av any
	var bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
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

func TestDeleteWebhookSubscription_WithDeliveriesDetachesHistory(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	sub := &domain.WebhookSubscription{
		ProjectID:  "proj-delete-ws-delivery-" + newID(),
		WebhookURL: "https://example.com/del-with-history",
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription() error = %v", err)
	}

	delivery := &domain.WebhookDelivery{
		SubscriptionID: sub.ID,
		WebhookURL:     sub.WebhookURL,
		Status:         domain.WebhookStatusDelivered,
		Attempts:       1,
		MaxAttempts:    3,
		Payload:        json.RawMessage(`{"status":"completed"}`),
	}
	if err := q.CreateWebhookDelivery(ctx, delivery); err != nil {
		t.Fatalf("CreateWebhookDelivery() error = %v", err)
	}

	if err := q.DeleteWebhookSubscription(ctx, sub.ID); err != nil {
		t.Fatalf("DeleteWebhookSubscription() error = %v", err)
	}

	if _, err := q.GetWebhookSubscription(ctx, sub.ID); !errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
		t.Fatalf("GetWebhookSubscription(deleted) error = %v, want ErrWebhookSubscriptionNotFound", err)
	}
	gotDelivery, err := q.GetWebhookDelivery(ctx, delivery.ID)
	if err != nil {
		t.Fatalf("GetWebhookDelivery(history) error = %v", err)
	}
	if gotDelivery.SubscriptionID != "" {
		t.Fatalf("SubscriptionID = %q, want detached empty value", gotDelivery.SubscriptionID)
	}
	if gotDelivery.ProjectID != sub.ProjectID {
		t.Fatalf("ProjectID = %q, want %q", gotDelivery.ProjectID, sub.ProjectID)
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
