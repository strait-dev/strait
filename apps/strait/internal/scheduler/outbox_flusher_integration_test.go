//go:build integration

package scheduler_test

import (
	"context"
	"encoding/json"
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
