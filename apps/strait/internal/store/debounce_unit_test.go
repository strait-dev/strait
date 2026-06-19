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

func fillDebouncePendingDest(dest []any, id string, createdAt time.Time, withOptionalFields bool) {
	fireAt := createdAt.Add(time.Minute)
	tags := []byte(`{"tags":["a"]}`)

	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "job-1"
	*(dest[2].(*string)) = "project-1"
	*(dest[3].(*string)) = "debounce-key"
	*(dest[4].(*json.RawMessage)) = json.RawMessage(`{"payload":true}`)
	*(dest[5].(*[]byte)) = tags
	*(dest[6].(*int)) = 7
	if withOptionalFields {
		concurrencyKey := "concurrency-1"
		ttlSecs := 60
		createdBy := "user-1"
		*(dest[7].(**string)) = &concurrencyKey
		*(dest[8].(**int)) = &ttlSecs
		*(dest[10].(**string)) = &createdBy
	}
	*(dest[9].(*string)) = "api"
	*(dest[11].(*time.Time)) = fireAt
	*(dest[12].(*time.Time)) = createdAt
}

func TestDebouncePendingStore(t *testing.T) {
	t.Parallel()

	t.Run("upserts debounce pending with generated id and nullable fields", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		var capturedArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO debounce_pending")
				require.Contains(t, sql, "ON CONFLICT")
				capturedArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = args[0].(string)
					*(dest[1].(*time.Time)) = createdAt
					return nil
				}}
			},
		}
		d := &domain.DebouncePending{
			JobID:       "job-1",
			ProjectID:   "project-1",
			DebounceKey: "debounce-key",
			Payload:     json.RawMessage(`{"payload":true}`),
			Tags:        json.RawMessage(`{"tags":["a"]}`),
			Priority:    7,
			TriggeredBy: "api",
			FireAt:      createdAt.Add(time.Minute),
		}

		require.NoError(t, New(db).UpsertDebouncePending(context.Background(), d))
		require.NotEmpty(t, d.ID)
		require.Equal(t, createdAt, d.CreatedAt)
		require.Equal(t, d.ID, capturedArgs[0])
		require.Nil(t, capturedArgs[7])
		require.Nil(t, capturedArgs[10])

		scanErr := errors.New("upsert failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return scanErr }}
		}
		err := New(db).UpsertDebouncePending(context.Background(), d)
		require.ErrorContains(t, err, "upsert debounce pending")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("upserts debounce pending preserves custom id and optional fields", func(t *testing.T) {
		t.Parallel()

		ttlSecs := 60
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
				require.Equal(t, "debounce-1", args[0])
				require.Equal(t, "concurrency-1", *args[7].(*string))
				require.Equal(t, &ttlSecs, args[8])
				require.Equal(t, "user-1", *args[10].(*string))
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = "debounce-1"
					*(dest[1].(*time.Time)) = time.Now().UTC()
					return nil
				}}
			},
		}
		d := &domain.DebouncePending{
			ID:             "debounce-1",
			ConcurrencyKey: "concurrency-1",
			TTLSecs:        &ttlSecs,
			CreatedBy:      "user-1",
		}

		require.NoError(t, New(db).UpsertDebouncePending(context.Background(), d))
		require.Equal(t, "debounce-1", d.ID)
	})

	t.Run("lists due debounce pending and scans optional fields", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "row_number() OVER")
				require.Empty(t, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillDebouncePendingDest(dest, "debounce-1", createdAt, true)
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListDueDebouncePending(context.Background())
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "debounce-1", got[0].ID)
		require.Equal(t, "concurrency-1", got[0].ConcurrencyKey)
		require.Equal(t, "user-1", got[0].CreatedBy)
		require.NotNil(t, got[0].TTLSecs)
		require.JSONEq(t, `{"tags":["a"]}`, string(got[0].Tags))
	})

	t.Run("list due debounce pending maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list due debounce pending"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "list due debounce pending scan"},
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

				_, err := New(db).ListDueDebouncePending(context.Background())
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})

	t.Run("claims due debounce pending", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "fire_at <= NOW()")
				require.Equal(t, []any{"debounce-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					fillDebouncePendingDest(dest, "debounce-1", createdAt, false)
					return nil
				}}
			},
		}

		got, ok, err := New(db).ClaimDueDebouncePending(context.Background(), "debounce-1")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "debounce-1", got.ID)
		require.Empty(t, got.ConcurrencyKey)
		require.Empty(t, got.CreatedBy)
		require.Nil(t, got.TTLSecs)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		got, ok, err = New(db).ClaimDueDebouncePending(context.Background(), "missing")
		require.NoError(t, err)
		require.False(t, ok)
		require.Nil(t, got)

		scanErr := errors.New("claim failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return scanErr }}
		}
		_, _, err = New(db).ClaimDueDebouncePending(context.Background(), "debounce-1")
		require.ErrorContains(t, err, "claim due debounce pending")
		require.ErrorIs(t, err, scanErr)
	})
}

