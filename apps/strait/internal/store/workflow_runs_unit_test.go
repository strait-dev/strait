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

type workflowRunScanFunc func(dest ...any) error

func (f workflowRunScanFunc) Scan(dest ...any) error {
	return f(dest...)
}

func fillWorkflowRunDest(dest []any, id string, createdAt time.Time) {
	runError := "failed downstream"
	startedAt := createdAt.Add(time.Second)
	finishedAt := createdAt.Add(2 * time.Second)
	expiresAt := createdAt.Add(time.Hour)
	retryOfRunID := "retry-source"
	parentWorkflowRunID := "parent-run"
	parentStepRunID := "parent-step"
	versionID := "version-1"
	createdBy := "user-1"
	snapshotID := "snapshot-1"
	expectedCompletionAt := createdAt.Add(10 * time.Minute)

	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "workflow-1"
	*(dest[2].(*string)) = "project-1"
	*(dest[3].(*domain.WorkflowRunStatus)) = domain.WfStatusRunning
	*(dest[4].(*string)) = domain.TriggerManual
	*(dest[5].(*[]byte)) = []byte(`{"deploy":true}`)
	*(dest[6].(*int)) = 3
	*(dest[7].(*int)) = 4
	*(dest[8].(**string)) = &runError
	*(dest[9].(**time.Time)) = &startedAt
	*(dest[10].(**time.Time)) = &finishedAt
	*(dest[11].(**time.Time)) = &expiresAt
	*(dest[12].(**string)) = &retryOfRunID
	*(dest[13].(**string)) = &parentWorkflowRunID
	*(dest[14].(**string)) = &parentStepRunID
	*(dest[15].(*time.Time)) = createdAt
	*(dest[16].(*[]byte)) = []byte(`{"release":"v2"}`)
	*(dest[17].(**string)) = &versionID
	*(dest[18].(**string)) = &createdBy
	*(dest[19].(*[]byte)) = []byte(`{"trace_id":"abc"}`)
	*(dest[20].(**string)) = &snapshotID
	*(dest[21].(**time.Time)) = &expectedCompletionAt
	if len(dest) > 22 {
		*(dest[22].(*int64)) = 42
	}
}

func TestCreateWorkflowRun(t *testing.T) {
	t.Parallel()

	t.Run("sets defaults verifies snapshot and inserts encoded fields", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		queryRows := 0
		var insertArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				queryRows++
				if strings.Contains(sql, "workflow_snapshots") {
					require.Equal(t, []any{"snapshot-1"}, args)
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*bool)) = true
						return nil
					}}
				}
				require.Contains(t, sql, "INSERT INTO workflow_runs")
				insertArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = createdAt
					return nil
				}}
			},
		}
		run := &domain.WorkflowRun{
			WorkflowID:         "workflow-1",
			ProjectID:          "project-1",
			MaxParallelSteps:   4,
			Tags:               map[string]string{"release": "v2"},
			TraceContext:       map[string]string{"trace_id": "abc"},
			WorkflowSnapshotID: "snapshot-1",
		}

		require.NoError(t, New(db).CreateWorkflowRun(context.Background(), run))
		require.NotEmpty(t, run.ID)
		require.Equal(t, domain.WfStatusPending, run.Status)
		require.Equal(t, domain.TriggerManual, run.TriggeredBy)
		require.Equal(t, 1, run.WorkflowVersion)
		require.Equal(t, createdAt, run.CreatedAt)
		require.Equal(t, 2, queryRows)
		require.Len(t, insertArgs, 21)
		require.Equal(t, run.ID, insertArgs[0])
		require.Equal(t, domain.WfStatusPending, insertArgs[3])
		require.JSONEq(t, `{"release":"v2"}`, string(insertArgs[15].([]byte)))
		require.JSONEq(t, `{"trace_id":"abc"}`, string(insertArgs[18].([]byte)))
		require.Equal(t, "snapshot-1", insertArgs[19])
	})

	t.Run("rejects missing snapshot before insert", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				require.Contains(t, sql, "workflow_snapshots")
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = false
					return nil
				}}
			},
		}

		err := New(db).CreateWorkflowRun(context.Background(), &domain.WorkflowRun{WorkflowSnapshotID: "missing"})
		require.ErrorContains(t, err, `workflow snapshot "missing" not found`)
	})

	t.Run("wraps snapshot verification and insert errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			verify   bool
			scanErr  error
			wantText string
		}{
			{name: "verify", verify: true, scanErr: errors.New("verify failed"), wantText: "verify workflow snapshot"},
			{name: "insert", scanErr: errors.New("insert failed"), wantText: "create workflow run snapshot_id"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
						if strings.Contains(sql, "workflow_snapshots") || tt.verify {
							return &mockRow{scanFn: func(...any) error {
								return tt.scanErr
							}}
						}
						return &mockRow{scanFn: func(...any) error {
							return tt.scanErr
						}}
					},
				}
				run := &domain.WorkflowRun{}
				if tt.verify {
					run.WorkflowSnapshotID = "snapshot-1"
				}

				err := New(db).CreateWorkflowRun(context.Background(), run)
				require.ErrorContains(t, err, tt.wantText)
				require.ErrorContains(t, err, tt.scanErr.Error())
			})
		}
	})
}

