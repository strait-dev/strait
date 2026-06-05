//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

	return sub
}

func TestRotateWebhookSecret_ReplacesCurrentKeepsPrevious(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rot-" + newID()
	sub := createSubForRotation(t, ctx, q, projectID, "secret-original")

	gracePoint := time.Now().Add(time.Hour).UTC()
	require.NoError(t, q.RotateWebhookSecret(ctx,
		sub.ID,
		"secret-new",
		gracePoint,
	))

	current, previous, grace, err := q.GetWebhookSubscriptionSecrets(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, "secret-new",

		current,
	)
	require.Equal(t, "secret-original",

		previous,
	)
	require.NotNil(t, grace)
	require.True(t, grace.Round(time.
		Second).Equal(gracePoint.
		Round(time.
			Second,
		)))

}

func TestRotateWebhookSecret_ChainedRotation_LatestTwoSecretsRetained(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rot-chain-" + newID()
	sub := createSubForRotation(t, ctx, q, projectID, "secret-v1")
	require.NoError(t, q.RotateWebhookSecret(ctx,
		sub.ID,
		"secret-v2",
		time.Now().Add(time.Hour)))
	require.NoError(t, q.RotateWebhookSecret(ctx,
		sub.ID,
		"secret-v3",
		time.Now().Add(time.Hour)))

	// Rotate v1 -> v2.

	// Rotate v2 -> v3. The previous slot now holds v2, not v1.

	current, previous, _, err := q.GetWebhookSubscriptionSecrets(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, "secret-v3",

		current,
	)
	require.Equal(t, "secret-v2",

		previous,
	)

}

func TestRotateWebhookSecret_NotFound_ReturnsErr(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.RotateWebhookSecret(ctx, "sub-does-not-exist-"+newID(), "secret-x", time.Now().Add(time.Hour))
	require.True(t, errors.Is(err, store.
		ErrWebhookSubscriptionNotFound,
	))

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

	for _, e := range errs {
		require.Nil(t, e)

	}

	current, previous, _, err := q.GetWebhookSubscriptionSecrets(ctx, sub.ID)
	require.NoError(t, err)
	require.False(t, current !=
		"secret-race-a" &&
		current !=
			"secret-race-b",
	)
	require.NotEqual(t, "",

		previous)

}
