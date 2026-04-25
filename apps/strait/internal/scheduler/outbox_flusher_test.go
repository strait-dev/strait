package scheduler

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestClassifyOutboxEnqueueError_ContextDeadlineExceeded(t *testing.T) {
	t.Parallel()

	if classifyOutboxEnqueueError(context.DeadlineExceeded) != outboxEnqueueRetryable {
		t.Fatal("context deadline exceeded should be retryable")
	}
}

func TestClassifyOutboxEnqueueError_IdempotencyConflictIsTerminal(t *testing.T) {
	t.Parallel()

	if classifyOutboxEnqueueError(domain.ErrIdempotencyConflict) != outboxEnqueueTerminal {
		t.Fatal("idempotency conflict should be terminal")
	}
}

func TestClassifyOutboxEnqueueError_ForeignKeyViolationIsTerminal(t *testing.T) {
	t.Parallel()

	err := &pgconn.PgError{Code: "23503"}
	if classifyOutboxEnqueueError(err) != outboxEnqueueTerminal {
		t.Fatal("foreign key violation should be terminal")
	}
}

func TestClassifyOutboxEnqueueError_SerializationFailureIsRetryable(t *testing.T) {
	t.Parallel()

	err := errors.Join(errors.New("wrapped"), &pgconn.PgError{Code: "40001"})
	if classifyOutboxEnqueueError(err) != outboxEnqueueRetryable {
		t.Fatal("serialization failure should be retryable")
	}
}
