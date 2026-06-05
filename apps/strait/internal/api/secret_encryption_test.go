package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestHandleCreateSecret_DelegatesValueEncryptionToStore(t *testing.T) {
	t.Parallel()

	apiEncryptor := &countingSecretEncryptor{}
	var storedSecret *domain.JobSecret
	ms := &APIStoreMock{
		CreateJobSecretFunc: func(_ context.Context, secret *domain.JobSecret) error {
			storedSecret = secret
			secret.ID = "sec-enc-1"
			secret.KeyVersion = 1
			secret.CreatedAt = time.Now().UTC()
			secret.UpdatedAt = secret.CreatedAt
			return nil
		},
	}

	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, apiEncryptor)

	body := `{"project_id":"proj-1","job_id":"job-1","environment":"production","secret_key":"DB_PASSWORD","value":"super-secret-value"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets/", body))
	require.Equal(t, http.StatusCreated,
		w.Code)
	require.NotNil(t, storedSecret)
	require.Equal(t, "super-secret-value",
		storedSecret.
			EncryptedValue,
	)
	require.EqualValues(t, 0, apiEncryptor.
		calls)

}

func TestHandleCreateSecret_WithoutEncryptor_StoresRaw(t *testing.T) {
	t.Parallel()

	var storedSecret *domain.JobSecret
	ms := &APIStoreMock{
		CreateJobSecretFunc: func(_ context.Context, secret *domain.JobSecret) error {
			storedSecret = secret
			secret.ID = "sec-raw-1"
			secret.KeyVersion = 1
			secret.CreatedAt = time.Now().UTC()
			secret.UpdatedAt = secret.CreatedAt
			return nil
		},
	}

	// Use server with encryption key configured but no encryptor injected.
	srv := newTestServerWithEncryption(t, ms, &mockQueue{})

	body := `{"project_id":"proj-1","job_id":"job-1","environment":"production","secret_key":"API_KEY","value":"plain-value"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets/", body))
	require.Equal(t, http.StatusCreated,
		w.Code)
	require.NotNil(t, storedSecret)
	require.Equal(t, "plain-value",
		storedSecret.EncryptedValue,
	)

	// Without encryptor, value is stored as-is (backward compatible fallback).

}

func TestHandleCreateSecret_PassesPlaintextValuesToStore(t *testing.T) {
	t.Parallel()

	plaintexts := []string{
		"short",
		"a-longer-secret-with-special-chars!@#$%^&*()",
	}

	for _, pt := range plaintexts {
		t.Run("plaintext_"+pt, func(t *testing.T) {
			t.Parallel()

			var storedSecret *domain.JobSecret
			ms := &APIStoreMock{
				CreateJobSecretFunc: func(_ context.Context, secret *domain.JobSecret) error {
					storedSecret = secret
					secret.ID = "sec-diff-1"
					secret.KeyVersion = 1
					secret.CreatedAt = time.Now().UTC()
					secret.UpdatedAt = secret.CreatedAt
					return nil
				},
			}

			apiEncryptor := &countingSecretEncryptor{}
			srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, apiEncryptor)

			body := `{"project_id":"proj-1","job_id":"job-1","environment":"production","secret_key":"KEY","value":"` + pt + `"}`
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets/", body))
			require.Equal(t, http.StatusCreated,
				w.Code)
			require.NotNil(t, storedSecret)
			require.Equal(t, pt, storedSecret.
				EncryptedValue,
			)
			require.EqualValues(t, 0, apiEncryptor.
				calls)

		})
	}
}
