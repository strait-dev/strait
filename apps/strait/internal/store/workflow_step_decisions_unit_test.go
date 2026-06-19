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
	"github.com/stretchr/testify/require"
)

func TestCreateWorkflowStepDecision(t *testing.T) {
	t.Parallel()

	t.Run("sets default id and stores empty details as null", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		var insertArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO workflow_step_decisions")
				insertArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = createdAt
					return nil
				}}
			},
		}
		decision := &domain.WorkflowStepDecision{
			WorkflowRunID: "workflow-run-1",
			StepRunID:     "step-run-1",
			StepRef:       "review",
			DecisionType:  "approval",
			Decision:      "approved",
			Explanation:   "manual approval",
			Details:       json.RawMessage{},
		}

		require.NoError(t, New(db).CreateWorkflowStepDecision(context.Background(), decision))
		require.NotEmpty(t, decision.ID)
		require.Equal(t, createdAt, decision.CreatedAt)
		require.Len(t, insertArgs, 8)
		require.Equal(t, decision.ID, insertArgs[0])
		require.Equal(t, "workflow-run-1", insertArgs[1])
		require.Equal(t, "step-run-1", insertArgs[2])
		require.Equal(t, "review", insertArgs[3])
		require.Equal(t, "approval", insertArgs[4])
		require.Equal(t, "approved", insertArgs[5])
		require.Equal(t, "manual approval", insertArgs[6])
		require.Nil(t, insertArgs[7])
	})

	t.Run("preserves explicit details", func(t *testing.T) {
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
		decision := &domain.WorkflowStepDecision{
			ID:            "decision-1",
			WorkflowRunID: "workflow-run-1",
			StepRunID:     "step-run-1",
			StepRef:       "review",
			DecisionType:  "approval",
			Decision:      "rejected",
			Explanation:   "manual rejection",
			Details:       json.RawMessage(`{"reason":"risk"}`),
		}

		require.NoError(t, New(db).CreateWorkflowStepDecision(context.Background(), decision))
		require.JSONEq(t, `{"reason":"risk"}`, string(insertArgs[7].(json.RawMessage)))
	})

	t.Run("wraps insert errors", func(t *testing.T) {
		t.Parallel()

		insertErr := errors.New("insert failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return insertErr
				}}
			},
		}

		err := New(db).CreateWorkflowStepDecision(context.Background(), &domain.WorkflowStepDecision{})
		require.ErrorContains(t, err, "create workflow step decision")
		require.ErrorIs(t, err, insertErr)
	})
}

func TestListWorkflowStepDecisions(t *testing.T) {
	t.Parallel()

	t.Run("defaults limit and scans empty details", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		var capturedSQL string
		var capturedArgs []any
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				capturedSQL = sql
				capturedArgs = append([]any(nil), args...)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "decision-1"
						*(dest[1].(*string)) = "workflow-run-1"
						*(dest[2].(*string)) = "step-run-1"
						*(dest[3].(*string)) = "review"
						*(dest[4].(*string)) = "approval"
						*(dest[5].(*string)) = "approved"
						*(dest[6].(*string)) = "ok"
						*(dest[8].(*time.Time)) = createdAt
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListWorkflowStepDecisions(context.Background(), "workflow-run-1", "", "", 0, nil)
		require.NoError(t, err)
		require.Contains(t, capturedSQL, "LIMIT $2")
		require.Equal(t, []any{"workflow-run-1", 100}, capturedArgs)
		require.Len(t, got, 1)
		require.Equal(t, "decision-1", got[0].ID)
		require.Equal(t, "workflow-run-1", got[0].WorkflowRunID)
		require.Equal(t, "step-run-1", got[0].StepRunID)
		require.Empty(t, got[0].Details)
		require.Equal(t, createdAt, got[0].CreatedAt)
	})

	t.Run("applies filters cursor and explicit limit", func(t *testing.T) {
		t.Parallel()

		cursor := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.True(t, strings.Contains(sql, "step_ref = $2") && strings.Contains(sql, "decision_type = $3"))
				require.Contains(t, sql, "created_at < $4")
				require.Contains(t, sql, "LIMIT $5")
				require.Equal(t, []any{"workflow-run-1", "review", "approval", cursor, 25}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "decision-1"
						*(dest[1].(*string)) = "workflow-run-1"
						*(dest[2].(*string)) = "step-run-1"
						*(dest[3].(*string)) = "review"
						*(dest[4].(*string)) = "approval"
						*(dest[5].(*string)) = "rejected"
						*(dest[6].(*string)) = "risk"
						*(dest[7].(*[]byte)) = []byte(`{"reason":"risk"}`)
						*(dest[8].(*time.Time)) = cursor.Add(-time.Minute)
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListWorkflowStepDecisions(context.Background(), "workflow-run-1", "review", "approval", 25, &cursor)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.JSONEq(t, `{"reason":"risk"}`, string(got[0].Details))
	})

	t.Run("wraps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "list workflow step decisions"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "list workflow step decisions scan"},
			{name: "rows", rowErr: errors.New("rows failed"), wantString: "list workflow step decisions rows"},
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

				_, err := New(db).ListWorkflowStepDecisions(context.Background(), "workflow-run-1", "", "", 10, nil)
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})
}
