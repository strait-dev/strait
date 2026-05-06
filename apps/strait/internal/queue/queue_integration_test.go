//go:build integration

package queue_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
)

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		log.Fatalf("setup test db: %v", err)
	}

	code := m.Run()
	testDB.Cleanup(ctx)
	os.Exit(code)
}

func TestEnqueue(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-enqueue")
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Priority:  1,
	}

	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if run.Status != domain.StatusQueued {
		t.Fatalf("Enqueue() status = %q, want %q", run.Status, domain.StatusQueued)
	}
	if run.CreatedAt.IsZero() {
		t.Fatal("Enqueue() did not set created_at")
	}

	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusQueued {
		t.Fatalf("stored status = %q, want %q", got.Status, domain.StatusQueued)
	}
}

func TestEnqueue_Delayed(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-enqueue-delayed")
	scheduledAt := time.Now().Add(30 * time.Minute)
	run := &domain.JobRun{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		ScheduledAt: &scheduledAt,
	}

	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if run.Status != domain.StatusDelayed {
		t.Fatalf("Enqueue() status = %q, want %q", run.Status, domain.StatusDelayed)
	}
}

func TestEnqueue_DefaultsApplied(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-enqueue-defaults")
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
	}

	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if run.Attempt != 1 {
		t.Fatalf("Enqueue() attempt = %d, want 1", run.Attempt)
	}
	if run.TriggeredBy != domain.TriggerManual {
		t.Fatalf("Enqueue() triggered_by = %q, want %q", run.TriggeredBy, domain.TriggerManual)
	}
}

func TestDequeue(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-dequeue")
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued == nil {
		t.Fatal("Dequeue() returned nil")
	}
	if dequeued.Status != domain.StatusDequeued {
		t.Fatalf("Dequeue() status = %q, want %q", dequeued.Status, domain.StatusDequeued)
	}
	if dequeued.StartedAt == nil {
		t.Fatal("Dequeue() did not set started_at")
	}
}

func TestDequeue_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	mustClean(t, ctx)

	run, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if run != nil {
		t.Fatalf("Dequeue() run = %+v, want nil", run)
	}
}

func TestDequeue_PriorityOrdering(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-dequeue-priority")
	runs := []*domain.JobRun{
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 0},
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 5},
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 10},
	}
	for _, run := range runs {
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued == nil {
		t.Fatal("Dequeue() returned nil")
	}
	if dequeued.Priority != 10 {
		t.Fatalf("Dequeue() priority = %d, want 10", dequeued.Priority)
	}
}

func TestDequeueN(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-dequeue-n")
	for range 5 {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	dequeued, err := q.DequeueN(ctx, 3)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(dequeued) != 3 {
		t.Fatalf("DequeueN() len = %d, want 3", len(dequeued))
	}

	status := domain.StatusQueued
	remaining, err := st.ListRunsByProject(ctx, job.ProjectID, &status, nil, nil, nil, nil, nil, nil, nil, 20, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() error = %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining queued runs = %d, want 2", len(remaining))
	}
}

func TestDequeueN_PriorityOrdering(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-dequeue-n-priority")
	for _, priority := range []int{10, 5, 0} {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: priority}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	dequeued, err := q.DequeueN(ctx, 3)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(dequeued) != 3 {
		t.Fatalf("DequeueN() len = %d, want 3", len(dequeued))
	}
	if dequeued[0].Priority != 10 {
		t.Fatalf("DequeueN() first priority = %d, want 10", dequeued[0].Priority)
	}
}

func TestDequeueN_RespectsScheduledAt(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-dequeue-n-scheduled-at")
	scheduledAt := time.Now().Add(15 * time.Minute)
	run := &domain.JobRun{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		ScheduledAt: &scheduledAt,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dequeued, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(dequeued) != 0 {
		t.Fatalf("DequeueN() len = %d, want 0", len(dequeued))
	}
}

func TestDequeueN_RespectsNextRetryAt(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-dequeue-n-next-retry")
	nextRetryAt := time.Now().Add(20 * time.Minute)
	run := &domain.JobRun{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		NextRetryAt: &nextRetryAt,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dequeued, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(dequeued) != 0 {
		t.Fatalf("DequeueN() len = %d, want 0", len(dequeued))
	}
}

func TestDequeueN_RespectsMaxConcurrency(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-dequeue-n-max-concurrency")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency = 1 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max_concurrency error = %v", err)
	}

	active := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Status:    domain.StatusExecuting,
		Attempt:   1,
	}
	if err := st.CreateRun(ctx, active); err != nil {
		t.Fatalf("CreateRun() active error = %v", err)
	}

	candidate := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, candidate); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dequeued, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(dequeued) != 0 {
		t.Fatalf("DequeueN() len = %d, want 0", len(dequeued))
	}
}

