//go:build integration

package scheduler_test

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/scheduler"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

type outboxTestQueue struct {
	enqueueInTxFn func(ctx context.Context, tx store.DBTX, run *domain.JobRun) error
}

func (q *outboxTestQueue) Enqueue(_ context.Context, _ *domain.JobRun) error {
	return nil
}

func (q *outboxTestQueue) EnqueueInTx(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
	if q.enqueueInTxFn != nil {
		return q.enqueueInTxFn(ctx, tx, run)
	}
	return nil
}

func (q *outboxTestQueue) EnqueueBatch(_ context.Context, runs []*domain.JobRun) (int64, error) {
	return int64(len(runs)), nil
}

func TestOutboxFlusher_ConcurrentFlushersSameIdempotencyKeyNoDuplicateRuns(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-concurrent-idempotency")

	entries := []queue.OutboxEntry{
		{
			ID:             intNewID(),
			ProjectID:      job.ProjectID,
			JobID:          job.ID,
			IdempotencyKey: "shared-key",
			Payload:        json.RawMessage(`{"n":1}`),
		},
		{
			ID:             intNewID(),
			ProjectID:      job.ProjectID,
			JobID:          job.ID,
			IdempotencyKey: "shared-key",
			Payload:        json.RawMessage(`{"n":2}`),
		},
	}
	intWriteOutboxEntries(t, ctx, entries)

	q := intTestQueue(t)
	flusherA := scheduler.NewOutboxFlusher(getTestDB(t).Pool, q, scheduler.OutboxFlusherConfig{BatchSize: 1})
	flusherB := scheduler.NewOutboxFlusher(getTestDB(t).Pool, q, scheduler.OutboxFlusherConfig{BatchSize: 1})

	start := make(chan struct{})
	errCh := make(chan error, 2)
	concWG.Go(func() {
		<-start
		errCh <- flusherA.FlushOnceForTest(ctx)
	})
	concWG.Go(func() {
		<-start
		errCh <- flusherB.FlushOnceForTest(ctx)
	})
	close(start)

	for range 2 {
		require.NoError(t, <-errCh)

	}

	assertRunCount(t, ctx, job.ID, "shared-key", 1)
	assertOutboxState(t, ctx, entries[0].ID, false, true)
	assertOutboxState(t, ctx, entries[1].ID, true, true)
}

func TestOutboxFlusher_TerminalFailureMarksErrorAndConsumes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)

	poisonJob := intCreateJob(t, ctx, st, "proj-outbox-terminal")
	goodJob := intCreateJob(t, ctx, st, "proj-outbox-terminal")
	entries := []queue.OutboxEntry{
		{
			ID:        intNewID(),
			ProjectID: poisonJob.ProjectID,
			JobID:     poisonJob.ID,
			Payload:   json.RawMessage(`{"poison":true}`),
		},
		{
			ID:        intNewID(),
			ProjectID: goodJob.ProjectID,
			JobID:     goodJob.ID,
			Payload:   json.RawMessage(`{"good":true}`),
		},
	}
	intWriteOutboxEntries(t, ctx, entries)

	if _, err := getTestDB(t).Pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, poisonJob.ID); err != nil {
		require.Failf(t, "test failure",

			"delete poison job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 10,
	})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)

	assertOutboxState(t, ctx, entries[0].ID, true, true)
	assertOutboxState(t, ctx, entries[1].ID, false, true)
	assertRunsForJob(t, ctx, goodJob.ID, 1)

	count, err := st.CountUnconsumedOutbox(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func TestOutboxFlusher_RetryableFailureLeavesRowUnconsumed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-retryable")
	entries := []queue.OutboxEntry{{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"retry":true}`),
	}}
	intWriteOutboxEntries(t, ctx, entries)

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, &outboxTestQueue{
		enqueueInTxFn: func(context.Context, store.DBTX, *domain.JobRun) error {
			return context.DeadlineExceeded
		},
	}, scheduler.OutboxFlusherConfig{BatchSize: 1})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)

	assertOutboxState(t, ctx, entries[0].ID, false, false)
	count, err := st.CountUnconsumedOutbox(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	successFlusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 1,
	})
	require.NoError(t, successFlusher.
		FlushOnceForTest(ctx))

	assertOutboxState(t, ctx, entries[0].ID, false, true)
	assertRunsForJob(t, ctx, job.ID, 1)
}

func TestOutboxFlusher_UnknownEnqueueFailureLeavesRowUnconsumed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-unknown-retry")
	entries := []queue.OutboxEntry{{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"unknown":true}`),
	}}
	intWriteOutboxEntries(t, ctx, entries)

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, &outboxTestQueue{
		enqueueInTxFn: func(context.Context, store.DBTX, *domain.JobRun) error {
			return errors.New("network path disappeared while promoting outbox row")
		},
	}, scheduler.OutboxFlusherConfig{BatchSize: 1})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)

	assertOutboxState(t, ctx, entries[0].ID, false, false)
	count, err := st.CountUnconsumedOutbox(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	successFlusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 1,
	})
	require.NoError(t, successFlusher.
		FlushOnceForTest(ctx))

	assertOutboxState(t, ctx, entries[0].ID, false, true)
	assertRunsForJob(t, ctx, job.ID, 1)
}

