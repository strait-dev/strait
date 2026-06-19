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

type workflowStepRunScanFunc func(dest ...any) error

func (f workflowStepRunScanFunc) Scan(dest ...any) error {
	return f(dest...)
}

func fillWorkflowStepRunDest(dest []any, id string, createdAt time.Time) {
	jobRunID := "job-run-1"
	stepRunError := "step failed"
	startedAt := createdAt.Add(time.Second)
	finishedAt := createdAt.Add(2 * time.Second)

	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "workflow-run-1"
	*(dest[2].(*string)) = "workflow-step-1"
	*(dest[3].(*string)) = "step-a"
	*(dest[4].(**string)) = &jobRunID
	*(dest[5].(*domain.StepRunStatus)) = domain.StepCompleted
	*(dest[6].(*int)) = 2
	*(dest[7].(*int)) = 3
	*(dest[8].(*[]byte)) = []byte(`{"ok":true}`)
	*(dest[9].(**string)) = &stepRunError
	*(dest[10].(**time.Time)) = &startedAt
	*(dest[11].(**time.Time)) = &finishedAt
	*(dest[12].(*int)) = 4
	*(dest[13].(*time.Time)) = createdAt
}

func TestCreateWorkflowStepRun(t *testing.T) {
	t.Parallel()

	t.Run("sets defaults and stores empty optional fields as null", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		var insertArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO workflow_step_runs")
				insertArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = createdAt
					return nil
				}}
			},
		}

		stepRun := &domain.WorkflowStepRun{
			WorkflowRunID:  "workflow-run-1",
			WorkflowStepID: "workflow-step-1",
			StepRef:        "step-a",
			Output:         json.RawMessage{},
		}

		require.NoError(t, New(db).CreateWorkflowStepRun(context.Background(), stepRun))
		require.NotEmpty(t, stepRun.ID)
		require.Equal(t, domain.StepPending, stepRun.Status)
		require.Equal(t, 1, stepRun.Attempt)
		require.Equal(t, createdAt, stepRun.CreatedAt)
		require.Len(t, insertArgs, 13)
		require.Equal(t, stepRun.ID, insertArgs[0])
		require.Equal(t, domain.StepPending, insertArgs[5])
		require.Nil(t, insertArgs[4])
		require.Nil(t, insertArgs[8])
		require.Nil(t, insertArgs[9])
		require.Equal(t, 1, insertArgs[12])
	})

	t.Run("wraps insert error", func(t *testing.T) {
		t.Parallel()

		insertErr := errors.New("insert failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return insertErr
				}}
			},
		}

		err := New(db).CreateWorkflowStepRun(context.Background(), &domain.WorkflowStepRun{})
		require.ErrorContains(t, err, "create workflow step run")
		require.ErrorIs(t, err, insertErr)
	})
}

func TestWorkflowStepRunLookupsAndScan(t *testing.T) {
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

		_, err := New(db).GetWorkflowStepRun(context.Background(), "missing")
		require.ErrorIs(t, err, ErrWorkflowStepRunNotFound)
	})

	t.Run("get by job run returns nil when missing", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return pgx.ErrNoRows
				}}
			},
		}

		got, err := New(db).GetStepRunByJobRunID(context.Background(), "job-run-1")
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("get by job run wraps scan errors", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("scan failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return scanErr
				}}
			},
		}

		_, err := New(db).GetStepRunByJobRunID(context.Background(), "job-run-1")
		require.ErrorContains(t, err, "get step run by job run id")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("scan populates optional fields", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		got, err := scanWorkflowStepRun(workflowStepRunScanFunc(func(dest ...any) error {
			fillWorkflowStepRunDest(dest, "step-run-1", createdAt)
			return nil
		}))
		require.NoError(t, err)
		require.Equal(t, "step-run-1", got.ID)
		require.Equal(t, "job-run-1", got.JobRunID)
		require.Equal(t, domain.StepCompleted, got.Status)
		require.JSONEq(t, `{"ok":true}`, string(got.Output))
		require.Equal(t, "step failed", got.Error)
		require.NotNil(t, got.StartedAt)
		require.NotNil(t, got.FinishedAt)
		require.Equal(t, 4, got.Attempt)
		require.Equal(t, createdAt, got.CreatedAt)
	})

	t.Run("scan keeps absent optional fields at zero values", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		got, err := scanWorkflowStepRun(workflowStepRunScanFunc(func(dest ...any) error {
			*(dest[0].(*string)) = "step-run-1"
			*(dest[1].(*string)) = "workflow-run-1"
			*(dest[2].(*string)) = "workflow-step-1"
			*(dest[3].(*string)) = "step-a"
			*(dest[5].(*domain.StepRunStatus)) = domain.StepPending
			*(dest[12].(*int)) = 1
			*(dest[13].(*time.Time)) = createdAt
			return nil
		}))
		require.NoError(t, err)
		require.Empty(t, got.JobRunID)
		require.Empty(t, got.Output)
		require.Empty(t, got.Error)
		require.Nil(t, got.StartedAt)
		require.Nil(t, got.FinishedAt)
		require.Equal(t, 1, got.Attempt)
	})
}

