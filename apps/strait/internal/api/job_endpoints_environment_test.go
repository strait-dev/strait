package api

import (
	"context"
	"net/http"
	"testing"

	"strait/internal/domain"
)

func TestHandleSetJobEndpoint_EnvironmentScopedCallerCannotReplaceOtherEnvironmentEndpoint(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
		UpdateJobEndpointFunc: func(_ context.Context, _, _, _, _ string) error {
			t.Fatal("UpdateJobEndpoint should not be called for a mismatched environment")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleSetJobEndpoint(ctx, &SetJobEndpointInput{
		JobID: "job-1",
		Body:  SetJobEndpointRequest{EndpointURL: "https://executor.example.com/run"},
	})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 for environment mismatch, got %v", err)
	}
}

func TestHandleVerifyJobEndpoint_EnvironmentScopedCallerCannotVerifyOtherEnvironmentEndpoint(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:            id,
				ProjectID:     "proj-1",
				EnvironmentID: "env-staging",
				EndpointURL:   "https://executor.example.com/run",
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleVerifyJobEndpoint(ctx, &VerifyJobEndpointInput{JobID: "job-1"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 for environment mismatch, got %v", err)
	}
}
