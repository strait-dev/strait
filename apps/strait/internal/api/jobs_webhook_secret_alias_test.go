package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"
)

func requireEncryptedSecretPlaintext(t *testing.T, enc Encryptor, encrypted, want string) {
	t.Helper()
	if !straitcrypto.IsEncryptedField(encrypted) {
		t.Fatalf("secret = %q, want encrypted field", encrypted)
	}
	got, err := straitcrypto.DecryptField(enc, encrypted)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if got != want {
		t.Fatalf("decrypted secret = %q, want %q", got, want)
	}
}

func TestHandleCreateJob_WebhookSecretAliasPersisted(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = &cp
			job.ID = "job-alias"
			return nil
		},
	}

	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateJob(ctx, &CreateJobInput{Body: CreateJobRequest{
		ProjectID:     "proj-1",
		Name:          "Aliased Job",
		Slug:          "aliased-job",
		EndpointURL:   "https://example.com/hook",
		WebhookSecret: "sdk-supplied-secret-32b-long",
		ExecutionMode: string(domain.ExecutionModeHTTP),
	}})
	if err != nil {
		t.Fatalf("handleCreateJob: %v", err)
	}
	if captured == nil {
		t.Fatal("expected CreateJob to be called")
	}
	requireEncryptedSecretPlaintext(t, enc, captured.EndpointSigningSecret, "sdk-supplied-secret-32b-long")
}

func TestHandleCreateJob_WebhookSecretWinsOverEndpointSigningSecret(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = &cp
			job.ID = "job-conflict"
			return nil
		},
	}

	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateJob(ctx, &CreateJobInput{Body: CreateJobRequest{
		ProjectID:             "proj-1",
		Name:                  "Conflict Job",
		Slug:                  "conflict-job",
		EndpointURL:           "https://example.com/hook",
		EndpointSigningSecret: "legacy-platform-secret-32b-long",
		WebhookSecret:         "sdk-supplied-secret-32b-long",
		ExecutionMode:         string(domain.ExecutionModeHTTP),
	}})
	if err != nil {
		t.Fatalf("handleCreateJob: %v", err)
	}
	requireEncryptedSecretPlaintext(t, enc, captured.EndpointSigningSecret, "sdk-supplied-secret-32b-long")
}

func TestHandleCreateJob_EndpointSigningSecretAloneStillPersisted(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = &cp
			job.ID = "job-legacy"
			return nil
		},
	}

	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateJob(ctx, &CreateJobInput{Body: CreateJobRequest{
		ProjectID:             "proj-1",
		Name:                  "Legacy Job",
		Slug:                  "legacy-job",
		EndpointURL:           "https://example.com/hook",
		EndpointSigningSecret: "legacy-platform-secret-32b-long",
		ExecutionMode:         string(domain.ExecutionModeHTTP),
	}})
	if err != nil {
		t.Fatalf("handleCreateJob: %v", err)
	}
	requireEncryptedSecretPlaintext(t, enc, captured.EndpointSigningSecret, "legacy-platform-secret-32b-long")
}

func TestHandleUpdateJob_WebhookSecretAliasApplied(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID:                    "job-1",
				ProjectID:             "proj-1",
				EnvironmentID:         "env-1",
				ExecutionMode:         domain.ExecutionModeHTTP,
				EndpointURL:           "https://example.com/hook",
				EndpointSigningSecret: "old-secret-32-bytes-long-abcdef",
				Queue:                 "default",
			}, nil
		},
		UpdateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = &cp
			return nil
		},
	}

	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-1")

	newSecret := "rotated-via-webhook-secret-32b"
	_, err := srv.handleUpdateJob(ctx, &UpdateJobInput{
		JobID: "job-1",
		Body:  UpdateJobRequest{WebhookSecret: &newSecret},
	})
	if err != nil {
		t.Fatalf("handleUpdateJob: %v", err)
	}
	if captured == nil {
		t.Fatal("expected UpdateJob to be called")
	}
	requireEncryptedSecretPlaintext(t, enc, captured.EndpointSigningSecret, newSecret)
}

func TestHandleUpdateJob_EndpointSigningSecretFieldApplied(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID:                    "job-1",
				ProjectID:             "proj-1",
				EnvironmentID:         "env-1",
				ExecutionMode:         domain.ExecutionModeHTTP,
				EndpointURL:           "https://example.com/hook",
				EndpointSigningSecret: "old-secret-32-bytes-long-abcdef",
				Queue:                 "default",
			}, nil
		},
		UpdateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = &cp
			return nil
		},
	}

	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-1")

	newSecret := "rotated-via-endpoint-field-32b"
	_, err := srv.handleUpdateJob(ctx, &UpdateJobInput{
		JobID: "job-1",
		Body:  UpdateJobRequest{EndpointSigningSecret: &newSecret},
	})
	if err != nil {
		t.Fatalf("handleUpdateJob: %v", err)
	}
	requireEncryptedSecretPlaintext(t, enc, captured.EndpointSigningSecret, newSecret)
}

