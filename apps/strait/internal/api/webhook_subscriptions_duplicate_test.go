package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// TestCreateWebhookSubscription_DuplicateURLReturns409 exercises the new
// partial-unique-index (migration 000302) handling: when the store reports a
// duplicate active subscription for (project_id, webhook_url), the handler
// must respond 409 Conflict and MUST NOT leak the freshly generated
// plaintext signing secret. Replaying the create response on retry would
// re-expose that one-shot secret, so the conflict path returns no body
// material that begins with the whsec_ prefix.
func TestCreateWebhookSubscription_DuplicateURLReturns409(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, _ *domain.WebhookSubscription) error {
			return store.ErrWebhookSubscriptionDuplicate
		},
	}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))
	require.Equal(t, http.StatusConflict,
		w.Code)
	require.NotContains(t, w.Body.String(), "whsec_")
}
