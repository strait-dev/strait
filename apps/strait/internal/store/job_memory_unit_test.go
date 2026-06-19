package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func fillJobMemoryDest(dest []any, id, key string, size int, now time.Time) {
	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "job-1"
	*(dest[2].(*string)) = "project-1"
	*(dest[3].(*string)) = key
	*(dest[4].(*json.RawMessage)) = json.RawMessage(`{"ok":true}`)
	*(dest[5].(*int)) = size
	*(dest[6].(**time.Time)) = nil
	*(dest[7].(*time.Time)) = now
	*(dest[8].(*time.Time)) = now.Add(time.Second)
}

type jobMemoryTxBeginner struct {
	mockDBTX
	tx       *jobMemoryTx
	beginErr error
}

func (b *jobMemoryTxBeginner) Begin(context.Context) (pgx.Tx, error) {
	if b.beginErr != nil {
		return nil, b.beginErr
	}
	return b.tx, nil
}

type jobMemoryTx struct {
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
	committed  bool
	rolledBack bool
}

func (tx *jobMemoryTx) Begin(context.Context) (pgx.Tx, error) { return tx, nil }

func (tx *jobMemoryTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *jobMemoryTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

func (tx *jobMemoryTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (tx *jobMemoryTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }

func (tx *jobMemoryTx) LargeObjects() pgx.LargeObjects { return pgx.LargeObjects{} }

func (tx *jobMemoryTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (tx *jobMemoryTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if tx.execFn != nil {
		return tx.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (tx *jobMemoryTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if tx.queryFn != nil {
		return tx.queryFn(ctx, sql, args...)
	}
	return nil, nil
}

func (tx *jobMemoryTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx.queryRowFn != nil {
		return tx.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{}
}

func (tx *jobMemoryTx) Conn() *pgx.Conn { return nil }

func TestUpsertJobMemoryUnit(t *testing.T) {
	t.Parallel()

	t.Run("scans returned identity and timestamps", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
			require.Contains(t, sql, "INSERT INTO job_memory")
			require.Contains(t, sql, "ON CONFLICT (job_id, memory_key) DO NOTHING")
			args = append([]any(nil), gotArgs...)
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*string)) = "mem-1"
				*(dest[1].(*time.Time)) = now
				*(dest[2].(*time.Time)) = now.Add(time.Second)
				return nil
			}}
		}}
		expiresAt := now.Add(time.Hour)
		mem := &domain.JobMemory{
			JobID:        "job-1",
			ProjectID:    "project-1",
			MemoryKey:    "state",
			Value:        json.RawMessage(`{"count":1}`),
			SizeBytes:    11,
			TTLExpiresAt: &expiresAt,
		}

		require.NoError(t, New(db).UpsertJobMemory(context.Background(), mem))
		require.Equal(t, "mem-1", mem.ID)
		require.Equal(t, now, mem.CreatedAt)
		require.Equal(t, now.Add(time.Second), mem.UpdatedAt)
		require.Equal(t, []any{"job-1", "project-1", "state", json.RawMessage(`{"count":1}`), 11, &expiresAt}, args)
	})

	t.Run("wraps scan errors", func(t *testing.T) {
		t.Parallel()

		writeErr := errors.New("write failed")
		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return writeErr }}
		}}

		err := New(db).UpsertJobMemory(context.Background(), &domain.JobMemory{})
		require.ErrorIs(t, err, writeErr)
		require.ErrorContains(t, err, "upsert job memory")
	})
}

func TestUpsertJobMemoryWithQuotaEarlyFailuresUnit(t *testing.T) {
	t.Parallel()

	t.Run("rejects per key quota before opening a transaction", func(t *testing.T) {
		t.Parallel()

		called := false
		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			called = true
			return &mockRow{}
		}}
		err := New(db).UpsertJobMemoryWithQuota(context.Background(), &domain.JobMemory{SizeBytes: 12}, 11, 100)
		require.ErrorIs(t, err, ErrJobMemoryPerKeyLimitExceeded)
		require.False(t, called)
	})

	t.Run("requires transactional database when quotas need current totals", func(t *testing.T) {
		t.Parallel()

		err := New(&mockDBTX{}).UpsertJobMemoryWithQuota(context.Background(), &domain.JobMemory{SizeBytes: 10}, 10, 100)
		require.ErrorContains(t, err, "db does not support transactions")
	})

	t.Run("active run variant keeps the same early guards", func(t *testing.T) {
		t.Parallel()

		q := New(&mockDBTX{})
		err := q.UpsertJobMemoryWithQuotaForActiveRun(context.Background(), "run-1", &domain.JobMemory{SizeBytes: 6}, 5, 100, 1)
		require.ErrorIs(t, err, ErrJobMemoryPerKeyLimitExceeded)

		err = q.UpsertJobMemoryWithQuotaForActiveRun(context.Background(), "run-1", &domain.JobMemory{SizeBytes: 5}, 5, 100, 1)
		require.ErrorContains(t, err, "db does not support transactions")
	})
}