func TestDebouncePendingMutations(t *testing.T) {
	t.Parallel()

	t.Run("deletes debounce pending", func(t *testing.T) {
		t.Parallel()

		var capturedArgs []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "DELETE FROM debounce_pending")
				capturedArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("DELETE 1"), nil
			},
		}
		require.NoError(t, New(db).DeleteDebouncePending(context.Background(), "debounce-1"))
		require.Equal(t, []any{"debounce-1"}, capturedArgs)

		execErr := errors.New("delete failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		err := New(db).DeleteDebouncePending(context.Background(), "debounce-1")
		require.ErrorContains(t, err, "delete debounce pending")
		require.ErrorIs(t, err, execErr)
	})

	t.Run("complete and reschedule report affected rows", func(t *testing.T) {
		t.Parallel()

		fireAt := time.Now().UTC()
		nextFireAt := fireAt.Add(time.Minute)
		tests := []struct {
			name string
			call func(*Queries) (bool, error)
		}{
			{
				name: "complete",
				call: func(q *Queries) (bool, error) {
					return q.CompleteDebouncePending(context.Background(), "debounce-1", fireAt)
				},
			},
			{
				name: "reschedule",
				call: func(q *Queries) (bool, error) {
					return q.RescheduleDebouncePending(context.Background(), "debounce-1", fireAt, nextFireAt)
				},
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("UPDATE 1"), nil
					},
				}
				ok, err := tc.call(New(db))
				require.NoError(t, err)
				require.True(t, ok)

				db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 0"), nil
				}
				ok, err = tc.call(New(db))
				require.NoError(t, err)
				require.False(t, ok)

				execErr := errors.New("exec failed")
				db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, execErr
				}
				_, err = tc.call(New(db))
				require.ErrorContains(t, err, tc.name+" debounce pending")
				require.ErrorIs(t, err, execErr)
			})
		}
	})

	t.Run("inserts debounce pending if absent with generated id and nullable fields", func(t *testing.T) {
		t.Parallel()

		var capturedArgs []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "INSERT INTO debounce_pending")
				require.Contains(t, sql, "ON CONFLICT")
				capturedArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		}
		d := &domain.DebouncePending{}

		ok, err := New(db).InsertDebouncePendingIfAbsent(context.Background(), d)
		require.NoError(t, err)
		require.True(t, ok)
		require.NotEmpty(t, d.ID)
		require.Equal(t, d.ID, capturedArgs[0])
		require.Nil(t, capturedArgs[7])
		require.Nil(t, capturedArgs[10])

		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 0"), nil
		}
		ok, err = New(db).InsertDebouncePendingIfAbsent(context.Background(), d)
		require.NoError(t, err)
		require.False(t, ok)

		execErr := errors.New("insert failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		_, err = New(db).InsertDebouncePendingIfAbsent(context.Background(), d)
		require.ErrorContains(t, err, "insert debounce pending if absent")
		require.ErrorIs(t, err, execErr)
	})

	t.Run("nilIfEmpty returns nil only for empty strings", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, nilIfEmpty(""))
		got := nilIfEmpty("value")
		require.NotNil(t, got)
		require.Equal(t, "value", *got)
	})
}