func TestOutboxFlusher_BackpressureThrottleLeavesRowUnconsumed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-backpressure")
	entries := []queue.OutboxEntry{{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"throttled":true}`),
	}}
	intWriteOutboxEntries(t, ctx, entries)

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, &outboxTestQueue{
		enqueueInTxFn: func(context.Context, store.DBTX, *domain.JobRun) error {
			return queue.ErrEnqueueThrottled
		},
	}, scheduler.OutboxFlusherConfig{BatchSize: 1})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)

	assertOutboxState(t, ctx, entries[0].ID, false, false)
	count, err := st.CountUnconsumedOutbox(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	successFlusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{BatchSize: 1})
	require.NoError(t, successFlusher.
		FlushOnceForTest(ctx))

	assertOutboxState(t, ctx, entries[0].ID, false, true)
	assertRunsForJob(t, ctx, job.ID, 1)
}

func TestOutboxFlusher_PanicReturnsErrorAndLeavesRowUnconsumed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)

	job := intCreateJob(t, ctx, st, "proj-outbox-panic")
	entry := queue.OutboxEntry{
		ID:             intNewID(),
		ProjectID:      job.ProjectID,
		JobID:          job.ID,
		IdempotencyKey: "panic-" + intNewID(),
		Payload:        json.RawMessage(`{"panic":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, &outboxTestQueue{
		enqueueInTxFn: func(context.Context, store.DBTX, *domain.JobRun) error {
			panic("enqueue panic")
		},
	}, scheduler.OutboxFlusherConfig{BatchSize: 1})

	err := flusher.FlushOnceForTest(ctx)
	require.Error(t, err)
	require.EqualValues(t, 1, flusher.
		Errors())
	require.EqualValues(t, 1, flusher.
		Iterations())

	assertOutboxState(t, ctx, entry.ID, false, false)
	assertRunCount(t, ctx, job.ID, entry.IdempotencyKey, 0)
}

func TestOutboxFlusher_PropagatesWorkerExecutionModeAndQueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-worker-routing", func(j *domain.Job) {
		j.ExecutionMode = domain.ExecutionModeWorker
		j.Queue = "priority"
	})
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"n":1}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	var captured *domain.JobRun
	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, &outboxTestQueue{
		enqueueInTxFn: func(_ context.Context, _ store.DBTX, run *domain.JobRun) error {
			cp := *run
			captured = &cp
			return nil
		},
	}, scheduler.OutboxFlusherConfig{BatchSize: 1})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)
	require.NotNil(t, captured)
	require.Equal(t, domain.ExecutionModeWorker,

		captured.
			ExecutionMode,
	)
	require.Equal(t, "priority",

		captured.
			QueueName)

}

