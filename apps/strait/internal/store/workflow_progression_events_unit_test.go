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

func fillWorkflowProgressionEventDest(dest []any, id int64, createdAt time.Time) {
	*(dest[0].(*int64)) = id
	*(dest[1].(*string)) = "workflow-run-1"
	*(dest[2].(*string)) = "step-run-1"
	*(dest[3].(*string)) = "step-a"
	*(dest[4].(*string)) = "completed"
	*(dest[5].(*int)) = 2
	*(dest[6].(*time.Time)) = createdAt
}

func TestWorkflowProgressionEventStore(t *testing.T) {
	t.Parallel()

	t.Run("creates workflow progression event", func(t *testing.T) {
		t.Parallel()

		var capturedArgs []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "INSERT INTO workflow_progression_events")
				require.Contains(t, sql, "ON CONFLICT")
				capturedArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		}

		err := New(db).CreateWorkflowProgressionEvent(context.Background(), "workflow-run-1", "step-run-1", "step-a", "completed")
		require.NoError(t, err)
		require.Equal(t, []any{"workflow-run-1", "step-run-1", "step-a", "completed"}, capturedArgs)

		execErr := errors.New("insert failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		err = New(db).CreateWorkflowProgressionEvent(context.Background(), "workflow-run-1", "step-run-1", "step-a", "completed")
		require.ErrorContains(t, err, "create workflow progression event")
		require.ErrorIs(t, err, execErr)
	})

	t.Run("claims workflow progression events with default limit", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "workflow_progression_event_claims")
				require.Contains(t, sql, "GREATEST($1::int - COUNT(*)")
				require.Equal(t, []any{100}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillWorkflowProgressionEventDest(dest, 11, createdAt)
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ClaimWorkflowProgressionEvents(context.Background(), 0)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, int64(11), got[0].ID)
		require.Equal(t, "workflow-run-1", got[0].WorkflowRunID)
		require.Equal(t, "step-run-1", got[0].StepRunID)
		require.Equal(t, "step-a", got[0].StepRef)
		require.Equal(t, "completed", got[0].Status)
		require.Equal(t, 2, got[0].Attempts)
		require.Equal(t, createdAt, got[0].CreatedAt)
	})

	t.Run("claim workflow progression events maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "claim workflow progression events"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan workflow progression event"},
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

				_, err := New(db).ClaimWorkflowProgressionEvents(context.Background(), 5)
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})

	t.Run("marks workflow progression events processed", func(t *testing.T) {
		t.Parallel()

		var captured []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "workflow_progression_event_processed")
				require.Contains(t, sql, "DELETE FROM workflow_progression_event_claims")
				captured = append([]any(nil), args...)
				return pgconn.NewCommandTag("DELETE 1"), nil
			},
		}
		q := New(db)

		require.NoError(t, q.MarkWorkflowProgressionEventProcessed(context.Background(), 11))
		require.Equal(t, []any{int64(11)}, captured)
		require.NoError(t, q.MarkWorkflowProgressionEventsProcessed(context.Background(), nil))
		require.NoError(t, q.MarkWorkflowProgressionEventsProcessed(context.Background(), []int64{11, 12}))
		require.Equal(t, []any{[]int64{11, 12}}, captured)

		execErr := errors.New("process failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		err := q.MarkWorkflowProgressionEventProcessed(context.Background(), 11)
		require.ErrorContains(t, err, "mark workflow progression event processed")
		require.ErrorIs(t, err, execErr)
		err = q.MarkWorkflowProgressionEventsProcessed(context.Background(), []int64{11})
		require.ErrorContains(t, err, "mark workflow progression events processed")
		require.ErrorIs(t, err, execErr)
	})

	t.Run("releases workflow progression events", func(t *testing.T) {
		t.Parallel()

		var captured []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "DELETE FROM workflow_progression_event_claims")
				require.Contains(t, sql, "processed_at IS NULL")
				captured = append([]any(nil), args...)
				return pgconn.NewCommandTag("DELETE 1"), nil
			},
		}
		q := New(db)

		require.NoError(t, q.ReleaseWorkflowProgressionEvent(context.Background(), 11))
		require.Equal(t, []any{int64(11)}, captured)
		require.NoError(t, q.ReleaseWorkflowProgressionEvents(context.Background(), nil))
		require.NoError(t, q.ReleaseWorkflowProgressionEvents(context.Background(), []int64{11, 12}))
		require.Equal(t, []any{[]int64{11, 12}}, captured)

		execErr := errors.New("release failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		err := q.ReleaseWorkflowProgressionEvent(context.Background(), 11)
		require.ErrorContains(t, err, "release workflow progression event")
		require.ErrorIs(t, err, execErr)
		err = q.ReleaseWorkflowProgressionEvents(context.Background(), []int64{11})
		require.ErrorContains(t, err, "release workflow progression events")
		require.ErrorIs(t, err, execErr)
	})

	t.Run("deletes processed workflow progression events with defaults", func(t *testing.T) {
		t.Parallel()

		var capturedArgs []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "DELETE FROM workflow_progression_events")
				require.Contains(t, sql, "workflow_progression_event_processed")
				capturedArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("DELETE 4"), nil
			},
		}

		got, err := New(db).DeleteProcessedWorkflowProgressionEvents(context.Background(), -time.Second, 0)
		require.NoError(t, err)
		require.Equal(t, int64(4), got)
		require.Equal(t, time.Duration(0), capturedArgs[0])
		require.Equal(t, 1000, capturedArgs[1])
	})

	t.Run("delete processed workflow progression events uses explicit arguments and wraps errors", func(t *testing.T) {
		t.Parallel()

		var capturedArgs []any
		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
				capturedArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("DELETE 2"), nil
			},
		}

		got, err := New(db).DeleteProcessedWorkflowProgressionEvents(context.Background(), time.Hour, 25)
		require.NoError(t, err)
		require.Equal(t, int64(2), got)
		require.Equal(t, []any{time.Hour, 25}, capturedArgs)

		execErr := errors.New("delete failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		_, err = New(db).DeleteProcessedWorkflowProgressionEvents(context.Background(), time.Hour, 25)
		require.ErrorContains(t, err, "delete processed workflow progression events")
		require.ErrorIs(t, err, execErr)
	})
}

func TestWorkflowProgressionEventSQLShape(t *testing.T) {
	t.Parallel()

	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			require.True(t, strings.Contains(sql, "stale_candidates") && strings.Contains(sql, "inserted"))
			return &mockRows{}, nil
		},
	}
	_, err := New(db).ClaimWorkflowProgressionEvents(context.Background(), 1)
	require.NoError(t, err)
}