func TestWorkflowStepRunLists(t *testing.T) {
	t.Parallel()

	t.Run("list by ids returns nil for empty input", func(t *testing.T) {
		t.Parallel()

		got, err := New(&mockDBTX{}).ListWorkflowStepRunsByIDs(context.Background(), nil)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("list by ids scans rows and wraps row errors", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		rowErr := errors.New("row failed")
		db := &mockDBTX{
			queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
				require.Equal(t, []any{[]string{"step-run-1"}}, args)
				return &mockRows{
					scanFns: []func(dest ...any) error{
						func(dest ...any) error {
							fillWorkflowStepRunDest(dest, "step-run-1", createdAt)
							return nil
						},
					},
					err: rowErr,
				}, nil
			},
		}

		_, err := New(db).ListWorkflowStepRunsByIDs(context.Background(), []string{"step-run-1"})
		require.ErrorContains(t, err, "list workflow step runs by ids rows")
		require.ErrorIs(t, err, rowErr)
	})

	t.Run("list by workflow run includes cursor and limit", func(t *testing.T) {
		t.Parallel()

		cursor := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "created_at > $2")
				require.Contains(t, sql, "LIMIT $3")
				require.Equal(t, []any{"workflow-run-1", cursor, 25}, args)
				return &mockRows{}, nil
			},
		}

		got, err := New(db).ListStepRunsByWorkflowRun(context.Background(), "workflow-run-1", 25, &cursor)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("list by workflow run wraps scan errors", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("scan failed")
		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{scanFns: []func(dest ...any) error{
					func(...any) error {
						return scanErr
					},
				}}, nil
			},
		}

		_, err := New(db).ListStepRunsByWorkflowRun(context.Background(), "workflow-run-1", 10, nil)
		require.ErrorContains(t, err, "list step runs by workflow run scan")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("runnable and running lists default invalid limits", func(t *testing.T) {
		t.Parallel()

		var seenDefaultLimits int
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "LIMIT $2")
				require.Equal(t, "workflow-run-1", args[0])
				require.Equal(t, 10000, args[1])
				seenDefaultLimits++
				return &mockRows{}, nil
			},
		}
		q := New(db)

		_, err := q.ListRunnableStepRunsByWorkflowRun(context.Background(), "workflow-run-1", 0)
		require.NoError(t, err)
		_, err = q.ListRunningStepRunsByWorkflowRun(context.Background(), "workflow-run-1", -1)
		require.NoError(t, err)
		require.Equal(t, 2, seenDefaultLimits)
	})

	t.Run("runnable and running lists wrap scan errors", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("scan failed")
		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{scanFns: []func(dest ...any) error{
					func(...any) error {
						return scanErr
					},
				}}, nil
			},
		}
		q := New(db)

		_, err := q.ListRunnableStepRunsByWorkflowRun(context.Background(), "workflow-run-1", 1)
		require.ErrorContains(t, err, "list runnable step runs by workflow run scan")
		require.ErrorIs(t, err, scanErr)

		_, err = q.ListRunningStepRunsByWorkflowRun(context.Background(), "workflow-run-1", 1)
		require.ErrorContains(t, err, "list running step runs by workflow run scan")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("statuses scan into status map", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "SELECT step_ref, status")
				require.Equal(t, []any{"workflow-run-1"}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "step-a"
						*(dest[1].(*string)) = string(domain.StepRunning)
						return nil
					},
					func(dest ...any) error {
						*(dest[0].(*string)) = "step-b"
						*(dest[1].(*string)) = string(domain.StepCompleted)
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListStepRunStatusesByWorkflowRun(context.Background(), "workflow-run-1")
		require.NoError(t, err)
		require.Equal(t, map[string]domain.StepRunStatus{
			"step-a": domain.StepRunning,
			"step-b": domain.StepCompleted,
		}, got)
	})
}