func TestUpsertJobMemoryWithQuotaTransactionUnit(t *testing.T) {
	t.Parallel()

	t.Run("inserts when total stays within per job quota", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		tx := &jobMemoryTx{}
		tx.execFn = func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			require.Equal(t, "SELECT pg_advisory_xact_lock($1)", sql)
			require.Len(t, args, 1)
			return pgconn.NewCommandTag("SELECT 1"), nil
		}
		tx.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM job_memory") && strings.Contains(sql, "memory_key = $2"):
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			case strings.Contains(sql, "COALESCE(SUM(size_bytes), 0)"):
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 30
					return nil
				}}
			case strings.Contains(sql, "INSERT INTO job_memory"):
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = "mem-1"
					*(dest[1].(*time.Time)) = now
					*(dest[2].(*time.Time)) = now.Add(time.Second)
					return nil
				}}
			default:
				t.Fatalf("unexpected query: %s", sql)
				return &mockRow{}
			}
		}
		mem := &domain.JobMemory{JobID: "job-1", ProjectID: "project-1", MemoryKey: "state", SizeBytes: 20}

		err := New(&jobMemoryTxBeginner{tx: tx}).UpsertJobMemoryWithQuota(context.Background(), mem, 20, 50)
		require.NoError(t, err)
		require.True(t, tx.committed)
		require.False(t, tx.rolledBack)
		require.Equal(t, "mem-1", mem.ID)
	})

	t.Run("subtracts existing size before enforcing per job quota", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		tx := &jobMemoryTx{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		}}
		tx.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM job_memory") && strings.Contains(sql, "memory_key = $2"):
				return &mockRow{scanFn: func(dest ...any) error {
					fillJobMemoryDest(dest, "mem-existing", "state", 25, now)
					return nil
				}}
			case strings.Contains(sql, "COALESCE(SUM(size_bytes), 0)"):
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 40
					return nil
				}}
			case strings.Contains(sql, "INSERT INTO job_memory"):
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = "mem-existing"
					*(dest[1].(*time.Time)) = now
					*(dest[2].(*time.Time)) = now.Add(time.Second)
					return nil
				}}
			default:
				t.Fatalf("unexpected query: %s", sql)
				return &mockRow{}
			}
		}
		mem := &domain.JobMemory{JobID: "job-1", ProjectID: "project-1", MemoryKey: "state", SizeBytes: 30}

		err := New(&jobMemoryTxBeginner{tx: tx}).UpsertJobMemoryWithQuota(context.Background(), mem, 30, 50)
		require.NoError(t, err)
		require.True(t, tx.committed)
		require.False(t, tx.rolledBack)
	})

	t.Run("rejects per job quota and rolls back", func(t *testing.T) {
		t.Parallel()

		tx := &jobMemoryTx{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		}}
		tx.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM job_memory") && strings.Contains(sql, "memory_key = $2"):
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			case strings.Contains(sql, "COALESCE(SUM(size_bytes), 0)"):
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 40
					return nil
				}}
			default:
				t.Fatalf("unexpected query: %s", sql)
				return &mockRow{}
			}
		}
		mem := &domain.JobMemory{JobID: "job-1", ProjectID: "project-1", MemoryKey: "state", SizeBytes: 11}

		err := New(&jobMemoryTxBeginner{tx: tx}).UpsertJobMemoryWithQuota(context.Background(), mem, 11, 50)
		require.ErrorIs(t, err, ErrJobMemoryPerJobLimitExceeded)
		require.False(t, tx.committed)
		require.True(t, tx.rolledBack)
	})

	t.Run("wraps transaction step errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			configure  func(*jobMemoryTx)
			wantString string
			wantErr    error
		}{
			{
				name: "begin",
				configure: func(*jobMemoryTx) {
				},
				wantString: "begin transaction",
				wantErr:    errors.New("begin failed"),
			},
			{
				name: "advisory lock",
				configure: func(tx *jobMemoryTx) {
					tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, errors.New("lock failed")
					}
				},
				wantString: "advisory lock",
			},
			{
				name: "get existing",
				configure: func(tx *jobMemoryTx) {
					tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("SELECT 1"), nil
					}
					tx.queryRowFn = func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: func(...any) error { return errors.New("get failed") }}
					}
				},
				wantString: "get existing job memory",
			},
			{
				name: "sum",
				configure: func(tx *jobMemoryTx) {
					tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("SELECT 1"), nil
					}
					tx.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
						if strings.Contains(sql, "FROM job_memory") && strings.Contains(sql, "memory_key = $2") {
							return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
						}
						return &mockRow{scanFn: func(...any) error { return errors.New("sum failed") }}
					}
				},
				wantString: "sum job memory size",
			},
			{
				name: "upsert",
				configure: func(tx *jobMemoryTx) {
					tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("SELECT 1"), nil
					}
					tx.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
						switch {
						case strings.Contains(sql, "FROM job_memory") && strings.Contains(sql, "memory_key = $2"):
							return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
						case strings.Contains(sql, "COALESCE(SUM(size_bytes), 0)"):
							return &mockRow{scanFn: func(dest ...any) error {
								*(dest[0].(*int)) = 1
								return nil
							}}
						default:
							return &mockRow{scanFn: func(...any) error { return errors.New("upsert failed") }}
						}
					}
				},
				wantString: "upsert job memory",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				tx := &jobMemoryTx{}
				tt.configure(tx)
				beginner := &jobMemoryTxBeginner{tx: tx}
				if tt.wantErr != nil {
					beginner.beginErr = tt.wantErr
				}
				err := New(beginner).UpsertJobMemoryWithQuota(
					context.Background(),
					&domain.JobMemory{JobID: "job-1", ProjectID: "project-1", MemoryKey: "state", SizeBytes: 1},
					1,
					10,
				)
				require.Error(t, err)
				require.ErrorContains(t, err, tt.wantString)
				if tt.wantErr == nil {
					require.True(t, tx.rolledBack)
				}
			})
		}
	})
}

