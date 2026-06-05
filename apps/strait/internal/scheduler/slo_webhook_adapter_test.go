package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.NoError(t,
		adapter.NotifySLOBudgetWarning(context.
			Background(), "proj-1",
			payload,
		))
	require.Len(t, store.
		deliveries, 1)

	d := store.deliveries[0]
	require.Equal(t, "sub-1",
		d.SubscriptionID,
	)
	require.Equal(t, "proj-1",
		d.ProjectID,
	)
	require.Equal(t, string(payload), string(d.Payload))
	require.Equal(t, domain.
		WebhookRetryPolicyExponential,

		d.
			RetryPolicy)
	require.False(t, d.
		NextRetryAt == nil ||
		d.NextRetryAt.
			After(time.Now().UTC().Add(
				time.
					Second)))
}