func TestOutboxFlusher_InvalidMetadataQuarantinesRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-invalid-metadata")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"n":1}`),
		Metadata:  map[string]any{"attempt": 1},
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	enqueueCalled := false
	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, &outboxTestQueue{
		enqueueInTxFn: func(context.Context, store.DBTX, *domain.JobRun) error {
			enqueueCalled = true
			return nil
		},
	}, scheduler.OutboxFlusherConfig{BatchSize: 1})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)
	require.False(t, enqueueCalled)

	assertOutboxState(t, ctx, entry.ID, true, true)
	assertRunsForJob(t, ctx, job.ID, 0)
}

func TestOutboxFlusher_MixedBatchContinuesPastRetryableAndTerminalRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)

	goodJobA := intCreateJob(t, ctx, st, "proj-outbox-mixed")
	retryableJob := intCreateJob(t, ctx, st, "proj-outbox-mixed")
	terminalJob := intCreateJob(t, ctx, st, "proj-outbox-mixed")
	goodJobB := intCreateJob(t, ctx, st, "proj-outbox-mixed")

	entries := []queue.OutboxEntry{
		{
			ID:        intNewID(),
			ProjectID: goodJobA.ProjectID,
			JobID:     goodJobA.ID,
			Payload:   json.RawMessage(`{"ordinal":1}`),
		},
		{
			ID:        intNewID(),
			ProjectID: retryableJob.ProjectID,
			JobID:     retryableJob.ID,
			Payload:   json.RawMessage(`{"ordinal":2}`),
		},
		{
			ID:        intNewID(),
			ProjectID: terminalJob.ProjectID,
			JobID:     terminalJob.ID,
			Payload:   json.RawMessage(`{"ordinal":3}`),
		},
		{
			ID:        intNewID(),
			ProjectID: goodJobB.ProjectID,
			JobID:     goodJobB.ID,
			Payload:   json.RawMessage(`{"ordinal":4}`),
		},
	}
	intWriteOutboxEntries(t, ctx, entries)

	realQueue := intTestQueue(t)
	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, &outboxTestQueue{
		enqueueInTxFn: func(runCtx context.Context, tx store.DBTX, run *domain.JobRun) error {
			switch run.JobID {
			case retryableJob.ID:
				return context.DeadlineExceeded
			case terminalJob.ID:
				return &queue.TerminalEnqueueError{
					Reason: "validation",
					Err:    errors.New("payload rejected"),
				}
			default:
				return realQueue.EnqueueInTx(runCtx, tx, run)
			}
		},
	}, scheduler.OutboxFlusherConfig{BatchSize: len(entries)})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)

	assertOutboxState(t, ctx, entries[0].ID, false, true)
	assertOutboxState(t, ctx, entries[1].ID, false, false)
	assertOutboxState(t, ctx, entries[2].ID, true, true)
	assertOutboxState(t, ctx, entries[3].ID, false, true)

	assertRunsForJob(t, ctx, goodJobA.ID, 1)
	assertRunsForJob(t, ctx, retryableJob.ID, 0)
	assertRunsForJob(t, ctx, terminalJob.ID, 0)
	assertRunsForJob(t, ctx, goodJobB.ID, 1)

	count, err := st.CountUnconsumedOutbox(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

}

func TestOutboxFlusher_PoisonRowDoesNotStarveFollowingRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)

	poisonJob := intCreateJob(t, ctx, st, "proj-outbox-starvation")
	goodJob := intCreateJob(t, ctx, st, "proj-outbox-starvation")
	entries := []queue.OutboxEntry{
		{
			ID:        intNewID(),
			ProjectID: poisonJob.ProjectID,
			JobID:     poisonJob.ID,
			Payload:   json.RawMessage(`{"ordinal":1}`),
		},
		{
			ID:        intNewID(),
			ProjectID: goodJob.ProjectID,
			JobID:     goodJob.ID,
			Payload:   json.RawMessage(`{"ordinal":2}`),
		},
	}
	intWriteOutboxEntries(t, ctx, entries)

	if _, err := getTestDB(t).Pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, poisonJob.ID); err != nil {
		require.Failf(t, "test failure",

			"delete poison job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 2,
	})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)

	assertOutboxState(t, ctx, entries[0].ID, true, true)
	assertOutboxState(t, ctx, entries[1].ID, false, true)
	assertRunsForJob(t, ctx, goodJob.ID, 1)
}

