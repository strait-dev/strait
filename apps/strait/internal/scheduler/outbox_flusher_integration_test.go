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

func (q *outboxTestQueue) Dequeue(_ context.Context) (*domain.JobRun, error) {
	return nil, nil
}

func (q *outboxTestQueue) DequeueN(_ context.Context, _ int) ([]domain.JobRun, error) {
	return nil, nil
}

func (q *outboxTestQueue) DequeueNFair(_ context.Context, _ int) ([]domain.JobRun, error) {
	return nil, nil
}

func (q *outboxTestQueue) DequeueNByProject(_ context.Context, _ int, _ string) ([]domain.JobRun, error) {
	return nil, nil
}

func TestOutboxFlusher_ConcurrentFlushersSameIdempotencyKeyNoDuplicateRuns(t *testing.T) {
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

	q := queue.NewPostgresQueue(getTestDB(t).Pool)
	flusherA := scheduler.NewOutboxFlusher(getTestDB(t).Pool, q, scheduler.OutboxFlusherConfig{BatchSize: 1})
	flusherB := scheduler.NewOutboxFlusher(getTestDB(t).Pool, q, scheduler.OutboxFlusherConfig{BatchSize: 1})

	start := make(chan struct{})
	errCh := make(chan error, 2)
	go func() {
		<-start
		errCh <- flusherA.FlushOnceForTest(ctx)
	}()
	go func() {
		<-start
		errCh <- flusherB.FlushOnceForTest(ctx)
	}()
	close(start)

	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("FlushOnceForTest() error = %v", err)
		}
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
		t.Fatalf("delete poison job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 10,
	})
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}

	assertOutboxState(t, ctx, entries[0].ID, true, true)
	assertOutboxState(t, ctx, entries[1].ID, false, true)
	assertRunsForJob(t, ctx, goodJob.ID, 1)

	count, err := st.CountUnconsumedOutbox(ctx)
	if err != nil {
		t.Fatalf("CountUnconsumedOutbox() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("CountUnconsumedOutbox() = %d, want 0 after terminal quarantine", count)
	}
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
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}

	assertOutboxState(t, ctx, entries[0].ID, false, false)
	count, err := st.CountUnconsumedOutbox(ctx)
	if err != nil {
		t.Fatalf("CountUnconsumedOutbox() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("CountUnconsumedOutbox() = %d, want 1 after retryable failure", count)
	}

	successFlusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 1,
	})
	if err := successFlusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("retry FlushOnceForTest() error = %v", err)
	}

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
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}

	assertOutboxState(t, ctx, entries[0].ID, false, false)
	count, err := st.CountUnconsumedOutbox(ctx)
	if err != nil {
		t.Fatalf("CountUnconsumedOutbox() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("CountUnconsumedOutbox() = %d, want 1 after unknown enqueue failure", count)
	}

	successFlusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 1,
	})
	if err := successFlusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("retry FlushOnceForTest() error = %v", err)
	}

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
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}

	assertOutboxState(t, ctx, entries[0].ID, false, false)
	count, err := st.CountUnconsumedOutbox(ctx)
	if err != nil {
		t.Fatalf("CountUnconsumedOutbox() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("CountUnconsumedOutbox() = %d, want 1 after throttle", count)
	}

	successFlusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{BatchSize: 1})
	if err := successFlusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("retry FlushOnceForTest() error = %v", err)
	}
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
	if err == nil {
		t.Fatal("FlushOnceForTest() error = nil, want recovered panic error")
	}
	if flusher.Errors() != 1 {
		t.Fatalf("Errors() = %d, want 1", flusher.Errors())
	}
	if flusher.Iterations() != 1 {
		t.Fatalf("Iterations() = %d, want 1", flusher.Iterations())
	}
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

	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}
	if captured == nil {
		t.Fatal("expected outbox flusher to enqueue a run")
	}
	if captured.ExecutionMode != domain.ExecutionModeWorker {
		t.Fatalf("ExecutionMode = %q, want %q", captured.ExecutionMode, domain.ExecutionModeWorker)
	}
	if captured.QueueName != "priority" {
		t.Fatalf("QueueName = %q, want priority", captured.QueueName)
	}
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

	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}
	if enqueueCalled {
		t.Fatal("invalid metadata row was enqueued")
	}
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
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}

	assertOutboxState(t, ctx, entries[0].ID, false, true)
	assertOutboxState(t, ctx, entries[1].ID, false, false)
	assertOutboxState(t, ctx, entries[2].ID, true, true)
	assertOutboxState(t, ctx, entries[3].ID, false, true)

	assertRunsForJob(t, ctx, goodJobA.ID, 1)
	assertRunsForJob(t, ctx, retryableJob.ID, 0)
	assertRunsForJob(t, ctx, terminalJob.ID, 0)
	assertRunsForJob(t, ctx, goodJobB.ID, 1)

	count, err := st.CountUnconsumedOutbox(ctx)
	if err != nil {
		t.Fatalf("CountUnconsumedOutbox() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("CountUnconsumedOutbox() = %d, want 1 with only retryable row left", count)
	}
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
		t.Fatalf("delete poison job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 2,
	})
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}

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
		t.Fatalf("delete poison job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 1,
	})
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("first FlushOnceForTest() error = %v", err)
	}

	assertOutboxState(t, ctx, entry.ID, true, true)

	restoredJob := intCreateJob(t, ctx, st, poisonJob.ProjectID, func(job *domain.Job) {
		job.ID = poisonJob.ID
		job.Name = "job-restored-" + intNewID()
		job.Slug = "slug-restored-" + intNewID()
	})
	if restoredJob.ID != poisonJob.ID {
		t.Fatalf("restored job ID = %q, want %q", restoredJob.ID, poisonJob.ID)
	}

	cloned, err := st.RetryQuarantinedOutbox(ctx, poisonJob.ProjectID, entry.ID)
	if err != nil {
		t.Fatalf("RetryQuarantinedOutbox() error = %v", err)
	}
	if cloned.RetryOfOutboxID == nil || *cloned.RetryOfOutboxID != entry.ID {
		t.Fatalf("RetryOfOutboxID = %v, want %q", cloned.RetryOfOutboxID, entry.ID)
	}

	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("second FlushOnceForTest() error = %v", err)
	}

	assertOutboxState(t, ctx, entry.ID, true, true)
	assertOutboxState(t, ctx, cloned.ID, false, true)
	assertRunCount(t, ctx, poisonJob.ID, entry.IdempotencyKey, 1)

	count, err := st.CountUnconsumedOutbox(ctx)
	if err != nil {
		t.Fatalf("CountUnconsumedOutbox() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("CountUnconsumedOutbox() = %d, want 0 after retry clone promotion", count)
	}
}

