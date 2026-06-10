package api

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestEnqueueTriggerRunUsesDirectQueueWithoutTx(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-direct"}
	queue := &triggerEnqueueQueue{
		enqueueFunc: func(_ context.Context, got *domain.JobRun) error {
			require.Equal(t, run, got)

			return nil
		},
		enqueueInTxFunc: func(context.Context, store.DBTX, *domain.JobRun) error {
			require.Fail(t,

				"EnqueueInTx must not run without a transaction")
			return nil
		},
	}
	srv := &Server{queue: queue}
	require.NoError(t, srv.enqueueTriggerRun(context.Background(), nil, run))
	require.Equal(t, 1, queue.
		enqueueCalls)
}

func TestEnqueueTriggerRunUsesTransactionalQueueWithTx(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-tx"}
	tx := triggerEnqueueDBTX{}
	queue := &triggerEnqueueQueue{
		enqueueFunc: func(context.Context, *domain.JobRun) error {
			require.Fail(t,

				"Enqueue must not run with a transaction")
			return nil
		},
		enqueueInTxFunc: func(_ context.Context, gotTx store.DBTX, got *domain.JobRun) error {
			require.Equal(t, tx, gotTx)
			require.Equal(t, run, got)

			return nil
		},
	}
	srv := &Server{queue: queue}
	require.NoError(t, srv.enqueueTriggerRun(context.Background(), tx, run))
	require.Equal(t, 1, queue.
		enqueueInTxCalls)
}

func TestEnqueueTriggerRunUsesAmbientRLSTransaction(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-ambient-tx"}
	tx := triggerEnqueueDBTX{}
	queue := &triggerEnqueueQueue{
		enqueueFunc: func(context.Context, *domain.JobRun) error {
			require.Fail(t,

				"Enqueue must not run when an ambient RLS transaction is present")
			return nil
		},
		enqueueInTxFunc: func(_ context.Context, gotTx store.DBTX, got *domain.JobRun) error {
			require.Equal(t, tx, gotTx)
			require.Equal(t, run, got)

			return nil
		},
	}
	srv := &Server{queue: queue}
	ctx := store.ContextWithTx(context.Background(), tx)
	require.NoError(t, srv.enqueueTriggerRun(ctx, nil, run))
	require.Equal(t, 1, queue.
		enqueueInTxCalls)
}

func TestEnqueueTriggerRunExplicitTxOverridesAmbientRLSTransaction(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-explicit-tx"}
	explicitTx := triggerEnqueueDBTX{name: "explicit"}
	ambientTx := triggerEnqueueDBTX{name: "ambient"}
	queue := &triggerEnqueueQueue{
		enqueueFunc: func(context.Context, *domain.JobRun) error {
			require.Fail(t,

				"Enqueue must not run with a transaction")
			return nil
		},
		enqueueInTxFunc: func(_ context.Context, gotTx store.DBTX, got *domain.JobRun) error {
			require.Equal(t, explicitTx, gotTx)
			require.Equal(t, run, got)

			return nil
		},
	}
	srv := &Server{queue: queue}
	ctx := store.ContextWithTx(context.Background(), ambientTx)
	require.NoError(t, srv.enqueueTriggerRun(ctx, explicitTx, run))
	require.Equal(t, 1, queue.
		enqueueInTxCalls)
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

type triggerEnqueueDBTX struct {
	pgx.Tx
	name string
}

func (triggerEnqueueDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (triggerEnqueueDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (triggerEnqueueDBTX) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}
