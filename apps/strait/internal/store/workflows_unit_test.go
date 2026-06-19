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

type workflowScanFunc func(dest ...any) error

func (f workflowScanFunc) Scan(dest ...any) error {
	return f(dest...)
}

func fillWorkflowDest(dest []any, id string, createdAt time.Time) {
	description := "ships jobs"
	cron := "*/5 * * * *"
	cronTimezone := "UTC"
	versionID := "version-1"
	versionPolicy := string(domain.VersionPolicyLatest)
	createdBy := "user-create"
	updatedBy := "user-update"

	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "project-1"
	*(dest[2].(*string)) = "Deploy"
	*(dest[3].(*string)) = "deploy"
	*(dest[4].(**string)) = &description
	*(dest[5].(*bool)) = true
	*(dest[6].(*int)) = 3
	*(dest[7].(*int)) = 300
	*(dest[8].(*int)) = 4
	*(dest[9].(*int)) = 2
	*(dest[10].(**string)) = &cron
	*(dest[11].(**string)) = &cronTimezone
	*(dest[12].(*bool)) = true
	*(dest[13].(*[]byte)) = []byte(`{"team":"platform"}`)
	*(dest[14].(**string)) = &versionID
	*(dest[15].(**string)) = &versionPolicy
	*(dest[16].(*bool)) = true
	*(dest[17].(**string)) = &createdBy
	*(dest[18].(**string)) = &updatedBy
	*(dest[19].(*time.Time)) = createdAt
	*(dest[20].(*time.Time)) = createdAt.Add(time.Minute)
}

func testWorkflow() *domain.Workflow {
	return &domain.Workflow{
		ProjectID:           "project-1",
		Name:                "Deploy",
		Slug:                "deploy",
		Description:         "ships jobs",
		Tags:                map[string]string{"team": "platform"},
		Enabled:             true,
		TimeoutSecs:         300,
		MaxConcurrentRuns:   4,
		MaxParallelSteps:    2,
		Cron:                "*/5 * * * *",
		CronTimezone:        "UTC",
		SkipIfRunning:       true,
		BackwardsCompatible: true,
		CreatedBy:           "user-create",
		UpdatedBy:           "user-update",
	}
}

func TestCreateWorkflow(t *testing.T) {
	t.Parallel()

	t.Run("sets defaults and inserts with current project context", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		queryRows := 0
		var insertArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				queryRows++
				if strings.Contains(sql, "current_setting") {
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*string)) = "project-1"
						return nil
					}}
				}
				require.Contains(t, sql, "INSERT INTO workflows")
				insertArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = createdAt
					*(dest[1].(*time.Time)) = createdAt.Add(time.Second)
					*(dest[2].(*int)) = 1
					return nil
				}}
			},
		}
		wf := testWorkflow()

		require.NoError(t, New(db).CreateWorkflow(context.Background(), wf))
		require.NotEmpty(t, wf.ID)
		require.NotEmpty(t, wf.VersionID)
		require.Equal(t, domain.VersionPolicyPin, wf.VersionPolicy)
		require.Equal(t, 1, wf.Version)
		require.Equal(t, createdAt, wf.CreatedAt)
		require.Equal(t, 2, queryRows)
		require.Len(t, insertArgs, 18)
		require.Equal(t, "project-1", insertArgs[1])
		require.JSONEq(t, `{"team":"platform"}`, string(insertArgs[12].([]byte)))
	})

	t.Run("preserves provided id version and policy inputs", func(t *testing.T) {
		t.Parallel()

		var insertArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				if strings.Contains(sql, "current_setting") {
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*string)) = ""
						return nil
					}}
				}
				insertArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = time.Now().UTC()
					*(dest[1].(*time.Time)) = time.Now().UTC()
					*(dest[2].(*int)) = 1
					return nil
				}}
			},
		}
		wf := testWorkflow()
		wf.ID = "workflow-1"
		wf.VersionID = "version-custom"
		wf.VersionPolicy = domain.VersionPolicyLatest

		require.NoError(t, New(db).CreateWorkflow(context.Background(), wf))
		require.Equal(t, "workflow-1", wf.ID)
		require.Equal(t, "version-custom", wf.VersionID)
		require.Equal(t, domain.VersionPolicyLatest, wf.VersionPolicy)
		require.Equal(t, "version-custom", insertArgs[13])
		require.Equal(t, string(domain.VersionPolicyLatest), insertArgs[14])
	})

	t.Run("rejects mismatched project context before insert", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				require.Contains(t, sql, "current_setting")
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = "different-project"
					return nil
				}}
			},
		}

		err := New(db).CreateWorkflow(context.Background(), testWorkflow())
		require.ErrorIs(t, err, ErrProjectContextMismatch)
		require.ErrorContains(t, err, "create workflow")
	})
}