func TestOutboxFlusher_RetryClonePromotesAfterUnderlyingIssueIsFixed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)

	poisonJob := intCreateJob(t, ctx, st, "proj-outbox-retry-clone")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: poisonJob.ProjectID,
		JobID:     poisonJob.ID,
		Payload:   json.RawMessage(`{"clone":true}`),
		Metadata: map[string]any{
			"source": "retry-clone-test",
		},
		IdempotencyKey: "retry-clone-key",
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	if _, err := getTestDB(t).Pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, poisonJob.ID); err != nil {
		require.Failf(t, "test failure",

			"delete poison job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 1,
	})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)

	assertOutboxState(t, ctx, entry.ID, true, true)

	restoredJob := intCreateJob(t, ctx, st, poisonJob.ProjectID, func(job *domain.Job) {
		job.ID = poisonJob.ID
		job.Name = "job-restored-" + intNewID()
		job.Slug = "slug-restored-" + intNewID()
	})
	require.Equal(t, poisonJob.
		ID,
		restoredJob.
			ID)

	cloned, err := st.RetryQuarantinedOutbox(ctx, poisonJob.ProjectID, entry.ID)
	require.NoError(t, err)
	require.False(t, cloned.RetryOfOutboxID ==
		nil ||
		*cloned.RetryOfOutboxID !=
			entry.
				ID)
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)

	assertOutboxState(t, ctx, entry.ID, true, true)
	assertOutboxState(t, ctx, cloned.ID, false, true)
	assertRunCount(t, ctx, poisonJob.ID, entry.IdempotencyKey, 1)

	count, err := st.CountUnconsumedOutbox(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func TestOutboxFlusherClaimLog_ConcurrentFlushersDoNotDoublePromote(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-claimlog-concurrent")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"claimlog":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	q := intTestQueue(t)
	cfg := scheduler.OutboxFlusherConfig{BatchSize: 1}
	flusherA := scheduler.NewOutboxFlusher(getTestDB(t).Pool, q, cfg)
	flusherB := scheduler.NewOutboxFlusher(getTestDB(t).Pool, q, cfg)
	errCh := make(chan error, 2)
	start := make(chan struct{})
	var concWG conc.WaitGroup
	concWG.Go(func() {
		<-start
		errCh <- flusherA.FlushOnceForTest(ctx)
	})
	concWG.Go(func() {
		<-start
		errCh <- flusherB.FlushOnceForTest(ctx)
	})
	close(start)
	concWG.Wait()
	for range 2 {
		require.NoError(t, <-errCh)

	}
	assertRunsForJob(t, ctx, job.ID, 1)
}

func TestOutboxFlusherClaimLog_RetryableFailureStaysClaimable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-claimlog-retry")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"retry":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	failFlusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, &outboxTestQueue{
		enqueueInTxFn: func(context.Context, store.DBTX, *domain.JobRun) error {
			return context.DeadlineExceeded
		},
	}, scheduler.OutboxFlusherConfig{BatchSize: 1})
	require.NoError(t, failFlusher.
		FlushOnceForTest(
			ctx))

	assertOutboxState(t, ctx, entry.ID, false, false)

	var claimStatus string
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT status FROM outbox_claims WHERE outbox_id = $1`,

			entry.ID).Scan(&claimStatus))
	require.Equal(t, "ready", claimStatus)

	successFlusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{BatchSize: 1})
	require.NoError(t, successFlusher.
		FlushOnceForTest(ctx))

	assertRunsForJob(t, ctx, job.ID, 1)
}

func TestOutboxFlusherClaimLog_WriteCreatesClaimBeforeFlush(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-claimlog-write-claim")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"claim":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	var status string
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT status FROM outbox_claims WHERE outbox_id = $1`,

			entry.ID).Scan(&status))
	require.Equal(t, "ready", status)

}