func TestDequeueNByProject_FiltersPartition(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	jobA := mustCreateJob(t, ctx, st, "project-queue-partition-a")
	jobB := mustCreateJob(t, ctx, st, "project-queue-partition-b")

	runA := &domain.JobRun{ID: newID(), JobID: jobA.ID, ProjectID: jobA.ProjectID}
	runB := &domain.JobRun{ID: newID(), JobID: jobB.ID, ProjectID: jobB.ProjectID}
	if err := q.Enqueue(ctx, runA); err != nil {
		t.Fatalf("Enqueue() runA error = %v", err)
	}
	if err := q.Enqueue(ctx, runB); err != nil {
		t.Fatalf("Enqueue() runB error = %v", err)
	}

	dequeued, err := q.DequeueNByProject(ctx, 10, jobA.ProjectID)
	if err != nil {
		t.Fatalf("DequeueNByProject() error = %v", err)
	}
	if len(dequeued) != 1 {
		t.Fatalf("DequeueNByProject() len = %d, want 1", len(dequeued))
	}
	if dequeued[0].ProjectID != jobA.ProjectID {
		t.Fatalf("DequeueNByProject() project_id = %q, want %q", dequeued[0].ProjectID, jobA.ProjectID)
	}

	other, err := st.GetRun(ctx, runB.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if other.Status != domain.StatusQueued {
		t.Fatalf("other run status = %q, want %q", other.Status, domain.StatusQueued)
	}
}

func TestDequeueN_ConcurrentWorkers(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-dequeue-n-concurrent-workers")
	for range 20 {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	var (
		wg    conc.WaitGroup
		mu    sync.Mutex
		seen  = make(map[string]int)
		total int
	)

	errCh := make(chan error, 5)
	for range 5 {
		wg.Go(func() {
			runs, err := q.DequeueN(ctx, 10)
			if err != nil {
				errCh <- err
				return
			}

			mu.Lock()
			defer mu.Unlock()
			for i := range runs {
				seen[runs[i].ID]++
				total++
			}
		})
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("DequeueN() error = %v", err)
		}
	}

	for runID, count := range seen {
		if count > 1 {
			t.Fatalf("run %s dequeued %d times, want 1", runID, count)
		}
	}
	if total != 20 {
		t.Fatalf("total dequeued = %d, want 20", total)
	}
	if len(seen) != 20 {
		t.Fatalf("unique dequeued runs = %d, want 20", len(seen))
	}
}

func TestDequeueN_SkipLocked_UnderContention(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-dequeue-n-skip-locked-contention")
	for range 50 {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	var (
		wg   conc.WaitGroup
		mu   sync.Mutex
		seen = make(map[string]int)
	)

	errCh := make(chan error, 5)
	for range 5 {
		wg.Go(func() {
			for {
				runs, err := q.DequeueN(ctx, 5)
				if err != nil {
					errCh <- err
					return
				}
				if len(runs) == 0 {
					return
				}

				mu.Lock()
				for i := range runs {
					seen[runs[i].ID]++
				}
				mu.Unlock()
			}
		})
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("DequeueN() error = %v", err)
		}
	}

	for runID, count := range seen {
		if count > 1 {
			t.Fatalf("run %s dequeued %d times, want 1", runID, count)
		}
	}
	if len(seen) != 50 {
		t.Fatalf("unique dequeued runs = %d, want 50", len(seen))
	}
}

func TestEnqueue_IdempotencyConflict(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-enqueue-idempotency-conflict")
	runA := &domain.JobRun{
		ID:             newID(),
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		IdempotencyKey: "key-1",
	}
	if err := q.Enqueue(ctx, runA); err != nil {
		t.Fatalf("Enqueue() first run error = %v", err)
	}

	runB := &domain.JobRun{
		ID:             newID(),
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		IdempotencyKey: "key-1",
	}
	err := q.Enqueue(ctx, runB)
	if err == nil {
		t.Fatal("Enqueue() second run error = nil, want idempotency conflict")
	}
	if !errors.Is(err, domain.ErrIdempotencyConflict) && !strings.Contains(strings.ToLower(err.Error()), "idempotency") {
		t.Fatalf("Enqueue() second run error = %v, want idempotency conflict", err)
	}
}

func TestEnqueue_IdempotencyAllowsAfterTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-enqueue-idempotency-terminal")
	runA := &domain.JobRun{
		ID:             newID(),
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		IdempotencyKey: "key-2",
	}
	if err := q.Enqueue(ctx, runA); err != nil {
		t.Fatalf("Enqueue() first run error = %v", err)
	}

	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status = $1, finished_at = NOW() WHERE id = $2`, domain.StatusCompleted, runA.ID); err != nil {
		t.Fatalf("set completed status error = %v", err)
	}

	runB := &domain.JobRun{
		ID:             newID(),
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		IdempotencyKey: "key-2",
	}
	if err := q.Enqueue(ctx, runB); err != nil {
		t.Fatalf("Enqueue() second run error = %v", err)
	}
}

func TestEnqueue_ContextCancellation(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-enqueue-context-cancel")
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(cancelledCtx, run); err == nil {
		t.Fatal("Enqueue() error = nil, want context cancellation error")
	}
}

func TestDequeue_ContextCancellation(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	mustClean(t, ctx)

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	run, err := q.Dequeue(cancelledCtx)
	if err == nil {
		t.Fatal("Dequeue() error = nil, want context cancellation error")
	}
	if run != nil {
		t.Fatalf("Dequeue() run = %+v, want nil", run)
	}
}

func TestDequeueN_LargePayload(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-dequeue-n-large-payload")
	payload, err := json.Marshal(map[string]string{"data": strings.Repeat("x", 100*1024)})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Payload: payload}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dequeued, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(dequeued) != 1 {
		t.Fatalf("DequeueN() len = %d, want 1", len(dequeued))
	}

	var gotCompact bytes.Buffer
	if err := json.Compact(&gotCompact, dequeued[0].Payload); err != nil {
		t.Fatalf("json.Compact(got) error = %v", err)
	}
	var wantCompact bytes.Buffer
	if err := json.Compact(&wantCompact, payload); err != nil {
		t.Fatalf("json.Compact(want) error = %v", err)
	}
	if !bytes.Equal(gotCompact.Bytes(), wantCompact.Bytes()) {
		t.Fatalf("DequeueN() payload mismatch after compact: got %d bytes, want %d bytes", gotCompact.Len(), wantCompact.Len())
	}
}

func TestDequeueN_EmptyTags(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-dequeue-n-empty-tags")
	runWithEmptyTags := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Tags:      map[string]string{},
	}
	runWithNilTags := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Tags:      nil,
	}

	if err := q.Enqueue(ctx, runWithEmptyTags); err != nil {
		t.Fatalf("Enqueue() runWithEmptyTags error = %v", err)
	}
	if err := q.Enqueue(ctx, runWithNilTags); err != nil {
		t.Fatalf("Enqueue() runWithNilTags error = %v", err)
	}

	dequeued, err := q.DequeueN(ctx, 2)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(dequeued) != 2 {
		t.Fatalf("DequeueN() len = %d, want 2", len(dequeued))
	}

	ids := map[string]struct{}{}
	for i := range dequeued {
		ids[dequeued[i].ID] = struct{}{}
	}
	if _, ok := ids[runWithEmptyTags.ID]; !ok {
		t.Fatalf("dequeued runs missing runWithEmptyTags id %s", runWithEmptyTags.ID)
	}
	if _, ok := ids[runWithNilTags.ID]; !ok {
		t.Fatalf("dequeued runs missing runWithNilTags id %s", runWithNilTags.ID)
	}
}

// Retry priority boost integration tests.

func TestDequeue_RetryPriorityBoostOrdering(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-retry-boost-ordering")

	// Enqueue 5 "new" runs at priority 0
	for range 5 {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 0}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	// Enqueue 1 "retried" run at priority 2 (simulating boost after 1 retry with boost=2)
	boostedID := newID()
	boostedRun := &domain.JobRun{ID: boostedID, JobID: job.ID, ProjectID: job.ProjectID, Priority: 2}
	if err := q.Enqueue(ctx, boostedRun); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Dequeue should return the boosted run first
	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued == nil {
		t.Fatal("Dequeue() returned nil")
	}
	if dequeued.ID != boostedID {
		t.Fatalf("expected boosted run %s to dequeue first, got %s", boostedID, dequeued.ID)
	}
	if dequeued.Priority != 2 {
		t.Fatalf("expected priority=2, got %d", dequeued.Priority)
	}
}

func TestDequeue_RetryBoostDoesNotJumpHigherPriority(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-boost-no-jump")

	// High-priority new run
	highPriorityID := newID()
	highRun := &domain.JobRun{ID: highPriorityID, JobID: job.ID, ProjectID: job.ProjectID, Priority: 5}
	if err := q.Enqueue(ctx, highRun); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Boosted retry run at lower priority
	boostedRun := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 2}
	if err := q.Enqueue(ctx, boostedRun); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Natural high-priority run should still win
	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued.ID != highPriorityID {
		t.Fatalf("expected high-priority run %s to dequeue first, got %s", highPriorityID, dequeued.ID)
	}
	if dequeued.Priority != 5 {
		t.Fatalf("expected priority=5, got %d", dequeued.Priority)
	}
}

func TestDequeueN_MixedPriorityWithBoostedRetries(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-mixed-boost")

	// Enqueue runs with mixed priorities: 3 new + 2 boosted + 1 high-priority.
	// DequeueN claims the highest-priority runs but returns them ordered by
	// created_at ASC (not priority), so we verify the set of priorities, not order.
	type runSpec struct {
		priority int
	}
	specs := []runSpec{
		{priority: 0}, {priority: 0}, {priority: 0},
		{priority: 2}, {priority: 2},
		{priority: 5},
	}
	for _, s := range specs {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: s.priority}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	dequeued, err := q.DequeueN(ctx, 6)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(dequeued) != 6 {
		t.Fatalf("DequeueN() len = %d, want 6", len(dequeued))
	}

	// Count priorities in the dequeued set.
	priorityCounts := map[int]int{}
	for _, d := range dequeued {
		priorityCounts[d.Priority]++
	}
	if priorityCounts[5] != 1 {
		t.Fatalf("expected 1 run with priority=5, got %d", priorityCounts[5])
	}
	if priorityCounts[2] != 2 {
		t.Fatalf("expected 2 runs with priority=2, got %d", priorityCounts[2])
	}
	if priorityCounts[0] != 3 {
		t.Fatalf("expected 3 runs with priority=0, got %d", priorityCounts[0])
	}
}

func TestDequeue_SamePriorityFIFO(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-same-priority-fifo")

	// Enqueue 3 runs at the same boosted priority, sequentially
	ids := make([]string, 3)
	for i := range 3 {
		ids[i] = newID()
		run := &domain.JobRun{ID: ids[i], JobID: job.ID, ProjectID: job.ProjectID, Priority: 2}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
		// Small sleep to ensure created_at ordering
		time.Sleep(5 * time.Millisecond)
	}

	// Dequeue all 3 and verify FIFO ordering (earliest created_at first)
	dequeued, err := q.DequeueN(ctx, 3)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(dequeued) != 3 {
		t.Fatalf("DequeueN() len = %d, want 3", len(dequeued))
	}
	for i, id := range ids {
		if dequeued[i].ID != id {
			t.Fatalf("position %d: expected run %s (FIFO), got %s", i, id, dequeued[i].ID)
		}
	}
}

func TestDequeue_BoostSaturationDoesNotStarve(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-boost-no-starve")

	// Enqueue 10 boosted runs at priority 2
	for range 10 {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 2}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	// Enqueue 1 new run at priority 5
	highPriorityID := newID()
	highRun := &domain.JobRun{ID: highPriorityID, JobID: job.ID, ProjectID: job.ProjectID, Priority: 5}
	if err := q.Enqueue(ctx, highRun); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// High-priority run should dequeue first despite many boosted runs
	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued.ID != highPriorityID {
		t.Fatalf("expected high-priority run to dequeue first, got run with priority %d", dequeued.Priority)
	}
}

func TestDequeue_BoostedRunTimingTiebreaker(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-boost-timing")

	// Two runs at same priority but different created_at
	firstID := newID()
	first := &domain.JobRun{ID: firstID, JobID: job.ID, ProjectID: job.ProjectID, Priority: 2}
	if err := q.Enqueue(ctx, first); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	secondID := newID()
	second := &domain.JobRun{ID: secondID, JobID: job.ID, ProjectID: job.ProjectID, Priority: 2}
	if err := q.Enqueue(ctx, second); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// First enqueued should dequeue first (FIFO tiebreaker)
	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued.ID != firstID {
		t.Fatalf("expected first-enqueued run %s to dequeue first (created_at tiebreaker), got %s", firstID, dequeued.ID)
	}
}

func mustQueue(t *testing.T) *queue.PostgresQueue {
	t.Helper()

	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}

	return queue.NewPostgresQueue(testDB.Pool)
}

func mustStore(t *testing.T) *store.Queries {
	t.Helper()

	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}

	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()

	if err := testDB.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
}

func mustCreateJob(t *testing.T, ctx context.Context, st *store.Queries, projectID string) *domain.Job {
	t.Helper()

	job := &domain.Job{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "job-" + newID(),
		Slug:        "slug-" + newID(),
		EndpointURL: "https://example.com/queue-job",
		MaxAttempts: 3,
		TimeoutSecs: 300,
		Enabled:     true,
	}

	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	return job
}

func markWorkerJobQueue(t *testing.T, ctx context.Context, job *domain.Job, queueName string) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE jobs SET execution_mode = 'worker', queue_name = $2 WHERE id = $1`,
		job.ID, queueName,
	); err != nil {
		t.Fatalf("mark worker job queue: %v", err)
	}
	job.ExecutionMode = domain.ExecutionModeWorker
	job.Queue = queueName
}

