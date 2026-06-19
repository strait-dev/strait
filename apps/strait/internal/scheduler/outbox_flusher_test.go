package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestNewOutboxFlusher_DefaultsAndAccessors(t *testing.T) {
	t.Parallel()

	flusher := NewOutboxFlusher(nil, nil, OutboxFlusherConfig{})
	require.Equal(t, time.Second, flusher.interval)
	require.Equal(t, 500, flusher.batchSize)
	require.Equal(t, 30*time.Second, flusher.leaseDuration)
	require.Equal(t, 30*time.Second, flusher.reclaimInterval)
	require.NotNil(t, flusher.logger)
	require.Zero(t, flusher.Iterations())
	require.Zero(t, flusher.Flushed())
	require.Zero(t, flusher.Errors())
	require.Zero(t, flusher.ReclaimedExpiredLeases())

	flusher.iterations.Add(1)
	flusher.flushed.Add(2)
	flusher.errors.Add(3)
	flusher.reclaimedExpiredLease.Add(4)
	require.Equal(t, int64(1), flusher.Iterations())
	require.Equal(t, int64(2), flusher.Flushed())
	require.Equal(t, int64(3), flusher.Errors())
	require.Equal(t, int64(4), flusher.ReclaimedExpiredLeases())
}

func TestNewOutboxFlusher_PreservesExplicitConfig(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	flusher := NewOutboxFlusher(nil, nil, OutboxFlusherConfig{
		Interval:        2 * time.Second,
		BatchSize:       17,
		LeaseDuration:   time.Minute,
		ReclaimInterval: 3 * time.Minute,
		Logger:          logger,
	})

	require.Equal(t, 2*time.Second, flusher.interval)
	require.Equal(t, 17, flusher.batchSize)
	require.Equal(t, time.Minute, flusher.leaseDuration)
	require.Equal(t, 3*time.Minute, flusher.reclaimInterval)
	require.Same(t, logger, flusher.logger)
}

func TestOutboxFlusher_FlushOnceForTestRecoversPanic(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	flusher := NewOutboxFlusher(nil, nil, OutboxFlusherConfig{Logger: logger})

	err := flusher.FlushOnceForTest(context.Background())

	require.ErrorContains(t, err, "outbox flusher panic")
	require.Equal(t, int64(1), flusher.Iterations())
	require.Equal(t, int64(1), flusher.Errors())
	require.Zero(t, flusher.Flushed())
}

func TestOutboxFlusherToJobRunMapsOutboxFields(t *testing.T) {
	t.Parallel()

	idempotencyKey := "run-once"
	scheduledAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	flusher := NewOutboxFlusher(nil, nil, OutboxFlusherConfig{})

	run, err := flusher.toJobRun(store.OutboxRow{
		ID:             "outbox-1",
		ProjectID:      "project-1",
		JobID:          "job-1",
		Payload:        json.RawMessage(`{"ok":true}`),
		Metadata:       json.RawMessage(`{"source":"outbox","attempt":"1"}`),
		IdempotencyKey: &idempotencyKey,
		ScheduledAt:    &scheduledAt,
		Priority:       42,
		ExecutionMode:  string(domain.ExecutionModeWorker),
		QueueName:      "critical",
	})

	require.NoError(t, err)
	require.NotEmpty(t, run.ID)
	require.Equal(t, "project-1", run.ProjectID)
	require.Equal(t, "job-1", run.JobID)
	require.JSONEq(t, `{"ok":true}`, string(run.Payload))
	require.Equal(t, map[string]string{"source": "outbox", "attempt": "1"}, run.Metadata)
	require.Equal(t, "run-once", run.IdempotencyKey)
	require.Same(t, &scheduledAt, run.ScheduledAt)
	require.Equal(t, 42, run.Priority)
	require.Equal(t, domain.TriggerManual, run.TriggeredBy)
	require.Equal(t, domain.ExecutionModeWorker, run.ExecutionMode)
	require.Equal(t, "critical", run.QueueName)
}

func TestOutboxFlusherToJobRunAllowsMissingOptionalFields(t *testing.T) {
	t.Parallel()

	flusher := NewOutboxFlusher(nil, nil, OutboxFlusherConfig{})

	run, err := flusher.toJobRun(store.OutboxRow{
		ID:            "outbox-2",
		ProjectID:     "project-2",
		JobID:         "job-2",
		Payload:       json.RawMessage(`{}`),
		ExecutionMode: string(domain.ExecutionModeHTTP),
	})

	require.NoError(t, err)
	require.Empty(t, run.IdempotencyKey)
	require.Nil(t, run.ScheduledAt)
	require.Nil(t, run.Metadata)
	require.Equal(t, domain.ExecutionModeHTTP, run.ExecutionMode)
}

func TestOutboxFlusherToJobRunRejectsInvalidMetadata(t *testing.T) {
	t.Parallel()

	flusher := NewOutboxFlusher(nil, nil, OutboxFlusherConfig{})

	run, err := flusher.toJobRun(store.OutboxRow{
		ID:       "outbox-bad-metadata",
		Metadata: json.RawMessage(`{"broken"`),
	})

	require.Nil(t, run)
	require.ErrorContains(t, err, "decode outbox metadata for row outbox-bad-metadata")
}

func TestClassifyOutboxEnqueueError_NilIsRetryable(t *testing.T) {
	t.Parallel()
	require.Equal(t, outboxEnqueueRetryable, classifyOutboxEnqueueError(nil))
}

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
