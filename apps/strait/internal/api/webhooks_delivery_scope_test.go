package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestReplayWebhookDelivery_ScopesStoreByProject is the regression guard for the
// dual-layer tenant scoping on webhook deliveries: the replay handler must pass
// the caller's project to the (now project-scoped) GetWebhookDelivery/Replay
// store calls, not just rely on RLS.
func TestReplayWebhookDelivery_ScopesStoreByProject(t *testing.T) {
	t.Parallel()

	var gotGetProject string
	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(_ context.Context, projectID, id string) (*domain.WebhookDelivery, error) {
			gotGetProject = projectID
			return &domain.WebhookDelivery{ID: id, ProjectID: projectID}, nil
		},
		ReplayWebhookDeliveryFunc: func(_ context.Context, projectID, id string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: "replay-" + id, ProjectID: projectID}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/webhooks/deliveries/del-1/replay", "", "proj-1"))

	require.Equal(t, "proj-1", gotGetProject,
		"GetWebhookDelivery must be scoped to the caller's project")
}