func mustCreateEnvironment(t *testing.T, ctx context.Context, st *store.Queries, projectID, slug string) string {
	t.Helper()
	env := &domain.Environment{
		ProjectID: projectID,
		Name:      slug,
		Slug:      slug,
	}
	if err := st.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment(%s): %v", slug, err)
	}
	return env.ID
}

func markWorkerJobQueueEnvironment(t *testing.T, ctx context.Context, job *domain.Job, queueName, environmentID string) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE jobs SET execution_mode = 'worker', queue_name = $2, environment_id = $3 WHERE id = $1`,
		job.ID, queueName, environmentID,
	); err != nil {
		t.Fatalf("mark worker job queue environment: %v", err)
	}
	job.ExecutionMode = domain.ExecutionModeWorker
	job.Queue = queueName
	job.EnvironmentID = environmentID
}

func assertClaimRouting(t *testing.T, ctx context.Context, runID string, wantMode domain.ExecutionMode, wantQueue string) {
	t.Helper()
	var gotMode, gotQueue string
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT execution_mode, queue_name FROM job_run_queue WHERE run_id = $1`,
		runID,
	).Scan(&gotMode, &gotQueue); err != nil {
		t.Fatalf("query claim routing: %v", err)
	}
	if gotMode != string(wantMode) {
		t.Fatalf("claim execution_mode = %q, want %q", gotMode, wantMode)
	}
	if gotQueue != wantQueue {
		t.Fatalf("claim queue_name = %q, want %q", gotQueue, wantQueue)
	}
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func TestEnqueueBatch_500Items_Integration(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-batch-500")

	runs := make([]*domain.JobRun, 500)
	for i := range runs {
		runs[i] = &domain.JobRun{
			JobID:     job.ID,
			ProjectID: job.ProjectID,
		}
	}

	n, err := q.EnqueueBatch(ctx, runs)
	if err != nil {
		t.Fatalf("EnqueueBatch() error = %v", err)
	}
	if n != 500 {
		t.Fatalf("expected 500 inserted, got %d", n)
	}

	// Verify all runs got IDs assigned.
	ids := make(map[string]bool, 500)
	for i, run := range runs {
		if run.ID == "" {
			t.Fatalf("run %d: expected ID to be assigned", i)
		}
		if ids[run.ID] {
			t.Fatalf("run %d: duplicate ID %s", i, run.ID)
		}
		ids[run.ID] = true
	}

	// Verify all 500 are in the database as queued.
	var count int
	err = testDB.Pool.QueryRow(ctx,
		"SELECT count(*) FROM job_runs WHERE job_id = $1 AND status = 'queued'",
		job.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count query error: %v", err)
	}
	if count != 500 {
		t.Fatalf("expected 500 queued runs in DB, got %d", count)
	}
}

