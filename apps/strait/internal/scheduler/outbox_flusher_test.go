package scheduler

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/queue"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestClassifyOutboxEnqueueError_ContextDeadlineExceeded(t *testing.T) {
	t.Parallel()
	require.Equal(t, outboxEnqueueRetryable,

		classifyOutboxEnqueueError(context.DeadlineExceeded),
	)
}

func TestClassifyOutboxEnqueueError_IdempotencyConflictIsTerminal(t *testing.T) {
	t.Parallel()
	require.Equal(t, outboxEnqueueTerminal,

		classifyOutboxEnqueueError(
			domain.ErrIdempotencyConflict,
		))
}

func TestClassifyOutboxEnqueueError_BackpressureThrottleIsRetryable(t *testing.T) {
	t.Parallel()

	err := errors.Join(errors.New("wrapped"), queue.ErrEnqueueThrottled)
	require.Equal(t, outboxEnqueueRetryable,

		classifyOutboxEnqueueError(err))
}

func TestClassifyOutboxEnqueueError_ForeignKeyViolationIsTerminal(t *testing.T) {
	t.Parallel()

	err := &pgconn.PgError{Code: "23503"}
	require.Equal(t, outboxEnqueueTerminal,

		classifyOutboxEnqueueError(
			err))
}

func TestClassifyOutboxEnqueueError_SerializationFailureIsRetryable(t *testing.T) {
	t.Parallel()

	err := errors.Join(errors.New("wrapped"), &pgconn.PgError{Code: "40001"})
	require.Equal(t, outboxEnqueueRetryable,

		classifyOutboxEnqueueError(err))
}

func TestClassifyOutboxEnqueueError_UnknownErrorIsRetryable(t *testing.T) {
	t.Parallel()
	require.Equal(t, outboxEnqueueRetryable,

		classifyOutboxEnqueueError(errors.New("temporary unknown enqueue failure")))
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
				require.Failf(t, "test failure",

					"code %q classified as %v, want retryable", code, disp)
			}
		case len(code) >= 2 && code[:2] == "08":
			if disp != outboxEnqueueRetryable {
				require.Failf(t, "test failure",

					"code %q classified as %v, want retryable", code, disp)
			}
		case len(code) >= 2 && (code[:2] == "22" || code[:2] == "23"):
			if disp != outboxEnqueueTerminal {
				require.Failf(t, "test failure",

					"code %q classified as %v, want terminal", code, disp)
			}
		}
	})
}
