//go:build integration

package queue_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	remaining, err := st.ListRunsByProject(ctx, job.ProjectID, &status, nil, nil, nil, nil, nil, 20, nil)
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
		wg    sync.WaitGroup
		mu    sync.Mutex
		seen  = make(map[string]int)
		total int
	)

	errCh := make(chan error, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()

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
		}()
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
		wg   sync.WaitGroup
		mu   sync.Mutex
		seen = make(map[string]int)
	)

	errCh := make(chan error, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()

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
		}()
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

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
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
	for i := 0; i < preload; i++ {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: i % 10}
		if err := q.Enqueue(ctx, run); err != nil {
			b.Fatalf("Enqueue() error = %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
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
