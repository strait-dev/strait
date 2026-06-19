package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type partitionTx struct {
	fakeTx
	commitErr  error
	rollbacked bool
	committed  bool
}

func (tx *partitionTx) Commit(context.Context) error {
	tx.committed = true
	return tx.commitErr
}

func (tx *partitionTx) Rollback(context.Context) error {
	tx.rollbacked = true
	return nil
}

type partitionBeginner struct {
	mockDBTX
	tx       pgx.Tx
	beginErr error
}

func (b *partitionBeginner) Begin(context.Context) (pgx.Tx, error) {
	if b.beginErr != nil {
		return nil, b.beginErr
	}
	return b.tx, nil
}

func TestPartitionEnsureAndMetadata(t *testing.T) {
	t.Parallel()

	t.Run("ensures job run partitions with default months ahead", func(t *testing.T) {
		t.Parallel()

		var existsCalls int
		var execCalls int
		db := &mockDBTX{}
		db.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
			require.Contains(t, sql, "SELECT EXISTS")
			existsCalls++
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = existsCalls%2 == 0
				return nil
			}}
		}
		db.execFn = func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			require.Contains(t, sql, "partman.run_maintenance_proc")
			execCalls++
			return pgconn.NewCommandTag("DO"), nil
		}

		require.NoError(t, New(db).EnsureJobRunsPartitions(context.Background(), 0))
		require.Equal(t, 4, existsCalls)
		require.Equal(t, 2, execCalls)
	})

	t.Run("ensure job run partitions falls back to raw create", func(t *testing.T) {
		t.Parallel()

		var existsCalls int
		db := &mockDBTX{}
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			existsCalls++
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = false
				return nil
			}}
		}
		db.execFn = func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "partman.run_maintenance_proc") {
				return pgconn.NewCommandTag("DO"), nil
			}
			require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS")
			require.Contains(t, sql, "PARTITION OF job_runs")
			return pgconn.NewCommandTag("CREATE TABLE"), nil
		}

		require.NoError(t, New(db).EnsureJobRunsPartitions(context.Background(), 1))
		require.Equal(t, 4, existsCalls)
	})

	t.Run("ensure job run partitions wraps errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("exists failed") }}
			},
		}
		err := New(db).EnsureJobRunsPartitions(context.Background(), 1)
		require.ErrorContains(t, err, "ensure partition for")
		require.ErrorContains(t, err, "exists failed")

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = false
				return nil
			}}
		}
		db.execFn = func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "partman.run_maintenance_proc") {
				return pgconn.CommandTag{}, errors.New("partman failed")
			}
			return pgconn.CommandTag{}, errors.New("create failed")
		}
		err = New(db).EnsureJobRunsPartitions(context.Background(), 1)
		require.ErrorContains(t, err, "fallback CREATE TABLE")
	})

	t.Run("create partition via partman wraps exec errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, errors.New("exec failed")
			},
		}

		err := New(db).createPartitionViaPartman(context.Background(), time.Now())
		require.ErrorContains(t, err, "run_maintenance_proc")
	})

	t.Run("partition reloptions parse missing and errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*[]string)) = []string{"fillfactor=80", "autovacuum_enabled=false"}
					return nil
				}}
			},
		}
		q := New(db)
		got, err := q.PartitionReloption(context.Background(), "p1", "fillfactor")
		require.NoError(t, err)
		require.Equal(t, "80", got)
		got, err = q.PartitionReloption(context.Background(), "p1", "toast_tuple_target")
		require.NoError(t, err)
		require.Empty(t, got)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		got, err = q.PartitionReloption(context.Background(), "missing", "fillfactor")
		require.NoError(t, err)
		require.Empty(t, got)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return errors.New("read failed") }}
		}
		_, err = q.PartitionReloption(context.Background(), "p1", "fillfactor")
		require.ErrorContains(t, err, "read reloptions")
	})

	t.Run("partition exists maps scan errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					return nil
				}}
			},
		}
		got, err := New(db).PartitionExists(context.Background(), "p1")
		require.NoError(t, err)
		require.True(t, got)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return errors.New("scan failed") }}
		}
		_, err = New(db).PartitionExists(context.Background(), "p1")
		require.ErrorContains(t, err, "check partition p1")
	})

	t.Run("month helpers normalize UTC starts", func(t *testing.T) {
		t.Parallel()

		loc := time.FixedZone("offset", -3*60*60)
		input := time.Date(2026, time.January, 31, 23, 0, 0, 0, loc)
		require.Equal(t, time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC), addMonths(input, 1))
		require.Equal(t, time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC), startOfMonth(input))
	})
}

