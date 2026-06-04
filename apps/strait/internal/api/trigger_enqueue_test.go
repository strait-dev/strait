package api

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestEnqueueTriggerRunUsesDirectQueueWithoutTx(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-direct"}
	queue := &triggerEnqueueQueue{
		enqueueFunc: func(_ context.Context, got *domain.JobRun) error {
			if got != run {
				t.Fatalf("run = %p, want %p", got, run)
			}
			return nil
		},
		enqueueInTxFunc: func(context.Context, store.DBTX, *domain.JobRun) error {
			t.Fatal("EnqueueInTx must not run without a transaction")
			return nil
		},
	}
	srv := &Server{queue: queue}

	if err := srv.enqueueTriggerRun(context.Background(), nil, run); err != nil {
		t.Fatalf("enqueueTriggerRun() error = %v", err)
	}
	if queue.enqueueCalls != 1 {
		t.Fatalf("enqueueCalls = %d, want 1", queue.enqueueCalls)
	}
}

func TestEnqueueTriggerRunUsesTransactionalQueueWithTx(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-tx"}
	tx := triggerEnqueueDBTX{}
	queue := &triggerEnqueueQueue{
		enqueueFunc: func(context.Context, *domain.JobRun) error {
			t.Fatal("Enqueue must not run with a transaction")
			return nil
		},
		enqueueInTxFunc: func(_ context.Context, gotTx store.DBTX, got *domain.JobRun) error {
			if gotTx != tx {
				t.Fatalf("tx = %T, want triggerEnqueueDBTX", gotTx)
			}
			if got != run {
				t.Fatalf("run = %p, want %p", got, run)
			}
			return nil
		},
	}
	srv := &Server{queue: queue}

	if err := srv.enqueueTriggerRun(context.Background(), tx, run); err != nil {
		t.Fatalf("enqueueTriggerRun() error = %v", err)
	}
	if queue.enqueueInTxCalls != 1 {
		t.Fatalf("enqueueInTxCalls = %d, want 1", queue.enqueueInTxCalls)
	}
}

type triggerEnqueueQueue struct {
	enqueueCalls     int
	enqueueInTxCalls int
	enqueueFunc      func(context.Context, *domain.JobRun) error
	enqueueInTxFunc  func(context.Context, store.DBTX, *domain.JobRun) error
}

func (q *triggerEnqueueQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	q.enqueueCalls++
	if q.enqueueFunc != nil {
		return q.enqueueFunc(ctx, run)
	}
	return nil
}

func (q *triggerEnqueueQueue) EnqueueInTx(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
	q.enqueueInTxCalls++
	if q.enqueueInTxFunc != nil {
		return q.enqueueInTxFunc(ctx, tx, run)
	}
	return nil
}

func (q *triggerEnqueueQueue) EnqueueBatch(context.Context, []*domain.JobRun) (int64, error) {
	return 0, nil
}

func (q *triggerEnqueueQueue) Dequeue(context.Context) (*domain.JobRun, error) {
	return nil, nil
}

func (q *triggerEnqueueQueue) DequeueN(context.Context, int) ([]domain.JobRun, error) {
	return nil, nil
}

func (q *triggerEnqueueQueue) DequeueNFair(context.Context, int) ([]domain.JobRun, error) {
	return nil, nil
}

func (q *triggerEnqueueQueue) DequeueNByProject(context.Context, int, string) ([]domain.JobRun, error) {
	return nil, nil
}

type triggerEnqueueDBTX struct{}

func (triggerEnqueueDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (triggerEnqueueDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (triggerEnqueueDBTX) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}
