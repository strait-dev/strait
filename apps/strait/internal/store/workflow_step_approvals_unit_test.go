package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type workflowStepApprovalScanFunc func(dest ...any) error

func (f workflowStepApprovalScanFunc) Scan(dest ...any) error {
	return f(dest...)
}

func fillWorkflowStepApprovalDest(dest []any, id string, requestedAt time.Time) {
	approvedBy := "user-1"
	approvedAt := requestedAt.Add(time.Minute)
	expiresAt := requestedAt.Add(time.Hour)
	errText := "approval rejected"

	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "workflow-run-1"
	*(dest[2].(*string)) = "step-run-1"
	*(dest[3].(*[]string)) = []string{"user-1", "user-2"}
	*(dest[4].(*string)) = domain.ApprovalStatusRejected
	*(dest[5].(**string)) = &approvedBy
	*(dest[6].(*time.Time)) = requestedAt
	*(dest[7].(**time.Time)) = &approvedAt
	*(dest[8].(**time.Time)) = &expiresAt
	*(dest[9].(**string)) = &errText
}

func TestCreateWorkflowStepApprovalStore(t *testing.T) {
	origHook := OnApprovalChanged
	t.Cleanup(func() { OnApprovalChanged = origHook })

	t.Run("stores nullable fields and calls hook", func(t *testing.T) {
		var captured *domain.WorkflowStepApproval
		var mu sync.Mutex
		OnApprovalChanged = func(_ context.Context, approval *domain.WorkflowStepApproval) {
			mu.Lock()
			defer mu.Unlock()
			captured = approval
		}

		requestedAt := time.Now().UTC()
		expiresAt := requestedAt.Add(time.Hour)
		var execArgs []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "INSERT INTO workflow_step_approvals")
				execArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		}
		approval := &domain.WorkflowStepApproval{
			ID:                "approval-1",
			WorkflowRunID:     "workflow-run-1",
			WorkflowStepRunID: "step-run-1",
			Approvers:         []string{"user-1"},
			Status:            domain.ApprovalStatusPending,
			RequestedAt:       requestedAt,
			ExpiresAt:         &expiresAt,
		}

		require.NoError(t, New(db).CreateWorkflowStepApproval(context.Background(), approval))
		require.Equal(t, []any{
			"approval-1",
			"workflow-run-1",
			"step-run-1",
			[]string{"user-1"},
			domain.ApprovalStatusPending,
			nil,
			requestedAt,
			(*time.Time)(nil),
			&expiresAt,
			nil,
		}, execArgs)

		mu.Lock()
		defer mu.Unlock()
		require.Same(t, approval, captured)
	})

	t.Run("wraps insert errors", func(t *testing.T) {
		OnApprovalChanged = nil
		insertErr := errors.New("insert failed")
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, insertErr
			},
		}

		err := New(db).CreateWorkflowStepApproval(context.Background(), &domain.WorkflowStepApproval{})
		require.ErrorContains(t, err, "create workflow step approval")
		require.ErrorIs(t, err, insertErr)
	})
}

func TestGetWorkflowStepApprovalByStepRunIDStore(t *testing.T) {
	t.Run("returns nil for missing row", func(t *testing.T) {
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return pgx.ErrNoRows
				}}
			},
		}

		got, err := New(db).GetWorkflowStepApprovalByStepRunID(context.Background(), "step-run-1")
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("scans approval and returns other errors", func(t *testing.T) {
		requestedAt := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "WHERE workflow_step_run_id = $1")
				require.Equal(t, []any{"step-run-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					fillWorkflowStepApprovalDest(dest, "approval-1", requestedAt)
					return nil
				}}
			},
		}

		got, err := New(db).GetWorkflowStepApprovalByStepRunID(context.Background(), "step-run-1")
		require.NoError(t, err)
		require.Equal(t, "approval-1", got.ID)
		require.Equal(t, "user-1", got.ApprovedBy)
		require.Equal(t, "approval rejected", got.Error)
		require.NotNil(t, got.ApprovedAt)
		require.NotNil(t, got.ExpiresAt)

		scanErr := errors.New("scan failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error {
				return scanErr
			}}
		}
		_, err = New(db).GetWorkflowStepApprovalByStepRunID(context.Background(), "step-run-1")
		require.ErrorIs(t, err, scanErr)
	})
}

