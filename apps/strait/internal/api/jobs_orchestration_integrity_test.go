package api

import (
	"context"
	"encoding/json"
	"testing"

	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestHandleCreateJob_WorkerModeDefaultsQueueName(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = &cp
			job.ID = "job-worker"
			return nil
		},
	}

	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateJob(ctx, &CreateJobInput{Body: CreateJobRequest{
		ProjectID:     "proj-1",
		Name:          "Worker Job",
		Slug:          "worker-job",
		ExecutionMode: string(domain.ExecutionModeWorker),
	}})
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.Equal(t, defaultJobQueueName,
		captured.
			Queue)

}

func TestHandleCreateJob_PersistsEndpointSigningSecret(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = &cp
			job.ID = "job-http"
			return nil
		},
	}

	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateJob(ctx, &CreateJobInput{Body: CreateJobRequest{
		ProjectID:             "proj-1",
		Name:                  "Signed HTTP Job",
		Slug:                  "signed-http-job",
		EndpointURL:           "https://example.com/hook",
		EndpointSigningSecret: "loadtest-secret-32-bytes-long",
		ExecutionMode:         string(domain.ExecutionModeHTTP),
	}})
	require.NoError(t, err)
	require.NotNil(t, captured)

	requireEncryptedSecretPlaintext(t, enc, captured.EndpointSigningSecret, "loadtest-secret-32-bytes-long")
}

func TestHandleCreateJob_EncryptsEndpointSigningSecretAtRest(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = &cp
			job.ID = "job-http-encrypted"
			return nil
		},
	}

	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateJob(ctx, &CreateJobInput{Body: CreateJobRequest{
		ProjectID:             "proj-1",
		Name:                  "Signed HTTP Job",
		Slug:                  "signed-http-job",
		EndpointURL:           "https://example.com/hook",
		EndpointSigningSecret: "loadtest-secret-32-bytes-long",
		ExecutionMode:         string(domain.ExecutionModeHTTP),
	}})
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.NotEqual(t, "loadtest-secret-32-bytes-long",

		captured.EndpointSigningSecret,
	)
	require.True(
		t, straitcrypto.IsEncryptedField(captured.EndpointSigningSecret))

	plaintext, err := straitcrypto.DecryptField(enc, captured.EndpointSigningSecret)
	require.NoError(t, err)
	require.Equal(t, "loadtest-secret-32-bytes-long",

		plaintext)

}

func TestHandleUpdateJob_HTTPModeRequiresEndpoint(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID:            "job-worker",
				ProjectID:     "proj-1",
				EnvironmentID: "env-1",
				ExecutionMode: domain.ExecutionModeWorker,
				Queue:         "default",
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	mode := "http"

	_, err := srv.handleUpdateJob(ctx, &UpdateJobInput{
		JobID: "job-worker",
		Body:  UpdateJobRequest{ExecutionMode: &mode},
	})
	require.Error(t, err)
	require.True(
		t, isBadRequest(err))

}

func TestHandleCloneJob_EnvironmentMismatchReturns404(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1", EnvironmentID: "env-staging", EndpointURL: "https://example.com"}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleCloneJob(ctx, &CloneJobInput{
		JobID: "job-1",
		Body:  CloneJobRequest{Name: "Clone", Slug: "clone"},
	})
	require.Error(t, err)
	require.True(
		t, isNotFound(err))

}

func TestHandleCloneJob_PreservesOrchestrationFields(t *testing.T) {
	t.Parallel()

	poison := 4
	dlq := 7
	depth := 9
	sourceJob := &domain.Job{
		ID:                        "job-source",
		ProjectID:                 "proj-1",
		EnvironmentID:             "env-1",
		Name:                      "Source",
		Slug:                      "source",
		ExecutionMode:             domain.ExecutionModeWorker,
		Queue:                     "priority",
		EndpointURL:               "https://example.com/hook",
		PoisonPillThreshold:       &poison,
		DLQAlertThreshold:         &dlq,
		QueueDepthAlertThreshold:  &depth,
		OnCompleteTriggerWorkflow: "wf-1",
		OnCompleteTriggerJob:      "job-next",
		OnCompletePayloadMapping:  json.RawMessage(`{"ok":true}`),
		OnFailureTriggerJob:       "job-fail",
		OnFailureTriggerWorkflow:  "wf-fail",
		OnFailurePayloadMapping:   json.RawMessage(`{"error":true}`),
		EndpointSigningSecret:     "signing-secret",
	}

	var captured *domain.Job
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return sourceJob, nil
		},
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = &cp
			job.ID = "job-clone"
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCloneJob(ctx, &CloneJobInput{
		JobID: "job-source",
		Body:  CloneJobRequest{Name: "Clone", Slug: "clone"},
	})
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.Equal(t, sourceJob.Queue,
		captured.
			Queue)
	require.Equal(t, sourceJob.PoisonPillThreshold,

		captured.PoisonPillThreshold)
	require.False(t, captured.DLQAlertThreshold !=
		sourceJob.DLQAlertThreshold || captured.
		QueueDepthAlertThreshold !=
		sourceJob.QueueDepthAlertThreshold)
	require.False(t, string(captured.
		OnCompletePayloadMapping,
	) != string(sourceJob.OnCompletePayloadMapping) ||
		string(captured.OnFailurePayloadMapping) != string(sourceJob.OnFailurePayloadMapping),
	)
	require.False(t, captured.OnCompleteTriggerWorkflow !=
		sourceJob.OnCompleteTriggerWorkflow ||
		captured.OnCompleteTriggerJob !=
			sourceJob.OnCompleteTriggerJob)
	require.False(t, captured.OnFailureTriggerWorkflow !=
		sourceJob.OnFailureTriggerWorkflow ||
		captured.OnFailureTriggerJob !=
			sourceJob.OnFailureTriggerJob)
	require.Equal(t, sourceJob.EndpointSigningSecret,

		captured.EndpointSigningSecret)

}

func isBadRequest(err error) bool {
	if err == nil {
		return false
	}
	type statuser interface{ GetStatus() int }
	if s, ok := err.(statuser); ok {
		return s.GetStatus() == 400
	}
	return false
}