func TestOutboxBatchlog_ConcurrentFlushersDoNotDoublePromote(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-batchlog-concurrent")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"batchlog":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	q := intTestQueue(t)
	cfg := scheduler.OutboxFlusherConfig{BatchSize: 1, Engine: "batchlog"}
	flusherA := scheduler.NewOutboxFlusher(getTestDB(t).Pool, q, cfg)
	flusherB := scheduler.NewOutboxFlusher(getTestDB(t).Pool, q, cfg)
	errCh := make(chan error, 2)
	start := make(chan struct{})
	go func() {
		<-start
		errCh <- flusherA.FlushOnceForTest(ctx)
	}()
	go func() {
		<-start
		errCh <- flusherB.FlushOnceForTest(ctx)
	}()
	close(start)
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("FlushOnceForTest() error = %v", err)
		}
	}
	assertRunsForJob(t, ctx, job.ID, 1)
}

func TestOutboxBatchlog_RetryableFailureStaysClaimable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-batchlog-retry")
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
	}, scheduler.OutboxFlusherConfig{BatchSize: 1, Engine: "batchlog"})
	if err := failFlusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("failed FlushOnceForTest() error = %v", err)
	}
	assertOutboxState(t, ctx, entry.ID, false, false)

	var claimStatus string
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT status FROM outbox_claims WHERE outbox_id = $1`, entry.ID).Scan(&claimStatus); err != nil {
		t.Fatalf("claim status: %v", err)
	}
	if claimStatus != "ready" {
		t.Fatalf("claim status = %q, want ready", claimStatus)
	}

	successFlusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{BatchSize: 1, Engine: "batchlog"})
	if err := successFlusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("success FlushOnceForTest() error = %v", err)
	}
	assertRunsForJob(t, ctx, job.ID, 1)
}

func TestOutboxBatchlog_WriteCreatesClaimBeforeFlush(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-batchlog-write-claim")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"claim":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	var status string
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT status FROM outbox_claims WHERE outbox_id = $1`, entry.ID).Scan(&status); err != nil {
		t.Fatalf("claim status: %v", err)
	}
	if status != "ready" {
		t.Fatalf("claim status = %q, want ready", status)
	}
}

