package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestPostgresRunWriterEnqueueInTxAcquiresIdempotencyLock(t *testing.T) {
	t.Parallel()

	var advisoryLocked bool
	tx := &readyMockTx{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			require.Contains(t, sql, "pg_advisory_xact_lock")
			require.Equal(t, []any{"job-a", "idem-a"}, args)
			advisoryLocked = true
			return pgconn.CommandTag{}, nil
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			require.True(t, advisoryLocked)
			require.Contains(t, sql, "INSERT INTO job_runs")
			return &mockRow{scanFn: func(dest ...any) error {
				return assignTimeDest(dest[0], time.Now())
			}}
		},
	}
	q := NewPostgresRunWriter(tx)

	err := q.EnqueueInTx(context.Background(), tx, &domain.JobRun{
		ID:             "run-a",
		JobID:          "job-a",
		ProjectID:      "project-a",
		IdempotencyKey: "idem-a",
	})

	require.NoError(t, err)
	require.True(t, advisoryLocked)
}

func TestPostgresRunWriterManagedTxErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("begin error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("begin failed")
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return nil, wantErr
			},
		}
		q := NewPostgresRunWriter(db)

		err := q.Enqueue(ctx, &domain.JobRun{
			ID:             "run-a",
			JobID:          "job-a",
			ProjectID:      "project-a",
			IdempotencyKey: "idem-a",
		})

		require.ErrorContains(t, err, "enqueue run: begin tx")
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("advisory lock error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("advisory failed")
		tx := &readyMockTx{
			execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "pg_advisory_xact_lock")
				return pgconn.CommandTag{}, wantErr
			},
		}
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		q := NewPostgresRunWriter(db)

		err := q.Enqueue(ctx, &domain.JobRun{
			ID:             "run-a",
			JobID:          "job-a",
			ProjectID:      "project-a",
			IdempotencyKey: "idem-a",
		})

		require.ErrorContains(t, err, "enqueue run: advisory lock")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, tx.commitCalls)
		require.Equal(t, 1, tx.rollbackCalls)
	})

	t.Run("backpressure throttle passes through", func(t *testing.T) {
		t.Parallel()

		tx := &readyMockTx{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				require.Contains(t, sql, "project_rate_limits")
				return &mockRow{scanFn: func(...any) error {
					return pgx.ErrNoRows
				}}
			},
		}
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		bp := NewBackpressure(db, BackpressureConfig{
			DefaultMaxTokens:    1,
			DefaultRefillPerSec: 1,
			LocalLeaseSize:      1,
		}, true)
		q := NewPostgresRunWriter(db, WithBackpressureController(bp))

		err := q.Enqueue(ctx, &domain.JobRun{
			ID:        "run-a",
			JobID:     "job-a",
			ProjectID: "project-a",
		})

		require.ErrorIs(t, err, ErrEnqueueThrottled)
		require.NotContains(t, err.Error(), "backpressure")
		require.Zero(t, tx.commitCalls)
		require.Equal(t, 1, tx.rollbackCalls)
	})

	t.Run("backpressure database error is wrapped", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("rate limit store unavailable")
		tx := &readyMockTx{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				require.Contains(t, sql, "project_rate_limits")
				return &mockRow{scanFn: func(...any) error {
					return wantErr
				}}
			},
		}
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		bp := NewBackpressure(db, BackpressureConfig{
			DefaultMaxTokens:    1,
			DefaultRefillPerSec: 1,
			LocalLeaseSize:      1,
		}, true)
		q := NewPostgresRunWriter(db, WithBackpressureController(bp))

		err := q.Enqueue(ctx, &domain.JobRun{
			ID:        "run-a",
			JobID:     "job-a",
			ProjectID: "project-a",
		})

		require.ErrorContains(t, err, "enqueue run: backpressure")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, tx.commitCalls)
		require.Equal(t, 1, tx.rollbackCalls)
	})

	t.Run("insert error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("insert failed")
		tx := &readyMockTx{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO job_runs")
				return &mockRow{scanFn: func(...any) error {
					return wantErr
				}}
			},
		}
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		q := NewPostgresRunWriter(db)

		err := q.Enqueue(ctx, &domain.JobRun{
			ID:             "run-a",
			JobID:          "job-a",
			ProjectID:      "project-a",
			IdempotencyKey: "idem-a",
		})

		require.ErrorContains(t, err, "enqueue run")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, tx.commitCalls)
		require.Equal(t, 1, tx.rollbackCalls)
	})

	t.Run("commit error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("commit failed")
		tx := &readyMockTx{
			commitErr: wantErr,
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO job_runs")
				return &mockRow{scanFn: func(dest ...any) error {
					return assignTimeDest(dest[0], time.Now())
				}}
			},
		}
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		q := NewPostgresRunWriter(db)

		err := q.Enqueue(ctx, &domain.JobRun{
			ID:             "run-a",
			JobID:          "job-a",
			ProjectID:      "project-a",
			IdempotencyKey: "idem-a",
		})

		require.ErrorContains(t, err, "enqueue run: commit")
		require.ErrorIs(t, err, wantErr)
		require.Equal(t, 1, tx.commitCalls)
		require.Equal(t, 1, tx.rollbackCalls)
	})
}