func TestUpsertJobMemoryWithQuotaForActiveRunTransactionUnit(t *testing.T) {
	t.Parallel()

	t.Run("maps missing active run to conflict", func(t *testing.T) {
		t.Parallel()

		tx := &jobMemoryTx{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}}

		err := New(&jobMemoryTxBeginner{tx: tx}).UpsertJobMemoryWithQuotaForActiveRun(
			context.Background(),
			"run-1",
			&domain.JobMemory{JobID: "job-1", ProjectID: "project-1", MemoryKey: "state", SizeBytes: 1},
			1,
			10,
			2,
		)
		require.ErrorIs(t, err, ErrRunConflict)
		require.True(t, tx.rolledBack)
	})

	t.Run("checks active run before quota and upsert", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		tx := &jobMemoryTx{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		}}
		tx.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM job_runs"):
				require.Equal(t, []any{"run-1", "job-1", 2}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					return nil
				}}
			case strings.Contains(sql, "FROM job_memory") && strings.Contains(sql, "memory_key = $2"):
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			case strings.Contains(sql, "COALESCE(SUM(size_bytes), 0)"):
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 4
					return nil
				}}
			case strings.Contains(sql, "INSERT INTO job_memory"):
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = "mem-1"
					*(dest[1].(*time.Time)) = now
					*(dest[2].(*time.Time)) = now.Add(time.Second)
					return nil
				}}
			default:
				t.Fatalf("unexpected query: %s", sql)
				return &mockRow{}
			}
		}
		mem := &domain.JobMemory{JobID: "job-1", ProjectID: "project-1", MemoryKey: "state", SizeBytes: 5}

		err := New(&jobMemoryTxBeginner{tx: tx}).UpsertJobMemoryWithQuotaForActiveRun(
			context.Background(), "run-1", mem, 5, 10, 2,
		)
		require.NoError(t, err)
		require.True(t, tx.committed)
		require.Equal(t, "mem-1", mem.ID)
	})

	t.Run("wraps active check errors", func(t *testing.T) {
		t.Parallel()

		checkErr := errors.New("check failed")
		tx := &jobMemoryTx{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return checkErr }}
		}}

		err := New(&jobMemoryTxBeginner{tx: tx}).UpsertJobMemoryWithQuotaForActiveRun(
			context.Background(),
			"run-1",
			&domain.JobMemory{JobID: "job-1", ProjectID: "project-1", MemoryKey: "state", SizeBytes: 1},
			1,
			10,
			2,
		)
		require.ErrorIs(t, err, checkErr)
		require.ErrorContains(t, err, "verify active run for job memory")
		require.True(t, tx.rolledBack)
	})
}