func TestHandleUpdateJob_WebhookSecretWinsOverEndpointSigningSecret(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID:            "job-1",
				ProjectID:     "proj-1",
				EnvironmentID: "env-1",
				ExecutionMode: domain.ExecutionModeHTTP,
				EndpointURL:   "https://example.com/hook",
				Queue:         "default",
			}, nil
		},
		UpdateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = &cp
			return nil
		},
	}

	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-1")

	webhookSecret := "sdk-supplied-secret-32-bytes-ok"
	endpointSecret := "platform-supplied-32-bytes-okay"
	_, err := srv.handleUpdateJob(ctx, &UpdateJobInput{
		JobID: "job-1",
		Body: UpdateJobRequest{
			WebhookSecret:         &webhookSecret,
			EndpointSigningSecret: &endpointSecret,
		},
	})
	if err != nil {
		t.Fatalf("handleUpdateJob: %v", err)
	}
	requireEncryptedSecretPlaintext(t, enc, captured.EndpointSigningSecret, webhookSecret)
}

func TestHandleUpdateJob_AuditDetailsDoNotLeakSigningSecrets(t *testing.T) {
	t.Parallel()

	var capturedAudit *domain.AuditEvent
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID:            "job-1",
				ProjectID:     "proj-1",
				EnvironmentID: "env-1",
				ExecutionMode: domain.ExecutionModeHTTP,
				EndpointURL:   "https://example.com/hook",
				Queue:         "default",
			}, nil
		},
		UpdateJobFunc: func(_ context.Context, _ *domain.Job) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			clone := *ev
			capturedAudit = &clone
			return nil
		},
	}

	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	webhookSecret := "sdk-supplied-audit-secret-32-bytes"
	endpointSecret := "platform-audit-secret-32-bytes-ok"
	name := "renamed"
	_, err := srv.handleUpdateJob(ctx, &UpdateJobInput{
		JobID: "job-1",
		Body: UpdateJobRequest{
			Name:                  &name,
			WebhookSecret:         &webhookSecret,
			EndpointSigningSecret: &endpointSecret,
		},
	})
	if err != nil {
		t.Fatalf("handleUpdateJob: %v", err)
	}
	if capturedAudit == nil {
		t.Fatal("expected audit event")
	}
	rawDetails := string(capturedAudit.Details)
	for _, forbidden := range []string{webhookSecret, endpointSecret, "webhook_secret", "endpoint_signing_secret"} {
		if strings.Contains(rawDetails, forbidden) {
			t.Fatalf("audit details leaked %q: %s", forbidden, rawDetails)
		}
	}

	var details map[string]any
	if err := json.Unmarshal(capturedAudit.Details, &details); err != nil {
		t.Fatalf("unmarshal audit details: %v", err)
	}
	changes, ok := details["changes"].(map[string]any)
	if !ok {
		t.Fatalf("changes = %#v, want object", details["changes"])
	}
	if changes["name"] != name {
		t.Fatalf("changes.name = %v, want %q", changes["name"], name)
	}
	if changes["signing_credential_changed"] != true {
		t.Fatalf("signing_credential_changed = %v, want true", changes["signing_credential_changed"])
	}
}

func TestHandleBatchCreateJobs_EncryptsEndpointSigningSecrets(t *testing.T) {
	t.Parallel()

	var captured []domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = append(captured, cp)
			job.ID = "job-" + job.Slug
			return nil
		},
	}

	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleBatchCreateJobs(ctx, &BatchCreateJobsInput{Body: BatchCreateJobsRequest{
		Jobs: []CreateJobRequest{
			{
				ProjectID:             "proj-1",
				Name:                  "Endpoint Secret Job",
				Slug:                  "endpoint-secret",
				EndpointURL:           "https://example.com/endpoint",
				EndpointSigningSecret: "batch-endpoint-secret-32-bytes",
				ExecutionMode:         string(domain.ExecutionModeHTTP),
			},
			{
				ProjectID:             "proj-1",
				Name:                  "Webhook Secret Job",
				Slug:                  "webhook-secret",
				EndpointURL:           "https://example.com/webhook",
				EndpointSigningSecret: "ignored-endpoint-secret-32-bytes",
				WebhookSecret:         "batch-webhook-secret-32-bytes-ok",
				ExecutionMode:         string(domain.ExecutionModeHTTP),
			},
		},
	}})
	if err != nil {
		t.Fatalf("handleBatchCreateJobs: %v", err)
	}
	if len(captured) != 2 {
		t.Fatalf("captured jobs = %d, want 2", len(captured))
	}
	requireEncryptedSecretPlaintext(t, enc, captured[0].EndpointSigningSecret, "batch-endpoint-secret-32-bytes")
	requireEncryptedSecretPlaintext(t, enc, captured[1].EndpointSigningSecret, "batch-webhook-secret-32-bytes-ok")
}
