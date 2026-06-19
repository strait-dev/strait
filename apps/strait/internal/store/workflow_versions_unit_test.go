package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func fillWorkflowVersionDest(dest []any, id string, createdAt time.Time) {
	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "workflow-1"
	*(dest[2].(*int)) = 2
	*(dest[3].(*string)) = "project-1"
	*(dest[4].(*string)) = "Deploy"
	*(dest[5].(*string)) = "deploy"
	*(dest[6].(*string)) = "deploy workflow"
	*(dest[7].(*bool)) = true
	*(dest[8].(*int)) = 300
	*(dest[9].(*int)) = 4
	*(dest[10].(*int)) = 2
	*(dest[11].(*string)) = "*/5 * * * *"
	*(dest[12].(*string)) = "UTC"
	*(dest[13].(*bool)) = true
	*(dest[14].(*string)) = "version-id-2"
	*(dest[15].(*string)) = "user-a"
	*(dest[16].(*string)) = "user-b"
	*(dest[17].(*time.Time)) = createdAt
}

func requireWorkflowVersion(t *testing.T, got *domain.WorkflowVersion, id string, createdAt time.Time) {
	t.Helper()

	require.Equal(t, id, got.ID)
	require.Equal(t, "workflow-1", got.WorkflowID)
	require.Equal(t, 2, got.Version)
	require.Equal(t, "project-1", got.ProjectID)
	require.Equal(t, "Deploy", got.Name)
	require.Equal(t, "deploy", got.Slug)
	require.Equal(t, "deploy workflow", got.Description)
	require.True(t, got.Enabled)
	require.Equal(t, 300, got.TimeoutSecs)
	require.Equal(t, 4, got.MaxConcurrentRuns)
	require.Equal(t, 2, got.MaxParallelSteps)
	require.Equal(t, "*/5 * * * *", got.Cron)
	require.Equal(t, "UTC", got.CronTimezone)
	require.True(t, got.SkipIfRunning)
	require.Equal(t, "version-id-2", got.VersionID)
	require.Equal(t, "user-a", got.CreatedBy)
	require.Equal(t, "user-b", got.UpdatedBy)
	require.Equal(t, createdAt, got.CreatedAt)
}

func TestWorkflowVersionSnapshotID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "workflow-1:v7", workflowVersionSnapshotID("workflow-1", 7))
}

func TestCreateWorkflowVersionSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("executes version and step snapshot sequence", func(t *testing.T) {
		t.Parallel()

		var seen []string
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				switch {
				case strings.Contains(sql, "INSERT INTO workflow_versions"):
					seen = append(seen, "version")
					require.Equal(t, []any{"workflow-1:v2", "workflow-1", 2}, args)
					return pgconn.NewCommandTag("INSERT 0 1"), nil
				case strings.Contains(sql, "DELETE FROM workflow_version_steps"):
					seen = append(seen, "clear")
					require.Equal(t, []any{"workflow-1:v2"}, args)
					return pgconn.NewCommandTag("DELETE 3"), nil
				case strings.Contains(sql, "INSERT INTO workflow_version_steps"):
					seen = append(seen, "steps")
					require.Equal(t, []any{"workflow-1:v2", "workflow-1"}, args)
					return pgconn.NewCommandTag("INSERT 0 3"), nil
				default:
					require.Failf(t, "unexpected SQL", "%s", sql)
					return pgconn.CommandTag{}, nil
				}
			},
		}

		require.NoError(t, New(db).CreateWorkflowVersionSnapshot(context.Background(), "workflow-1", 2))
		require.Equal(t, []string{"version", "clear", "steps"}, seen)
	})

	t.Run("maps empty version insert to workflow not found", func(t *testing.T) {
		t.Parallel()

		var execs int
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				execs++
				return pgconn.NewCommandTag("INSERT 0 0"), nil
			},
		}

		err := New(db).CreateWorkflowVersionSnapshot(context.Background(), "missing", 1)
		require.ErrorIs(t, err, ErrWorkflowNotFound)
		require.Equal(t, 1, execs)
	})

	t.Run("wraps each exec failure", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			failAt     int
			wantString string
		}{
			{name: "version", failAt: 1, wantString: "insert workflow version snapshot"},
			{name: "clear", failAt: 2, wantString: "clear workflow version steps"},
			{name: "steps", failAt: 3, wantString: "insert workflow version steps snapshot"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				execErr := errors.New("exec failed")
				var execs int
				db := &mockDBTX{
					execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						execs++
						if execs == tc.failAt {
							return pgconn.CommandTag{}, execErr
						}
						return pgconn.NewCommandTag("INSERT 0 1"), nil
					},
				}

				err := New(db).CreateWorkflowVersionSnapshot(context.Background(), "workflow-1", 1)
				require.ErrorContains(t, err, tc.wantString)
				require.ErrorIs(t, err, execErr)
			})
		}
	})
}

func TestListStepsByWorkflowVersion(t *testing.T) {
	t.Parallel()

	t.Run("scans steps and wraps row errors", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		rowErr := errors.New("rows failed")
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "JOIN workflow_versions")
				require.Equal(t, []any{"workflow-1", 2}, args)
				return &mockRows{
					scanFns: []func(dest ...any) error{
						func(dest ...any) error {
							fillWorkflowStepDest(dest, "step-1", createdAt)
							return nil
						},
					},
					err: rowErr,
				}, nil
			},
		}

		_, err := New(db).ListStepsByWorkflowVersion(context.Background(), "workflow-1", 2)
		require.ErrorContains(t, err, "list steps by workflow version rows")
		require.ErrorIs(t, err, rowErr)
	})

	t.Run("wraps query and scan errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list steps by workflow version"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "list steps by workflow version scan"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tc.queryErr != nil {
							return nil, tc.queryErr
						}
						return &mockRows{scanFns: []func(dest ...any) error{
							func(...any) error {
								return tc.scanErr
							},
						}}, nil
					},
				}

				_, err := New(db).ListStepsByWorkflowVersion(context.Background(), "workflow-1", 2)
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})
}