func TestOutboxBatchlog_EmptyClaimDoesNotCreateBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	intTestClean(t, ctx)
	var before int
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_batches`).Scan(&before); err != nil {
		t.Fatalf("count outbox batches before: %v", err)
	}
	tx, err := getTestDB(t).Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := store.ClaimOutboxBatchlogInTx(ctx, tx, 10, "test-flusher", time.Second)
	if err != nil {
		t.Fatalf("ClaimOutboxBatchlogInTx: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("claimed rows = %d, want 0", len(rows))
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}

	var batches int
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_batches`).Scan(&batches); err != nil {
		t.Fatalf("count outbox batches: %v", err)
	}
	if batches != before {
		t.Fatalf("outbox_batches count = %d, want unchanged %d", batches, before)
	}
}

func TestOutboxBatchlog_ClaimDoesNotCreateBatchMetadataRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-batchlog-no-claim-batch")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"batch":false}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	var before int
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_batches`).Scan(&before); err != nil {
		t.Fatalf("count outbox batches before: %v", err)
	}
	tx, err := getTestDB(t).Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := store.ClaimOutboxBatchlogInTx(ctx, tx, 10, "test-flusher", time.Minute)
	if err != nil {
		t.Fatalf("ClaimOutboxBatchlogInTx: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != entry.ID {
		t.Fatalf("claimed rows = %+v, want entry %s", rows, entry.ID)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}

	var batches int
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_batches`).Scan(&batches); err != nil {
		t.Fatalf("count outbox batches: %v", err)
	}
	if batches != before {
		t.Fatalf("outbox_batches count = %d, want unchanged %d", batches, before)
	}
	var batchID *int64
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT batch_id FROM outbox_claims WHERE outbox_id = $1`, entry.ID).Scan(&batchID); err != nil {
		t.Fatalf("select outbox claim batch_id: %v", err)
	}
	if batchID != nil {
		t.Fatalf("outbox claim batch_id = %d, want nil", *batchID)
	}
}

func TestOutboxBatchlog_ReclaimExpiredLeaseRedelivers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-batchlog-reclaim")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"reclaim":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	tx, err := getTestDB(t).Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin claim tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := store.ClaimOutboxBatchlogInTx(ctx, tx, 1, "test-flusher", time.Hour)
	if err != nil {
		t.Fatalf("initial ClaimOutboxBatchlogInTx: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != entry.ID {
		t.Fatalf("initial claimed rows = %+v, want entry %s", rows, entry.ID)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit initial claim: %v", err)
	}

	if _, err := getTestDB(t).Pool.Exec(ctx, `
		UPDATE outbox_claims
		SET lease_expires_at = NOW() - INTERVAL '1 second'
		WHERE outbox_id = $1
	`, entry.ID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	tx, err = getTestDB(t).Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin reclaim tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	reclaimed, err := store.ReclaimExpiredOutboxBatchlogClaimsInTx(ctx, tx)
	if err != nil {
		t.Fatalf("ReclaimExpiredOutboxBatchlogClaimsInTx: %v", err)
	}
	if reclaimed != 1 {
		t.Fatalf("reclaimed = %d, want 1", reclaimed)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit reclaim: %v", err)
	}

	tx, err = getTestDB(t).Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin second claim tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err = store.ClaimOutboxBatchlogInTx(ctx, tx, 1, "test-flusher", time.Hour)
	if err != nil {
		t.Fatalf("second ClaimOutboxBatchlogInTx: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != entry.ID {
		t.Fatalf("second claimed rows = %+v, want entry %s", rows, entry.ID)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit second claim: %v", err)
	}
}

func TestOutboxBatchlog_PropagatesWorkerExecutionModeAndQueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-batchlog-worker-routing", func(j *domain.Job) {
		j.ExecutionMode = domain.ExecutionModeWorker
		j.Queue = "priority"
	})
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"batchlog":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	var captured *domain.JobRun
	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, &outboxTestQueue{
		enqueueInTxFn: func(_ context.Context, _ store.DBTX, run *domain.JobRun) error {
			cp := *run
			captured = &cp
			return nil
		},
	}, scheduler.OutboxFlusherConfig{BatchSize: 1, Engine: "batchlog"})

	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}
	if captured == nil {
		t.Fatal("expected outbox flusher to enqueue a run")
	}
	if captured.ExecutionMode != domain.ExecutionModeWorker {
		t.Fatalf("ExecutionMode = %q, want %q", captured.ExecutionMode, domain.ExecutionModeWorker)
	}
	if captured.QueueName != "priority" {
		t.Fatalf("QueueName = %q, want priority", captured.QueueName)
	}
}

func TestOutboxBatchlog_TerminalFailureQuarantinesOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-batchlog-terminal")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"terminal":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})
	if _, err := getTestDB(t).Pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("delete job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{BatchSize: 1, Engine: "batchlog"})
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("second FlushOnceForTest() error = %v", err)
	}
	assertOutboxState(t, ctx, entry.ID, true, true)
	var claimStatus string
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT status FROM outbox_claims WHERE outbox_id = $1`, entry.ID).Scan(&claimStatus); err != nil {
		t.Fatalf("claim status: %v", err)
	}
	if claimStatus != "acked" {
		t.Fatalf("claim status = %q, want acked", claimStatus)
	}
}

