package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func fillOutboxRowDest(dest []any, id string, createdAt time.Time) {
	idempotencyKey := "idem-1"
	scheduledAt := createdAt.Add(time.Hour)
	retryOf := "source-outbox"

	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "project-1"
	*(dest[2].(*string)) = "job-1"
	*(dest[3].(*json.RawMessage)) = json.RawMessage(`{"payload":true}`)
	*(dest[4].(*json.RawMessage)) = json.RawMessage(`{"meta":true}`)
	*(dest[5].(**string)) = &idempotencyKey
	*(dest[6].(**time.Time)) = &scheduledAt
	*(dest[7].(*int)) = 7
	*(dest[8].(*time.Time)) = createdAt
	*(dest[9].(**string)) = &retryOf
	*(dest[10].(*string)) = "worker"
	*(dest[11].(*string)) = "critical"
}

func fillQuarantinedOutboxDest(dest []any, id string, createdAt time.Time) {
	idempotencyKey := "idem-1"
	scheduledAt := createdAt.Add(time.Hour)
	consumedAt := createdAt.Add(2 * time.Hour)
	retryOf := "source-outbox"

	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "project-1"
	*(dest[2].(*string)) = "job-1"
	*(dest[3].(*json.RawMessage)) = json.RawMessage(`{"payload":true}`)
	*(dest[4].(*json.RawMessage)) = json.RawMessage(`{"meta":true}`)
	*(dest[5].(**string)) = &idempotencyKey
	*(dest[6].(**time.Time)) = &scheduledAt
	*(dest[7].(*int)) = 7
	*(dest[8].(*time.Time)) = createdAt
	*(dest[9].(*time.Time)) = consumedAt
	*(dest[10].(*string)) = "terminal failure"
	*(dest[11].(**string)) = &retryOf
}

func fillOutboxRowStateDest(dest []any, id string, createdAt time.Time, quarantined bool) {
	idempotencyKey := "idem-1"
	scheduledAt := createdAt.Add(time.Hour)
	consumedAt := createdAt.Add(2 * time.Hour)
	errText := "terminal failure"
	retryOf := "source-outbox"

	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "project-1"
	*(dest[2].(*string)) = "job-1"
	*(dest[3].(*json.RawMessage)) = json.RawMessage(`{"payload":true}`)
	*(dest[4].(*json.RawMessage)) = json.RawMessage(`{"meta":true}`)
	*(dest[5].(**string)) = &idempotencyKey
	*(dest[6].(**time.Time)) = &scheduledAt
	*(dest[7].(*int)) = 7
	*(dest[8].(*time.Time)) = createdAt
	if quarantined {
		*(dest[9].(**time.Time)) = &consumedAt
		*(dest[10].(**string)) = &errText
	}
	*(dest[11].(**string)) = &retryOf
}

func fillRetryCloneDest(dest []any, id string, createdAt time.Time) {
	idempotencyKey := "idem-1"
	scheduledAt := createdAt.Add(time.Hour)
	retryOf := "source-outbox"

	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "project-1"
	*(dest[2].(*string)) = "job-1"
	*(dest[3].(*json.RawMessage)) = json.RawMessage(`{"payload":true}`)
	*(dest[4].(*json.RawMessage)) = json.RawMessage(`{"meta":true}`)
	*(dest[5].(**string)) = &idempotencyKey
	*(dest[6].(**time.Time)) = &scheduledAt
	*(dest[7].(*int)) = 7
	*(dest[8].(*time.Time)) = createdAt
	*(dest[9].(**string)) = &retryOf
}

type outboxRetryTx struct {
	fakeTx
	committed  bool
	rolledBack bool
}

