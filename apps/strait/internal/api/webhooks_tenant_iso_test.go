package api

import (
	"context"
	"net/http"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestTenantIso_Webhooks_ReplayDelivery_RejectsEmptyProjectID verifies that
// a webhook delivery with no JobID and no ProjectID (e.g. a system delivery
// not bound to a project) cannot be replayed by a project-scoped caller.
func TestTenantIso_Webhooks_ReplayDelivery_RejectsEmptyProjectID(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(_ context.Context, _ string, id string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: id, JobID: "", ProjectID: ""}, nil
		},
		ReplayWebhookDeliveryFunc: func(_ context.Context, _, _ string) (*domain.WebhookDelivery, error) {
			require.Fail(t,

				"ReplayWebhookDelivery must not be called for empty-project delivery")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleReplayWebhookDelivery(ctx, &ReplayWebhookDeliveryInput{ID: "wd-1"})
	require.True(
		t, isHumaStatusError(err,

			http.StatusNotFound))
}