func TestEnqueueBatch_VerifyAllFieldsPersisted_Integration(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-batch-fields")

	payload := json.RawMessage(`{"key":"value"}`)
	runs := []*domain.JobRun{
		{
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Payload:   payload,
			Priority:  5,
			Tags:      map[string]string{"env": "test", "region": "us-east-1"},
		},
	}

	n, err := q.EnqueueBatch(ctx, runs)
	if err != nil {
		t.Fatalf("EnqueueBatch() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 inserted, got %d", n)
	}

	// Read back from DB and verify fields.
	var (
		gotStatus    string
		gotAttempt   int
		gotPriority  int
		gotPayload   []byte
		gotTags      []byte
		gotTriggered string
	)
	err = testDB.Pool.QueryRow(ctx,
		`SELECT status, attempt, priority, payload, tags, triggered_by
		 FROM job_runs WHERE id = $1`, runs[0].ID,
	).Scan(&gotStatus, &gotAttempt, &gotPriority, &gotPayload, &gotTags, &gotTriggered)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if gotStatus != "queued" {
		t.Fatalf("status = %q, want queued", gotStatus)
	}
	if gotAttempt != 1 {
		t.Fatalf("attempt = %d, want 1", gotAttempt)
	}
	if gotPriority != 5 {
		t.Fatalf("priority = %d, want 5", gotPriority)
	}
	// Compare semantically — Postgres JSONB normalizes whitespace.
	var wantPayload, gotPayloadMap map[string]any
	if err := json.Unmarshal(payload, &wantPayload); err != nil {
		t.Fatalf("unmarshal expected payload: %v", err)
	}
	if err := json.Unmarshal(gotPayload, &gotPayloadMap); err != nil {
		t.Fatalf("unmarshal got payload: %v", err)
	}
	if wantPayload["key"] != gotPayloadMap["key"] {
		t.Fatalf("payload = %s, want %s", gotPayload, payload)
	}
	if gotTriggered != "manual" {
		t.Fatalf("triggered_by = %q, want manual", gotTriggered)
	}

	var tags map[string]string
	if err := json.Unmarshal(gotTags, &tags); err != nil {
		t.Fatalf("unmarshal tags: %v", err)
	}
	if tags["env"] != "test" || tags["region"] != "us-east-1" {
		t.Fatalf("tags = %v, want env=test region=us-east-1", tags)
	}
}