func TestWorkflowRunLookupsAndScan(t *testing.T) {
	t.Parallel()

	t.Run("get maps missing row", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return pgx.ErrNoRows
				}}
			},
		}

		_, err := New(db).GetWorkflowRun(context.Background(), "missing")
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})

	t.Run("get with cache version decodes cache and optional fields", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "cache_version")
				require.Equal(t, []any{"run-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					fillWorkflowRunDest(dest, "run-1", createdAt)
					return nil
				}}
			},
		}

		got, cacheVersion, err := New(db).GetWorkflowRunWithCacheVersion(context.Background(), "run-1")
		require.NoError(t, err)
		require.Equal(t, int64(42), cacheVersion)
		require.Equal(t, int64(42), got.CacheVersion)
		require.Equal(t, "run-1", got.ID)
		require.JSONEq(t, `{"deploy":true}`, string(got.Payload))
		require.Equal(t, "failed downstream", got.Error)
		require.Equal(t, "retry-source", got.RetryOfRunID)
		require.Equal(t, "parent-run", got.ParentWorkflowRunID)
		require.Equal(t, "parent-step", got.ParentStepRunID)
		require.Equal(t, "version-1", got.WorkflowVersionID)
		require.Equal(t, "snapshot-1", got.WorkflowSnapshotID)
		require.Equal(t, "v2", got.Tags["release"])
		require.Equal(t, "abc", got.TraceContext["trace_id"])
		require.NotNil(t, got.ExpectedCompletionAt)
	})

	t.Run("scan leaves empty json fields at zero values", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		got, err := scanWorkflowRun(workflowRunScanFunc(func(dest ...any) error {
			*(dest[0].(*string)) = "run-1"
			*(dest[1].(*string)) = "workflow-1"
			*(dest[2].(*string)) = "project-1"
			*(dest[3].(*domain.WorkflowRunStatus)) = domain.WfStatusPending
			*(dest[4].(*string)) = domain.TriggerManual
			*(dest[6].(*int)) = 1
			*(dest[7].(*int)) = 2
			*(dest[15].(*time.Time)) = createdAt
			*(dest[16].(*[]byte)) = []byte(`{}`)
			*(dest[19].(*[]byte)) = []byte(`{}`)
			return nil
		}))

		require.NoError(t, err)
		require.Empty(t, got.Payload)
		require.Nil(t, got.Tags)
		require.Nil(t, got.TraceContext)
		require.Empty(t, got.WorkflowVersionID)
		require.Empty(t, got.WorkflowSnapshotID)
	})

	t.Run("scan returns json decode errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			destIndex int
		}{
			{name: "tags", destIndex: 16},
			{name: "trace", destIndex: 19},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				_, err := scanWorkflowRun(workflowRunScanFunc(func(dest ...any) error {
					*(dest[0].(*string)) = "run-1"
					*(dest[1].(*string)) = "workflow-1"
					*(dest[2].(*string)) = "project-1"
					*(dest[3].(*domain.WorkflowRunStatus)) = domain.WfStatusPending
					*(dest[4].(*string)) = domain.TriggerManual
					*(dest[6].(*int)) = 1
					*(dest[7].(*int)) = 2
					*(dest[15].(*time.Time)) = time.Now().UTC()
					*(dest[tt.destIndex].(*[]byte)) = []byte(`{"broken"`)
					return nil
				}))
				require.Error(t, err)
			})
		}
	})
}

