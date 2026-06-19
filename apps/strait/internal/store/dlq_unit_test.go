package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func dlqRunScanFn(now time.Time, includeOptional bool) func(dest ...any) error {
	return func(dest ...any) error {
		fillRunScanDest(dest, now, includeOptional)
		*(dest[3].(*domain.RunStatus)) = domain.StatusDeadLetter
		return nil
	}
}

func TestDLQListAndDepthUnit(t *testing.T) {
	t.Parallel()

	t.Run("lists dead-letter runs with default limit and cursor", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		cursor := now.Add(-time.Hour)
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "COALESCE(s.status, jr.status) = 'dead_letter'")
				require.Contains(t, sql, "jr.created_at < $2")
				require.Equal(t, []any{"project-1", &cursor, 50}, args)
				return &mockRows{scanFns: []func(dest ...any) error{dlqRunScanFn(now, true)}}, nil
			},
		}

		got, err := New(db).ListDeadLetterRuns(context.Background(), "project-1", 0, &cursor)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, domain.StatusDeadLetter, got[0].Status)
		require.Equal(t, "replay-1", got[0].ReplayedRunID)
	})

	t.Run("filtered list normalizes optional filters", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		jobID := "job-1"
		emptyJobID := ""
		masked := true
		cursor := now.Add(-time.Minute)
		calls := 0
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				calls++
				require.Contains(t, sql, "$2::text IS NULL OR jr.job_id = $2::text")
				require.Contains(t, sql, "$3::bool IS NULL")
				require.Contains(t, sql, "$4::timestamptz IS NULL")
				if calls == 1 {
					require.Equal(t, []any{"project-1", "job-1", true, cursor, 25}, args)
				} else {
					require.Nil(t, args[1])
					require.Nil(t, args[2])
					require.Nil(t, args[3])
					require.Equal(t, 50, args[4])
				}
				return &mockRows{scanFns: []func(dest ...any) error{dlqRunScanFn(now, false)}}, nil
			},
		}
		q := New(db)

		got, err := q.ListDeadLetterRunsFiltered(context.Background(), "project-1", &jobID, &masked, 25, &cursor)
		require.NoError(t, err)
		require.Len(t, got, 1)

		got, err = q.ListDeadLetterRunsFiltered(context.Background(), "project-1", &emptyJobID, nil, 0, nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("list methods map query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			filtered   bool
			queryErr   error
			scanErr    error
			rowsErr    error
			wantString string
		}{
			{name: "list query", queryErr: errors.New("query failed"), wantString: "list dead letter runs"},
			{name: "list scan", scanErr: errors.New("scan failed"), wantString: "list dead letter runs scan"},
			{name: "list rows", rowsErr: errors.New("rows failed"), wantString: "list dead letter runs rows"},
			{name: "filtered query", filtered: true, queryErr: errors.New("query failed"), wantString: "list dead letter runs filtered"},
			{name: "filtered scan", filtered: true, scanErr: errors.New("scan failed"), wantString: "list dead letter runs filtered scan"},
			{name: "filtered rows", filtered: true, rowsErr: errors.New("rows failed"), wantString: "list dead letter runs filtered rows"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tt.queryErr != nil {
							return nil, tt.queryErr
						}
						rows := &mockRows{err: tt.rowsErr}
						if tt.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error { return tt.scanErr }}
						}
						return rows, nil
					},
				}
				if tt.filtered {
					_, err := New(db).ListDeadLetterRunsFiltered(context.Background(), "project-1", nil, nil, 10, nil)
					require.ErrorContains(t, err, tt.wantString)
					return
				}
				_, err := New(db).ListDeadLetterRuns(context.Background(), "project-1", 10, nil)
				require.ErrorContains(t, err, tt.wantString)
			})
		}
	})

	t.Run("lists dlq depth by job", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "HAVING COUNT(*) >= j.dlq_alert_threshold")
				require.Empty(t, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "job-1"
						*(dest[1].(*string)) = "https://example.com/webhook"
						*(dest[2].(*int)) = 5
						*(dest[3].(*int)) = 3
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListDLQDepthByJob(context.Background())
		require.NoError(t, err)
		require.Equal(t, []DLQJobDepth{{
			JobID:             "job-1",
			WebhookURL:        "https://example.com/webhook",
			DLQCount:          5,
			DLQAlertThreshold: 3,
		}}, got)
	})
}

