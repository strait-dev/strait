package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
)

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

	srv := newTestServer(t, ms, &mockQueue{}, nil)

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

func TestHandleCreateSecret_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets/", `{"project_id":"proj-1"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
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
		DeleteJobSecretFunc: func(_ context.Context, id string) error {
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
