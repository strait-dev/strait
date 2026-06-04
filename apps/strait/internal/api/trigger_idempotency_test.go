package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestTriggerIdempotencyKeyPrefersPrimaryHeader(t *testing.T) {
	t.Parallel()

	key, err := triggerIdempotencyKey(&TriggerJobInput{
		XIdempotencyKey:   "primary-key",
		IdempotencyKeyAlt: "standard-key",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if key != "primary-key" {
		t.Fatalf("expected primary header key, got %q", key)
	}
}

func TestTriggerIdempotencyKeyFallsBackToStandardHeader(t *testing.T) {
	t.Parallel()

	key, err := triggerIdempotencyKey(&TriggerJobInput{IdempotencyKeyAlt: "standard-key"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if key != "standard-key" {
		t.Fatalf("expected standard header key, got %q", key)
	}
}

func TestTriggerIdempotencyKeyRejectsTooLong(t *testing.T) {
	t.Parallel()

	_, err := triggerIdempotencyKey(&TriggerJobInput{
		XIdempotencyKey: strings.Repeat("x", maxIdempotencyKeyLength+1),
	})
	if err == nil {
		t.Fatal("expected error for too-long idempotency key")
	}
	if !strings.Contains(err.Error(), "idempotency key must be 256 characters or fewer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTriggerIdempotencyHitReturnsExistingRun(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			if jobID != "job-1" {
				t.Fatalf("expected job ID job-1, got %q", jobID)
			}
			if key != "idem-key" {
				t.Fatalf("expected idempotency key idem-key, got %q", key)
			}
			return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
		},
	}}

	hit, err := srv.triggerIdempotencyHit(context.Background(), &domain.Job{ID: "job-1"}, "idem-key")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	assertIdempotencyResponse(t, hit, "run-existing", domain.StatusQueued)
}

func TestResolveTriggerIdempotencyConflictReturnsWinningRun(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			if jobID != "job-1" {
				t.Fatalf("expected job ID job-1, got %q", jobID)
			}
			if key != "idem-key" {
				t.Fatalf("expected idempotency key idem-key, got %q", key)
			}
			return &domain.JobRun{ID: "run-winner", Status: domain.StatusExecuting}, nil
		},
	}}

	err := srv.resolveTriggerIdempotencyConflict(
		context.Background(),
		&domain.Job{ID: "job-1"},
		"idem-key",
		domain.ErrIdempotencyConflict,
	)

	var statusErr *rawStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected raw status error, got %T: %v", err, err)
	}
	assertIdempotencyResponse(t, statusErr, "run-winner", domain.StatusExecuting)
}

func assertIdempotencyResponse(t *testing.T, err *rawStatusError, runID string, status domain.RunStatus) {
	t.Helper()

	if err == nil {
		t.Fatal("expected idempotency response, got nil")
		return
	}
	if err.status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", err.status)
	}
	body, ok := err.body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response body, got %T", err.body)
	}
	if body["id"] != runID {
		t.Fatalf("expected run ID %q, got %v", runID, body["id"])
	}
	if body["status"] != status {
		t.Fatalf("expected run status %q, got %v", status, body["status"])
	}
	if body["idempotency_hit"] != true {
		t.Fatalf("expected idempotency hit response, got %v", body["idempotency_hit"])
	}
}