func TestOutboxArchiver_PromotedBatchlogRowsArchivedHistoryVisible(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-outbox-batchlog-archive")
	entry := queue.OutboxEntry{
		ID:        intNewID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"archive":true}`),
	}
	intWriteOutboxEntries(t, ctx, []queue.OutboxEntry{entry})

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{BatchSize: 1, Engine: "batchlog"})
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}

	var hotCount, historyCount int
	var consumedAt *time.Time
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT COUNT(*) FROM enqueue_outbox WHERE id = $1`, entry.ID).Scan(&hotCount); err != nil {
		t.Fatalf("hot count: %v", err)
	}
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT COUNT(*) FROM enqueue_outbox_history WHERE id = $1`, entry.ID).Scan(&historyCount); err != nil {
		t.Fatalf("history count: %v", err)
	}
	if hotCount != 1 || historyCount != 0 {
		t.Fatalf("hot/history counts after flush = %d/%d, want 1/0", hotCount, historyCount)
	}
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT consumed_at FROM enqueue_outbox WHERE id = $1`, entry.ID).Scan(&consumedAt); err != nil {
		t.Fatalf("consumed_at after flush: %v", err)
	}
	if consumedAt != nil {
		t.Fatalf("consumed_at after batchlog flush = %v, want nil until archive", *consumedAt)
	}
	var claimStatus string
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT status FROM outbox_claims WHERE outbox_id = $1`, entry.ID).Scan(&claimStatus); err != nil {
		t.Fatalf("claim status after flush: %v", err)
	}
	if claimStatus != "acked" {
		t.Fatalf("claim status after flush = %q, want acked", claimStatus)
	}
	claimable, err := st.CountClaimableOutboxBatchlog(ctx)
	if err != nil {
		t.Fatalf("CountClaimableOutboxBatchlog() error = %v", err)
	}
	if claimable != 0 {
		t.Fatalf("claimable batchlog outbox = %d, want 0 after ack", claimable)
	}

	archiver := scheduler.NewOutboxArchiver(store.New(getTestDB(t).Pool), scheduler.OutboxArchiverConfig{
		BatchSize: 10,
	})
	if err := archiver.ArchiveOnceForTest(ctx); err != nil {
		t.Fatalf("ArchiveOnceForTest() error = %v", err)
	}
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT COUNT(*) FROM enqueue_outbox WHERE id = $1`, entry.ID).Scan(&hotCount); err != nil {
		t.Fatalf("hot count after archive: %v", err)
	}
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT COUNT(*) FROM enqueue_outbox_history WHERE id = $1`, entry.ID).Scan(&historyCount); err != nil {
		t.Fatalf("history count after archive: %v", err)
	}
	if hotCount != 0 || historyCount != 1 {
		t.Fatalf("hot/history counts after archive = %d/%d, want 0/1", hotCount, historyCount)
	}
	var claimCount int
	if err := getTestDB(t).Pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_claims WHERE outbox_id = $1`, entry.ID).Scan(&claimCount); err != nil {
		t.Fatalf("claim count after archive: %v", err)
	}
	if claimCount != 0 {
		t.Fatalf("claim count after archive = %d, want 0", claimCount)
	}
}