func TestWorkflowRunListQueries(t *testing.T) {
	t.Parallel()

	t.Run("list workflow runs applies cursor", func(t *testing.T) {
		t.Parallel()

		cursor := time.Now().UTC()
		createdAt := cursor.Add(-time.Minute)
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "created_at < $3")
				require.Equal(t, []any{"workflow-1", 2, cursor}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillWorkflowRunDest(dest, "run-1", createdAt)
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListWorkflowRuns(context.Background(), "workflow-1", 2, &cursor)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "run-1", got[0].ID)
	})

	t.Run("list by project builds status cursor query", func(t *testing.T) {
		t.Parallel()

		cursor := time.Now().UTC()
		status := domain.WfStatusRunning
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "status = $2")
				require.Contains(t, sql, "created_at < $3")
				require.Contains(t, sql, "LIMIT $4")
				require.Equal(t, []any{"project-1", status, cursor, 5}, args)
				return &mockRows{}, nil
			},
		}

		_, err := New(db).ListWorkflowRunsByProject(context.Background(), "project-1", &status, 5, &cursor)
		require.NoError(t, err)
	})

	t.Run("list methods wrap query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			call func(*Queries) error
			rows pgx.Rows
			err  error
			want string
		}{
			{name: "list query", call: func(q *Queries) error {
				_, err := q.ListWorkflowRuns(context.Background(), "workflow-1", 2, nil)
				return err
			}, err: errors.New("query failed"), want: "list workflow runs"},
			{name: "list scan", call: func(q *Queries) error {
				_, err := q.ListWorkflowRuns(context.Background(), "workflow-1", 2, nil)
				return err
			}, rows: &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("scan failed") }}}, want: "list workflow runs scan"},
			{name: "project rows", call: func(q *Queries) error {
				_, err := q.ListWorkflowRunsByProject(context.Background(), "project-1", nil, 2, nil)
				return err
			}, rows: &mockRows{err: errors.New("rows failed")}, want: "list workflow runs by project rows"},
			{name: "project scan", call: func(q *Queries) error {
				_, err := q.ListWorkflowRunsByProject(context.Background(), "project-1", nil, 2, nil)
				return err
			}, rows: &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("project scan failed") }}}, want: "list workflow runs by project scan"},
			{name: "parent scan", call: func(q *Queries) error {
				_, err := q.GetWorkflowRunsByParent(context.Background(), "parent-1")
				return err
			}, rows: &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("parent scan failed") }}}, want: "get workflow runs by parent scan"},
			{name: "parent rows", call: func(q *Queries) error {
				_, err := q.GetWorkflowRunsByParent(context.Background(), "parent-1")
				return err
			}, rows: &mockRows{err: errors.New("parent rows failed")}, want: "get workflow runs by parent rows"},
			{name: "stalled rows", call: func(q *Queries) error {
				_, err := q.ListStalledWorkflowRuns(context.Background(), time.Minute)
				return err
			}, rows: &mockRows{err: errors.New("stalled rows failed")}, want: "list stalled workflow runs rows"},
			{name: "stalled scan", call: func(q *Queries) error {
				_, err := q.ListStalledWorkflowRuns(context.Background(), time.Minute)
				return err
			}, rows: &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("stalled scan failed") }}}, want: "list stalled workflow runs scan"},
			{name: "tag scan", call: func(q *Queries) error {
				_, err := q.ListWorkflowRunsByTag(context.Background(), "project-1", "release", "v2", 2, nil)
				return err
			}, rows: &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("tag scan failed") }}}, want: "list workflow runs by tag scan"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						return tt.rows, tt.err
					},
				}
				err := tt.call(New(db))
				require.ErrorContains(t, err, tt.want)
			})
		}
	})

	t.Run("list by tag builds key-only and key-value filters", func(t *testing.T) {
		t.Parallel()

		cursor := time.Now().UTC()
		calls := 0
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				calls++
				switch calls {
				case 1:
					require.Contains(t, sql, "tags ? $2")
					require.Equal(t, []any{"project-1", "release", 3}, args)
				case 2:
					require.Contains(t, sql, "tags ->> $2 = $3")
					require.Contains(t, sql, "created_at < $4")
					require.Equal(t, []any{"project-1", "release", "v2", cursor, 3}, args)
				default:
					require.Fail(t, "unexpected query")
				}
				return &mockRows{}, nil
			},
		}
		q := New(db)

		_, err := q.ListWorkflowRunsByTag(context.Background(), "project-1", "release", "", 3, nil)
		require.NoError(t, err)
		_, err = q.ListWorkflowRunsByTag(context.Background(), "project-1", "release", "v2", 3, &cursor)
		require.NoError(t, err)
		require.Equal(t, 2, calls)
	})
}

