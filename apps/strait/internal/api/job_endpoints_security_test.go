package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestSetJobEndpoint_RejectsHostnameResolvingToPrivateIP(t *testing.T) {
	t.Parallel()

	store := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		UpdateJobEndpointFunc: func(context.Context, string, string, string, string) error {
			require.Fail(t,

				"UpdateJobEndpoint must not be called for private DNS target")
			return nil
		},
	}
	srv := newTestServer(t, store, &mockQueue{}, nil)
	body := `{"endpoint_url":"https://internal.example.com/hook"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/endpoint", body))
	require.GreaterOrEqual(t,
		w.Code, 400)
}

func TestSetJobEndpoint_RejectsPrivateFallbackHostname(t *testing.T) {
	t.Parallel()

	store := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		UpdateJobEndpointFunc: func(context.Context, string, string, string, string) error {
			require.Fail(t,

				"UpdateJobEndpoint must not be called for private fallback DNS target")
			return nil
		},
	}
	srv := newTestServer(t, store, &mockQueue{}, nil)
	body := `{"endpoint_url":"https://example.com/hook","fallback_endpoint_url":"https://loopback.example.com/hook"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/endpoint", body))
	require.GreaterOrEqual(t,
		w.Code, 400)
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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotEmpty(t, storedSecret)
	require.True(
		t, straitcrypto.
			IsEncryptedField(
				storedSecret,
			))

	plaintext, err := straitcrypto.DecryptField(enc, storedSecret)
	require.NoError(t, err)
	require.False(t, plaintext ==
		"" || plaintext ==
		storedSecret,
	)
}

func TestSetJobEndpoint_RejectsSigningSecretWriteWithoutEncryptor(t *testing.T) {
	t.Parallel()

	store := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		UpdateJobEndpointFunc: func(context.Context, string, string, string, string) error {
			require.Fail(t,

				"UpdateJobEndpoint must not be called when signing secret encryption is unavailable")
			return nil
		},
	}
	srv := newTestServer(t, store, &mockQueue{}, nil)
	body := `{"endpoint_url":"https://example.com/hook","rotate_signing_secret":true}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/endpoint", body))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}