func (tx *outboxRetryTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *outboxRetryTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

type outboxRetryBeginner struct {
	mockDBTX
	tx *outboxRetryTx
}

func (b *outboxRetryBeginner) Begin(context.Context) (pgx.Tx, error) {
	return b.tx, nil
}

func TestOutboxClaimHelpers(t *testing.T) {
	t.Parallel()

	t.Run("claim with lease defaults scans rows", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		tx := &fakeTx{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Equal(t, claimOutboxSQL, sql)
				require.Equal(t, []any{500, "outbox-flusher", 30 * time.Second}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillOutboxRowDest(dest, "outbox-1", createdAt)
						return nil
					},
				}}, nil
			},
		}

		got, err := ClaimOutboxInTx(context.Background(), tx, 0, "", 0)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "outbox-1", got[0].ID)
		require.Equal(t, "worker", got[0].ExecutionMode)
		require.Equal(t, "critical", got[0].QueueName)
	})

	t.Run("claim with lease wraps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "claim outbox claim-log rows"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan outbox claim-log row"},
			{name: "rows", rowErr: errors.New("rows failed"), wantString: "rows failed"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				tx := &fakeTx{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tc.queryErr != nil {
							return nil, tc.queryErr
						}
						rows := &mockRows{err: tc.rowErr}
						if tc.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error {
								return tc.scanErr
							}}
						}
						return rows, nil
					},
				}

				_, err := ClaimOutboxInTx(context.Background(), tx, 10, "owner", time.Second)
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})

	t.Run("claim unconsumed wrappers use shared query path", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "FROM enqueue_outbox")
				require.Equal(t, []any{500}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillOutboxRowDest(dest, "outbox-1", createdAt)
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ClaimUnconsumedOutbox(context.Background(), 0)
		require.NoError(t, err)
		require.Len(t, got, 1)

		tx := &fakeTx{queryFn: db.queryFn}
		got, err = ClaimUnconsumedOutboxInTx(context.Background(), tx, 0)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("shared claim wraps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "claim outbox"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan outbox"},
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
							rows.scanFns = []func(dest ...any) error{func(...any) error {
								return tc.scanErr
							}}
						}
						return rows, nil
					},
				}

				_, err := claimOutboxOnConn(context.Background(), db, 10)
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})
}

func TestOutboxClaimStatusUpdates(t *testing.T) {
	t.Parallel()

	t.Run("reclaims expired claims", func(t *testing.T) {
		t.Parallel()

		tx := &fakeTx{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "lease_expires_at <= NOW()")
				require.Empty(t, args)
				return pgconn.NewCommandTag("UPDATE 3"), nil
			},
		}

		got, err := ReclaimExpiredOutboxClaimsInTx(context.Background(), tx)
		require.NoError(t, err)
		require.Equal(t, int64(3), got)
	})

	t.Run("claim status updates skip empty ids and wrap errors", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, MarkOutboxClaimsReadyInTx(context.Background(), &fakeTx{}, nil))
		require.NoError(t, MarkOutboxClaimsAckedInTx(context.Background(), &fakeTx{}, nil))

		execErr := errors.New("exec failed")
		tx := &fakeTx{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, execErr
			},
		}
		require.ErrorContains(t, MarkOutboxClaimsReadyInTx(context.Background(), tx, []string{"outbox-1"}), "mark outbox claims ready")
		require.ErrorContains(t, MarkOutboxClaimsAckedInTx(context.Background(), tx, []string{"outbox-1"}), "mark outbox claims acked")
	})

	t.Run("marks consumed through shared exec path", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, markOutboxOnExec(context.Background(), &mockDBTX{}, nil))

		var capturedArgs []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "consumed_at = NOW()")
				capturedArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("UPDATE 2"), nil
			},
		}
		require.NoError(t, New(db).MarkOutboxConsumed(context.Background(), []string{"outbox-1", "outbox-2"}))
		require.Equal(t, []any{[]string{"outbox-1", "outbox-2"}}, capturedArgs)

		execErr := errors.New("exec failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		require.ErrorContains(t, MarkOutboxConsumedInTx(context.Background(), &fakeTx{execFn: db.execFn}, []string{"outbox-1"}), "mark outbox consumed")
	})
}