func TestPartitionListsAndCounts(t *testing.T) {
	t.Parallel()

	t.Run("lists job run and outbox history partitions", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
				if strings.Contains(sql, "enqueue_outbox_history") {
					return &mockRows{scanFns: []func(dest ...any) error{
						func(dest ...any) error {
							*(dest[0].(*string)) = "enqueue_outbox_history_p2026_01"
							return nil
						},
					}}, nil
				}
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "job_runs_p2026_01"
						return nil
					},
				}}, nil
			},
		}
		q := New(db)
		jobPartitions, err := q.ListJobRunsPartitions(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"job_runs_p2026_01"}, jobPartitions)
		outboxPartitions, err := q.ListOutboxHistoryPartitions(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"enqueue_outbox_history_p2026_01"}, outboxPartitions)
	})

	t.Run("list partitions maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list partitions"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan"},
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
				_, err := New(db).ListJobRunsPartitions(context.Background())
				require.ErrorContains(t, err, tc.wantString)
			})
		}

		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return nil, pgx.ErrNoRows
			},
		}
		got, err := New(db).ListJobRunsPartitions(context.Background())
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("list outbox history partitions maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list outbox history partitions"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan"},
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
				_, err := New(db).ListOutboxHistoryPartitions(context.Background())
				require.ErrorContains(t, err, tc.wantString)
			})
		}

		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return nil, pgx.ErrNoRows
			},
		}
		got, err := New(db).ListOutboxHistoryPartitions(context.Background())
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("counts and estimates partition rows", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					if strings.Contains(sql, "COUNT(*)") {
						require.Empty(t, args)
						*(dest[0].(*int64)) = 11
						return nil
					}
					require.Equal(t, []any{"job_runs_p2026_01"}, args)
					*(dest[0].(*int64)) = 12
					return nil
				}}
			},
		}
		q := New(db)
		count, err := q.PartitionRowCount(context.Background(), "job_runs_p2026_01")
		require.NoError(t, err)
		require.Equal(t, int64(11), count)
		estimate, err := q.PartitionEstimatedRowCount(context.Background(), "job_runs_p2026_01")
		require.NoError(t, err)
		require.Equal(t, int64(12), estimate)

		_, err = q.PartitionRowCount(context.Background(), "bad-name")
		require.ErrorContains(t, err, "partition row count")

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return errors.New("scan failed") }}
		}
		_, err = q.PartitionRowCount(context.Background(), "job_runs_p2026_01")
		require.ErrorContains(t, err, "partition row count job_runs_p2026_01")
		_, err = q.PartitionEstimatedRowCount(context.Background(), "job_runs_p2026_01")
		require.ErrorContains(t, err, "partition estimated row count job_runs_p2026_01")
	})

	t.Run("exec ddl wraps errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{}
		require.NoError(t, New(db).ExecDDL(context.Background(), "ALTER TABLE x"))

		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("exec failed")
		}
		err := New(db).ExecDDL(context.Background(), "ALTER TABLE x")
		require.ErrorContains(t, err, "exec ddl")
	})
}