func TestOutboxFlusherClaimLog_EmptyClaimDoesNotCreateBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	intTestClean(t, ctx)
	var before int
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT COUNT(*) FROM outbox_batches`,
		).Scan(&before))

	tx, err := getTestDB(t).Pool.Begin(ctx)
	require.NoError(t, err)

	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := store.ClaimOutboxInTx(ctx, tx, 10, "test-flusher", time.Second)
	require.NoError(t, err)
	require.Len(t, rows, 0)
	require.NoError(t, tx.Commit(ctx))

	var batches int
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT COUNT(*) FROM outbox_batches`,
		).Scan(&batches))
	require.Equal(t, before, batches)

}

func TestOutboxFlusherClaimLog_ClaimDoesNotCreateBatchMetadataRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-claimlog-no-claim-batch")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"batch":false}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	var before int
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT COUNT(*) FROM outbox_batches`,
		).Scan(&before))

	tx, err := getTestDB(t).Pool.Begin(ctx)
	require.NoError(t, err)

	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := store.ClaimOutboxInTx(ctx, tx, 10, "test-flusher", time.Minute)
	require.NoError(t, err)
	require.False(t, len(rows) !=
		1 || rows[0].ID !=
		entry.ID)
	require.NoError(t, tx.Commit(ctx))

	var batches int
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT COUNT(*) FROM outbox_batches`,
		).Scan(&batches))
	require.Equal(t, before, batches)

	var batchID *int64
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT batch_id FROM outbox_claims WHERE outbox_id = $1`,

			entry.ID).Scan(&batchID))
	require.Nil(t, batchID)

}

func TestOutboxFlusherClaimLog_ReclaimExpiredLeaseRedelivers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-claimlog-reclaim")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"reclaim":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	tx, err := getTestDB(t).Pool.Begin(ctx)
	require.NoError(t, err)

	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := store.ClaimOutboxInTx(ctx, tx, 1, "test-flusher", time.Hour)
	require.NoError(t, err)
	require.False(t, len(rows) !=
		1 || rows[0].ID !=
		entry.ID)
	require.NoError(t, tx.Commit(ctx))

	if _, err := getTestDB(t).Pool.Exec(ctx, `
		UPDATE outbox_claims
		SET lease_expires_at = NOW() - INTERVAL '1 second'
		WHERE outbox_id = $1
	`, entry.ID); err != nil {
		require.Failf(t, "test failure",

			"expire lease: %v", err)
	}

	tx, err = getTestDB(t).Pool.Begin(ctx)
	require.NoError(t, err)

	defer tx.Rollback(ctx) //nolint:errcheck
	reclaimed, err := store.ReclaimExpiredOutboxClaimsInTx(ctx, tx)
	require.NoError(t, err)
	require.EqualValues(t, 1, reclaimed)
	require.NoError(t, tx.Commit(ctx))

	tx, err = getTestDB(t).Pool.Begin(ctx)
	require.NoError(t, err)

	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err = store.ClaimOutboxInTx(ctx, tx, 1, "test-flusher", time.Hour)
	require.NoError(t, err)
	require.False(t, len(rows) !=
		1 || rows[0].ID !=
		entry.ID)
	require.NoError(t, tx.Commit(ctx))

}

func TestOutboxFlusherClaimLog_PropagatesWorkerExecutionModeAndQueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-claimlog-worker-routing", func(j *domain.Job) {
		j.ExecutionMode = domain.ExecutionModeWorker
		j.Queue = "priority"
	})
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"claimlog":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	var captured *domain.JobRun
	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, &outboxTestQueue{
		enqueueInTxFn: func(_ context.Context, _ store.DBTX, run *domain.JobRun) error {
			cp := *run
			captured = &cp
			return nil
		},
	}, scheduler.OutboxFlusherConfig{BatchSize: 1})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)
	require.NotNil(t, captured)
	require.Equal(t, domain.ExecutionModeWorker,

		captured.
			ExecutionMode,
	)
	require.Equal(t, "priority",

		captured.
			QueueName)

}

func TestOutboxFlusherClaimLog_TerminalFailureQuarantinesOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-claimlog-terminal")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"terminal":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})
	if _, err := getTestDB(t).Pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job.ID); err != nil {
		require.Failf(t, "test failure",

			"delete job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{BatchSize: 1})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)

	assertOutboxState(t, ctx, entry.ID, true, true)
	var claimStatus string
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT status FROM outbox_claims WHERE outbox_id = $1`,

			entry.ID).Scan(&claimStatus))
	require.Equal(t, "acked", claimStatus)

}