func BenchmarkEnqueueBatch_500_Integration(b *testing.B) {
	ctx := context.Background()
	q := queue.NewPostgresQueue(testDB.Pool)
	st := store.New(testDB.Pool)

	if err := testDB.CleanTables(ctx); err != nil {
		b.Fatalf("CleanTables() error = %v", err)
	}

	job := &domain.Job{
		ID:          newID(),
		ProjectID:   "project-bench-batch",
		Name:        "bench-batch-job",
		Slug:        "bench-batch-" + newID(),
		EndpointURL: "https://example.com/queue-job",
		MaxAttempts: 3,
		TimeoutSecs: 300,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		b.Fatalf("CreateJob() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		runs := make([]*domain.JobRun, 500)
		for j := range runs {
			runs[j] = &domain.JobRun{
				JobID:     job.ID,
				ProjectID: job.ProjectID,
			}
		}
		n, err := q.EnqueueBatch(ctx, runs)
		if err != nil {
			b.Fatalf("EnqueueBatch() error = %v", err)
		}
		if n != 500 {
			b.Fatalf("expected 500, got %d", n)
		}

		b.StopTimer()
		if _, cleanErr := testDB.Pool.Exec(ctx, "DELETE FROM job_runs WHERE job_id = $1", job.ID); cleanErr != nil {
			b.Fatalf("cleanup error: %v", cleanErr)
		}
		b.StartTimer()
	}
}

// --- Metadata round-trip integration tests ---.

func TestEnqueue_MetadataRoundTrip(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-metadata-roundtrip")
	meta := map[string]string{
		"_trace_parent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"foo":           "bar",
	}
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Metadata:  meta,
	}

	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued == nil {
		t.Fatal("Dequeue() returned nil")
	}
	if dequeued.Metadata == nil {
		t.Fatal("Dequeue() metadata is nil, want non-nil")
	}
	if dequeued.Metadata["_trace_parent"] != meta["_trace_parent"] {
		t.Fatalf("metadata[_trace_parent] = %q, want %q", dequeued.Metadata["_trace_parent"], meta["_trace_parent"])
	}
	if dequeued.Metadata["foo"] != "bar" {
		t.Fatalf("metadata[foo] = %q, want %q", dequeued.Metadata["foo"], "bar")
	}
}

