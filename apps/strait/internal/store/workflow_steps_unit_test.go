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

type workflowStepScanFunc func(dest ...any) error

func (f workflowStepScanFunc) Scan(dest ...any) error {
	return f(dest...)
}

func fillWorkflowStepDest(dest []any, id string, createdAt time.Time) {
	jobID := "job-1"
	subWorkflowID := "sub-workflow-1"
	eventKey := "event-1"
	eventNotifyURL := "https://notify.example.test/hook"
	eventEmitKey := "emit-1"
	concurrencyKey := "tenant-1"
	resourceClass := "large"
	costGateThreshold := int64(12_000)
	costGateTimeout := 45
	costGateDefaultAction := "skip"
	compensationJobID := "compensate-job-1"

	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "workflow-1"
	*(dest[2].(**string)) = &jobID
	*(dest[3].(*string)) = "step-a"
	*(dest[4].(*[]string)) = []string{"root"}
	*(dest[5].(*[]byte)) = []byte(`{"if":true}`)
	*(dest[6].(*string)) = string(domain.SkipDependents)
	*(dest[7].(*[]byte)) = []byte(`{"payload":true}`)
	*(dest[8].(*string)) = string(domain.WorkflowStepTypeWaitForEvent)
	*(dest[9].(*int)) = 60
	*(dest[10].(*[]string)) = []string{"user-1"}
	*(dest[11].(*int)) = 3
	*(dest[12].(*string)) = string(domain.RetryBackoffFixed)
	*(dest[13].(*int)) = 5
	*(dest[14].(*int)) = 120
	*(dest[15].(*int)) = 300
	*(dest[16].(*string)) = ".result"
	*(dest[17].(**string)) = &subWorkflowID
	*(dest[18].(*int)) = 2
	*(dest[19].(**string)) = &eventKey
	*(dest[20].(*int)) = 3600
	*(dest[21].(**string)) = &eventNotifyURL
	*(dest[22].(*int)) = 90
	*(dest[23].(**string)) = &eventEmitKey
	*(dest[24].(**string)) = &concurrencyKey
	*(dest[25].(**string)) = &resourceClass
	*(dest[26].(**int64)) = &costGateThreshold
	*(dest[27].(**int)) = &costGateTimeout
	*(dest[28].(**string)) = &costGateDefaultAction
	*(dest[29].(*int)) = 180
	*(dest[30].(*[]byte)) = []byte(`{"on_complete":true}`)
	*(dest[31].(**string)) = &compensationJobID
	*(dest[32].(*int)) = 15
	*(dest[33].(*time.Time)) = createdAt
}