func TestPartitionDropWithTimeout(t *testing.T) {
	t.Parallel()

	t.Run("drop partition requires transaction support", func(t *testing.T) {
		t.Parallel()

		err := New(&mockDBTX{}).DropPartitionWithTimeout(context.Background(), "job_runs_p2026_01", time.Second)
		require.ErrorContains(t, err, "does not support transactions")
		_, err = New(&mockDBTX{}).DropPartitionIfEmptyWithTimeout(context.Background(), "job_runs_p2026_01", time.Second)
		require.ErrorContains(t, err, "does not support transactions")
	})

	t.Run("drop partition handles begin and setup errors", func(t *testing.T) {
		t.Parallel()

		db := &partitionBeginner{beginErr: errors.New("begin failed")}
		err := New(db).DropPartitionWithTimeout(context.Background(), "job_runs_p2026_01", time.Second)
		require.ErrorContains(t, err, "begin tx")

		tx := &partitionTx{}
		tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("timeout failed")
		}
		err = New(&partitionBeginner{tx: tx}).DropPartitionWithTimeout(context.Background(), "job_runs_p2026_01", time.Second)
		require.ErrorContains(t, err, "set lock_timeout")
		require.True(t, tx.rollbacked)

		tx = &partitionTx{}
		err = New(&partitionBeginner{tx: tx}).DropPartitionWithTimeout(context.Background(), "bad-name", time.Second)
		require.ErrorContains(t, err, "invalid name")
		require.True(t, tx.rollbacked)
	})

	t.Run("drop partition verifies managed child and commits", func(t *testing.T) {
		t.Parallel()

		tx := &partitionTx{}
		var execs []string
		tx.execFn = func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			execs = append(execs, sql)
			return pgconn.NewCommandTag("OK"), nil
		}
		tx.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = true
				return nil
			}}
		}

		err := New(&partitionBeginner{tx: tx}).DropPartitionWithTimeout(context.Background(), "job_runs_p2026_01", 1500*time.Millisecond)
		require.NoError(t, err)
		require.True(t, tx.committed)
		require.Contains(t, execs[0], "SET LOCAL lock_timeout = 1500")
		require.Contains(t, execs[1], "DROP TABLE IF EXISTS")
	})

	t.Run("drop partition maps verify drop and commit errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			known      bool
			dropErr    error
			commitErr  error
			wantString string
		}{
			{name: "verify error", queryErr: errors.New("verify failed"), wantString: "verify parent"},
			{name: "unknown child", known: false, wantString: "not a managed history partition"},
			{name: "drop error", known: true, dropErr: errors.New("drop failed"), wantString: "drop partition job_runs_p2026_01"},
			{name: "commit error", known: true, commitErr: errors.New("commit failed"), wantString: "commit"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				tx := &partitionTx{commitErr: tc.commitErr}
				var execCalls int
				tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					execCalls++
					if execCalls == 2 && tc.dropErr != nil {
						return pgconn.CommandTag{}, tc.dropErr
					}
					return pgconn.NewCommandTag("OK"), nil
				}
				tx.queryRowFn = func(context.Context, string, ...any) pgx.Row {
					return &mockRow{scanFn: func(dest ...any) error {
						if tc.queryErr != nil {
							return tc.queryErr
						}
						*(dest[0].(*bool)) = tc.known
						return nil
					}}
				}

				err := New(&partitionBeginner{tx: tx}).DropPartitionWithTimeout(context.Background(), "job_runs_p2026_01", time.Second)
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})
}

