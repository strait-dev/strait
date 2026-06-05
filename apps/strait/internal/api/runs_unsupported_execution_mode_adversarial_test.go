package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestCreateJob_UnsupportedExecutionMode_Rejected asserts that unsupported
// execution modes are rejected; clients must use http or worker mode.
func TestCreateJob_UnsupportedExecutionMode_Rejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		// GetJob and store methods are never reached for this validation error.
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id":     "proj-1",
		"name":           "unsupported-mode-job",
		"slug":           "unsupported-mode-job",
		"endpoint_url":   "https://example.com/run",
		"execution_mode": "managed",
		"timeout_secs":   60,
		"max_attempts":   3
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs", body))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
	require.True(
		t, strings.Contains(w.Body.String(), "ExecutionMode"))

	// The oneof validation tag rejects unrecognised execution modes with a validation_error.

}

// TestUpdateJob_UnsupportedExecutionMode_Rejected asserts that unsupported
// execution-mode updates are rejected.
func TestUpdateJob_UnsupportedExecutionMode_Rejected(t *testing.T) {
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
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
	require.True(
		t, strings.Contains(w.Body.String(), "ExecutionMode"))

	// The oneof validation tag rejects unrecognised execution modes with a validation_error.

}
