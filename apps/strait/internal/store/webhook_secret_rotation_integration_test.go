//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sourcegraph/conc"

	"strait/internal/domain"
	"strait/internal/store"
)

// Integration tests for RotateWebhookSecret, which was previously only
// exercised indirectly through webhook-signing tests and had no direct
// coverage of the rotation semantics (current -> previous move, grace
// period, not-found error, concurrent rotation).

func createSubForRotation(t *testing.T, ctx context.Context, q *store.Queries, projectID, secret string) *domain.WebhookSubscription {
	t.Helper()
	sub := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: "https://example.com/rotate-" + newID(),
		EventTypes: []string{"run.completed"},
		Secret:     secret,
		Active:     true,
	}
	if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription: %v", err)
	}
	return sub
}

func TestRotateWebhookSecret_ReplacesCurrentKeepsPrevious(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rot-" + newID()
	sub := createSubForRotation(t, ctx, q, projectID, "secret-original")

	gracePoint := time.Now().Add(time.Hour).UTC()
	if err := q.RotateWebhookSecret(ctx, sub.ID, "secret-new", gracePoint); err != nil {
		t.Fatalf("RotateWebhookSecret: %v", err)
	}

	current, previous, grace, err := q.GetWebhookSubscriptionSecrets(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetWebhookSubscriptionSecrets: %v", err)
	}
	if current != "secret-new" {
		t.Fatalf("current = %q, want %q", current, "secret-new")
	}
	if previous != "secret-original" {
		t.Fatalf("previous = %q, want %q", previous, "secret-original")
	}
	if grace == nil {
		t.Fatal("grace_expires_at should be set after rotation")
	}
	if !grace.Round(time.Second).Equal(gracePoint.Round(time.Second)) {
		t.Fatalf("grace = %v, want %v", grace, gracePoint)
	}
}

func TestRotateWebhookSecret_ChainedRotation_LatestTwoSecretsRetained(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rot-chain-" + newID()
	sub := createSubForRotation(t, ctx, q, projectID, "secret-v1")

	// Rotate v1 -> v2.
	if err := q.RotateWebhookSecret(ctx, sub.ID, "secret-v2", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("first rotation: %v", err)
	}
	// Rotate v2 -> v3. The previous slot now holds v2, not v1.
	if err := q.RotateWebhookSecret(ctx, sub.ID, "secret-v3", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("second rotation: %v", err)
	}

	current, previous, _, err := q.GetWebhookSubscriptionSecrets(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetWebhookSubscriptionSecrets: %v", err)
	}
	if current != "secret-v3" {
		t.Fatalf("current = %q, want secret-v3", current)
	}
	if previous != "secret-v2" {
		t.Fatalf("previous = %q, want secret-v2 (v1 should have been overwritten)", previous)
	}
}

func TestRotateWebhookSecret_NotFound_ReturnsErr(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.RotateWebhookSecret(ctx, "sub-does-not-exist-"+newID(), "secret-x", time.Now().Add(time.Hour))
	if !errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
		t.Fatalf("err = %v, want ErrWebhookSubscriptionNotFound", err)
	}
}

func TestRotateWebhookSecret_ConcurrentRotations_BothSucceedFinalStateConsistent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rot-conc-" + newID()
	sub := createSubForRotation(t, ctx, q, projectID, "secret-original")

	// Two goroutines race to rotate. Both UPDATEs will succeed (there's
	// no conditional WHERE beyond the id match), so the final state
	// depends on Postgres ordering. The invariants to verify are:
	//   1. Both calls returned nil error.
	//   2. Current secret is one of the two new values.
	//   3. Previous secret is non-empty (one of the rotations staged it).
	var wg conc.WaitGroup
	errs := make([]error, 2)
	secrets := []string{"secret-race-a", "secret-race-b"}
	for i := range 2 {
		wg.Go(func() {
			errs[i] = q.RotateWebhookSecret(ctx, sub.ID, secrets[i], time.Now().Add(time.Hour))
		})
	}
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Fatalf("rotation %d error: %v", i, e)
		}
	}

	current, previous, _, err := q.GetWebhookSubscriptionSecrets(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetWebhookSubscriptionSecrets: %v", err)
	}
	if current != "secret-race-a" && current != "secret-race-b" {
		t.Fatalf("current = %q, want one of the race secrets", current)
	}
	if previous == "" {
		t.Fatal("previous should be set after any rotation")
	}
}