func TestEnqueue_NilMetadata(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-nil-metadata")
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Metadata:  nil,
	}

	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued == nil {
		t.Fatal("Dequeue() returned nil")
	}
	if len(dequeued.Metadata) != 0 {
		t.Fatalf("Dequeue() metadata = %v, want empty", dequeued.Metadata)
	}
}

func TestEnqueue_EmptyMetadata(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-empty-metadata")
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Metadata:  map[string]string{},
	}

	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued == nil {
		t.Fatal("Dequeue() returned nil")
	}
	if len(dequeued.Metadata) != 0 {
		t.Fatalf("Dequeue() metadata = %v, want empty", dequeued.Metadata)
	}
}

func TestEnqueueBatch_MetadataRoundTrip(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-batch-metadata")
	runs := []*domain.JobRun{
		{
			ID:        newID(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Metadata:  map[string]string{"env": "prod", "region": "us-west-2"},
		},
		{
			ID:        newID(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Metadata:  nil,
		},
		{
			ID:        newID(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Metadata:  map[string]string{},
		},
	}

	n, err := q.EnqueueBatch(ctx, runs)
	if err != nil {
		t.Fatalf("EnqueueBatch() error = %v", err)
	}
	if n != 3 {
		t.Fatalf("EnqueueBatch() inserted = %d, want 3", n)
	}

	// Read back via store.GetRun and assert metadata for each.
	got0, err := st.GetRun(ctx, runs[0].ID)
	if err != nil {
		t.Fatalf("GetRun(runs[0]) error = %v", err)
	}
	if got0.Metadata == nil {
		t.Fatal("runs[0] metadata is nil, want non-nil")
	}
	if got0.Metadata["env"] != "prod" {
		t.Fatalf("runs[0] metadata[env] = %q, want %q", got0.Metadata["env"], "prod")
	}
	if got0.Metadata["region"] != "us-west-2" {
		t.Fatalf("runs[0] metadata[region] = %q, want %q", got0.Metadata["region"], "us-west-2")
	}

	got1, err := st.GetRun(ctx, runs[1].ID)
	if err != nil {
		t.Fatalf("GetRun(runs[1]) error = %v", err)
	}
	if len(got1.Metadata) != 0 {
		t.Fatalf("runs[1] metadata = %v, want empty", got1.Metadata)
	}

	got2, err := st.GetRun(ctx, runs[2].ID)
	if err != nil {
		t.Fatalf("GetRun(runs[2]) error = %v", err)
	}
	if len(got2.Metadata) != 0 {
		t.Fatalf("runs[2] metadata = %v, want empty", got2.Metadata)
	}
}

// --- Metadata adversarial integration tests ---.

func TestEnqueue_MetadataLargeValue(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-metadata-large")
	largeValue := strings.Repeat("a", 1024)
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Metadata:  map[string]string{"big": largeValue},
	}

	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Metadata["big"] != largeValue {
		t.Fatalf("metadata[big] len = %d, want %d (silent truncation detected)", len(got.Metadata["big"]), len(largeValue))
	}
}

func TestEnqueue_MetadataManyKeys(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-metadata-many-keys")
	meta := make(map[string]string, 60)
	for i := range 60 {
		meta[fmt.Sprintf("key_%03d", i)] = fmt.Sprintf("value_%03d", i)
	}
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Metadata:  meta,
	}

	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if len(got.Metadata) != 60 {
		t.Fatalf("metadata key count = %d, want 60", len(got.Metadata))
	}
	for k, v := range meta {
		if got.Metadata[k] != v {
			t.Fatalf("metadata[%s] = %q, want %q", k, got.Metadata[k], v)
		}
	}
}

func TestEnqueue_MetadataSpecialChars(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-metadata-special")
	meta := map[string]string{
		"unicode_key_\u00e9\u00e8\u00ea": "value with unicode \u2603\u2764",
		"quotes_key_\"double\"":          "value with 'single' and \"double\" quotes",
		"backslash_key_\\\\":             "back\\slash\\value",
		"newline_key_\n":                 "value\nwith\nnewlines",
		"tab_key_\t":                     "value\twith\ttabs",
		"empty_value":                    "",
		"json_like":                      `{"nested":"not really"}`,
	}
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Metadata:  meta,
	}

	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if len(got.Metadata) != len(meta) {
		t.Fatalf("metadata key count = %d, want %d", len(got.Metadata), len(meta))
	}
	for k, v := range meta {
		if got.Metadata[k] != v {
			t.Fatalf("metadata[%q] = %q, want %q", k, got.Metadata[k], v)
		}
	}
}