func TestCreateWorkflowStep(t *testing.T) {
	t.Parallel()

	t.Run("sets defaults and stores empty optional fields as null", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		var insertArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO workflow_steps")
				insertArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = createdAt
					return nil
				}}
			},
		}

		step := &domain.WorkflowStep{
			WorkflowID:         "workflow-1",
			StepRef:            "step-a",
			Condition:          json.RawMessage{},
			Payload:            json.RawMessage{},
			StageNotifications: json.RawMessage{},
		}

		require.NoError(t, New(db).CreateWorkflowStep(context.Background(), step))
		require.NotEmpty(t, step.ID)
		require.Equal(t, domain.FailWorkflow, step.OnFailure)
		require.Equal(t, domain.WorkflowStepTypeJob, step.StepType)
		require.Equal(t, domain.RetryBackoffExponential, step.RetryBackoff)
		require.Empty(t, step.DependsOn)
		require.Empty(t, step.ApprovalApprovers)
		require.Equal(t, "small", step.ResourceClass)
		require.Equal(t, createdAt, step.CreatedAt)
		require.Len(t, insertArgs, 33)
		require.Equal(t, step.ID, insertArgs[0])
		require.Nil(t, insertArgs[2])
		require.Nil(t, insertArgs[5])
		require.Equal(t, string(domain.FailWorkflow), insertArgs[6])
		require.Nil(t, insertArgs[7])
		require.Equal(t, string(domain.WorkflowStepTypeJob), insertArgs[8])
		require.Equal(t, []string{}, insertArgs[10])
		require.Equal(t, string(domain.RetryBackoffExponential), insertArgs[12])
		require.Nil(t, insertArgs[17])
		require.Nil(t, insertArgs[19])
		require.Nil(t, insertArgs[21])
		require.Nil(t, insertArgs[23])
		require.Equal(t, "small", insertArgs[25])
		require.Nil(t, insertArgs[26])
		require.Nil(t, insertArgs[27])
		require.Nil(t, insertArgs[28])
		require.Nil(t, insertArgs[30])
		require.Nil(t, insertArgs[31])
	})

	t.Run("preserves explicit values", func(t *testing.T) {
		t.Parallel()

		var insertArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
				insertArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = time.Now().UTC()
					return nil
				}}
			},
		}
		step := &domain.WorkflowStep{
			ID:                        "step-1",
			WorkflowID:                "workflow-1",
			JobID:                     "job-1",
			StepRef:                   "step-a",
			DependsOn:                 []string{"root"},
			Condition:                 json.RawMessage(`{"if":true}`),
			OnFailure:                 domain.Continue,
			Payload:                   json.RawMessage(`{"input":1}`),
			StepType:                  domain.WorkflowStepTypeSleep,
			ApprovalApprovers:         []string{"user-1"},
			RetryBackoff:              domain.RetryBackoffFixed,
			SubWorkflowID:             "sub-workflow-1",
			EventKey:                  "event-1",
			EventNotifyURL:            "https://notify.example.test/hook",
			EventEmitKey:              "emit-1",
			ResourceClass:             "large",
			CostGateThresholdMicrousd: 12_000,
			CostGateTimeoutSecs:       45,
			CostGateDefaultAction:     "skip",
			StageNotifications:        json.RawMessage(`{"on_complete":true}`),
			CompensationJobID:         "compensate-job-1",
		}

		require.NoError(t, New(db).CreateWorkflowStep(context.Background(), step))
		require.Equal(t, "job-1", insertArgs[2])
		require.JSONEq(t, `{"if":true}`, string(insertArgs[5].(json.RawMessage)))
		require.JSONEq(t, `{"input":1}`, string(insertArgs[7].(json.RawMessage)))
		require.Equal(t, string(domain.WorkflowStepTypeSleep), insertArgs[8])
		require.Equal(t, []string{"user-1"}, insertArgs[10])
		require.Equal(t, "sub-workflow-1", insertArgs[17])
		require.Equal(t, "event-1", insertArgs[19])
		require.Equal(t, "https://notify.example.test/hook", insertArgs[21])
		require.Equal(t, "emit-1", insertArgs[23])
		require.Equal(t, "large", insertArgs[25])
		require.Equal(t, int64(12_000), insertArgs[26])
		require.Equal(t, 45, insertArgs[27])
		require.Equal(t, "skip", insertArgs[28])
		require.JSONEq(t, `{"on_complete":true}`, string(insertArgs[30].(json.RawMessage)))
		require.Equal(t, "compensate-job-1", insertArgs[31])
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

		err := New(db).CreateWorkflowStep(context.Background(), &domain.WorkflowStep{})
		require.ErrorContains(t, err, "create workflow step")
		require.ErrorIs(t, err, insertErr)
	})
}

func TestWorkflowStepLookupsAndLists(t *testing.T) {
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

		_, err := New(db).GetWorkflowStep(context.Background(), "missing")
		require.ErrorIs(t, err, ErrWorkflowStepNotFound)
	})

	t.Run("get wraps scan errors", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("scan failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return scanErr
				}}
			},
		}

		_, err := New(db).GetWorkflowStep(context.Background(), "step-1")
		require.ErrorContains(t, err, "get workflow step")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("list scans steps and wraps row errors", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		rowErr := errors.New("rows failed")
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "ORDER BY ws.created_at ASC")
				require.Equal(t, []any{"workflow-1"}, args)
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

		_, err := New(db).ListStepsByWorkflow(context.Background(), "workflow-1")
		require.ErrorContains(t, err, "list workflow steps rows")
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
			{name: "query", queryErr: errors.New("query failed"), wantString: "list workflow steps"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "list workflow steps scan"},
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

				_, err := New(db).ListStepsByWorkflow(context.Background(), "workflow-1")
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})
}