func TestUpdateStepRunStatus(t *testing.T) {
	t.Parallel()

	t.Run("sorts allowed fields and normalizes empty values", func(t *testing.T) {
		t.Parallel()

		var capturedSQL string
		var capturedArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				capturedSQL = sql
				capturedArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					*(dest[1].(*bool)) = true
					return nil
				}}
			},
		}

		err := New(db).UpdateStepRunStatus(context.Background(), "step-run-1", domain.StepRunning, map[string]any{
			"output":     json.RawMessage{},
			"job_run_id": "",
			"attempt":    2,
			"error":      "",
		})
		require.NoError(t, err)
		require.Less(t, strings.Index(capturedSQL, "attempt = $3"), strings.Index(capturedSQL, "error = $4"))
		require.Less(t, strings.Index(capturedSQL, "error = $4"), strings.Index(capturedSQL, "job_run_id = $5"))
		require.Less(t, strings.Index(capturedSQL, "job_run_id = $5"), strings.Index(capturedSQL, "output = $6"))
		require.Equal(t, []any{domain.StepRunning, "step-run-1", 2, nil, nil, nil}, capturedArgs)
	})

	t.Run("rejects unsupported fields", func(t *testing.T) {
		t.Parallel()

		err := New(&mockDBTX{}).UpdateStepRunStatus(context.Background(), "step-run-1", domain.StepRunning, map[string]any{
			"event_key": "not allowed",
		})
		var fieldErr *domain.FieldError
		require.ErrorAs(t, err, &fieldErr)
		require.Equal(t, "event_key", fieldErr.Field)
	})

	t.Run("maps missing target and wraps scan errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			scanFn     func(dest ...any) error
			wantIs     error
			wantString string
		}{
			{
				name: "missing",
				scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = false
					*(dest[1].(*bool)) = false
					return nil
				},
				wantIs: ErrWorkflowStepRunNotFound,
			},
			{
				name: "scan error",
				scanFn: func(...any) error {
					return errors.New("scan failed")
				},
				wantString: "update step run status",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: tc.scanFn}
					},
				}
				err := New(db).UpdateStepRunStatus(context.Background(), "step-run-1", domain.StepRunning, nil)
				if tc.wantIs != nil {
					require.ErrorIs(t, err, tc.wantIs)
				} else {
					require.ErrorContains(t, err, tc.wantString)
				}
			})
		}
	})
}

