package store

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

func jobDependencyScanFn(dep domain.JobDependency) func(dest ...any) error {
	return func(dest ...any) error {
		*(dest[0].(*string)) = dep.ID
		*(dest[1].(*string)) = dep.JobID
		*(dest[2].(*string)) = dep.DependsOnJobID
		*(dest[3].(*string)) = dep.Condition
		*(dest[4].(*time.Time)) = dep.CreatedAt
		*(dest[5].(*int64)) = dep.CacheVersion
		return nil
	}
}

func TestCreateJobDependencyUnit(t *testing.T) {
	t.Parallel()

	t.Run("rejects self dependency before database call", func(t *testing.T) {
		t.Parallel()

		called := false
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				called = true
				return &mockRow{}
			},
		}

		dep := &domain.JobDependency{JobID: "job-1", DependsOnJobID: "job-1"}
		err := New(db).CreateJobDependency(context.Background(), dep)

		require.ErrorContains(t, err, "job cannot depend on itself")
		require.False(t, called)
		require.Empty(t, dep.ID)
	})

	t.Run("fills defaults and scans created version", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO job_dependencies")
				require.Contains(t, sql, "UPDATE jobs")
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = now
					*(dest[1].(*int64)) = 17
					return nil
				}}
			},
		}

		dep := &domain.JobDependency{JobID: "job-1", DependsOnJobID: "job-2"}
		require.NoError(t, New(db).CreateJobDependency(context.Background(), dep))

		require.NotEmpty(t, dep.ID)
		require.Equal(t, "completed", dep.Condition)
		require.Equal(t, now, dep.CreatedAt)
		require.Equal(t, int64(17), dep.CacheVersion)
		require.Equal(t, []any{dep.ID, "job-1", "job-2", "completed"}, args)
	})

	t.Run("preserves explicit condition and wraps scan error", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("insert failed")
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, gotArgs ...any) pgx.Row {
				require.Equal(t, "failed", gotArgs[3])
				return &mockRow{scanFn: func(...any) error { return scanErr }}
			},
		}

		dep := &domain.JobDependency{ID: "dep-1", JobID: "job-1", DependsOnJobID: "job-2", Condition: "failed"}
		err := New(db).CreateJobDependency(context.Background(), dep)

		require.ErrorIs(t, err, scanErr)
		require.ErrorContains(t, err, "create job dependency")
		require.Equal(t, "dep-1", dep.ID)
	})
}

func TestListJobDependenciesUnit(t *testing.T) {
	t.Parallel()

	t.Run("uses cursor and scans rows", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		cursor := now.Add(time.Minute)
		dep := domain.JobDependency{
			ID:             "dep-1",
			JobID:          "job-1",
			DependsOnJobID: "job-2",
			Condition:      "any",
			CreatedAt:      now,
			CacheVersion:   9,
		}
		var args []any
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, gotArgs ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "jd.created_at < $2")
				require.Contains(t, sql, "LIMIT $3")
				args = append([]any(nil), gotArgs...)
				return &mockRows{scanFns: []func(dest ...any) error{jobDependencyScanFn(dep)}}, nil
			},
		}

		deps, err := New(db).ListJobDependencies(context.Background(), "job-1", 25, &cursor)

		require.NoError(t, err)
		require.Equal(t, []domain.JobDependency{dep}, deps)
		require.Equal(t, []any{"job-1", cursor, 25}, args)
	})

	t.Run("wraps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			rows       pgx.Rows
			queryErr   error
			wantSubstr string
		}{
			{
				name:       "query",
				queryErr:   errors.New("query failed"),
				wantSubstr: "list job dependencies",
			},
			{
				name: "scan",
				rows: &mockRows{scanFns: []func(dest ...any) error{
					func(...any) error { return errors.New("scan failed") },
				}},
				wantSubstr: "list job dependencies scan",
			},
			{
				name:       "rows",
				rows:       &mockRows{err: errors.New("rows failed")},
				wantSubstr: "list job dependencies rows",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tt.queryErr != nil {
							return nil, tt.queryErr
						}
						return tt.rows, nil
					},
				}

				deps, err := New(db).ListJobDependencies(context.Background(), "job-1", 10, nil)

				require.Nil(t, deps)
				require.ErrorContains(t, err, tt.wantSubstr)
			})
		}
	})
}

