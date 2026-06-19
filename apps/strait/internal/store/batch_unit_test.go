package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func fillBatchBufferItemDest(dest []any, id string, createdBy *string, createdAt time.Time) {
	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "job-1"
	*(dest[2].(*string)) = "project-1"
	*(dest[3].(*string)) = "batch-key"
	*(dest[4].(*json.RawMessage)) = json.RawMessage(`{"payload":true}`)
	*(dest[5].(*json.RawMessage)) = json.RawMessage(`{"tags":["a"]}`)
	*(dest[6].(*int)) = 7
	*(dest[7].(*string)) = "api"
	*(dest[8].(**string)) = createdBy
	*(dest[9].(*time.Time)) = createdAt
}

func TestBatchBufferStore(t *testing.T) {
	t.Parallel()

	t.Run("inserts batch buffer item with generated id and nil created_by", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		var capturedArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO batch_buffer")
				capturedArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = createdAt
					return nil
				}}
			},
		}
		item := &domain.BatchBufferItem{
			JobID:       "job-1",
			ProjectID:   "project-1",
			BatchKey:    "batch-key",
			Payload:     json.RawMessage(`{"payload":true}`),
			Tags:        json.RawMessage(`{"tags":["a"]}`),
			Priority:    7,
			TriggeredBy: "api",
		}

		require.NoError(t, New(db).InsertBatchBufferItem(context.Background(), item))
		require.NotEmpty(t, item.ID)
		require.Equal(t, createdAt, item.CreatedAt)
		require.Equal(t, item.ID, capturedArgs[0])
		require.Nil(t, capturedArgs[8])
	})

	t.Run("inserts batch buffer item with custom id and created_by", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
				require.Equal(t, "item-1", args[0])
				require.NotNil(t, args[8])
				require.Equal(t, "user-1", *args[8].(*string))
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = time.Now().UTC()
					return nil
				}}
			},
		}
		item := &domain.BatchBufferItem{ID: "item-1", CreatedBy: "user-1"}

		require.NoError(t, New(db).InsertBatchBufferItem(context.Background(), item))
		require.Equal(t, "item-1", item.ID)
	})

	t.Run("insert batch buffer wraps scan errors", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("insert failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return scanErr }}
			},
		}

		err := New(db).InsertBatchBufferItem(context.Background(), &domain.BatchBufferItem{})
		require.ErrorContains(t, err, "insert batch buffer item")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("counts batch buffer items", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "COUNT(*)")
				require.Equal(t, []any{"job-1", "batch-key"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 3
					return nil
				}}
			},
		}

		got, err := New(db).CountBatchBufferItems(context.Background(), "job-1", "batch-key")
		require.NoError(t, err)
		require.Equal(t, 3, got)

		scanErr := errors.New("count failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return scanErr }}
		}
		_, err = New(db).CountBatchBufferItems(context.Background(), "job-1", "batch-key")
		require.ErrorContains(t, err, "count batch buffer items")
		require.ErrorIs(t, err, scanErr)
	})
}

func TestBatchBufferDrainAndList(t *testing.T) {
	t.Parallel()

	t.Run("drains batch buffer items and restores created_by", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		createdBy := "user-1"
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "DELETE FROM batch_buffer")
				require.Equal(t, []any{"job-1", "batch-key", 10}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillBatchBufferItemDest(dest, "item-1", &createdBy, createdAt)
						return nil
					},
					func(dest ...any) error {
						fillBatchBufferItemDest(dest, "item-2", nil, createdAt.Add(time.Second))
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).DrainBatchBuffer(context.Background(), "job-1", "batch-key", 10)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, "user-1", got[0].CreatedBy)
		require.Empty(t, got[1].CreatedBy)
		require.JSONEq(t, `{"payload":true}`, string(got[0].Payload))

		txItems, err := New(&mockDBTX{}).DrainBatchBufferInTx(context.Background(), db, "job-1", "batch-key", 10)
		require.NoError(t, err)
		require.Len(t, txItems, 2)
	})

	t.Run("lists batch buffer items", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		createdBy := "user-1"
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "SELECT id, job_id")
				require.NotContains(t, sql, "DELETE FROM")
				require.Equal(t, []any{"job-1", "batch-key", 5}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillBatchBufferItemDest(dest, "item-1", &createdBy, createdAt)
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListBatchBufferItems(context.Background(), "job-1", "batch-key", 5)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "user-1", got[0].CreatedBy)
	})

	t.Run("drain batch buffer maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		assertBatchRowsErrors(t, "drain", func(q *Queries, ctx context.Context) ([]domain.BatchBufferItem, error) {
			return q.DrainBatchBuffer(ctx, "job-1", "batch-key", 10)
		})
	})

	t.Run("list batch buffer maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		assertBatchRowsErrors(t, "list", func(q *Queries, ctx context.Context) ([]domain.BatchBufferItem, error) {
			return q.ListBatchBufferItems(ctx, "job-1", "batch-key", 10)
		})
	})
}

