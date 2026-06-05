package api

import (
	"context"
	"encoding/json"
	"testing"

	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"
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
	if err != nil {
		t.Fatalf("handleCreateJob: %v", err)
	}
	if captured == nil {
		t.Fatal("expected CreateJob to be called")
	}
	if captured.Queue != defaultJobQueueName {
		t.Fatalf("captured.Queue = %q, want %q", captured.Queue, defaultJobQueueName)
	}
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
	if err != nil {
		t.Fatalf("handleCreateJob: %v", err)
	}
	if captured == nil {
		t.Fatal("expected CreateJob to be called")
	}
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
	if err != nil {
		t.Fatalf("handleCreateJob: %v", err)
	}
	if captured == nil {
		t.Fatal("expected CreateJob to be called")
	}
	if captured.EndpointSigningSecret == "loadtest-secret-32-bytes-long" {
		t.Fatal("endpoint signing secret was stored in plaintext")
	}
	if !straitcrypto.IsEncryptedField(captured.EndpointSigningSecret) {
		t.Fatalf("endpoint signing secret = %q, want encrypted field prefix", captured.EndpointSigningSecret)
	}
	plaintext, err := straitcrypto.DecryptField(enc, captured.EndpointSigningSecret)
	if err != nil {
		t.Fatalf("decrypt stored endpoint signing secret: %v", err)
	}
	if plaintext != "loadtest-secret-32-bytes-long" {
		t.Fatalf("decrypted endpoint signing secret = %q", plaintext)
	}
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
	if err == nil {
		t.Fatal("expected missing endpoint to reject http mode update")
	}
	if !isBadRequest(err) {
		t.Fatalf("expected 400 bad request, got %v", err)
	}
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
	if err == nil {
		t.Fatal("expected environment mismatch")
	}
	if !isNotFound(err) {
		t.Fatalf("expected 404 not found, got %v", err)
	}
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
	if err != nil {
		t.Fatalf("handleCloneJob: %v", err)
	}
	if captured == nil {
		t.Fatal("expected CreateJob to be called")
	}

	if captured.Queue != sourceJob.Queue {
		t.Fatalf("captured.Queue = %q, want %q", captured.Queue, sourceJob.Queue)
	}
	if captured.PoisonPillThreshold != sourceJob.PoisonPillThreshold {
		t.Fatalf("captured.PoisonPillThreshold = %v, want %v", captured.PoisonPillThreshold, sourceJob.PoisonPillThreshold)
	}
	if captured.DLQAlertThreshold != sourceJob.DLQAlertThreshold || captured.QueueDepthAlertThreshold != sourceJob.QueueDepthAlertThreshold {
		t.Fatalf("captured alert thresholds = (%v,%v), want (%v,%v)", captured.DLQAlertThreshold, captured.QueueDepthAlertThreshold, sourceJob.DLQAlertThreshold, sourceJob.QueueDepthAlertThreshold)
	}
	if string(captured.OnCompletePayloadMapping) != string(sourceJob.OnCompletePayloadMapping) || string(captured.OnFailurePayloadMapping) != string(sourceJob.OnFailurePayloadMapping) {
		t.Fatalf("captured payload mappings = (%s,%s), want (%s,%s)", captured.OnCompletePayloadMapping, captured.OnFailurePayloadMapping, sourceJob.OnCompletePayloadMapping, sourceJob.OnFailurePayloadMapping)
	}
	if captured.OnCompleteTriggerWorkflow != sourceJob.OnCompleteTriggerWorkflow || captured.OnCompleteTriggerJob != sourceJob.OnCompleteTriggerJob {
		t.Fatalf("captured on_complete triggers = (%q,%q), want (%q,%q)", captured.OnCompleteTriggerWorkflow, captured.OnCompleteTriggerJob, sourceJob.OnCompleteTriggerWorkflow, sourceJob.OnCompleteTriggerJob)
	}
	if captured.OnFailureTriggerWorkflow != sourceJob.OnFailureTriggerWorkflow || captured.OnFailureTriggerJob != sourceJob.OnFailureTriggerJob {
		t.Fatalf("captured on_failure triggers = (%q,%q), want (%q,%q)", captured.OnFailureTriggerWorkflow, captured.OnFailureTriggerJob, sourceJob.OnFailureTriggerWorkflow, sourceJob.OnFailureTriggerJob)
	}
	if captured.EndpointSigningSecret != sourceJob.EndpointSigningSecret {
		t.Fatalf("captured.EndpointSigningSecret = %q, want %q", captured.EndpointSigningSecret, sourceJob.EndpointSigningSecret)
	}
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