func TestGetAndDeleteJobDependencyUnit(t *testing.T) {
	t.Parallel()

	t.Run("maps not found and wraps other get errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			scanErr error
			wantErr error
		}{
			{name: "not found", scanErr: pgx.ErrNoRows, wantErr: ErrJobDependencyNotFound},
			{name: "other", scanErr: errors.New("scan failed")},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
						require.Contains(t, sql, "FROM job_dependencies WHERE id = $1")
						require.Equal(t, []any{"dep-1"}, gotArgs)
						return &mockRow{scanFn: func(...any) error { return tt.scanErr }}
					},
				}

				dep, err := New(db).GetJobDependency(context.Background(), "dep-1")

				require.Nil(t, dep)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
				} else {
					require.ErrorContains(t, err, "get job dependency")
					require.ErrorIs(t, err, tt.scanErr)
				}
			})
		}
	})

	t.Run("deletes through cache-version bump query", func(t *testing.T) {
		t.Parallel()

		var args []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, gotArgs ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "DELETE FROM job_dependencies")
				require.Contains(t, sql, "UPDATE jobs")
				args = append([]any(nil), gotArgs...)
				return pgconn.CommandTag{}, nil
			},
		}

		require.NoError(t, New(db).DeleteJobDependency(context.Background(), "dep-1"))
		require.Equal(t, []any{"dep-1"}, args)
	})
}

func TestJobDependencyListVersionUnit(t *testing.T) {
	t.Parallel()

	t.Run("scans cache version", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				require.Contains(t, sql, "SELECT cache_version FROM jobs")
				require.Equal(t, []any{"job-1"}, gotArgs)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int64)) = 42
					return nil
				}}
			},
		}

		version, err := New(db).GetJobDependencyListVersion(context.Background(), "job-1")
		require.NoError(t, err)
		require.Equal(t, int64(42), version)
	})

	t.Run("maps not found and wraps other errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			scanErr error
			wantErr error
		}{
			{name: "not found", scanErr: pgx.ErrNoRows, wantErr: ErrJobNotFound},
			{name: "other", scanErr: errors.New("scan failed")},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: func(...any) error { return tt.scanErr }}
					},
				}

				version, err := New(db).GetJobDependencyListVersion(context.Background(), "job-1")

				require.Zero(t, version)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
				} else {
					require.ErrorContains(t, err, "get job dependency list version")
					require.ErrorIs(t, err, tt.scanErr)
				}
			})
		}
	})
}

func TestListDependentsByDependencyJobUnit(t *testing.T) {
	t.Parallel()

	t.Run("orders by created time and scans rows", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		dep := domain.JobDependency{
			ID:             "dep-1",
			JobID:          "job-1",
			DependsOnJobID: "job-2",
			Condition:      "completed",
			CreatedAt:      now,
			CacheVersion:   5,
		}
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, gotArgs ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "WHERE depends_on_job_id = $1")
				require.Contains(t, sql, "ORDER BY created_at DESC")
				require.Equal(t, []any{"job-2"}, gotArgs)
				return &mockRows{scanFns: []func(dest ...any) error{jobDependencyScanFn(dep)}}, nil
			},
		}

		deps, err := New(db).ListDependentsByDependencyJob(context.Background(), "job-2")

		require.NoError(t, err)
		require.Equal(t, []domain.JobDependency{dep}, deps)
	})

	t.Run("wraps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			rows       pgx.Rows
			queryErr   error
			wantSubstr string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantSubstr: "list dependents by dependency job"},
			{
				name: "scan",
				rows: &mockRows{scanFns: []func(dest ...any) error{
					func(...any) error { return errors.New("scan failed") },
				}},
				wantSubstr: "list dependents by dependency job scan",
			},
			{name: "rows", rows: &mockRows{err: errors.New("rows failed")}, wantSubstr: "list dependents by dependency job rows"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tt.queryErr != nil {
							return nil, tt.queryErr
						}
						return tt.rows, nil
					},
				}

				deps, err := New(db).ListDependentsByDependencyJob(context.Background(), "job-2")

				require.Nil(t, deps)
				require.ErrorContains(t, err, tt.wantSubstr)
			})
		}
	})
}

func TestListWaitingRunsByJobIDsUnit(t *testing.T) {
	t.Parallel()

	t.Run("returns nil without query for empty job list", func(t *testing.T) {
		t.Parallel()

		called := false
		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				called = true
				return nil, nil
			},
		}

		runs, err := New(db).ListWaitingRunsByJobIDs(context.Background(), nil, 10)

		require.NoError(t, err)
		require.Nil(t, runs)
		require.False(t, called)
	})

	t.Run("defaults non-positive limit and scans runs", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, gotArgs ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "status = 'waiting'")
				require.Contains(t, sql, "LIMIT $2")
				args = append([]any(nil), gotArgs...)
				return &mockRows{scanFns: []func(dest ...any) error{runScanFn(now, true)}}, nil
			},
		}

		runs, err := New(db).ListWaitingRunsByJobIDs(context.Background(), []string{"job-1"}, 0)

		require.NoError(t, err)
		require.Len(t, runs, 1)
		require.Equal(t, []any{[]string{"job-1"}, 1000}, args)
		require.Equal(t, "run-1", runs[0].ID)
	})
}

