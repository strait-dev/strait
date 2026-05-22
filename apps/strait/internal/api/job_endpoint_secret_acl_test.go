package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestSetJobEndpoint_SecretBearingJobRequiresSecretsWrite(t *testing.T) {
	t.Parallel()

	updateCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:            id,
				ProjectID:     "proj-1",
				EnvironmentID: "env-1",
				EndpointURL:   "https://old.example.com/run",
			}, nil
		},
		ListJobSecretsFunc: func(_ context.Context, projectID, jobID, environment string, limit int, _ *time.Time) ([]domain.JobSecret, error) {
			if projectID != "proj-1" || jobID != "job-1" || environment != "env-1" || limit != 1 {
				t.Fatalf("ListJobSecrets args = (%q, %q, %q, %d)", projectID, jobID, environment, limit)
			}
			return []domain.JobSecret{{ID: "sec-1", ProjectID: projectID, JobID: jobID, Environment: environment}}, nil
		},
		UpdateJobEndpointFunc: func(context.Context, string, string, string, string) error {
			updateCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsWrite})

	_, err := srv.handleSetJobEndpoint(ctx, &SetJobEndpointInput{
		JobID: "job-1",
		Body:  SetJobEndpointRequest{EndpointURL: "https://attacker.example.com/run"},
	})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403 for secret-bearing endpoint change without secrets:write, got %v", err)
	}
	if updateCalled {
		t.Fatal("UpdateJobEndpoint must not run without secrets:write")
	}
}

func TestSetJobEndpoint_SecretBearingJobAllowsSecretsWrite(t *testing.T) {
	t.Parallel()

	updateCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:            id,
				ProjectID:     "proj-1",
				EnvironmentID: "env-1",
				EndpointURL:   "https://old.example.com/run",
			}, nil
		},
		ListJobSecretsFunc: func(context.Context, string, string, string, int, *time.Time) ([]domain.JobSecret, error) {
			t.Fatal("ListJobSecrets should not be called when caller has secrets:write")
			return nil, nil
		},
		UpdateJobEndpointFunc: func(context.Context, string, string, string, string) error {
			updateCalled = true
			return nil
		},
	}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsWrite, domain.ScopeSecretsWrite})

	_, err := srv.handleSetJobEndpoint(ctx, &SetJobEndpointInput{
		JobID: "job-1",
		Body:  SetJobEndpointRequest{EndpointURL: "https://new.example.com/run"},
	})
	if err != nil {
		t.Fatalf("handleSetJobEndpoint: %v", err)
	}
	if !updateCalled {
		t.Fatal("UpdateJobEndpoint was not called")
	}
}

func TestUpdateJob_SecretBearingEndpointChangeRequiresSecretsWrite(t *testing.T) {
	t.Parallel()

	updateCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:            id,
				ProjectID:     "proj-1",
				EnvironmentID: "env-1",
				EndpointURL:   "https://old.example.com/run",
				Queue:         "default",
			}, nil
		},
		ListJobSecretsFunc: func(_ context.Context, projectID, jobID, environment string, limit int, _ *time.Time) ([]domain.JobSecret, error) {
			if projectID != "proj-1" || jobID != "job-1" || environment != "env-1" || limit != 1 {
				t.Fatalf("ListJobSecrets args = (%q, %q, %q, %d)", projectID, jobID, environment, limit)
			}
			return []domain.JobSecret{{ID: "sec-1", ProjectID: projectID, JobID: jobID, Environment: environment}}, nil
		},
		UpdateJobFunc: func(context.Context, *domain.Job) error {
			updateCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsWrite})

	nextEndpoint := "https://attacker.example.com/run"
	_, err := srv.handleUpdateJob(ctx, &UpdateJobInput{
		JobID: "job-1",
		Body:  UpdateJobRequest{EndpointURL: &nextEndpoint},
	})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403 for endpoint update without secrets:write, got %v", err)
	}
	if updateCalled {
		t.Fatal("UpdateJob must not run without secrets:write")
	}
}