func TestUpdateStepRunStatusFrom(t *testing.T) {
	t.Parallel()

	t.Run("sorts fields and normalizes empty values", func(t *testing.T) {
		t.Parallel()

		var capturedSQL string
		var capturedArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				capturedSQL = sql
				capturedArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					*(dest[1].(*bool)) = true
					return nil
				}}
			},
		}

		err := New(db).UpdateStepRunStatusFrom(context.Background(), "step-run-1", domain.StepPending, domain.StepRunning, map[string]any{
			"job_run_id": "",
			"output":     json.RawMessage{},
			"attempt":    2,
			"error":      "",
		})
		require.NoError(t, err)
		require.Less(t, strings.Index(capturedSQL, "attempt = $4"), strings.Index(capturedSQL, "error = $5"))
		require.Less(t, strings.Index(capturedSQL, "error = $5"), strings.Index(capturedSQL, "job_run_id = $6"))
		require.Less(t, strings.Index(capturedSQL, "job_run_id = $6"), strings.Index(capturedSQL, "output = $7"))
		require.Equal(t, []any{domain.StepRunning, "step-run-1", domain.StepPending, 2, nil, nil, nil}, capturedArgs)
	})

	tests := []struct {
		name       string
		found      bool
		updated    bool
		from       domain.StepRunStatus
		to         domain.StepRunStatus
		wantErr    bool
		wantString string
	}{
		{name: "updated", found: true, updated: true, from: domain.StepPending, to: domain.StepRunning},
		{name: "idempotent no update", found: true, updated: false, from: domain.StepRunning, to: domain.StepRunning},
		{name: "missing target conflicts", from: domain.StepPending, to: domain.StepRunning, wantErr: true, wantString: "conflict"},
		{name: "stale transition conflicts", found: true, from: domain.StepPending, to: domain.StepRunning, wantErr: true, wantString: "conflict"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := &mockDBTX{
				queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
					require.Contains(t, sql, "status = $3")
					require.Equal(t, []any{tc.to, "step-run-1", tc.from}, args[:3])
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*bool)) = tc.found
						*(dest[1].(*bool)) = tc.updated
						return nil
					}}
				},
			}

			err := New(db).UpdateStepRunStatusFrom(context.Background(), "step-run-1", tc.from, tc.to, nil)
			if tc.wantErr {
				require.ErrorContains(t, err, tc.wantString)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestStepDependencyAndOutputQueries(t *testing.T) {
	t.Parallel()

	t.Run("increment deps scans json payloads", func(t *testing.T) {
		t.Parallel()

		jobID := "job-1"
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "UPDATE workflow_step_runs")
				require.Equal(t, []any{"workflow-run-1", "step-a"}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "step-run-b"
						*(dest[1].(*string)) = "step-b"
						*(dest[2].(*int)) = 1
						*(dest[3].(*int)) = 2
						*(dest[4].(**string)) = &jobID
						*(dest[5].(*[]byte)) = []byte(`{"if":true}`)
						*(dest[6].(*[]byte)) = []byte(`{"input":1}`)
						*(dest[7].(*string)) = "workflow-run-1"
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).IncrementStepDeps(context.Background(), "workflow-run-1", "step-a")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "step-run-b", got[0].StepRunID)
		require.Equal(t, &jobID, got[0].JobID)
		require.JSONEq(t, `{"if":true}`, string(got[0].Condition))
		require.JSONEq(t, `{"input":1}`, string(got[0].Payload))
	})

	t.Run("increment deps batch returns nil for empty completed refs", func(t *testing.T) {
		t.Parallel()

		got, err := New(&mockDBTX{}).IncrementStepDepsBatch(context.Background(), "workflow-run-1", nil)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("increment deps batch scans results and wraps query errors", func(t *testing.T) {
		t.Parallel()

		t.Run("scans result", func(t *testing.T) {
			t.Parallel()

			rowErr := errors.New("rows failed")
			db := &mockDBTX{
				queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
					require.Equal(t, []any{"workflow-run-1", []string{"step-a", "step-b"}}, args)
					return &mockRows{
						scanFns: []func(dest ...any) error{
							func(dest ...any) error {
								*(dest[0].(*string)) = "step-run-c"
								*(dest[1].(*string)) = "step-c"
								*(dest[2].(*int)) = 2
								*(dest[3].(*int)) = 2
								*(dest[5].(*[]byte)) = []byte(`{"if":true}`)
								*(dest[6].(*[]byte)) = []byte(`{"input":2}`)
								*(dest[7].(*string)) = "workflow-run-1"
								return nil
							},
						},
						err: rowErr,
					}, nil
				},
			}

			_, err := New(db).IncrementStepDepsBatch(context.Background(), "workflow-run-1", []string{"step-a", "step-b"})
			require.ErrorContains(t, err, "increment step deps batch rows")
			require.ErrorIs(t, err, rowErr)
		})

		t.Run("query error", func(t *testing.T) {
			t.Parallel()

			queryErr := errors.New("query failed")
			db := &mockDBTX{
				queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return nil, queryErr
				},
			}

			_, err := New(db).IncrementStepDepsBatch(context.Background(), "workflow-run-1", []string{"step-a"})
			require.ErrorContains(t, err, "increment step deps batch")
			require.ErrorIs(t, err, queryErr)
		})
	})

	t.Run("increment attempt checks affected rows", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			tag      pgconn.CommandTag
			wantErr  error
			wantArgs []any
		}{
			{name: "updated", tag: pgconn.NewCommandTag("UPDATE 1"), wantArgs: []any{3, "step-run-1", 2}},
			{name: "missing", tag: pgconn.NewCommandTag("UPDATE 0"), wantErr: ErrWorkflowStepRunNotFound, wantArgs: []any{3, "step-run-1", 2}},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
						require.Contains(t, sql, "attempt = $3")
						require.Equal(t, tc.wantArgs, args)
						return tc.tag, nil
					},
				}
				err := New(db).IncrementStepRunAttempt(context.Background(), "step-run-1", 3)
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
					return
				}
				require.NoError(t, err)
			})
		}
	})

	t.Run("get step outputs skips null outputs", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
				require.Equal(t, []any{"workflow-run-1", []string{"step-a", "step-b"}}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "step-a"
						*(dest[1].(*[]byte)) = []byte(`{"ok":true}`)
						return nil
					},
					func(dest ...any) error {
						*(dest[0].(*string)) = "step-b"
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).GetStepOutputs(context.Background(), "workflow-run-1", []string{"step-a", "step-b"})
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.JSONEq(t, `{"ok":true}`, string(got["step-a"]))
		require.NotContains(t, got, "step-b")
	})
}