func TestWorkflowLookups(t *testing.T) {
	t.Parallel()

	t.Run("get workflow maps missing rows", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return pgx.ErrNoRows
				}}
			},
		}

		_, err := New(db).GetWorkflow(context.Background(), "missing")
		require.ErrorIs(t, err, ErrWorkflowNotFound)
	})

	t.Run("get workflow by slug maps scan errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return errors.New("scan failed")
				}}
			},
		}

		_, err := New(db).GetWorkflowBySlug(context.Background(), "project-1", "deploy")
		require.ErrorContains(t, err, "get workflow by slug")
		require.ErrorContains(t, err, "scan failed")
	})

	t.Run("get workflow scans optional fields", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "FROM workflows")
				require.Equal(t, []any{"workflow-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					fillWorkflowDest(dest, "workflow-1", createdAt)
					return nil
				}}
			},
		}

		got, err := New(db).GetWorkflow(context.Background(), "workflow-1")
		require.NoError(t, err)
		require.Equal(t, "workflow-1", got.ID)
		require.Equal(t, "ships jobs", got.Description)
		require.Equal(t, "*/5 * * * *", got.Cron)
		require.Equal(t, "UTC", got.CronTimezone)
		require.Equal(t, domain.VersionPolicyLatest, got.VersionPolicy)
		require.Equal(t, "platform", got.Tags["team"])
		require.Equal(t, "user-create", got.CreatedBy)
		require.Equal(t, "user-update", got.UpdatedBy)
	})
}

func TestWorkflowListQueries(t *testing.T) {
	t.Parallel()

	t.Run("list workflows applies cursor and limit", func(t *testing.T) {
		t.Parallel()

		cursor := time.Now().UTC()
		createdAt := cursor.Add(-time.Minute)
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "created_at < $2")
				require.Contains(t, sql, "LIMIT $3")
				require.Equal(t, []any{"project-1", cursor, 2}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillWorkflowDest(dest, "workflow-1", createdAt)
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListWorkflows(context.Background(), "project-1", 2, &cursor)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "workflow-1", got[0].ID)
	})

	t.Run("list workflows wraps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			rows     pgx.Rows
			queryErr error
			want     string
		}{
			{name: "query", queryErr: errors.New("query failed"), want: "list workflows"},
			{name: "scan", rows: &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("row failed") }}}, want: "list workflows scan"},
			{name: "rows", rows: &mockRows{err: errors.New("rows failed")}, want: "list workflows rows"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						return tt.rows, tt.queryErr
					},
				}

				_, err := New(db).ListWorkflows(context.Background(), "project-1", 2, nil)
				require.ErrorContains(t, err, tt.want)
			})
		}
	})

	t.Run("list cron workflows scans enabled cron rows", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "w.enabled = TRUE")
				require.Empty(t, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillWorkflowDest(dest, "workflow-cron", createdAt)
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListCronWorkflows(context.Background())
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "workflow-cron", got[0].ID)
	})

	t.Run("list workflows by tag builds key-only and key-value filters", func(t *testing.T) {
		t.Parallel()

		cursor := time.Now().UTC()
		calls := 0
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				calls++
				switch calls {
				case 1:
					require.Contains(t, sql, "tags ? $2")
					require.NotContains(t, sql, "tags ->> $2 = $3")
					require.Equal(t, []any{"project-1", "team", 5}, args)
				case 2:
					require.Contains(t, sql, "tags ->> $2 = $3")
					require.Contains(t, sql, "created_at < $4")
					require.Contains(t, sql, "LIMIT $5")
					require.Equal(t, []any{"project-1", "team", "platform", cursor, 5}, args)
				default:
					require.Fail(t, "unexpected query")
				}
				return &mockRows{}, nil
			},
		}
		q := New(db)

		_, err := q.ListWorkflowsByTag(context.Background(), "project-1", "team", "", 5, nil)
		require.NoError(t, err)
		_, err = q.ListWorkflowsByTag(context.Background(), "project-1", "team", "platform", 5, &cursor)
		require.NoError(t, err)
		require.Equal(t, 2, calls)
	})

	t.Run("list workflows by tag wraps scan errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{scanFns: []func(dest ...any) error{
					func(...any) error {
						return errors.New("tag row failed")
					},
				}}, nil
			},
		}

		_, err := New(db).ListWorkflowsByTag(context.Background(), "project-1", "team", "platform", 5, nil)
		require.ErrorContains(t, err, "list workflows by tag scan")
		require.ErrorContains(t, err, "tag row failed")
	})
}