func TestGetJobMemoryUnit(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for missing non-expired memory", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "ttl_expires_at IS NULL OR ttl_expires_at > NOW()")
			require.Equal(t, []any{"job-1", "state"}, args)
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}}

		got, err := New(db).GetJobMemory(context.Background(), "job-1", "state")
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("scans memory row", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				fillJobMemoryDest(dest, "mem-1", "state", 12, now)
				return nil
			}}
		}}

		got, err := New(db).GetJobMemory(context.Background(), "job-1", "state")
		require.NoError(t, err)
		require.Equal(t, "mem-1", got.ID)
		require.Equal(t, "state", got.MemoryKey)
		require.Equal(t, 12, got.SizeBytes)
		require.JSONEq(t, `{"ok":true}`, string(got.Value))
	})

	t.Run("wraps scan errors", func(t *testing.T) {
		t.Parallel()

		readErr := errors.New("read failed")
		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return readErr }}
		}}

		got, err := New(db).GetJobMemory(context.Background(), "job-1", "state")
		require.ErrorIs(t, err, readErr)
		require.Nil(t, got)
		require.ErrorContains(t, err, "get job memory")
	})
}

func TestJobMemoryActiveRunGuardsUnit(t *testing.T) {
	t.Parallel()

	t.Run("get rejects inactive run attempts", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = false
				return nil
			}}
		}}

		got, err := New(db).GetJobMemoryForActiveRun(context.Background(), "run-1", "job-1", "state", 2)
		require.ErrorIs(t, err, ErrRunConflict)
		require.Nil(t, got)
	})

	t.Run("get wraps active check errors", func(t *testing.T) {
		t.Parallel()

		checkErr := errors.New("check failed")
		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return checkErr }}
		}}

		got, err := New(db).GetJobMemoryForActiveRun(context.Background(), "run-1", "job-1", "state", 2)
		require.ErrorIs(t, err, checkErr)
		require.Nil(t, got)
		require.ErrorContains(t, err, "check run active for attempt")
	})

	t.Run("get delegates to memory lookup when run is active", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		calls := 0
		db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			calls++
			if calls == 1 {
				require.Contains(t, sql, "SELECT EXISTS")
				require.Equal(t, []any{"run-1", 2, "job-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					return nil
				}}
			}
			require.Contains(t, sql, "FROM job_memory")
			require.Equal(t, []any{"job-1", "state"}, args)
			return &mockRow{scanFn: func(dest ...any) error {
				fillJobMemoryDest(dest, "mem-1", "state", 9, now)
				return nil
			}}
		}}

		got, err := New(db).GetJobMemoryForActiveRun(context.Background(), "run-1", "job-1", "state", 2)
		require.NoError(t, err)
		require.Equal(t, "mem-1", got.ID)
		require.Equal(t, 2, calls)
	})

	t.Run("list rejects inactive run attempts", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = false
				return nil
			}}
		}}

		got, err := New(db).ListJobMemoryForActiveRun(context.Background(), "run-1", "job-1", 2)
		require.ErrorIs(t, err, ErrRunConflict)
		require.Nil(t, got)
	})

	t.Run("list delegates when run is active", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					return nil
				}}
			},
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "ORDER BY memory_key ASC")
				require.Equal(t, []any{"job-1"}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillJobMemoryDest(dest, "mem-1", "alpha", 7, now)
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListJobMemoryForActiveRun(context.Background(), "run-1", "job-1", 2)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "alpha", got[0].MemoryKey)
	})
}

func TestListJobMemoryUnit(t *testing.T) {
	t.Parallel()

	t.Run("returns scanned rows and row iteration error", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		rowsErr := errors.New("rows failed")
		db := &mockDBTX{queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
			require.Contains(t, sql, "LIMIT 10000")
			require.Equal(t, []any{"job-1"}, args)
			return &mockRows{
				scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillJobMemoryDest(dest, "mem-1", "alpha", 1, now)
						return nil
					},
					func(dest ...any) error {
						fillJobMemoryDest(dest, "mem-2", "beta", 2, now)
						return nil
					},
				},
				err: rowsErr,
			}, nil
		}}

		got, err := New(db).ListJobMemory(context.Background(), "job-1")
		require.ErrorIs(t, err, rowsErr)
		require.Len(t, got, 2)
		require.Equal(t, "alpha", got[0].MemoryKey)
		require.Equal(t, "beta", got[1].MemoryKey)
	})

	t.Run("wraps query and scan errors", func(t *testing.T) {
		t.Parallel()

		queryErr := errors.New("query failed")
		db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return nil, queryErr
		}}
		got, err := New(db).ListJobMemory(context.Background(), "job-1")
		require.ErrorIs(t, err, queryErr)
		require.Nil(t, got)
		require.ErrorContains(t, err, "list job memory")

		scanErr := errors.New("scan failed")
		db = &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{scanFns: []func(dest ...any) error{
				func(...any) error { return scanErr },
			}}, nil
		}}
		got, err = New(db).ListJobMemory(context.Background(), "job-1")
		require.ErrorIs(t, err, scanErr)
		require.Nil(t, got)
		require.ErrorContains(t, err, "list job memory scan")
	})
}

