package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Subscription  *domain.WebhookSubscription `json:"subscription"`
		SigningSecret string                      `json:"signing_secret"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SigningSecret == "" {
		t.Fatal("response signing_secret is empty")
	}
	if len(resp.SigningSecret) != len("whsec_")+64 || resp.SigningSecret[:6] != "whsec_" {
		t.Fatalf("signing_secret %q does not match whsec_ + 64 hex chars", resp.SigningSecret)
	}
	requireBase64EncryptedSecretPlaintext(t, enc, storedSecret, resp.SigningSecret)
	if resp.Subscription == nil || resp.Subscription.ID != "sub-1" {
		t.Fatalf("response subscription missing or wrong ID: %+v", resp.Subscription)
	}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		SigningSecret string `json:"signing_secret"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SigningSecret == attackerChosen {
		t.Fatal("client-supplied secret was used; expected server to generate its own")
	}
	if storedSecret == attackerChosen {
		t.Fatalf("client-supplied secret was stored: %q", storedSecret)
	}
	requireBase64EncryptedSecretPlaintext(t, enc, storedSecret, resp.SigningSecret)
	if len(resp.SigningSecret) != len("whsec_")+64 {
		t.Fatalf("server-generated signing_secret has wrong length: %q", resp.SigningSecret)
	}
}