func TestAreJobDependenciesSatisfiedUnit(t *testing.T) {
	t.Parallel()

	t.Run("returns true with no dependencies", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{}, nil
			},
		}

		ok, err := New(db).AreJobDependenciesSatisfied(context.Background(), &domain.JobRun{JobID: "job-1"})

		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("evaluates dependency condition against latest terminal run", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		tests := []struct {
			name       string
			condition  string
			runStatus  domain.RunStatus
			wantOK     bool
			wantErr    string
			wantFilter string
			runMeta    map[string]string
			runIDKey   string
		}{
			{name: "completed satisfied", condition: "completed", runStatus: domain.StatusCompleted, wantOK: true, wantFilter: "idempotency_key = $2", runIDKey: "idem-1"},
			{name: "completed not satisfied", condition: "completed", runStatus: domain.StatusFailed, wantOK: false, wantFilter: "metadata->>'dependency_key' = $2", runMeta: map[string]string{"dependency_key": "batch-1"}},
			{name: "failed satisfied", condition: "failed", runStatus: domain.StatusTimedOut, wantOK: true},
			{name: "failed not satisfied by canceled", condition: "failed", runStatus: domain.StatusCanceled, wantOK: false},
			{name: "any satisfied by terminal", condition: "any", runStatus: domain.StatusDeadLetter, wantOK: true},
			{name: "unknown condition", condition: "mystery", runStatus: domain.StatusCompleted, wantErr: `unknown dependency condition "mystery"`},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				dep := domain.JobDependency{
					ID:             "dep-1",
					JobID:          "job-1",
					DependsOnJobID: "job-2",
					Condition:      tt.condition,
					CreatedAt:      now,
				}
				db := &mockDBTX{}
				db.queryFn = func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
					require.Contains(t, sql, "FROM job_dependencies")
					return &mockRows{scanFns: []func(dest ...any) error{jobDependencyScanFn(dep)}}, nil
				}
				db.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
					require.Contains(t, sql, "FROM job_runs")
					require.Equal(t, "job-2", args[0])
					if tt.wantFilter != "" {
						require.Contains(t, sql, tt.wantFilter)
					}
					return &mockRow{scanFn: func(dest ...any) error {
						fillRunScanDest(dest, now, true)
						*(dest[1].(*string)) = "job-2"
						*(dest[3].(*domain.RunStatus)) = tt.runStatus
						return nil
					}}
				}

				ok, err := New(db).AreJobDependenciesSatisfied(context.Background(), &domain.JobRun{
					JobID:          "job-1",
					IdempotencyKey: tt.runIDKey,
					Metadata:       tt.runMeta,
				})

				if tt.wantErr != "" {
					require.ErrorContains(t, err, tt.wantErr)
					require.False(t, ok)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tt.wantOK, ok)
			})
		}
	})
}

func TestFindLatestTerminalDependencyRunUnit(t *testing.T) {
	t.Parallel()

	t.Run("prefers idempotency key over dependency key", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "idempotency_key = $2")
				require.NotContains(t, sql, "metadata->>'dependency_key'")
				require.Equal(t, []any{"job-2", "idem-1"}, args)
				return &mockRow{scanFn: runScanFn(now, true)}
			},
		}

		run, err := New(db).findLatestTerminalDependencyRun(context.Background(), "job-2", "idem-1", "batch-1")

		require.NoError(t, err)
		require.NotNil(t, run)
		require.Equal(t, "run-1", run.ID)
	})

	t.Run("maps no rows to nil run", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.NotContains(t, sql, "$2")
				require.Equal(t, []any{"job-2"}, args)
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}

		run, err := New(db).findLatestTerminalDependencyRun(context.Background(), "job-2", "", "")

		require.NoError(t, err)
		require.Nil(t, run)
	})
}

func TestIsFailureTerminalStatusUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status domain.RunStatus
		want   bool
	}{
		{name: "failed", status: domain.StatusFailed, want: true},
		{name: "timed out", status: domain.StatusTimedOut, want: true},
		{name: "crashed", status: domain.StatusCrashed, want: true},
		{name: "system failed", status: domain.StatusSystemFailed, want: true},
		{name: "expired", status: domain.StatusExpired, want: true},
		{name: "dead letter", status: domain.StatusDeadLetter, want: true},
		{name: "canceled", status: domain.StatusCanceled, want: false},
		{name: "completed", status: domain.StatusCompleted, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, isFailureTerminalStatus(tt.status))
		})
	}
}
