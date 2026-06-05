package api

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"
)

func newJobEndpointTestServer(t *testing.T, job *domain.Job, capture *atomic.Pointer[capturedEndpointUpdate]) *Server {
	t.Helper()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			snapshot := *job
			return &snapshot, nil
		},
		ListJobSecretsFunc: func(_ context.Context, _, _, _ string, _ int, _ *time.Time) ([]domain.JobSecret, error) {
			return nil, nil
		},
		UpdateJobEndpointFunc: func(_ context.Context, jobID, endpointURL, fallbackURL, signingSecret string) error {
			capture.Store(&capturedEndpointUpdate{
				JobID:         jobID,
				EndpointURL:   endpointURL,
				FallbackURL:   fallbackURL,
				SigningSecret: signingSecret,
			})
			job.EndpointURL = endpointURL
			job.FallbackEndpointURL = fallbackURL
			job.EndpointSigningSecret = signingSecret
			return nil
		},
	}
	return newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})
}

type capturedEndpointUpdate struct {
	JobID         string
	EndpointURL   string
	FallbackURL   string
	SigningSecret string
}

func TestSetJobEndpoint_SSRFErrorIsSanitized(t *testing.T) {
	t.Parallel()

	job := &domain.Job{ID: "job-1", ProjectID: "proj-1", EndpointSigningSecret: "esec_preexisting"}
	var capture atomic.Pointer[capturedEndpointUpdate]
	srv := newJobEndpointTestServer(t, job, &capture)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleSetJobEndpoint(ctx, &SetJobEndpointInput{
		JobID: "job-1",
		Body:  SetJobEndpointRequest{EndpointURL: "http://127.0.0.1:8080/run"},
	})
	if err == nil {
		t.Fatal("expected SSRF rejection, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "endpoint_url failed validation") {
		t.Fatalf("error should use sanitized message, got %q", msg)
	}
	if strings.Contains(msg, "127.0.0.1") || strings.Contains(msg, "private") || strings.Contains(msg, "loopback") {
		t.Fatalf("sanitized error must not leak IP / classification, got %q", msg)
	}
	if capture.Load() != nil {
		t.Fatal("UpdateJobEndpoint must not be called when SSRF validation fails")
	}
}

func TestSetJobEndpoint_URLOnlyUpdatePreservesSecret(t *testing.T) {
	t.Parallel()

	job := &domain.Job{
		ID:                    "job-1",
		ProjectID:             "proj-1",
		EndpointURL:           "https://old.example.com/run",
		EndpointSigningSecret: "esec_preexisting_secret",
	}
	var capture atomic.Pointer[capturedEndpointUpdate]
	srv := newJobEndpointTestServer(t, job, &capture)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	out, err := srv.handleSetJobEndpoint(ctx, &SetJobEndpointInput{
		JobID: "job-1",
		Body:  SetJobEndpointRequest{EndpointURL: "https://new.example.com/run"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cap := capture.Load()
	if cap == nil {
		t.Fatal("expected UpdateJobEndpoint to be invoked")
		return
	}
	if cap.SigningSecret == "esec_preexisting_secret" {
		t.Fatalf("signing secret persisted in plaintext")
	}
	if !straitcrypto.IsEncryptedField(cap.SigningSecret) {
		t.Fatalf("signing secret = %q, want encrypted field", cap.SigningSecret)
	}
	if out.Body.SigningSecret != "" {
		t.Fatalf("response signing_secret should be empty when not rotating, got %q", out.Body.SigningSecret)
	}
}

func TestSetJobEndpoint_RotateOptInReturnsNewSecret(t *testing.T) {
	t.Parallel()

	job := &domain.Job{
		ID:                    "job-1",
		ProjectID:             "proj-1",
		EndpointURL:           "https://old.example.com/run",
		EndpointSigningSecret: "esec_preexisting_secret",
	}
	var capture atomic.Pointer[capturedEndpointUpdate]
	srv := newJobEndpointTestServer(t, job, &capture)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	out, err := srv.handleSetJobEndpoint(ctx, &SetJobEndpointInput{
		JobID: "job-1",
		Body: SetJobEndpointRequest{
			EndpointURL:         "https://new.example.com/run",
			RotateSigningSecret: true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cap := capture.Load()
	if cap == nil {
		t.Fatal("expected UpdateJobEndpoint to be invoked")
		return
	}
	if cap.SigningSecret == "esec_preexisting_secret" {
		t.Fatal("rotation requested but signing secret was not rotated")
	}
	if !straitcrypto.IsEncryptedField(cap.SigningSecret) {
		t.Fatalf("stored signing secret = %q, want encrypted field", cap.SigningSecret)
	}
	if out.Body.SigningSecret == "" {
		t.Fatal("response signing_secret must be populated when rotating")
	}
	if !strings.HasPrefix(out.Body.SigningSecret, "esec_") {
		t.Fatalf("response signing secret missing esec_ prefix: %q", out.Body.SigningSecret)
	}
	if out.Body.SigningSecret == cap.SigningSecret {
		t.Fatalf("response signing_secret must be plaintext once, not stored ciphertext")
	}
}

func TestSetJobEndpoint_RotateFalseDoesNotReturnSecret(t *testing.T) {
	t.Parallel()

	job := &domain.Job{
		ID:                    "job-1",
		ProjectID:             "proj-1",
		EndpointSigningSecret: "esec_existing",
	}
	var capture atomic.Pointer[capturedEndpointUpdate]
	srv := newJobEndpointTestServer(t, job, &capture)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	out, err := srv.handleSetJobEndpoint(ctx, &SetJobEndpointInput{
		JobID: "job-1",
		Body: SetJobEndpointRequest{
			EndpointURL:         "https://new.example.com/run",
			RotateSigningSecret: false,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.SigningSecret != "" {
		t.Fatalf("response signing_secret must be empty when rotate_signing_secret=false, got %q", out.Body.SigningSecret)
	}
}
