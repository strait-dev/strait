package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
)

func newTestServerWithEncryption(t *testing.T, s APIStore, q *mockQueue) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
		SecretEncryptionKey: "test-encryption-key-32-chars-ok",
	}
	var p pubsub.Publisher
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   s,
		Queue:   q,
		PubSub:  p,
		Edition: domain.EditionCommunity,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestHandleCreateSecret_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateJobSecretFunc: func(_ context.Context, secret *domain.JobSecret) error {
			secret.ID = "sec-123"
			secret.KeyVersion = 1
			secret.CreatedAt = time.Now().UTC()
			secret.UpdatedAt = secret.CreatedAt
			return nil
		},
	}

	srv := newTestServerWithEncryption(t, ms, &mockQueue{})

	body := `{"project_id":"proj-1","job_id":"job-1","environment":"production","secret_key":"API_KEY","value":"super-secret"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["id"] != "sec-123" {
		t.Fatalf("expected id=sec-123, got %v", resp["id"])
	}
	if _, ok := resp["encrypted_value"]; ok {
		t.Fatal("response should not include encrypted_value")
	}
	if _, ok := resp["value"]; ok {
		t.Fatal("response should not include value")
	}
}

type countingSecretEncryptor struct {
	calls int
}

func (e *countingSecretEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	e.calls++
	return append([]byte("api-encrypted:"), plaintext...), nil
}

func (e *countingSecretEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	return ciphertext, nil
}

func TestHandleCreateSecret_LeavesEncryptionToStore(t *testing.T) {
	t.Parallel()

	apiEncryptor := &countingSecretEncryptor{}
	var storedValue string
	ms := &APIStoreMock{
		CreateJobSecretFunc: func(_ context.Context, secret *domain.JobSecret) error {
			storedValue = secret.EncryptedValue
			secret.ID = "sec-plain"
			secret.KeyVersion = 1
			secret.CreatedAt = time.Now().UTC()
			secret.UpdatedAt = secret.CreatedAt
			return nil
		},
	}
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
		SecretEncryptionKey: "test-encryption-key-32-chars-ok",
	}
	srv := NewServer(ServerDeps{
		Config:    cfg,
		Store:     ms,
		Queue:     &mockQueue{},
		Encryptor: apiEncryptor,
		Edition:   domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	body := `{"project_id":"proj-1","job_id":"job-1","environment":"production","secret_key":"API_KEY","value":"super-secret"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if storedValue != "super-secret" {
		t.Fatalf("store should receive plaintext exactly once, got %q", storedValue)
	}
	if apiEncryptor.calls != 0 {
		t.Fatalf("api encryptor should not encrypt job secrets; got %d calls", apiEncryptor.calls)
	}
}

func TestHandleCreateSecret_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets/", `{"project_id":"proj-1"}`))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleListSecrets_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListJobSecretsFunc: func(_ context.Context, projectID, jobID, environment string, _ int, _ *time.Time) ([]domain.JobSecret, error) {
			if projectID != "proj-1" || jobID != "job-1" || environment != "production" {
				t.Fatalf("unexpected params: %q %q %q", projectID, jobID, environment)
			}
			return []domain.JobSecret{{ID: "sec-1", ProjectID: projectID, JobID: jobID, Environment: environment, SecretKey: "API_KEY", KeyVersion: 1}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/secrets/?job_id=job-1&environment=production", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteSecret_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobSecretFunc: func(_ context.Context, id string, _ string) (*domain.JobSecret, error) {
			return &domain.JobSecret{ID: id, ProjectID: "test-project", SecretKey: "KEY"}, nil
		},
		DeleteJobSecretFunc: func(_ context.Context, id string, _ string) error {
			if id != "sec-1" {
				t.Fatalf("unexpected id: %q", id)
			}
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/secrets/sec-1", ""))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateSecret_NoEncryptionKey_Returns503(t *testing.T) {
	t.Parallel()
	// Default test server has no SecretEncryptionKey set.
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","job_id":"job-1","secret_key":"API_KEY","value":"super-secret"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets/", body))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