func TestUpdateWorkflowStepApprovalStore(t *testing.T) {
	origHook := OnApprovalChanged
	t.Cleanup(func() { OnApprovalChanged = origHook })

	t.Run("updates pending approval and calls hook", func(t *testing.T) {
		var captured *domain.WorkflowStepApproval
		var mu sync.Mutex
		OnApprovalChanged = func(_ context.Context, approval *domain.WorkflowStepApproval) {
			mu.Lock()
			defer mu.Unlock()
			captured = approval
		}

		approvedAt := time.Now().UTC()
		var execArgs []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "WHERE id = $5 AND status = 'pending'")
				execArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		err := New(db).UpdateWorkflowStepApproval(context.Background(), "approval-1", domain.ApprovalStatusApproved, "user-1", &approvedAt, "")
		require.NoError(t, err)
		require.Equal(t, []any{domain.ApprovalStatusApproved, "user-1", &approvedAt, nil, "approval-1"}, execArgs)

		mu.Lock()
		defer mu.Unlock()
		require.NotNil(t, captured)
		require.Equal(t, "approval-1", captured.ID)
		require.Equal(t, domain.ApprovalStatusApproved, captured.Status)
		require.Equal(t, "user-1", captured.ApprovedBy)
		require.Same(t, &approvedAt, captured.ApprovedAt)
		require.Empty(t, captured.Error)
	})

	t.Run("wraps update errors", func(t *testing.T) {
		OnApprovalChanged = nil
		updateErr := errors.New("update failed")
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, updateErr
			},
		}

		err := New(db).UpdateWorkflowStepApproval(context.Background(), "approval-1", domain.ApprovalStatusRejected, "", nil, "no")
		require.ErrorContains(t, err, "update workflow step approval")
		require.ErrorIs(t, err, updateErr)
	})

	t.Run("maps zero affected rows to not found, status check error, or conflict", func(t *testing.T) {
		tests := []struct {
			name       string
			scanFn     func(dest ...any) error
			wantIs     error
			wantString string
		}{
			{
				name: "missing",
				scanFn: func(...any) error {
					return pgx.ErrNoRows
				},
				wantIs: ErrWorkflowStepRunNotFound,
			},
			{
				name: "status check error",
				scanFn: func(...any) error {
					return errors.New("status failed")
				},
				wantString: "update workflow step approval status check",
			},
			{
				name: "conflict",
				scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = domain.ApprovalStatusApproved
					return nil
				},
				wantIs: ErrRunConflict,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				OnApprovalChanged = nil
				db := &mockDBTX{
					execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("UPDATE 0"), nil
					},
					queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
						require.Contains(t, sql, "SELECT status FROM workflow_step_approvals")
						require.Equal(t, []any{"approval-1"}, args)
						return &mockRow{scanFn: tc.scanFn}
					},
				}

				err := New(db).UpdateWorkflowStepApproval(context.Background(), "approval-1", domain.ApprovalStatusRejected, "", nil, "no")
				if tc.wantIs != nil {
					require.ErrorIs(t, err, tc.wantIs)
					return
				}
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})
}

