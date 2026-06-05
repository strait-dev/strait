package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestHandleSetJobEndpoint_EnvironmentScopedCallerCannotReplaceOtherEnvironmentEndpoint(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
		ListJobSecretsFunc: func(_ context.Context, _, _, _ string, _ int, _ *time.Time) ([]domain.JobSecret, error) {
			return nil, nil
		},
		UpdateJobEndpointFunc: func(_ context.Context, _, _, _, _ string) error {
			require.Fail(t,

				"UpdateJobEndpoint should not be called for a mismatched environment")
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
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusNotFound,
		))
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
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusNotFound,
		))
}
