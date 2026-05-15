package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"
)

func TestSetJobEndpoint_RejectsHostnameResolvingToPrivateIP(t *testing.T) {
	t.Parallel()

	store := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		UpdateJobEndpointFunc: func(context.Context, string, string, string, string) error {
			t.Fatal("UpdateJobEndpoint must not be called for private DNS target")
			return nil
		},
	}
	srv := newTestServer(t, store, &mockQueue{}, nil)
	body := `{"endpoint_url":"https://internal.example.com/hook"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/endpoint", body))

	if w.Code < 400 {
		t.Fatalf("expected 4xx for hostname resolving to private IP, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetJobEndpoint_RejectsPrivateFallbackHostname(t *testing.T) {
	t.Parallel()

	store := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		UpdateJobEndpointFunc: func(context.Context, string, string, string, string) error {
			t.Fatal("UpdateJobEndpoint must not be called for private fallback DNS target")
			return nil
		},
	}
	srv := newTestServer(t, store, &mockQueue{}, nil)
	body := `{"endpoint_url":"https://example.com/hook","fallback_endpoint_url":"https://loopback.example.com/hook"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/endpoint", body))

	if w.Code < 400 {
		t.Fatalf("expected 4xx for fallback hostname resolving to loopback, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetJobEndpoint_StoresEncryptedRotatedSigningSecret(t *testing.T) {
	t.Parallel()

	enc := &mockEncryptor{}
	var storedSecret string
	store := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", EndpointSigningSecret: storedSecret}, nil
		},
		UpdateJobEndpointFunc: func(_ context.Context, _, _, _, signingSecret string) error {
			storedSecret = signingSecret
			return nil
		},
	}
	srv := newTestServerWithEncryptor(t, store, &mockQueue{}, enc)
	body := `{"endpoint_url":"https://example.com/hook","rotate_signing_secret":true}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/endpoint", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if storedSecret == "" {
		t.Fatal("UpdateJobEndpoint did not receive a signing secret")
	}
	if !straitcrypto.IsEncryptedField(storedSecret) {
		t.Fatalf("stored signing secret = %q, want encrypted field", storedSecret)
	}
	plaintext, err := straitcrypto.DecryptField(enc, storedSecret)
	if err != nil {
		t.Fatalf("decrypt stored signing secret: %v", err)
	}
	if plaintext == "" || plaintext == storedSecret {
		t.Fatalf("decrypted signing secret = %q, stored = %q", plaintext, storedSecret)
	}
}
