//go:build integration

package queue_test

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

// Edge cases and boundary conditions.

func TestEdge_EmptyPayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-empty-payload")
	q := mustQueue(t)
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Payload: json.RawMessage(`{}`)}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	batch, _ := q.DequeueN(ctx, 1)
	if len(batch) != 1 {
		t.Fatal("dequeue empty")
	}
}

func TestEdge_NullPayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-null-payload")
	q := mustQueue(t)
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	batch, _ := q.DequeueN(ctx, 1)
	if len(batch) != 1 {
		t.Fatal("dequeue empty")
	}
}

func TestEdge_LargePayload_1MB(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-1mb")
	q := mustQueue(t)
	large := `{"data":"` + strings.Repeat("x", 1024*1024) + `"}`
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Payload: json.RawMessage(large)}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue large: %v", err)
	}
	batch, _ := q.DequeueN(ctx, 1)
	if len(batch) != 1 {
		t.Fatal("dequeue empty")
	}
	if len(batch[0].Payload) < 1024*1024 {
		t.Errorf("payload truncated: %d bytes", len(batch[0].Payload))
	}
}

func TestEdge_UnicodePayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-unicode")
	q := mustQueue(t)
	payload := `{"emoji":"` + "\U0001F680\U0001F525\u2764\uFE0F" + `","cjk":"` + "\u4F60\u597D\u4E16\u754C" + `"}`
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Payload: json.RawMessage(payload)}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	batch, _ := q.DequeueN(ctx, 1)
	if len(batch) != 1 {
		t.Fatal("dequeue")
	}
	if !strings.Contains(string(batch[0].Payload), "\U0001F680") {
		t.Error("emoji lost in round-trip")
	}
}

func TestEdge_PriorityZero(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-pri0")
	q := mustQueue(t)
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 0}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	batch, _ := q.DequeueN(ctx, 1)
	if len(batch) != 1 {
		t.Fatal("priority 0 run not claimable")
	}
}

func TestEdge_PriorityMaxInt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-primax")
	q := mustQueue(t)
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: math.MaxInt32}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	batch, _ := q.DequeueN(ctx, 1)
	if len(batch) != 1 || batch[0].Priority != math.MaxInt32 {
		t.Errorf("priority mismatch: got %d", batch[0].Priority)
	}
}

func TestEdge_PriorityOrdering_SamePriorityTiebreakByCreatedAt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-tiebreak")
	q := mustQueue(t)
	first := mustEnqueueRun(t, ctx, q, job)
	time.Sleep(5 * time.Millisecond)
	second := mustEnqueueRun(t, ctx, q, job)
	batch, _ := q.DequeueN(ctx, 2)
	if len(batch) != 2 {
		t.Fatal("expected 2")
	}
	if batch[0].ID != first.ID {
		t.Errorf("expected first-created %s, got %s (second was %s)", first.ID, batch[0].ID, second.ID)
	}
}

func TestEdge_DequeueNZero(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	q := mustQueue(t)
	batch, err := q.DequeueN(ctx, 0)
	if err != nil {
		t.Fatalf("dequeue(0): %v", err)
	}
	if len(batch) != 0 {
		t.Errorf("dequeue(0) returned %d runs", len(batch))
	}
}

func TestEdge_EnqueueBatchZero(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	n, err := q.EnqueueBatch(ctx, nil)
	if err != nil {
		t.Fatalf("batch nil: %v", err)
	}
	if n != 0 {
		t.Errorf("batch nil = %d", n)
	}
}

func TestEdge_ScheduledAtFarFuture(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-future")
	q := mustQueue(t)
	future := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, ScheduledAt: &future}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if run.Status != domain.StatusDelayed {
		t.Errorf("expected delayed, got %s", run.Status)
	}
	batch, _ := q.DequeueN(ctx, 1)
	if len(batch) != 0 {
		t.Error("future run should not be claimable")
	}
}

func TestEdge_NextRetryAtInPast(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-retrpast")
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)
	past := time.Now().Add(-1 * time.Hour)
	// Retry schedule lives in the job_retries side table; a past timestamp
	// means the run is claimable.
	_, _ = testDB.Pool.Exec(ctx, `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at)
		VALUES ($1, $2, 1, NOW())`,
		run.ID, past)
	batch, _ := q.DequeueN(ctx, 1)
	if len(batch) != 1 || batch[0].ID != run.ID {
		t.Error("run with past next_retry_at should be claimable")
	}
}

func TestEdge_NULLVsEmptyIdempotencyKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-idem-null")
	q := mustQueue(t)
	// Empty string key.
	r1 := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, IdempotencyKey: ""}
	if err := q.Enqueue(ctx, r1); err != nil {
		t.Fatalf("enqueue empty key: %v", err)
	}
	// Same empty string key should NOT be deduped (empty = no key).
	r2 := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, IdempotencyKey: ""}
	if err := q.Enqueue(ctx, r2); err != nil {
		t.Fatalf("enqueue empty key 2: %v", err)
	}
	var count int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs WHERE job_id=$1`, job.ID).Scan(&count)
	if count != 2 {
		t.Errorf("empty idempotency_key should not dedupe: count=%d", count)
	}
}

func TestEdge_NULLVsEmptyConcurrencyKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-ck-null")
	q := mustQueue(t)
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, ConcurrencyKey: ""}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue empty ck: %v", err)
	}
	batch, _ := q.DequeueN(ctx, 1)
	if len(batch) != 1 {
		t.Fatal("empty concurrency key run not claimable")
	}
}

func TestEdge_MetadataEmptyVsNil(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-meta")
	q := mustQueue(t)
	// nil metadata.
	r1 := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, r1); err != nil {
		t.Fatalf("nil meta: %v", err)
	}
	// empty map metadata.
	r2 := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Metadata: map[string]string{}}
	if err := q.Enqueue(ctx, r2); err != nil {
		t.Fatalf("empty meta: %v", err)
	}
	batch, _ := q.DequeueN(ctx, 2)
	if len(batch) != 2 {
		t.Errorf("expected 2, got %d", len(batch))
	}
}

func TestEdge_NestedJSONPayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-nested")
	q := mustQueue(t)
	nested := `{"a":{"b":{"c":{"d":{"e":"deep"}}}}}`
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Payload: json.RawMessage(nested)}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("nested: %v", err)
	}
	batch, _ := q.DequeueN(ctx, 1)
	if !strings.Contains(string(batch[0].Payload), `"deep"`) {
		t.Error("nested payload lost in round-trip")
	}
}

func TestEdge_EmptyQueueDequeueReturnsNil(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustClean(t, ctx)
	q := mustQueue(t)
	r, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue empty: %v", err)
	}
	if r != nil {
		t.Error("dequeue on empty queue should return nil")
	}
}

func TestEdge_LargeBatchEnqueue500(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-edge-batch500")
	q := mustQueue(t)

	runs := make([]*domain.JobRun, 500)
	for i := range runs {
		runs[i] = &domain.JobRun{JobID: job.ID, ProjectID: job.ProjectID}
	}
	n, err := q.EnqueueBatch(ctx, runs)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if n != 500 {
		t.Errorf("batch = %d, want 500", n)
	}
}