func TestUpdateWorkflow(t *testing.T) {
	t.Parallel()

	t.Run("updates mutable fields and version identity", func(t *testing.T) {
		t.Parallel()

		updatedAt := time.Now().UTC()
		var capturedArgs []any
		wf := testWorkflow()
		wf.ID = "workflow-1"
		wf.Version = 7
		wf.VersionPolicy = domain.VersionPolicyMinor

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "workflow_versions")
				require.Contains(t, sql, "version = version + 1")
				capturedArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = updatedAt
					*(dest[1].(*int)) = 8
					*(dest[2].(*string)) = "version-new"
					return nil
				}}
			},
		}

		require.NoError(t, New(db).UpdateWorkflow(context.Background(), wf))
		require.Equal(t, updatedAt, wf.UpdatedAt)
		require.Equal(t, 8, wf.Version)
		require.Equal(t, "version-new", wf.VersionID)
		require.Equal(t, "workflow-1:v7-pre", capturedArgs[16])
		require.NotEmpty(t, capturedArgs[12])
		require.Equal(t, string(domain.VersionPolicyMinor), capturedArgs[14])
	})

	t.Run("maps missing workflow and wraps scan errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			scanErr  error
			wantIs   error
			wantText string
		}{
			{name: "missing", scanErr: pgx.ErrNoRows, wantIs: ErrWorkflowNotFound},
			{name: "failed", scanErr: errors.New("scan failed"), wantText: "update workflow"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: func(...any) error {
							return tt.scanErr
						}}
					},
				}
				err := New(db).UpdateWorkflow(context.Background(), &domain.Workflow{ID: "workflow-1"})
				if tt.wantIs != nil {
					require.ErrorIs(t, err, tt.wantIs)
					return
				}
				require.ErrorContains(t, err, tt.wantText)
			})
		}
	})
}

