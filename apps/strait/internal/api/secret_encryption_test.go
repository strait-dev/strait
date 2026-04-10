package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestHandleCreateSecret_EncryptsValue(t *testing.T) {
	t.Parallel()

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

	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	body := `{"project_id":"proj-1","job_id":"job-1","environment":"production","secret_key":"DB_PASSWORD","value":"super-secret-value"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	if storedSecret == nil {
		t.Fatal("CreateJobSecret was not called")
	}

	// The encrypted value should NOT equal the plaintext input.
	if storedSecret.EncryptedValue == "super-secret-value" {
		t.Fatal("secret EncryptedValue was stored as plaintext, expected actually encrypted value")
	}

	// Verify decryption roundtrip.
	enc := &mockEncryptor{}
	decrypted, err := enc.Decrypt([]byte(storedSecret.EncryptedValue))
	if err != nil {
		t.Fatalf("failed to decrypt stored value: %v", err)
	}
	if string(decrypted) != "super-secret-value" {
		t.Fatalf("decrypted value = %q, want %q", string(decrypted), "super-secret-value")
	}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	if storedSecret == nil {
		t.Fatal("CreateJobSecret was not called")
	}

	// Without encryptor, value is stored as-is (backward compatible fallback).
	if storedSecret.EncryptedValue != "plain-value" {
		t.Fatalf("without encryptor, value should be stored raw, got %q", storedSecret.EncryptedValue)
	}
}

func TestHandleCreateSecret_EncryptedValueDiffersFromPlaintext(t *testing.T) {
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

			srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

			body := `{"project_id":"proj-1","job_id":"job-1","environment":"production","secret_key":"KEY","value":"` + pt + `"}`
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets/", body))

			if w.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
			}

			if storedSecret == nil {
				t.Fatal("CreateJobSecret was not called")
			}

			if storedSecret.EncryptedValue == pt {
				t.Fatalf("EncryptedValue should differ from plaintext %q", pt)
			}
		})
	}
}