func TestStepRunMaintenanceAndCostGate(t *testing.T) {
	t.Parallel()

	t.Run("summary and count helpers scan scalar results", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Equal(t, []any{"workflow-run-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					if strings.Contains(sql, "array_agg") {
						*(dest[0].(*int)) = 2
						*(dest[1].(*[]string)) = []string{"step-a", "step-c"}
						return nil
					}
					*(dest[0].(*int)) = 5
					return nil
				}}
			},
		}
		q := New(db)

		count, err := q.CountNonTerminalStepRuns(context.Background(), "workflow-run-1")
		require.NoError(t, err)
		require.Equal(t, 5, count)

		summary, err := q.GetWorkflowStepCompletionSummary(context.Background(), "workflow-run-1")
		require.NoError(t, err)
		require.Equal(t, 2, summary.NonTerminalCount)
		require.Equal(t, []string{"step-a", "step-c"}, summary.FailedStepRefs)
	})

	t.Run("failed refs and orphaned runs scan rows", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
				if strings.Contains(sql, "job_run_read_state") {
					return &mockRows{scanFns: []func(dest ...any) error{
						func(dest ...any) error {
							*(dest[0].(*string)) = "step-run-1"
							*(dest[1].(*string)) = "step-a"
							*(dest[2].(*string)) = "workflow-run-1"
							*(dest[3].(*string)) = "job-run-1"
							*(dest[4].(*domain.RunStatus)) = domain.StatusCompleted
							return nil
						},
					}}, nil
				}
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "step-a"
						return nil
					},
					func(dest ...any) error {
						*(dest[0].(*string)) = "step-b"
						return nil
					},
				}}, nil
			},
		}
		q := New(db)

		refs, err := q.ListFailedStepRunRefs(context.Background(), "workflow-run-1")
		require.NoError(t, err)
		require.Equal(t, []string{"step-a", "step-b"}, refs)

		orphaned, err := q.ListOrphanedStepRuns(context.Background())
		require.NoError(t, err)
		require.Equal(t, []OrphanedStepRun{{
			StepRunID:     "step-run-1",
			StepRef:       "step-a",
			WorkflowRunID: "workflow-run-1",
			JobRunID:      "job-run-1",
			JobStatus:     domain.StatusCompleted,
		}}, orphaned)
	})

	t.Run("cancel and skip return affected rows and skip empty refs", func(t *testing.T) {
		t.Parallel()

		var execs int
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
				execs++
				if strings.Contains(sql, "status = 'canceled'") {
					return pgconn.NewCommandTag("UPDATE 2"), nil
				}
				return pgconn.NewCommandTag("UPDATE 3"), nil
			},
		}
		q := New(db)

		canceled, err := q.CancelNonTerminalStepRuns(context.Background(), "workflow-run-1", time.Now().UTC(), "")
		require.NoError(t, err)
		require.Equal(t, int64(2), canceled)

		skipped, err := q.SkipStepRunsByRefs(context.Background(), "workflow-run-1", nil, time.Now().UTC())
		require.NoError(t, err)
		require.Zero(t, skipped)

		skipped, err = q.SkipStepRunsByRefs(context.Background(), "workflow-run-1", []string{"step-a"}, time.Now().UTC())
		require.NoError(t, err)
		require.Equal(t, int64(3), skipped)
		require.Equal(t, 2, execs)
	})

	t.Run("cost gate default action handles missing rows and errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			scanErr   error
			action    string
			want      string
			wantError bool
		}{
			{name: "found", action: "skip", want: "skip"},
			{name: "missing", scanErr: pgx.ErrNoRows},
			{name: "error", scanErr: errors.New("query failed"), wantError: true},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: func(dest ...any) error {
							if tc.scanErr != nil {
								return tc.scanErr
							}
							*(dest[0].(*string)) = tc.action
							return nil
						}}
					},
				}
				got, err := New(db).GetCostGateDefaultAction(context.Background(), "step-run-1")
				if tc.wantError {
					require.ErrorContains(t, err, "get cost gate default action")
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			})
		}
	})
}
