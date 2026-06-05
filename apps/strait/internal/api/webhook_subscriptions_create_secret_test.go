package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestCreateWebhookSubscription_ServerGeneratesSecret(t *testing.T) {
	t.Parallel()

	var storedSecret string
	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, sub *domain.WebhookSubscription) error {
			sub.ID = "sub-1"
			storedSecret = sub.Secret
			return nil
		},
	}
	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp struct {
		Subscription  *domain.WebhookSubscription `json:"subscription"`
		SigningSecret string                      `json:"signing_secret"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEqual(t, "", resp.SigningSecret)
	require.False(t, len(resp.SigningSecret) !=
		len("whsec_")+64 ||
		resp.SigningSecret[:6] != "whsec_")

	requireBase64EncryptedSecretPlaintext(t, enc, storedSecret, resp.SigningSecret)
	require.False(t, resp.Subscription ==
		nil ||
		resp.Subscription.
			ID != "sub-1",
	)

}

func TestCreateWebhookSubscription_ClientSecretIgnored(t *testing.T) {
	t.Parallel()

	var storedSecret string
	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, sub *domain.WebhookSubscription) error {
			sub.ID = "sub-1"
			storedSecret = sub.Secret
			return nil
		},
	}
	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)

	attackerChosen := "x"
	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"],"secret":"` + attackerChosen + `"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp struct {
		SigningSecret string `json:"signing_secret"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEqual(t, attackerChosen,
		resp.SigningSecret,
	)
	require.NotEqual(t, attackerChosen,
		storedSecret,
	)

	requireBase64EncryptedSecretPlaintext(t, enc, storedSecret, resp.SigningSecret)
	require.Len(t,
		resp.SigningSecret,
		len("whsec_")+64)

}