func TestOutboxErrorAndMetrics(t *testing.T) {
	t.Parallel()

	t.Run("truncates outbox error text", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "outbox promotion failed", TruncateOutboxError(" \n\t "))
		require.Equal(t, "failed", TruncateOutboxError(" failed "))
		long := strings.Repeat("x", maxOutboxErrorLength+10)
		require.Len(t, TruncateOutboxError(long), maxOutboxErrorLength)
	})

	t.Run("marks outbox errored", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, MarkOutboxErroredInTx(context.Background(), &fakeTx{}, "", "ignored"))

		var capturedArgs []any
		tx := &fakeTx{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "SET error = $2")
				capturedArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		require.NoError(t, MarkOutboxErroredInTx(context.Background(), tx, "outbox-1", " failed "))
		require.Equal(t, []any{"outbox-1", "failed"}, capturedArgs)

		execErr := errors.New("exec failed")
		tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		require.ErrorContains(t, MarkOutboxErroredInTx(context.Background(), tx, "outbox-1", "failed"), "mark outbox errored")
	})

	t.Run("counts and ages outbox rows", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					if strings.Contains(sql, "COUNT(*)") {
						*(dest[0].(*int)) = 12
						return nil
					}
					*(dest[0].(*float64)) = 2.5
					return nil
				}}
			},
		}
		q := New(db)

		count, err := q.CountUnconsumedOutbox(context.Background())
		require.NoError(t, err)
		require.Equal(t, 12, count)
		count, err = q.CountClaimableOutbox(context.Background())
		require.NoError(t, err)
		require.Equal(t, 12, count)
		age, err := q.OldestUnconsumedOutboxAge(context.Background())
		require.NoError(t, err)
		require.Equal(t, 2500*time.Millisecond, age)
		age, err = q.OldestClaimableOutboxAge(context.Background())
		require.NoError(t, err)
		require.Equal(t, 2500*time.Millisecond, age)
	})

	t.Run("count and age queries wrap errors", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("scan failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return scanErr
				}}
			},
		}
		q := New(db)

		_, err := q.CountUnconsumedOutbox(context.Background())
		require.ErrorContains(t, err, "count outbox")
		_, err = q.CountClaimableOutbox(context.Background())
		require.ErrorContains(t, err, "count claimable outbox claim-log rows")
		_, err = q.OldestUnconsumedOutboxAge(context.Background())
		require.ErrorContains(t, err, "oldest outbox age")
		_, err = q.OldestClaimableOutboxAge(context.Background())
		require.ErrorContains(t, err, "oldest claimable outbox claim-log age")
	})
}

