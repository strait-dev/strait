package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
)

// TestCreateJob_ManagedExecutionMode_Rejected asserts that any attempt to create
// a job with execution_mode="managed" returns HTTP 400. Managed execution is no
// longer supported; clients must use http or worker mode instead.
func TestCreateJob_ManagedExecutionMode_Rejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		// GetJob and store methods are never reached for this validation error.
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id":     "proj-1",
		"name":           "managed-job",
		"slug":           "managed-job",
		"endpoint_url":   "https://example.com/run",
		"execution_mode": "managed",
		"timeout_secs":   60,
		"max_attempts":   3
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for managed execution_mode, got %d: %s", w.Code, w.Body.String())
	}
	// The oneof validation tag rejects unrecognised execution modes with a validation_error.
	if !strings.Contains(w.Body.String(), "ExecutionMode") {
		t.Fatalf("expected ExecutionMode validation error in response body, got: %s", w.Body.String())
	}
}

// TestUpdateJob_ManagedExecutionMode_Rejected asserts that updating a job to
// execution_mode="managed" is rejected with HTTP 400.
func TestUpdateJob_ManagedExecutionMode_Rejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:            id,
				ProjectID:     "proj-1",
				Name:          "my-job",
				Slug:          "my-job",
				EndpointURL:   "https://example.com/run",
				ExecutionMode: domain.ExecutionModeHTTP,
				MaxAttempts:   3,
				TimeoutSecs:   60,
				Enabled:       true,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"execution_mode": "managed"}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-1", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for managed execution_mode on update, got %d: %s", w.Code, w.Body.String())
	}
	// The oneof validation tag rejects unrecognised execution modes with a validation_error.
	if !strings.Contains(w.Body.String(), "ExecutionMode") {
		t.Fatalf("expected ExecutionMode validation error in response body, got: %s", w.Body.String())
	}
}