func assertBatchRowsErrors(
	t *testing.T,
	mode string,
	call func(q *Queries, ctx context.Context) ([]domain.BatchBufferItem, error),
) {
	t.Helper()

	tests := []struct {
		name       string
		queryErr   error
		scanErr    error
		rowErr     error
		wantString string
	}{
		{name: "query", queryErr: errors.New("query failed"), wantString: mode + " batch buffer"},
		{name: "scan", scanErr: errors.New("scan failed"), wantString: mode + " batch buffer"},
		{name: "rows", rowErr: errors.New("rows failed"), wantString: "rows failed"},
	}
	for _, tc := range tests {
		t.Run(mode+" "+tc.name, func(t *testing.T) {
			t.Parallel()

			db := &mockDBTX{
				queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					if tc.queryErr != nil {
						return nil, tc.queryErr
					}
					rows := &mockRows{err: tc.rowErr}
					if tc.scanErr != nil {
						rows.scanFns = []func(dest ...any) error{func(...any) error { return tc.scanErr }}
					}
					return rows, nil
				},
			}

			_, err := call(New(db), context.Background())
			require.ErrorContains(t, err, tc.wantString)
		})
	}
}

func TestBatchBufferDeleteAndFlushable(t *testing.T) {
	t.Parallel()

	t.Run("deletes batch buffer items", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, New(&mockDBTX{}).DeleteBatchBufferItems(context.Background(), nil))

		var capturedArgs []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "DELETE FROM batch_buffer")
				capturedArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("DELETE 2"), nil
			},
		}
		require.NoError(t, New(db).DeleteBatchBufferItems(context.Background(), []string{"item-1", "item-2"}))
		require.Equal(t, []any{[]string{"item-1", "item-2"}}, capturedArgs)

		execErr := errors.New("delete failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		err := New(db).DeleteBatchBufferItems(context.Background(), []string{"item-1"})
		require.ErrorContains(t, err, "delete batch buffer items")
		require.ErrorIs(t, err, execErr)
	})

	t.Run("lists flushable batches", func(t *testing.T) {
		t.Parallel()

		oldestAt := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "batch_max_size")
				require.Empty(t, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "job-1"
						*(dest[1].(*string)) = "project-1"
						*(dest[2].(*string)) = "batch-key"
						*(dest[3].(*int)) = 3
						*(dest[4].(*time.Time)) = oldestAt
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListFlushableBatches(context.Background())
		require.NoError(t, err)
		require.Equal(t, []FlushableBatch{
			{JobID: "job-1", ProjectID: "project-1", BatchKey: "batch-key", ItemCount: 3, OldestAt: oldestAt},
		}, got)
	})

	t.Run("list flushable batches maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list flushable batches"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "list flushable batches scan"},
			{name: "rows", rowErr: errors.New("rows failed"), wantString: "rows failed"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tc.queryErr != nil {
							return nil, tc.queryErr
						}
						rows := &mockRows{err: tc.rowErr}
						if tc.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error { return tc.scanErr }}
						}
						return rows, nil
					},
				}

				_, err := New(db).ListFlushableBatches(context.Background())
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})
}