func TestEnqueue_MetadataOverwriteOnUpdate(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-metadata-overwrite")
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Metadata:  map[string]string{"original": "value1", "shared": "old"},
	}

	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Update metadata via store to merge new keys.
	if err := st.UpdateRunMetadata(ctx, run.ID, map[string]string{"added": "new", "shared": "updated"}); err != nil {
		t.Fatalf("UpdateRunMetadata() error = %v", err)
	}

	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Metadata["original"] != "value1" {
		t.Fatalf("metadata[original] = %q, want %q (original key should be preserved)", got.Metadata["original"], "value1")
	}
	if got.Metadata["added"] != "new" {
		t.Fatalf("metadata[added] = %q, want %q (new key should be added)", got.Metadata["added"], "new")
	}
	if got.Metadata["shared"] != "updated" {
		t.Fatalf("metadata[shared] = %q, want %q (shared key should be updated)", got.Metadata["shared"], "updated")
	}
}

func TestEnqueue_MetadataConcurrentDequeue(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-metadata-concurrent")

	const n = 20
	expectedMeta := make(map[string]map[string]string, n)
	for i := range n {
		id := newID()
		meta := map[string]string{
			"run_index": fmt.Sprintf("%d", i),
			"unique_id": id,
		}
		expectedMeta[id] = meta
		run := &domain.JobRun{
			ID:        id,
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Metadata:  meta,
		}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() run %d error = %v", i, err)
		}
	}

	var (
		wg   conc.WaitGroup
		mu   sync.Mutex
		runs []domain.JobRun
	)
	errCh := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			for {
				dequeued, err := q.DequeueN(ctx, 5)
				if err != nil {
					errCh <- err
					return
				}
				if len(dequeued) == 0 {
					return
				}
				mu.Lock()
				runs = append(runs, dequeued...)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("DequeueN() error = %v", err)
		}
	}

	if len(runs) != n {
		t.Fatalf("total dequeued = %d, want %d", len(runs), n)
	}

	for _, run := range runs {
		expected, ok := expectedMeta[run.ID]
		if !ok {
			t.Fatalf("unexpected run ID %s in dequeued set", run.ID)
		}
		if run.Metadata == nil {
			t.Fatalf("run %s metadata is nil", run.ID)
		}
		if run.Metadata["unique_id"] != expected["unique_id"] {
			t.Fatalf("run %s metadata[unique_id] = %q, want %q (cross-contamination)", run.ID, run.Metadata["unique_id"], expected["unique_id"])
		}
		if run.Metadata["run_index"] != expected["run_index"] {
			t.Fatalf("run %s metadata[run_index] = %q, want %q (cross-contamination)", run.ID, run.Metadata["run_index"], expected["run_index"])
		}
	}
}