func TestDLQUnmaskPurgeAndMarkUnit(t *testing.T) {
	t.Parallel()

	t.Run("unmasks and purges guarded dead-letter rows", func(t *testing.T) {
		t.Parallel()

		queries := 0
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				queries++
				require.Equal(t, []any{"run-1"}, args)
				switch {
				case strings.Contains(sql, "job_run_visibility_events"):
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*string)) = "run-1"
						return nil
					}}
				case strings.Contains(sql, "deleted_run"):
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*string)) = "run-1"
						return nil
					}}
				default:
					require.Failf(t, "unexpected query", "sql=%s", sql)
					return &mockRow{}
				}
			},
		}
		q := New(db)

		require.NoError(t, q.UnmaskDLQRun(context.Background(), "run-1"))
		require.NoError(t, q.PurgeDLQRun(context.Background(), "run-1"))
		require.Equal(t, 2, queries)
	})

	t.Run("unmask and purge disambiguate empty returning", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			purge      bool
			loadStatus domain.RunStatus
			loadErr    error
			wantErr    error
			wantString string
		}{
			{name: "unmask not found", loadErr: pgx.ErrNoRows, wantErr: ErrRunNotFound},
			{name: "unmask conflict", loadStatus: domain.StatusCompleted, wantErr: ErrRunConflict, wantString: "expected dead_letter"},
			{name: "purge not found", purge: true, loadErr: pgx.ErrNoRows, wantErr: ErrRunNotFound},
			{name: "purge conflict", purge: true, loadStatus: domain.StatusQueued, wantErr: ErrRunConflict, wantString: "expected dead_letter"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				call := 0
				db := &mockDBTX{
					queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
						call++
						if call == 1 {
							return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
						}
						return &mockRow{scanFn: func(dest ...any) error {
							if tt.loadErr != nil {
								return tt.loadErr
							}
							*(dest[0].(*domain.RunStatus)) = tt.loadStatus
							*(dest[1].(*int)) = 2
							return nil
						}}
					},
				}
				var err error
				if tt.purge {
					err = New(db).PurgeDLQRun(context.Background(), "run-1")
				} else {
					err = New(db).UnmaskDLQRun(context.Background(), "run-1")
				}
				require.ErrorIs(t, err, tt.wantErr)
				if tt.wantString != "" {
					require.ErrorContains(t, err, tt.wantString)
				}
			})
		}
	})

	t.Run("marks run replayed with stamp-once guard", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "UPDATE job_runs SET replayed_run_id")
				require.Equal(t, []any{"replay-1", "run-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = "run-1"
					return nil
				}}
			},
		}
		require.NoError(t, New(db).MarkRunReplayed(context.Background(), "run-1", "replay-1"))

		calls := 0
		db.queryRowFn = func(_ context.Context, _ string, _ ...any) pgx.Row {
			calls++
			if calls == 1 {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			}
			return &mockRow{scanFn: func(dest ...any) error {
				existing := "replay-1"
				*(dest[0].(**string)) = &existing
				return nil
			}}
		}
		err := New(db).MarkRunReplayed(context.Background(), "run-1", "replay-2")
		require.ErrorIs(t, err, ErrRunConflict)
		require.ErrorContains(t, err, "already has replayed_run_id")

		calls = 0
		db.queryRowFn = func(_ context.Context, _ string, _ ...any) pgx.Row {
			calls++
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		err = New(db).MarkRunReplayed(context.Background(), "missing", "replay-1")
		require.ErrorIs(t, err, ErrRunNotFound)
	})
}

func TestDLQBulkReplayUnit(t *testing.T) {
	t.Parallel()

	t.Run("validates selector arguments", func(t *testing.T) {
		t.Parallel()

		q := New(&mockDBTX{})
		_, err := q.BulkReplayDeadLetterRuns(context.Background(), nil, "", 0)
		require.ErrorContains(t, err, "at least one run id or project_id")
		_, err = q.BulkReplayDeadLetterRuns(context.Background(), []string{"run-1"}, "project-1", 0)
		require.ErrorContains(t, err, "provide either run_ids or project_id")
	})

	t.Run("selects project runs with default limit and reports none available", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "ORDER BY jr.created_at ASC")
				require.Equal(t, []any{"project-1", 100}, args)
				return &mockRows{}, nil
			},
		}

		_, err := New(db).BulkReplayDeadLetterRuns(context.Background(), nil, "project-1", 0)
		require.ErrorContains(t, err, "no dead_letter runs available for replay")
	})

	t.Run("project selection maps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowsErr    error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "select dead letter runs for bulk replay"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan dead letter run id for bulk replay"},
			{name: "rows", rowsErr: errors.New("rows failed"), wantString: "iterate dead letter run ids for bulk replay"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tt.queryErr != nil {
							return nil, tt.queryErr
						}
						rows := &mockRows{err: tt.rowsErr}
						if tt.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error { return tt.scanErr }}
						}
						return rows, nil
					},
				}
				_, err := New(db).BulkReplayDeadLetterRuns(context.Background(), nil, "project-1", 10)
				require.ErrorContains(t, err, tt.wantString)
			})
		}
	})
}
