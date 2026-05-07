//go:build integration

package scheduler_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/scheduler"
	"strait/internal/store"
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
