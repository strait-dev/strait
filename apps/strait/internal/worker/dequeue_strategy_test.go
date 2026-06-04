package worker

import (
	"context"
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
	return nil, nil
}
func (m *mockStrategyQueue) DequeueNByProject(_ context.Context, _ int, _ string) ([]domain.JobRun, error) {
	m.dequeueNByProject.Add(1)
	return nil, nil
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