func TestQuarantinedOutboxReadAndPurge(t *testing.T) {
	t.Parallel()

	t.Run("lists quarantined rows with default limit and cursor", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		cursor := createdAt.Add(time.Minute)
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "(consumed_at, id) < ($3, $4)")
				require.Equal(t, []any{"project-1", 50, cursor, "cursor-id"}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillQuarantinedOutboxDest(dest, "outbox-1", createdAt)
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListQuarantinedOutbox(context.Background(), "project-1", 0, &cursor, "cursor-id")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "outbox-1", got[0].ID)
		require.Equal(t, "terminal failure", got[0].Error)
	})

	t.Run("list quarantined wraps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list quarantined outbox"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan quarantined outbox"},
			{name: "rows", rowErr: errors.New("rows failed"), wantString: "list quarantined outbox"},
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
							rows.scanFns = []func(dest ...any) error{func(...any) error {
								return tc.scanErr
							}}
						}
						return rows, nil
					},
				}

				_, err := New(db).ListQuarantinedOutbox(context.Background(), "project-1", 10, nil, "")
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})

	t.Run("gets quarantined row and maps errors", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
				require.Equal(t, []any{"project-1", "outbox-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					fillQuarantinedOutboxDest(dest, "outbox-1", createdAt)
					return nil
				}}
			},
		}

		got, err := New(db).GetQuarantinedOutbox(context.Background(), "project-1", "outbox-1")
		require.NoError(t, err)
		require.Equal(t, "outbox-1", got.ID)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		_, err = New(db).GetQuarantinedOutbox(context.Background(), "project-1", "missing")
		require.ErrorIs(t, err, ErrOutboxRowNotFound)

		scanErr := errors.New("scan failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return scanErr }}
		}
		_, err = New(db).GetQuarantinedOutbox(context.Background(), "project-1", "outbox-1")
		require.ErrorContains(t, err, "get quarantined outbox")
	})

	t.Run("purges quarantined row and fallback conflicts", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					if strings.Contains(sql, "DELETE FROM enqueue_outbox") {
						fillQuarantinedOutboxDest(dest, "outbox-1", createdAt)
						return nil
					}
					require.Fail(t, "unexpected fallback query")
					return nil
				}}
			},
		}

		got, err := New(db).PurgeQuarantinedOutbox(context.Background(), "project-1", "outbox-1")
		require.NoError(t, err)
		require.Equal(t, "outbox-1", got.ID)

		db.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if strings.Contains(sql, "DELETE FROM enqueue_outbox") {
					return pgx.ErrNoRows
				}
				fillOutboxRowStateDest(dest, "outbox-1", createdAt, true)
				return nil
			}}
		}
		_, err = New(db).PurgeQuarantinedOutbox(context.Background(), "project-1", "outbox-1")
		require.ErrorIs(t, err, ErrOutboxRowConflict)

		scanErr := errors.New("delete failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return scanErr }}
		}
		_, err = New(db).PurgeQuarantinedOutbox(context.Background(), "project-1", "outbox-1")
		require.ErrorContains(t, err, "purge quarantined outbox")
	})

	t.Run("purges old quarantined rows", func(t *testing.T) {
		t.Parallel()

		cutoff := time.Now().UTC()
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "consumed_at < $1")
				require.Equal(t, []any{cutoff, 25}, args)
				return pgconn.NewCommandTag("DELETE 4"), nil
			},
		}

		got, err := New(db).PurgeQuarantinedOutboxOlderThan(context.Background(), cutoff, 25)
		require.NoError(t, err)
		require.Equal(t, int64(4), got)

		execErr := errors.New("exec failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		_, err = New(db).PurgeQuarantinedOutboxOlderThan(context.Background(), cutoff, 25)
		require.ErrorContains(t, err, "purge stale quarantined outbox")
	})
}