func BenchmarkPostgresQueueDequeueN(b *testing.B) {
	ctx := context.Background()
	q := queue.NewPostgresQueue(testDB.Pool)
	st := store.New(testDB.Pool)

	if err := testDB.CleanTables(ctx); err != nil {
		b.Fatalf("CleanTables() error = %v", err)
	}

	job := &domain.Job{
		ID:          newID(),
		ProjectID:   "project-benchmark-dequeue",
		Name:        "bench-job",
		Slug:        "bench-job-" + newID(),
		EndpointURL: "https://example.com/queue-job",
		MaxAttempts: 3,
		TimeoutSecs: 300,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		b.Fatalf("CreateJob() error = %v", err)
	}

	const preload = 512
	for i := range preload {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: i % 10}
		if err := q.Enqueue(ctx, run); err != nil {
			b.Fatalf("Enqueue() error = %v", err)
		}
	}

	b.ResetTimer()
	for i := range b.N {
		runs, err := q.DequeueN(ctx, 32)
		if err != nil {
			b.Fatalf("DequeueN() error = %v", err)
		}
		if len(runs) == 0 {
			run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: i % 10}
			if err := q.Enqueue(ctx, run); err != nil {
				b.Fatalf("Enqueue() error = %v", err)
			}
		}
	}
}
