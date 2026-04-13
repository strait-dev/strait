package scheduler

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsRetryableOutboxEnqueueError_ContextDeadlineExceeded(t *testing.T) {
	t.Parallel()

	if !isRetryableOutboxEnqueueError(context.DeadlineExceeded) {
		t.Fatal("context deadline exceeded should be retryable")
	}
}

func TestIsRetryableOutboxEnqueueError_IdempotencyConflictIsTerminal(t *testing.T) {
	t.Parallel()

	if isRetryableOutboxEnqueueError(domain.ErrIdempotencyConflict) {
		t.Fatal("idempotency conflict should be terminal")
	}
}

func TestIsRetryableOutboxEnqueueError_ForeignKeyViolationIsTerminal(t *testing.T) {
	t.Parallel()

	err := &pgconn.PgError{Code: "23503"}
	if isRetryableOutboxEnqueueError(err) {
		t.Fatal("foreign key violation should be terminal")
	}
}

func TestIsRetryableOutboxEnqueueError_SerializationFailureIsRetryable(t *testing.T) {
	t.Parallel()

	err := errors.Join(errors.New("wrapped"), &pgconn.PgError{Code: "40001"})
	if !isRetryableOutboxEnqueueError(err) {
		t.Fatal("serialization failure should be retryable")
	}
}