func TestDeleteStepsByWorkflow(t *testing.T) {
	t.Parallel()

	t.Run("uses workflow scope", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "EXISTS (SELECT 1 FROM workflows")
				require.Equal(t, []any{"workflow-1"}, args)
				return pgconn.NewCommandTag("DELETE 2"), nil
			},
		}

		require.NoError(t, New(db).DeleteStepsByWorkflow(context.Background(), "workflow-1"))
	})

	t.Run("wraps delete errors", func(t *testing.T) {
		t.Parallel()

		deleteErr := errors.New("delete failed")
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, deleteErr
			},
		}

		err := New(db).DeleteStepsByWorkflow(context.Background(), "workflow-1")
		require.ErrorContains(t, err, "delete workflow steps by workflow")
		require.ErrorIs(t, err, deleteErr)
	})
}

func TestScanWorkflowStep(t *testing.T) {
	t.Parallel()

	t.Run("populates optional fields", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		got, err := scanWorkflowStep(workflowStepScanFunc(func(dest ...any) error {
			fillWorkflowStepDest(dest, "step-1", createdAt)
			return nil
		}))
		require.NoError(t, err)
		require.Equal(t, "step-1", got.ID)
		require.Equal(t, "workflow-1", got.WorkflowID)
		require.Equal(t, "job-1", got.JobID)
		require.Equal(t, []string{"root"}, got.DependsOn)
		require.JSONEq(t, `{"if":true}`, string(got.Condition))
		require.Equal(t, domain.SkipDependents, got.OnFailure)
		require.JSONEq(t, `{"payload":true}`, string(got.Payload))
		require.Equal(t, domain.WorkflowStepTypeWaitForEvent, got.StepType)
		require.Equal(t, []string{"user-1"}, got.ApprovalApprovers)
		require.Equal(t, domain.RetryBackoffFixed, got.RetryBackoff)
		require.Equal(t, "sub-workflow-1", got.SubWorkflowID)
		require.Equal(t, "event-1", got.EventKey)
		require.Equal(t, "https://notify.example.test/hook", got.EventNotifyURL)
		require.Equal(t, "emit-1", got.EventEmitKey)
		require.Equal(t, "tenant-1", got.ConcurrencyKey)
		require.Equal(t, "large", got.ResourceClass)
		require.Equal(t, int64(12_000), got.CostGateThresholdMicrousd)
		require.Equal(t, 45, got.CostGateTimeoutSecs)
		require.Equal(t, "skip", got.CostGateDefaultAction)
		require.JSONEq(t, `{"on_complete":true}`, string(got.StageNotifications))
		require.Equal(t, "compensate-job-1", got.CompensationJobID)
		require.Equal(t, createdAt, got.CreatedAt)
	})

	t.Run("defaults missing resource class and keeps empty optional fields", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		got, err := scanWorkflowStep(workflowStepScanFunc(func(dest ...any) error {
			*(dest[0].(*string)) = "step-1"
			*(dest[1].(*string)) = "workflow-1"
			*(dest[3].(*string)) = "step-a"
			*(dest[4].(*[]string)) = []string{}
			*(dest[6].(*string)) = string(domain.FailWorkflow)
			*(dest[8].(*string)) = string(domain.WorkflowStepTypeJob)
			*(dest[10].(*[]string)) = []string{}
			*(dest[12].(*string)) = string(domain.RetryBackoffExponential)
			*(dest[33].(*time.Time)) = createdAt
			return nil
		}))
		require.NoError(t, err)
		require.Empty(t, got.JobID)
		require.Empty(t, got.Condition)
		require.Empty(t, got.Payload)
		require.Empty(t, got.SubWorkflowID)
		require.Empty(t, got.EventKey)
		require.Empty(t, got.EventNotifyURL)
		require.Empty(t, got.EventEmitKey)
		require.Empty(t, got.ConcurrencyKey)
		require.Equal(t, "small", got.ResourceClass)
		require.Zero(t, got.CostGateThresholdMicrousd)
		require.Zero(t, got.CostGateTimeoutSecs)
		require.Empty(t, got.CostGateDefaultAction)
		require.Empty(t, got.StageNotifications)
		require.Empty(t, got.CompensationJobID)
	})

	t.Run("returns scan errors", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("scan failed")
		_, err := scanWorkflowStep(workflowStepScanFunc(func(...any) error {
			return scanErr
		}))
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("select lists remain aligned", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
				require.True(t, strings.Contains(sql, "cost_gate_default_action") && strings.Contains(sql, "compensation_timeout_secs"))
				return &mockRows{}, nil
			},
		}

		_, err := New(db).ListStepsByWorkflow(context.Background(), "workflow-1")
		require.NoError(t, err)
	})
}