func TestDeleteJobMemoryUnit(t *testing.T) {
	t.Parallel()

	t.Run("deletes by job and key", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			require.Equal(t, `DELETE FROM job_memory WHERE job_id = $1 AND memory_key = $2`, sql)
			require.Equal(t, []any{"job-1", "state"}, args)
			return pgconn.NewCommandTag("DELETE 1"), nil
		}}

		require.NoError(t, New(db).DeleteJobMemory(context.Background(), "job-1", "state"))
	})

	t.Run("wraps exec errors", func(t *testing.T) {
		t.Parallel()

		deleteErr := errors.New("delete failed")
		db := &mockDBTX{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, deleteErr
		}}

		err := New(db).DeleteJobMemory(context.Background(), "job-1", "state")
		require.ErrorIs(t, err, deleteErr)
		require.ErrorContains(t, err, "delete job memory")
	})
}

func TestDeleteJobMemoryForActiveRunUnit(t *testing.T) {
	t.Parallel()

	t.Run("requires active run attempt", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			active     bool
			wantErr    error
			wantString string
		}{
			{name: "active", active: true},
			{name: "inactive", active: false, wantErr: ErrRunConflict, wantString: "not active for attempt 3"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
					require.Contains(t, sql, "DELETE FROM job_memory")
					require.Equal(t, []any{"run-1", "job-1", "state", 3}, args)
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*bool)) = tt.active
						return nil
					}}
				}}

				err := New(db).DeleteJobMemoryForActiveRun(context.Background(), "run-1", "job-1", "state", 3)
				if tt.wantErr == nil {
					require.NoError(t, err)
					return
				}
				require.ErrorIs(t, err, tt.wantErr)
				require.ErrorContains(t, err, tt.wantString)
			})
		}
	})

	t.Run("wraps active delete query errors", func(t *testing.T) {
		t.Parallel()

		deleteErr := errors.New("delete failed")
		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return deleteErr }}
		}}

		err := New(db).DeleteJobMemoryForActiveRun(context.Background(), "run-1", "job-1", "state", 3)
		require.ErrorIs(t, err, deleteErr)
		require.ErrorContains(t, err, "delete active job memory")
	})
}

func TestSumJobMemorySizeBytesUnit(t *testing.T) {
	t.Parallel()

	t.Run("returns active total", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "COALESCE(SUM(size_bytes), 0)")
			require.Equal(t, []any{"job-1"}, args)
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*int)) = 42
				return nil
			}}
		}}

		got, err := New(db).SumJobMemorySizeBytes(context.Background(), "job-1")
		require.NoError(t, err)
		require.Equal(t, 42, got)
	})

	t.Run("wraps scan errors", func(t *testing.T) {
		t.Parallel()

		sumErr := errors.New("sum failed")
		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return sumErr }}
		}}

		got, err := New(db).SumJobMemorySizeBytes(context.Background(), "job-1")
		require.ErrorIs(t, err, sumErr)
		require.Zero(t, got)
		require.ErrorContains(t, err, "sum job memory size")
	})
}

func TestDeleteExpiredJobMemoryUnit(t *testing.T) {
	t.Parallel()

	t.Run("continues while a full batch is deleted", func(t *testing.T) {
		t.Parallel()

		tags := []pgconn.CommandTag{
			pgconn.NewCommandTag("DELETE 10000"),
			pgconn.NewCommandTag("DELETE 3"),
		}
		db := &mockDBTX{execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			require.Contains(t, sql, "LIMIT 10000")
			tag := tags[0]
			tags = tags[1:]
			return tag, nil
		}}

		got, err := New(db).DeleteExpiredJobMemory(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, 10003, got)
		require.Empty(t, tags)
	})

	t.Run("returns partial count on later delete errors", func(t *testing.T) {
		t.Parallel()

		deleteErr := errors.New("delete failed")
		calls := 0
		db := &mockDBTX{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			calls++
			if calls == 1 {
				return pgconn.NewCommandTag("DELETE 10000"), nil
			}
			return pgconn.CommandTag{}, deleteErr
		}}

		got, err := New(db).DeleteExpiredJobMemory(context.Background())
		require.ErrorIs(t, err, deleteErr)
		require.EqualValues(t, 10000, got)
		require.ErrorContains(t, err, "delete expired job memory")
	})
}
