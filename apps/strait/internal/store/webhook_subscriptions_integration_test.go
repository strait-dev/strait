//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
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
	require.NoError(t, q.CreateWebhookSubscription(ctx, sub))
	require.NotEqual(t, "",

		sub.ID)

	subs, err := q.ListWebhookSubscriptions(ctx, sub.ProjectID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, sub.ID,

		subs[0].
			ID)
	require.NoError(t, q.DeleteWebhookSubscription(ctx, sub.
		ID))

	_, err = q.GetWebhookSubscription(ctx, sub.ID)
	require.True(t, errors.Is(err, store.
		ErrWebhookSubscriptionNotFound,
	))

}