func TestDeleteWorkflowTx(t *testing.T) {
	t.Parallel()

	t.Run("deletes workflow after active-run guard", func(t *testing.T) {
		t.Parallel()

		queryRows := 0
		execs := 0
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				queryRows++
				return &mockRow{scanFn: func(dest ...any) error {
					switch queryRows {
					case 1:
						*(dest[0].(*bool)) = true
					case 2:
						*(dest[0].(*int)) = 0
					default:
						require.Fail(t, "unexpected query row")
					}
					return nil
				}}
			},
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				execs++
				require.Equal(t, []any{"workflow-1"}, args)
				if strings.Contains(sql, "DELETE FROM workflows") {
					return pgconn.NewCommandTag("DELETE 1"), nil
				}
				return pgconn.NewCommandTag("DELETE 3"), nil
			},
		}

		require.NoError(t, New(db).DeleteWorkflow(context.Background(), "workflow-1"))
		require.Equal(t, 2, queryRows)
		require.Equal(t, 3, execs)
	})

	t.Run("rejects missing and active workflows before deletes", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			exists      bool
			activeCount int
			want        error
		}{
			{name: "missing", exists: false, want: ErrWorkflowNotFound},
			{name: "active", exists: true, activeCount: 2, want: ErrWorkflowHasActiveRuns},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				queryRows := 0
				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						queryRows++
						return &mockRow{scanFn: func(dest ...any) error {
							if queryRows == 1 {
								*(dest[0].(*bool)) = tt.exists
								return nil
							}
							*(dest[0].(*int)) = tt.activeCount
							return nil
						}}
					},
					execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						require.Fail(t, "delete should not execute")
						return pgconn.CommandTag{}, nil
					},
				}

				require.ErrorIs(t, New(db).DeleteWorkflow(context.Background(), "workflow-1"), tt.want)
			})
		}
	})

	t.Run("wraps final delete errors and zero rows", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			tag      pgconn.CommandTag
			execErr  error
			wantIs   error
			wantText string
		}{
			{name: "exec error", execErr: errors.New("delete failed"), wantText: "delete workflow"},
			{name: "zero rows", tag: pgconn.NewCommandTag("DELETE 0"), wantIs: ErrWorkflowNotFound},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				queryRows := 0
				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						queryRows++
						return &mockRow{scanFn: func(dest ...any) error {
							if queryRows == 1 {
								*(dest[0].(*bool)) = true
								return nil
							}
							*(dest[0].(*int)) = 0
							return nil
						}}
					},
					execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
						if strings.Contains(sql, "DELETE FROM workflows") {
							return tt.tag, tt.execErr
						}
						return pgconn.NewCommandTag("DELETE 1"), nil
					},
				}

				err := New(db).DeleteWorkflow(context.Background(), "workflow-1")
				if tt.wantIs != nil {
					require.ErrorIs(t, err, tt.wantIs)
					return
				}
				require.ErrorContains(t, err, tt.wantText)
			})
		}
	})
}

func TestScanWorkflow(t *testing.T) {
	t.Parallel()

	t.Run("leaves nil optional fields at zero values", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		got, err := scanWorkflow(workflowScanFunc(func(dest ...any) error {
			*(dest[0].(*string)) = "workflow-1"
			*(dest[1].(*string)) = "project-1"
			*(dest[2].(*string)) = "Deploy"
			*(dest[3].(*string)) = "deploy"
			*(dest[5].(*bool)) = true
			*(dest[6].(*int)) = 1
			*(dest[7].(*int)) = 300
			*(dest[8].(*int)) = 4
			*(dest[9].(*int)) = 2
			*(dest[12].(*bool)) = false
			*(dest[16].(*bool)) = false
			*(dest[19].(*time.Time)) = createdAt
			*(dest[20].(*time.Time)) = createdAt
			return nil
		}))

		require.NoError(t, err)
		require.Equal(t, "workflow-1", got.ID)
		require.Empty(t, got.Description)
		require.Empty(t, got.Cron)
		require.Empty(t, got.CronTimezone)
		require.Nil(t, got.Tags)
		require.Empty(t, got.VersionID)
		require.Empty(t, got.VersionPolicy)
		require.Empty(t, got.CreatedBy)
		require.Empty(t, got.UpdatedBy)
	})

	t.Run("returns tag decode errors", func(t *testing.T) {
		t.Parallel()

		_, err := scanWorkflow(workflowScanFunc(func(dest ...any) error {
			*(dest[13].(*[]byte)) = []byte(`{"broken"`)
			return nil
		}))
		require.ErrorContains(t, err, "unmarshal job tags")
	})
}