func TestCreateWorkflowRunBootstrapFallback(t *testing.T) {
	t.Parallel()

	t.Run("creates run marks running and inserts step runs without transaction support", func(t *testing.T) {
		t.Parallel()

		queryRows := 0
		execs := 0
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				queryRows++
				require.True(t, strings.Contains(sql, "INSERT INTO workflow_runs") || strings.Contains(sql, "INSERT INTO workflow_step_runs"))
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = time.Now().UTC()
					return nil
				}}
			},
			execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
				execs++
				require.Contains(t, sql, "UPDATE workflow_runs")
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		run := &domain.WorkflowRun{WorkflowID: "workflow-1", ProjectID: "project-1"}
		stepRuns := []domain.WorkflowStepRun{
			{WorkflowRunID: "run-1", WorkflowStepID: "step-1", StepRef: "build"},
			{WorkflowRunID: "run-1", WorkflowStepID: "step-2", StepRef: "deploy"},
		}

		require.NoError(t, New(db).CreateWorkflowRunBootstrap(context.Background(), run, stepRuns, time.Now().UTC()))
		require.Equal(t, 3, queryRows)
		require.Equal(t, 1, execs)
	})

	t.Run("returns create run update and step run errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			failQuery int
			failExec  bool
			want      string
		}{
			{name: "create run", failQuery: 1, want: "create run failed"},
			{name: "update", failExec: true, want: "update workflow run status"},
			{name: "step run", failQuery: 2, want: "create workflow step run"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				queryRows := 0
				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						queryRows++
						return &mockRow{scanFn: func(dest ...any) error {
							if queryRows == tt.failQuery {
								return errors.New(tt.want)
							}
							*(dest[0].(*time.Time)) = time.Now().UTC()
							return nil
						}}
					},
					execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						if tt.failExec {
							return pgconn.CommandTag{}, errors.New("update workflow run status")
						}
						return pgconn.NewCommandTag("UPDATE 1"), nil
					},
				}

				err := New(db).CreateWorkflowRunBootstrap(context.Background(), &domain.WorkflowRun{}, []domain.WorkflowStepRun{{StepRef: "build"}}, time.Now().UTC())
				require.ErrorContains(t, err, tt.want)
			})
		}
	})
}

type workflowRunBootstrapTx struct {
	*fakeTx
	commits   int
	rollbacks int
}

func (tx *workflowRunBootstrapTx) Commit(context.Context) error {
	tx.commits++
	return nil
}

func (tx *workflowRunBootstrapTx) Rollback(context.Context) error {
	tx.rollbacks++
	return nil
}

type workflowRunBootstrapBeginner struct {
	mockDBTX
	tx *workflowRunBootstrapTx
}

func (b *workflowRunBootstrapBeginner) Begin(context.Context) (pgx.Tx, error) {
	return b.tx, nil
}

func newWorkflowRunBootstrapTx(failQuery int, failExec bool, failText string) *workflowRunBootstrapTx {
	queryRows := 0
	tx := &workflowRunBootstrapTx{}
	tx.fakeTx = &fakeTx{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			queryRows++
			return &mockRow{scanFn: func(dest ...any) error {
				if queryRows == failQuery {
					return errors.New(failText)
				}
				*(dest[0].(*time.Time)) = time.Now().UTC()
				return nil
			}}
		},
		execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			if failExec {
				return pgconn.CommandTag{}, errors.New(failText)
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	return tx
}

func TestCreateWorkflowRunBootstrapTransaction(t *testing.T) {
	t.Parallel()

	t.Run("commits transaction after creating run and step runs", func(t *testing.T) {
		t.Parallel()

		tx := newWorkflowRunBootstrapTx(0, false, "")
		db := &workflowRunBootstrapBeginner{tx: tx}

		err := New(db).CreateWorkflowRunBootstrap(context.Background(), &domain.WorkflowRun{}, []domain.WorkflowStepRun{{StepRef: "build"}}, time.Now().UTC())
		require.NoError(t, err)
		require.Equal(t, 1, tx.commits)
		require.Zero(t, tx.rollbacks)
		require.Equal(t, 2, tx.queryRowCalls)
		require.Equal(t, 1, tx.execCalls)
	})

	t.Run("wraps transactional create update and step errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			failQuery int
			failExec  bool
			failText  string
			want      string
		}{
			{name: "create run", failQuery: 1, failText: "create failed", want: "create workflow run bootstrap"},
			{name: "update run", failExec: true, failText: "update failed", want: "mark workflow running bootstrap"},
			{name: "step run", failQuery: 2, failText: "step failed", want: "create workflow step run bootstrap build"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				tx := newWorkflowRunBootstrapTx(tt.failQuery, tt.failExec, tt.failText)
				err := New(&workflowRunBootstrapBeginner{tx: tx}).CreateWorkflowRunBootstrap(context.Background(), &domain.WorkflowRun{}, []domain.WorkflowStepRun{{StepRef: "build"}}, time.Now().UTC())
				require.ErrorContains(t, err, tt.want)
				require.ErrorContains(t, err, tt.failText)
				require.Zero(t, tx.commits)
				require.Equal(t, 1, tx.rollbacks)
			})
		}
	})
}

