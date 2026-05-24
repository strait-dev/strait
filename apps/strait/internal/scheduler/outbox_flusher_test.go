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

func FuzzOutbox(f *testing.F) {
	f.Add("40001")
	f.Add("23503")
	f.Add("08006")
	f.Add("22001")
	f.Fuzz(func(t *testing.T, code string) {
		if len(code) > 8 {
			code = code[:8]
		}
		disp := classifyOutboxEnqueueError(&pgconn.PgError{Code: code})
		switch {
		case code == "40001" || code == "40P01" || code == "55P03" || code == "57014":
			if disp != outboxEnqueueRetryable {
				t.Fatalf("code %q classified as %v, want retryable", code, disp)
			}
		case len(code) >= 2 && code[:2] == "08":
			if disp != outboxEnqueueRetryable {
				t.Fatalf("code %q classified as %v, want retryable", code, disp)
			}
		case len(code) >= 2 && (code[:2] == "22" || code[:2] == "23"):
			if disp != outboxEnqueueTerminal {
				t.Fatalf("code %q classified as %v, want terminal", code, disp)
			}
		}
	})
}