func BenchmarkOutbox(b *testing.B) {
	ctx := context.Background()
	for _, engine := range []string{"legacy", "batchlog"} {
		b.Run(engine, func(b *testing.B) {
			tdb := getTestDBTB(b)
			st := store.New(tdb.Pool)
			if err := tdb.CleanTables(ctx); err != nil {
				b.Fatalf("CleanTables() error = %v", err)
			}
			job := intCreateJobTB(b, ctx, st, "proj-outbox-bench-"+engine)
			q := queue.NewPostgresQueue(tdb.Pool)
			flusher := scheduler.NewOutboxFlusher(tdb.Pool, q, scheduler.OutboxFlusherConfig{
				BatchSize: 100,
				Engine:    engine,
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
		})
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
		t.Fatalf("delete job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getTestDB(t).Pool, intTestQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 1,
	})
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}

	count, err := st.CountUnconsumedOutbox(ctx)
	if err != nil {
		t.Fatalf("CountUnconsumedOutbox() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("CountUnconsumedOutbox() = %d, want 0 for terminal errored row", count)
	}
}

func intWriteOutboxEntries(t *testing.T, ctx context.Context, entries []queue.OutboxEntry) {
	t.Helper()

	tx, err := getTestDB(t).Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin outbox tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := queue.WriteOutboxInTx(ctx, tx, entries); err != nil {
		t.Fatalf("WriteOutboxInTx() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit outbox tx: %v", err)
	}
}

func getTestDBTB(tb testing.TB) *testutil.TestDB {
	tb.Helper()
	testDBOnce.Do(func() {
		ctx := context.Background()
		var err error
		testDB, err = testutil.SetupTestDB(ctx, "../../migrations")
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
	if err := getTestDB(t).Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_runs WHERE job_id = $1 AND idempotency_key = $2`,
		jobID, key,
	).Scan(&got); err != nil {
		t.Fatalf("count job_runs: %v", err)
	}
	if got != want {
		t.Fatalf("job_runs count = %d, want %d", got, want)
	}
}

func assertRunsForJob(t *testing.T, ctx context.Context, jobID string, want int) {
	t.Helper()

	var got int
	if err := getTestDB(t).Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_runs WHERE job_id = $1`,
		jobID,
	).Scan(&got); err != nil {
		t.Fatalf("count job runs for job %s: %v", jobID, err)
	}
	if got != want {
		t.Fatalf("job %s run count = %d, want %d", jobID, got, want)
	}
}

func assertOutboxState(t *testing.T, ctx context.Context, id string, wantError bool, wantConsumed bool) {
	t.Helper()

	var errorText *string
	var consumedAt *time.Time
	if err := getTestDB(t).Pool.QueryRow(ctx,
		`SELECT error, consumed_at FROM enqueue_outbox WHERE id = $1`,
		id,
	).Scan(&errorText, &consumedAt); err != nil {
		t.Fatalf("read outbox state %s: %v", id, err)
	}

	gotError := errorText != nil && *errorText != ""
	gotConsumed := consumedAt != nil
	if gotError != wantError {
		t.Fatalf("outbox %s hasError=%v, want %v (error=%v)", id, gotError, wantError, errorText)
	}
	if gotConsumed != wantConsumed {
		t.Fatalf("outbox %s consumed=%v, want %v", id, gotConsumed, wantConsumed)
	}
}