func TestOutboxRowStateHelpers(t *testing.T) {
	t.Parallel()

	t.Run("retry requires transaction support", func(t *testing.T) {
		t.Parallel()

		_, err := New(&mockDBTX{}).RetryQuarantinedOutbox(context.Background(), "project-1", "outbox-1")
		require.ErrorContains(t, err, "db does not support transactions")
	})

	t.Run("retry maps transaction body errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			queryRowFn  func(context.Context, string, ...any) pgx.Row
			wantErr     error
			wantContain string
		}{
			{
				name: "source not found",
				queryRowFn: func(context.Context, string, ...any) pgx.Row {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				},
				wantErr: ErrOutboxRowNotFound,
			},
			{
				name: "source is not quarantined",
				queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
					return &mockRow{scanFn: func(dest ...any) error {
						fillOutboxRowStateDest(dest, "outbox-1", time.Now().UTC(), false)
						return nil
					}}
				},
				wantErr: ErrOutboxRowConflict,
			},
			{
				name: "active clone already exists",
				queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
					return &mockRow{scanFn: func(dest ...any) error {
						if strings.Contains(sql, "FOR UPDATE") {
							fillOutboxRowStateDest(dest, "outbox-1", time.Now().UTC(), true)
							return nil
						}
						*(dest[0].(*string)) = "active-clone"
						return nil
					}}
				},
				wantErr: ErrOutboxRowConflict,
			},
			{
				name: "active clone check fails",
				queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
					return &mockRow{scanFn: func(dest ...any) error {
						if strings.Contains(sql, "FOR UPDATE") {
							fillOutboxRowStateDest(dest, "outbox-1", time.Now().UTC(), true)
							return nil
						}
						return errors.New("lookup failed")
					}}
				},
				wantContain: "check active clone",
			},
			{
				name: "unique conflict while inserting clone",
				queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
					return &mockRow{scanFn: func(dest ...any) error {
						switch {
						case strings.Contains(sql, "FOR UPDATE"):
							fillOutboxRowStateDest(dest, "outbox-1", time.Now().UTC(), true)
							return nil
						case strings.Contains(sql, "SELECT id"):
							return pgx.ErrNoRows
						default:
							return &pgconn.PgError{Code: "23505"}
						}
					}}
				},
				wantErr: ErrOutboxRowConflict,
			},
			{
				name: "insert clone fails",
				queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
					return &mockRow{scanFn: func(dest ...any) error {
						switch {
						case strings.Contains(sql, "FOR UPDATE"):
							fillOutboxRowStateDest(dest, "outbox-1", time.Now().UTC(), true)
							return nil
						case strings.Contains(sql, "SELECT id"):
							return pgx.ErrNoRows
						default:
							return errors.New("insert failed")
						}
					}}
				},
				wantContain: "insert clone",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				tx := &outboxRetryTx{}
				tx.queryRowFn = tc.queryRowFn
				db := &outboxRetryBeginner{tx: tx}

				_, err := New(db).RetryQuarantinedOutbox(context.Background(), "project-1", "outbox-1")
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
				} else {
					require.ErrorContains(t, err, tc.wantContain)
				}
				require.False(t, tx.committed)
				require.True(t, tx.rolledBack)
			})
		}
	})

	t.Run("retry clones quarantined row", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		tx := &outboxRetryTx{}
		tx.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				switch {
				case strings.Contains(sql, "FOR UPDATE"):
					require.Equal(t, []any{"project-1", "outbox-1"}, args)
					fillOutboxRowStateDest(dest, "outbox-1", createdAt, true)
					return nil
				case strings.Contains(sql, "SELECT id"):
					require.Equal(t, []any{"outbox-1"}, args)
					return pgx.ErrNoRows
				default:
					require.Len(t, args, 9)
					require.Equal(t, "project-1", args[1])
					require.Equal(t, "job-1", args[2])
					require.Equal(t, "outbox-1", args[8])
					fillRetryCloneDest(dest, args[0].(string), createdAt.Add(time.Minute))
					return nil
				}
			}}
		}
		db := &outboxRetryBeginner{tx: tx}

		got, err := New(db).RetryQuarantinedOutbox(context.Background(), "project-1", "outbox-1")
		require.NoError(t, err)
		require.NotEmpty(t, got.ID)
		require.Equal(t, "project-1", got.ProjectID)
		require.Equal(t, "job-1", got.JobID)
		require.NotNil(t, got.RetryOfOutboxID)
		require.Equal(t, "source-outbox", *got.RetryOfOutboxID)
		require.True(t, tx.committed)
		require.False(t, tx.rolledBack)
	})

	t.Run("loads outbox row states", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Equal(t, []any{"project-1", "outbox-1"}, args)
				if strings.Contains(sql, "FOR UPDATE") {
					require.Contains(t, sql, "FOR UPDATE")
				}
				return &mockRow{scanFn: func(dest ...any) error {
					fillOutboxRowStateDest(dest, "outbox-1", createdAt, true)
					return nil
				}}
			},
		}

		state, err := loadOutboxRowState(context.Background(), db, "project-1", "outbox-1")
		require.NoError(t, err)
		require.Equal(t, "outbox-1", state.ID)
		require.NotNil(t, state.ConsumedAt)
		require.NotNil(t, state.Error)

		state, err = loadOutboxRowStateForUpdate(context.Background(), db, "project-1", "outbox-1")
		require.NoError(t, err)
		require.Equal(t, "outbox-1", state.ID)
	})

	t.Run("scan outbox row state maps errors", func(t *testing.T) {
		t.Parallel()

		_, err := scanOutboxRowState(context.Background(), &mockRow{scanFn: func(...any) error {
			return pgx.ErrNoRows
		}}, "project-1", "missing")
		require.ErrorIs(t, err, ErrOutboxRowNotFound)

		scanErr := errors.New("scan failed")
		_, err = scanOutboxRowState(context.Background(), &mockRow{scanFn: func(...any) error {
			return scanErr
		}}, "project-1", "outbox-1")
		require.ErrorContains(t, err, "load outbox row project-1/outbox-1")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("requires quarantined state", func(t *testing.T) {
		t.Parallel()

		require.ErrorIs(t, requireQuarantinedOutbox(nil), ErrOutboxRowNotFound)
		require.ErrorIs(t, requireQuarantinedOutbox(&outboxRowState{ID: "outbox-1"}), ErrOutboxRowConflict)
		consumedAt := time.Now().UTC()
		blank := " "
		require.ErrorIs(t, requireQuarantinedOutbox(&outboxRowState{ID: "outbox-1", ConsumedAt: &consumedAt, Error: &blank}), ErrOutboxRowConflict)
		msg := "failed"
		require.NoError(t, requireQuarantinedOutbox(&outboxRowState{ID: "outbox-1", ConsumedAt: &consumedAt, Error: &msg}))
	})
}

func TestOutboxHistoryMaintenance(t *testing.T) {
	t.Parallel()

	t.Run("archives consumed outbox rows with retention cutoff", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO enqueue_outbox_history")
				require.Contains(t, sql, "DELETE FROM outbox_claims")
				require.Len(t, args, 2)
				require.WithinDuration(t, time.Now().Add(-time.Hour), args[0].(time.Time), time.Second)
				require.Equal(t, 25, args[1])
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int64)) = 3
					*(dest[1].(*int64)) = 3
					return nil
				}}
			},
		}

		got, err := New(db).ArchiveConsumedOutboxBatch(context.Background(), time.Hour, 25)
		require.NoError(t, err)
		require.Equal(t, int64(3), got)
	})

	t.Run("archive consumed outbox rows wraps errors", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("archive failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return scanErr }}
			},
		}

		_, err := New(db).ArchiveConsumedOutboxBatch(context.Background(), time.Hour, 25)
		require.ErrorContains(t, err, "archive consumed outbox batch")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("deletes outbox history past retention", func(t *testing.T) {
		t.Parallel()

		cutoff := time.Now().UTC()
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "DELETE FROM enqueue_outbox_history")
				require.Equal(t, []any{cutoff, 50}, args)
				return pgconn.NewCommandTag("DELETE 7"), nil
			},
		}

		got, err := New(db).DeleteOutboxHistoryPastRetention(context.Background(), cutoff, 50)
		require.NoError(t, err)
		require.Equal(t, int64(7), got)

		execErr := errors.New("delete failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		_, err = New(db).DeleteOutboxHistoryPastRetention(context.Background(), cutoff, 50)
		require.ErrorContains(t, err, "delete outbox history past retention")
		require.ErrorIs(t, err, execErr)
	})

	t.Run("ensures current and future outbox history partitions", func(t *testing.T) {
		t.Parallel()

		var queries []string
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Empty(t, args)
				require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS")
				require.Contains(t, sql, "PARTITION OF enqueue_outbox_history")
				require.Contains(t, sql, "FOR VALUES FROM")
				queries = append(queries, sql)
				return pgconn.NewCommandTag("CREATE TABLE"), nil
			},
		}

		require.NoError(t, New(db).EnsureOutboxHistoryPartitions(context.Background(), 1))
		require.Len(t, queries, 2)
		require.NotEqual(t, queries[0], queries[1])
	})

	t.Run("ensure outbox history partitions wraps exec errors", func(t *testing.T) {
		t.Parallel()

		execErr := errors.New("create failed")
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, execErr
			},
		}

		err := New(db).EnsureOutboxHistoryPartitions(context.Background(), 0)
		require.ErrorContains(t, err, "ensure outbox history partition enqueue_outbox_history_p")
		require.ErrorIs(t, err, execErr)
	})
}