func TestOutboxArchiver_PromotedClaimLogRowsArchivedHistoryVisible(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-claimlog-archive")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"archive":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{BatchSize: 1})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)

	var hotCount, historyCount int
	var consumedAt *time.Time
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT COUNT(*) FROM enqueue_outbox WHERE id = $1`,

			entry.ID).Scan(&hotCount))
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT COUNT(*) FROM enqueue_outbox_history WHERE id = $1`,

			entry.ID).Scan(&historyCount))
	require.False(t, hotCount !=

		1 || historyCount !=
		0)
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT consumed_at FROM enqueue_outbox WHERE id = $1`,

			entry.ID).Scan(&consumedAt))
	require.NotNil(t, consumedAt)

	var claimStatus string
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT status FROM outbox_claims WHERE outbox_id = $1`,

			entry.ID).Scan(&claimStatus))
	require.Equal(t, "acked", claimStatus)

	claimable, err := st.CountClaimableOutbox(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, claimable)

	archiver := scheduler.NewOutboxArchiver(store.New(getTestDB(t).Pool), scheduler.OutboxArchiverConfig{
		BatchSize: 10,
	})
	require.NoError(t, archiver.
		ArchiveOnceForTest(ctx))
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT COUNT(*) FROM enqueue_outbox WHERE id = $1`,

			entry.ID).Scan(&hotCount))
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT COUNT(*) FROM enqueue_outbox_history WHERE id = $1`,

			entry.ID).Scan(&historyCount))
	require.False(t, hotCount !=

		0 || historyCount !=
		1)

	var claimCount int
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT COUNT(*) FROM outbox_claims WHERE outbox_id = $1`,

			entry.ID).Scan(&claimCount))
	require.EqualValues(t, 0, claimCount)

}

func BenchmarkOutbox(b *testing.B) {
	ctx := context.Background()
	tdb := getTestDBTB(b)
	st := store.New(tdb.Pool)
	if err := tdb.CleanTables(ctx); err != nil {
		b.Fatalf("CleanTables() error = %v", err)
	}
	job := intCreateJobTB(b, ctx, st, "proj-outbox-bench")
	q := queue.NewPgQueQueue(tdb.Pool, queue.NewPostgresRunWriter(tdb.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "outbox-bench-" + intNewID(),
		ReceiveWindow: 100,
	})
	flusher := scheduler.NewOutboxFlusher(tdb.Pool, q, scheduler.OutboxFlusherConfig{
		BatchSize: 100,
	})
	seedOutboxBenchmarkEntries(b, ctx, job, 200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := flusher.FlushOnceForTest(ctx); err != nil {
			b.Fatalf("FlushOnceForTest: %v", err)
		}
		b.StopTimer()
		seedOutboxBenchmarkEntries(b, ctx, job, 100)
		b.StartTimer()
	}
}

func seedOutboxBenchmarkEntries(tb testing.TB, ctx context.Context, job *domain.Job, n int) {
	tb.Helper()
	entries := make([]queue.OutboxEntry, n)
	for i := range entries {
		entries[i] = queue.OutboxEntry{
			ID:        intNewID(),
			ProjectID: job.ProjectID,
			JobID:     job.ID,
			Payload:   json.RawMessage(`{"bench":true}`),
		}
	}
	intWriteOutboxEntriesTB(tb, ctx, entries)
}

func TestOutboxCountUnconsumed_ExcludesTerminalErroredRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)

	job := intCreateJob(t, ctx, st, "proj-outbox-count")
	entries := []queue.OutboxEntry{{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
	}}
	intWriteOutboxEntries(t, ctx, entries)

	if _, err := getTestDB(t).Pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job.ID); err != nil {
		require.Failf(t, "test failure",

			"delete job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 1,
	})
	require.NoError(t, flusher.
		FlushOnceForTest(ctx),
	)

	count, err := st.CountUnconsumedOutbox(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func intWriteOutboxEntries(t *testing.T, ctx context.Context, entries []queue.OutboxEntry) {
	t.Helper()

	tx, err := getTestDB(t).Pool.Begin(ctx)
	require.NoError(t, err)

	defer func() { _ = tx.Rollback(ctx) }()
	require.NoError(t, queue.WriteOutboxInTx(ctx, tx,
		entries))
	require.NoError(t, tx.Commit(ctx))

}

func getTestDBTB(tb testing.TB) *testutil.TestDB {
	tb.Helper()
	testDBOnce.Do(func() {
		ctx := context.Background()
		var err error
		testDB, err = testutil.SetupSharedTestDB(ctx, "../../migrations", "scheduler-outbox-flusher")
		if err != nil {
			log.Fatalf("setup test db: %v", err)
		}
	})
	if testDB == nil || testDB.Pool == nil {
		tb.Fatalf("testDB is not initialized")
	}
	return testDB
}

func intCreateJobTB(tb testing.TB, ctx context.Context, st *store.Queries, projectID string) *domain.Job {
	tb.Helper()
	job := &domain.Job{
		ID:          intNewID(),
		ProjectID:   projectID,
		Name:        "job-" + intNewID(),
		Slug:        "slug-" + intNewID(),
		EndpointURL: "https://example.com/integration-test",
		MaxAttempts: 3,
		TimeoutSecs: 300,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		tb.Fatalf("CreateJob() error = %v", err)
	}
	return job
}

func intWriteOutboxEntriesTB(tb testing.TB, ctx context.Context, entries []queue.OutboxEntry) {
	tb.Helper()

	tx, err := getTestDBTB(tb).Pool.Begin(ctx)
	if err != nil {
		tb.Fatalf("begin outbox tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := queue.WriteOutboxInTx(ctx, tx, entries); err != nil {
		tb.Fatalf("WriteOutboxInTx() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		tb.Fatalf("commit outbox tx: %v", err)
	}
}

func assertRunCount(t *testing.T, ctx context.Context, jobID, key string, want int) {
	t.Helper()

	var got int
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT COUNT(*) FROM job_runs WHERE job_id = $1 AND idempotency_key = $2`,

			jobID,
			key).Scan(&got))
	require.Equal(t, want, got)

}

func assertRunsForJob(t *testing.T, ctx context.Context, jobID string, want int) {
	t.Helper()

	var got int
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT COUNT(*) FROM job_runs WHERE job_id = $1`,

			jobID).Scan(&got))
	require.Equal(t, want, got)

}

func assertOutboxState(t *testing.T, ctx context.Context, id string, wantError bool, wantConsumed bool) {
	t.Helper()

	var errorText *string
	var consumedAt *time.Time
	require.NoError(t, getTestDB(t).Pool.
		QueryRow(ctx,
			`SELECT error, consumed_at FROM enqueue_outbox WHERE id = $1`,

			id).Scan(&errorText,
		&consumedAt))

	gotError := errorText != nil && *errorText != ""
	gotConsumed := consumedAt != nil
	require.Equal(t, wantError,

		gotError)
	require.Equal(t, wantConsumed,

		gotConsumed,
	)

}