func TestCountRunningWorkflowRuns(t *testing.T) {
	t.Parallel()

	t.Run("scans count", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "status = 'running'")
				require.Equal(t, []any{"workflow-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 3
					return nil
				}}
			},
		}

		got, err := New(db).CountRunningWorkflowRuns(context.Background(), "workflow-1")
		require.NoError(t, err)
		require.Equal(t, 3, got)
	})

	t.Run("wraps scan errors", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("scan failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return scanErr
				}}
			},
		}

		_, err := New(db).CountRunningWorkflowRuns(context.Background(), "workflow-1")
		require.ErrorContains(t, err, "count running workflow runs")
		require.ErrorIs(t, err, scanErr)
	})
}

func TestWorkflowVersionLookups(t *testing.T) {
	t.Parallel()

	t.Run("list scans versions and wraps row errors", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		rowErr := errors.New("rows failed")
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "ORDER BY version DESC")
				require.Equal(t, []any{"workflow-1", 10}, args)
				return &mockRows{
					scanFns: []func(dest ...any) error{
						func(dest ...any) error {
							fillWorkflowVersionDest(dest, "workflow-1:v2", createdAt)
							return nil
						},
					},
					err: rowErr,
				}, nil
			},
		}

		_, err := New(db).ListWorkflowVersions(context.Background(), "workflow-1", 10)
		require.ErrorContains(t, err, "list workflow versions rows")
		require.ErrorIs(t, err, rowErr)
	})

	t.Run("list wraps query and scan errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list workflow versions"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "scan workflow version"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tc.queryErr != nil {
							return nil, tc.queryErr
						}
						return &mockRows{scanFns: []func(dest ...any) error{
							func(...any) error {
								return tc.scanErr
							},
						}}, nil
					},
				}

				_, err := New(db).ListWorkflowVersions(context.Background(), "workflow-1", 10)
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})

	t.Run("get by numeric version scans row", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "WHERE workflow_id = $1 AND version = $2")
				require.Equal(t, []any{"workflow-1", 2}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					fillWorkflowVersionDest(dest, "workflow-1:v2", createdAt)
					return nil
				}}
			},
		}

		got, err := New(db).GetWorkflowVersion(context.Background(), "workflow-1", 2)
		require.NoError(t, err)
		requireWorkflowVersion(t, got, "workflow-1:v2", createdAt)
	})

	t.Run("get by numeric version maps missing and wraps errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			scanErr error
			wantIs  error
		}{
			{name: "missing", scanErr: pgx.ErrNoRows, wantIs: ErrWorkflowNotFound},
			{name: "scan error", scanErr: errors.New("scan failed")},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: func(...any) error {
							return tc.scanErr
						}}
					},
				}

				_, err := New(db).GetWorkflowVersion(context.Background(), "workflow-1", 2)
				if tc.wantIs != nil {
					require.ErrorIs(t, err, tc.wantIs)
					return
				}
				require.ErrorContains(t, err, "get workflow version")
				require.ErrorIs(t, err, tc.scanErr)
			})
		}
	})

	t.Run("get by version id scans row", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "WHERE workflow_id = $1 AND version_id = $2")
				require.Equal(t, []any{"workflow-1", "version-id-2"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					fillWorkflowVersionDest(dest, "workflow-1:v2", createdAt)
					return nil
				}}
			},
		}

		got, err := New(db).GetWorkflowVersionByVersionID(context.Background(), "workflow-1", "version-id-2")
		require.NoError(t, err)
		requireWorkflowVersion(t, got, "workflow-1:v2", createdAt)
	})

	t.Run("get by version id maps missing and wraps errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			scanErr error
			wantIs  error
		}{
			{name: "missing", scanErr: pgx.ErrNoRows, wantIs: ErrWorkflowVersionNotFound},
			{name: "scan error", scanErr: errors.New("scan failed")},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: func(...any) error {
							return tc.scanErr
						}}
					},
				}

				_, err := New(db).GetWorkflowVersionByVersionID(context.Background(), "workflow-1", "version-id-2")
				if tc.wantIs != nil {
					require.ErrorIs(t, err, tc.wantIs)
					return
				}
				require.ErrorContains(t, err, "get workflow version by version_id")
				require.ErrorIs(t, err, tc.scanErr)
			})
		}
	})
}

func TestListTimedOutWorkflowRuns(t *testing.T) {
	t.Parallel()

	t.Run("scans runs and wraps row errors", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		rowErr := errors.New("rows failed")
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "expires_at <= NOW()")
				require.Empty(t, args)
				return &mockRows{
					scanFns: []func(dest ...any) error{
						func(dest ...any) error {
							fillWorkflowRunDest(dest, "workflow-run-1", createdAt)
							return nil
						},
					},
					err: rowErr,
				}, nil
			},
		}

		_, err := New(db).ListTimedOutWorkflowRuns(context.Background())
		require.ErrorContains(t, err, "list timed out workflow runs rows")
		require.ErrorIs(t, err, rowErr)
	})

	t.Run("wraps query and scan errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list timed out workflow runs"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "list timed out workflow runs scan"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tc.queryErr != nil {
							return nil, tc.queryErr
						}
						return &mockRows{scanFns: []func(dest ...any) error{
							func(...any) error {
								return tc.scanErr
							},
						}}, nil
					},
				}

				_, err := New(db).ListTimedOutWorkflowRuns(context.Background())
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})
}
