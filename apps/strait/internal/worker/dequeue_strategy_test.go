package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
)

// mockStrategyQueue tracks which dequeue method was called.
type mockStrategyQueue struct {
	dequeueNCalled    atomic.Int32
	dequeueNByProject atomic.Int32
	workerQueueCalled atomic.Int32

	dequeueNRuns []domain.JobRun
	workerRuns   []domain.JobRun
	workerErr    error

	workerQueueN      atomic.Int32
	workerQueueInputs []domain.WorkerQueueRef
}

var _ queue.Queue = (*mockStrategyQueue)(nil)

func (m *mockStrategyQueue) Enqueue(context.Context, *domain.JobRun) error { return nil }
func (m *mockStrategyQueue) EnqueueInTx(context.Context, store.DBTX, *domain.JobRun) error {
	return nil
}
func (m *mockStrategyQueue) EnqueueBatch(context.Context, []*domain.JobRun) (int64, error) {
	return 0, nil
}
func (m *mockStrategyQueue) Dequeue(context.Context) (*domain.JobRun, error) { return nil, nil }
func (m *mockStrategyQueue) DequeueN(_ context.Context, _ int) ([]domain.JobRun, error) {
	m.dequeueNCalled.Add(1)
	return m.dequeueNRuns, nil
}
func (m *mockStrategyQueue) DequeueNByProject(_ context.Context, _ int, _ string) ([]domain.JobRun, error) {
	m.dequeueNByProject.Add(1)
	return nil, nil
}
func (m *mockStrategyQueue) DequeueNForWorkerQueues(_ context.Context, n int, queues []domain.WorkerQueueRef) ([]domain.JobRun, error) {
	m.workerQueueCalled.Add(1)
	m.workerQueueN.Store(int32(n))
	m.workerQueueInputs = queues
	if m.workerErr != nil {
		return nil, m.workerErr
	}
	return m.workerRuns, nil
}

type staticStrategySnapshotter struct {
	queues []domain.WorkerQueueRef
}

func (s staticStrategySnapshotter) SnapshotWorkerQueues() []domain.WorkerQueueRef {
	return s.queues
}

func TestPoll_DequeueUsesQueueDequeueN(t *testing.T) {
	t.Parallel()

	q := &mockStrategyQueue{}
	p := NewPool(4)
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:         p,
		Queue:        q,
		Store:        &mockExecutorStore{},
		PollInterval: time.Hour,
	})

	exec.poll(context.Background())

	if q.dequeueNCalled.Load() != 1 {
		t.Fatalf("DequeueN called %d times, want 1", q.dequeueNCalled.Load())
	}
}

// TestPoll_PartitionsOverrideAutoSelect verifies that partition-based
// dequeue takes precedence over the auto-select path.
func TestPoll_PartitionsOverrideAutoSelect(t *testing.T) {
	t.Parallel()

	q := &mockStrategyQueue{}
	p := NewPool(4)
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:         p,
		Queue:        q,
		Store:        &mockExecutorStore{},
		PollInterval: time.Hour,
		Partitions:   []string{"proj-1"},
	})

	exec.poll(context.Background())

	if q.dequeueNByProject.Load() != 1 {
		t.Fatalf("DequeueNByProject called %d times, want 1", q.dequeueNByProject.Load())
	}
	if q.dequeueNCalled.Load() != 0 {
		t.Fatalf("DequeueN called %d times, want 0 (partitions override)", q.dequeueNCalled.Load())
	}
}

func TestDequeueRuns_AppendsWorkerRunsWithRemainingCapacity(t *testing.T) {
	t.Parallel()

	workerQueues := []domain.WorkerQueueRef{{ProjectID: "proj-1", QueueName: "default"}}
	q := &mockStrategyQueue{
		dequeueNRuns: []domain.JobRun{{ID: "http-1"}},
		workerRuns:   []domain.JobRun{{ID: "worker-1"}, {ID: "worker-2"}},
	}
	exec := &Executor{
		queue:            q,
		queueSnapshotter: staticStrategySnapshotter{queues: workerQueues},
		logger:           slog.Default(),
	}

	runs, err := exec.dequeueRuns(context.Background(), 3)
	if err != nil {
		t.Fatalf("dequeueRuns() error = %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("runs len = %d, want 3", len(runs))
	}
	if runs[0].ID != "http-1" || runs[1].ID != "worker-1" || runs[2].ID != "worker-2" {
		t.Fatalf("runs = %+v, want HTTP claim followed by worker claims", runs)
	}
	if got := q.workerQueueN.Load(); got != 2 {
		t.Fatalf("worker dequeue n = %d, want remaining capacity 2", got)
	}
	if len(q.workerQueueInputs) != 1 || q.workerQueueInputs[0] != workerQueues[0] {
		t.Fatalf("worker queue inputs = %+v, want %+v", q.workerQueueInputs, workerQueues)
	}
}

func TestDequeueRuns_SkipsWorkerPassWhenHTTPClaimsFillCapacity(t *testing.T) {
	t.Parallel()

	q := &mockStrategyQueue{
		dequeueNRuns: []domain.JobRun{{ID: "http-1"}, {ID: "http-2"}},
		workerRuns:   []domain.JobRun{{ID: "worker-1"}},
	}
	exec := &Executor{
		queue:  q,
		logger: slog.Default(),
		queueSnapshotter: staticStrategySnapshotter{
			queues: []domain.WorkerQueueRef{{ProjectID: "proj-1", QueueName: "default"}},
		},
	}

	runs, err := exec.dequeueRuns(context.Background(), 2)
	if err != nil {
		t.Fatalf("dequeueRuns() error = %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs len = %d, want 2", len(runs))
	}
	if got := q.workerQueueCalled.Load(); got != 0 {
		t.Fatalf("worker dequeue calls = %d, want 0", got)
	}
}

func TestDequeueRuns_WorkerFailureKeepsHTTPClaims(t *testing.T) {
	t.Parallel()

	q := &mockStrategyQueue{
		dequeueNRuns: []domain.JobRun{{ID: "http-1"}},
		workerErr:    errors.New("worker queue unavailable"),
	}
	exec := &Executor{
		queue:  q,
		logger: slog.Default(),
		queueSnapshotter: staticStrategySnapshotter{
			queues: []domain.WorkerQueueRef{{ProjectID: "proj-1", QueueName: "default"}},
		},
	}

	runs, err := exec.dequeueRuns(context.Background(), 2)
	if err != nil {
		t.Fatalf("dequeueRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "http-1" {
		t.Fatalf("runs = %+v, want preserved HTTP claim", runs)
	}
	if got := q.workerQueueCalled.Load(); got != 1 {
		t.Fatalf("worker dequeue calls = %d, want 1", got)
	}
}