func TestPartitionDropIfEmptyWithTimeout(t *testing.T) {
	t.Parallel()

	t.Run("drops empty managed partition", func(t *testing.T) {
		t.Parallel()

		tx := &partitionTx{}
		var execs []string
		tx.execFn = func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			execs = append(execs, sql)
			return pgconn.NewCommandTag("OK"), nil
		}
		var queryCalls int
		tx.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			queryCalls++
			return &mockRow{scanFn: func(dest ...any) error {
				if queryCalls == 1 {
					*(dest[0].(*bool)) = true
					return nil
				}
				*(dest[0].(*int64)) = 0
				return nil
			}}
		}

		dropped, err := New(&partitionBeginner{tx: tx}).DropPartitionIfEmptyWithTimeout(context.Background(), "job_runs_p2026_01", time.Second)
		require.NoError(t, err)
		require.True(t, dropped)
		require.True(t, tx.committed)
		require.Contains(t, execs[1], "LOCK TABLE")
		require.Contains(t, execs[2], "DROP TABLE")
	})

	t.Run("keeps non-empty managed partition", func(t *testing.T) {
		t.Parallel()

		tx := &partitionTx{}
		tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("OK"), nil
		}
		var queryCalls int
		tx.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			queryCalls++
			return &mockRow{scanFn: func(dest ...any) error {
				if queryCalls == 1 {
					*(dest[0].(*bool)) = true
					return nil
				}
				*(dest[0].(*int64)) = 4
				return nil
			}}
		}

		dropped, err := New(&partitionBeginner{tx: tx}).DropPartitionIfEmptyWithTimeout(context.Background(), "job_runs_p2026_01", time.Second)
		require.NoError(t, err)
		require.False(t, dropped)
		require.True(t, tx.committed)
	})

	t.Run("drop empty partition maps setup and validation errors", func(t *testing.T) {
		t.Parallel()

		tx := &partitionTx{}
		tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("timeout failed")
		}
		_, err := New(&partitionBeginner{tx: tx}).DropPartitionIfEmptyWithTimeout(context.Background(), "job_runs_p2026_01", time.Second)
		require.ErrorContains(t, err, "set lock_timeout")

		tx = &partitionTx{}
		_, err = New(&partitionBeginner{tx: tx}).DropPartitionIfEmptyWithTimeout(context.Background(), "bad-name", time.Second)
		require.ErrorContains(t, err, "invalid name")

		tx = &partitionTx{}
		var execCalls int
		tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			execCalls++
			if execCalls == 2 {
				return pgconn.CommandTag{}, errors.New("lock failed")
			}
			return pgconn.NewCommandTag("OK"), nil
		}
		_, err = New(&partitionBeginner{tx: tx}).DropPartitionIfEmptyWithTimeout(context.Background(), "job_runs_p2026_01", time.Second)
		require.ErrorContains(t, err, "lock job_runs_p2026_01")
	})

	t.Run("drop empty partition maps verify count drop and commit errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			known      bool
			count      int64
			dropErr    error
			commitErr  error
			wantString string
		}{
			{name: "verify error", queryErr: errors.New("verify failed"), wantString: "verify parent"},
			{name: "unknown child", known: false, wantString: "not a managed history partition"},
			{name: "count error", known: true, queryErr: errors.New("count failed"), wantString: "verify parent"},
			{name: "drop error", known: true, dropErr: errors.New("drop failed"), wantString: "drop empty partition job_runs_p2026_01"},
			{name: "commit after drop error", known: true, commitErr: errors.New("commit failed"), wantString: "commit"},
			{name: "commit after non-empty error", known: true, count: 4, commitErr: errors.New("commit failed"), wantString: "commit after non-empty count"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				tx := &partitionTx{commitErr: tc.commitErr}
				var execCalls int
				tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					execCalls++
					if execCalls == 3 && tc.dropErr != nil {
						return pgconn.CommandTag{}, tc.dropErr
					}
					return pgconn.NewCommandTag("OK"), nil
				}
				var queryCalls int
				tx.queryRowFn = func(context.Context, string, ...any) pgx.Row {
					queryCalls++
					return &mockRow{scanFn: func(dest ...any) error {
						if tc.queryErr != nil {
							return tc.queryErr
						}
						if queryCalls == 1 {
							*(dest[0].(*bool)) = tc.known
							return nil
						}
						*(dest[0].(*int64)) = tc.count
						return nil
					}}
				}

				_, err := New(&partitionBeginner{tx: tx}).DropPartitionIfEmptyWithTimeout(context.Background(), "job_runs_p2026_01", time.Second)
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})
}