func TestUpdateWorkflowRunStatusUnit(t *testing.T) {
	t.Parallel()

	t.Run("sorts allowed fields and normalizes raw json and empty error", func(t *testing.T) {
		t.Parallel()

		var capturedSQL string
		var capturedArgs []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				capturedSQL = sql
				capturedArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		fields := map[string]any{
			"payload":    json.RawMessage(`{}`),
			"started_at": time.Unix(10, 0).UTC(),
			"error":      "",
		}

		err := New(db).UpdateWorkflowRunStatus(context.Background(), "run-1", domain.WfStatusPending, domain.WfStatusRunning, fields)
		require.NoError(t, err)
		require.Contains(t, capturedSQL, "error = $4, payload = $5, started_at = $6")
		require.Equal(t, []any{domain.WfStatusRunning, "run-1", domain.WfStatusPending, nil, json.RawMessage(`{}`), time.Unix(10, 0).UTC()}, capturedArgs)
	})

	t.Run("rejects invalid transitions and disallowed fields", func(t *testing.T) {
		t.Parallel()

		q := New(&mockDBTX{})
		err := q.UpdateWorkflowRunStatus(context.Background(), "run-1", domain.WfStatusCompleted, domain.WfStatusRunning, nil)
		require.ErrorContains(t, err, "invalid workflow status transition")

		err = q.UpdateWorkflowRunStatus(context.Background(), "run-1", domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"status": "bad"})
		require.Error(t, err)
		var fieldErr *domain.FieldError
		require.ErrorAs(t, err, &fieldErr)
		require.Equal(t, "status", fieldErr.Field)
	})

	t.Run("handles idempotent zero-row update and conflict", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name          string
			currentStatus domain.WorkflowRunStatus
			wantErr       string
		}{
			{name: "idempotent", currentStatus: domain.WfStatusRunning},
			{name: "conflict", currentStatus: domain.WfStatusPending, wantErr: "update workflow run status conflict"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("UPDATE 0"), nil
					},
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: func(dest ...any) error {
							*(dest[0].(*domain.WorkflowRunStatus)) = tt.currentStatus
							return nil
						}}
					},
				}
				err := New(db).UpdateWorkflowRunStatus(context.Background(), "run-1", domain.WfStatusPending, domain.WfStatusRunning, nil)
				if tt.wantErr == "" {
					require.NoError(t, err)
					return
				}
				require.ErrorContains(t, err, tt.wantErr)
			})
		}
	})
}

func TestWorkflowRunMaintenanceQueries(t *testing.T) {
	t.Parallel()

	t.Run("delete finished runs defaults non-positive limit", func(t *testing.T) {
		t.Parallel()

		var capturedArgs []any
		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
				capturedArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("DELETE 7"), nil
			},
		}

		deleted, err := New(db).DeleteWorkflowRunsFinishedBefore(context.Background(), time.Unix(100, 0).UTC(), 0)
		require.NoError(t, err)
		require.EqualValues(t, 7, deleted)
		require.Equal(t, 100, capturedArgs[1])
	})

	t.Run("count active workflow runs by version scans count", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "workflow_version_id = $2")
				require.Equal(t, []any{"workflow-1", "version-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 3
					return nil
				}}
			},
		}

		count, err := New(db).CountActiveWorkflowRunsByVersion(context.Background(), "workflow-1", "version-1")
		require.NoError(t, err)
		require.Equal(t, 3, count)
	})

	t.Run("list active versions scans grouped counts", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "COUNT(*) FILTER")
				require.Equal(t, []any{"workflow-1"}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "version-2"
						*(dest[1].(*int)) = 2
						*(dest[2].(*int)) = 1
						*(dest[3].(*int)) = 2
						*(dest[4].(*int)) = 3
						*(dest[5].(*int)) = 6
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListActiveWorkflowVersions(context.Background(), "workflow-1")
		require.NoError(t, err)
		require.Equal(t, []ActiveVersion{{VersionID: "version-2", Version: 2, Pending: 1, Running: 2, Paused: 3, Total: 6}}, got)
	})

	t.Run("bulk cancel returns ids and wraps scan errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "RETURNING id")
				require.Equal(t, "project-1", args[2])
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "run-1"
						return nil
					},
					func(dest ...any) error {
						*(dest[0].(*string)) = "run-2"
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).BulkCancelWorkflowRuns(context.Background(), "project-1", []string{"run-1", "run-2"}, time.Now().UTC())
		require.NoError(t, err)
		require.Equal(t, []string{"run-1", "run-2"}, got)
	})
}