func TestClassifyTerminalEnqueueInsertErrorClassPrefixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantReason string
	}{
		{
			name:       "data exception class",
			err:        &pgconn.PgError{Code: "22001"},
			wantReason: "data_exception",
		},
		{
			name:       "integrity constraint class",
			err:        &pgconn.PgError{Code: "23505"},
			wantReason: "integrity_constraint_violation",
		},
		{
			name:       "one character code",
			err:        &pgconn.PgError{Code: "2"},
			wantReason: "",
		},
		{
			name:       "non postgres error",
			err:        errors.New("plain error"),
			wantReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classifyTerminalEnqueueInsertError(tt.err)
			if tt.wantReason == "" {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, tt.wantReason, got.reason)
		})
	}
}

func TestEnqueueBatchConsumesBackpressureAndSerializesMetadata(t *testing.T) {
	t.Parallel()

	var batchConsumed bool
	var capturedMetadata string
	db := &mockBatchDB{
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "project_rate_limits")
			require.Equal(t, []any{"project-a", 10, 10, 2}, args)
			batchConsumed = true
			return &mockRow{scanFn: func(dest ...any) error {
				require.NoError(t, assignIntDest(dest[0], 8))
				require.NoError(t, assignIntDest(dest[1], 10))
				require.NoError(t, assignIntDest(dest[2], 10))
				return nil
			}}
		},
		copyFromFn: func(_ context.Context, _ pgx.Identifier, _ []string, rowSrc pgx.CopyFromSource) (int64, error) {
			var count int64
			for rowSrc.Next() {
				values, err := rowSrc.Values()
				require.NoError(t, err)
				metadata, ok := values[34].([]byte)
				require.True(t, ok)
				capturedMetadata = string(metadata)
				count++
			}
			return count, rowSrc.Err()
		},
	}
	bp := NewBackpressure(db, BackpressureConfig{
		DefaultMaxTokens:    10,
		DefaultRefillPerSec: 10,
		LocalLeaseSize:      1,
	}, true)
	q := NewPostgresRunWriter(db, WithBackpressureController(bp))

	inserted, err := q.EnqueueBatch(context.Background(), []*domain.JobRun{{
		ID:        "run-a",
		JobID:     "job-a",
		ProjectID: "project-a",
		Metadata:  map[string]string{"source": "api"},
	}, {
		ID:        "run-b",
		JobID:     "job-a",
		ProjectID: "project-a",
		Metadata:  map[string]string{"source": "bulk"},
	}})

	require.NoError(t, err)
	require.EqualValues(t, 2, inserted)
	require.True(t, batchConsumed)
	require.Contains(t, capturedMetadata, "source")
}

func TestRecordClaimMetricsHandlesRetryLagBoundaries(t *testing.T) {
	t.Parallel()

	q := NewPostgresRunWriter(&mockDBTX{})
	pastRetry := time.Now().Add(-time.Second)
	futureRetry := time.Now().Add(time.Hour)

	require.NotPanics(t, func() {
		q.recordClaimMetrics(context.Background(), &domain.JobRun{
			ID:          "run-past",
			CreatedAt:   time.Now().Add(-time.Minute),
			NextRetryAt: &pastRetry,
		})
		q.recordClaimMetrics(context.Background(), &domain.JobRun{
			ID:          "run-future",
			CreatedAt:   time.Now().Add(time.Hour),
			NextRetryAt: &futureRetry,
		})
		q.recordClaimMetrics(context.Background(), nil)
	})
}

func TestPostgresRunWriterAcquireIdempotencyLockWrapsExecError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("exec failed")
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			require.Contains(t, sql, "pg_advisory_xact_lock")
			require.Equal(t, []any{"job-a", "idem-a"}, args)
			return pgconn.CommandTag{}, wantErr
		},
	}
	q := NewPostgresRunWriter(db)

	err := q.acquireIdempotencyXactLock(context.Background(), db, "job-a", "idem-a", "enqueue test")

	require.ErrorContains(t, err, "enqueue test: advisory lock")
	require.ErrorIs(t, err, wantErr)
}