func TestWorkflowStepApprovalLists(t *testing.T) {
	t.Run("expired approvals scan rows and wrap row errors", func(t *testing.T) {
		requestedAt := time.Now().UTC()
		rowErr := errors.New("rows failed")
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "expires_at <= NOW()")
				require.Empty(t, args)
				return &mockRows{
					scanFns: []func(dest ...any) error{
						func(dest ...any) error {
							fillWorkflowStepApprovalDest(dest, "approval-1", requestedAt)
							return nil
						},
					},
					err: rowErr,
				}, nil
			},
		}

		_, err := New(db).ListExpiredWorkflowStepApprovals(context.Background())
		require.ErrorContains(t, err, "list expired workflow step approvals rows")
		require.ErrorIs(t, err, rowErr)
	})

	t.Run("past reminder approvals scan rows and wrap row errors", func(t *testing.T) {
		requestedAt := time.Now().UTC()
		rowErr := errors.New("rows failed")
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "requested_at + (expires_at - requested_at) * 0.5")
				return &mockRows{
					scanFns: []func(dest ...any) error{
						func(dest ...any) error {
							fillWorkflowStepApprovalDest(dest, "approval-1", requestedAt)
							return nil
						},
					},
					err: rowErr,
				}, nil
			},
		}

		_, err := New(db).ListApprovalsPastReminderPoint(context.Background())
		require.ErrorContains(t, err, "list approvals past reminder point rows")
		require.ErrorIs(t, err, rowErr)
	})

	t.Run("list functions wrap query and scan errors", func(t *testing.T) {
		tests := []struct {
			name       string
			call       func(*Queries) ([]domain.WorkflowStepApproval, error)
			queryErr   error
			scanErr    error
			wantString string
		}{
			{
				name: "expired query",
				call: func(q *Queries) ([]domain.WorkflowStepApproval, error) {
					return q.ListExpiredWorkflowStepApprovals(context.Background())
				},
				queryErr:   errors.New("query failed"),
				wantString: "list expired workflow step approvals",
			},
			{
				name: "expired scan",
				call: func(q *Queries) ([]domain.WorkflowStepApproval, error) {
					return q.ListExpiredWorkflowStepApprovals(context.Background())
				},
				scanErr:    errors.New("scan failed"),
				wantString: "scan workflow step approval",
			},
			{
				name: "reminder query",
				call: func(q *Queries) ([]domain.WorkflowStepApproval, error) {
					return q.ListApprovalsPastReminderPoint(context.Background())
				},
				queryErr:   errors.New("query failed"),
				wantString: "list approvals past reminder point",
			},
			{
				name: "reminder scan",
				call: func(q *Queries) ([]domain.WorkflowStepApproval, error) {
					return q.ListApprovalsPastReminderPoint(context.Background())
				},
				scanErr:    errors.New("scan failed"),
				wantString: "scan approval past reminder point",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
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

				_, err := tc.call(New(db))
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})
}

func TestWorkflowStepApprovalScanAndStats(t *testing.T) {
	t.Run("scan keeps missing optional fields empty", func(t *testing.T) {
		requestedAt := time.Now().UTC()
		got, err := scanWorkflowStepApproval(workflowStepApprovalScanFunc(func(dest ...any) error {
			*(dest[0].(*string)) = "approval-1"
			*(dest[1].(*string)) = "workflow-run-1"
			*(dest[2].(*string)) = "step-run-1"
			*(dest[3].(*[]string)) = []string{"user-1"}
			*(dest[4].(*string)) = domain.ApprovalStatusPending
			*(dest[6].(*time.Time)) = requestedAt
			return nil
		}))
		require.NoError(t, err)
		require.Empty(t, got.ApprovedBy)
		require.Empty(t, got.Error)
		require.Nil(t, got.ApprovedAt)
		require.Nil(t, got.ExpiresAt)
	})

	t.Run("scan returns errors", func(t *testing.T) {
		scanErr := errors.New("scan failed")
		_, err := scanWorkflowStepApproval(workflowStepApprovalScanFunc(func(...any) error {
			return scanErr
		}))
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("gets approval stats", func(t *testing.T) {
		from := time.Now().UTC().Add(-time.Hour)
		to := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "COUNT(*) AS total_requested")
				require.Equal(t, []any{"project-1", from, to}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 10
					*(dest[1].(*int)) = 7
					*(dest[2].(*int)) = 1
					*(dest[3].(*int)) = 2
					*(dest[4].(*float64)) = 12.5
					return nil
				}}
			},
		}

		got, err := New(db).GetApprovalStats(context.Background(), "project-1", from, to)
		require.NoError(t, err)
		require.Equal(t, &ApprovalStats{
			TotalRequested:      10,
			TotalApproved:       7,
			TotalTimedOut:       1,
			TotalPending:        2,
			AvgApprovalTimeSecs: 12.5,
		}, got)
	})

	t.Run("wraps approval stats errors", func(t *testing.T) {
		scanErr := errors.New("scan failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return scanErr
				}}
			},
		}

		_, err := New(db).GetApprovalStats(context.Background(), "project-1", time.Now(), time.Now())
		require.ErrorContains(t, err, "get approval stats")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("nilIfEmptyString distinguishes empty and non-empty values", func(t *testing.T) {
		require.Nil(t, nilIfEmptyString(""))
		require.Equal(t, "value", nilIfEmptyString("value"))
	})
}

func TestGetStepRunByWorkflowRunAndRefStore(t *testing.T) {
	createdAt := time.Now().UTC()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "WHERE workflow_run_id = $1 AND step_ref = $2")
			require.Equal(t, []any{"workflow-run-1", "step-a"}, args)
			return &mockRow{scanFn: func(dest ...any) error {
				fillWorkflowStepRunDest(dest, "step-run-1", createdAt)
				return nil
			}}
		},
	}

	got, err := New(db).GetStepRunByWorkflowRunAndRef(context.Background(), "workflow-run-1", "step-a")
	require.NoError(t, err)
	require.Equal(t, "step-run-1", got.ID)
	require.Equal(t, "step-a", got.StepRef)
}
