package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReaper_ReapStale_RetriesWhenAttemptsRemain(t *testing.T) {
	t.Parallel()
	var transitioned atomic.Int32
	var scheduled atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusExecuting, Attempt: 1},
				{ID: "run-2", JobID: "job-2", Status: domain.StatusExecuting, Attempt: 1},
			}, nil
		},
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{MaxAttempts: 3}, nil
		},
		scheduleRetryFn: func(_ context.Context, _ string, at time.Time, attempt int) error {
			assert.False(t, at.
				Before(time.Now()))
			assert.Equal(t, 2,
				attempt)

			scheduled.Add(1)
			return nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, fields map[string]any) error {
			assert.Equal(t, domain.
				StatusExecuting,
				from)
			assert.Equal(t, domain.
				StatusQueued,
				to)
			assert.EqualValues(t, 2,
				fields["attempt"],
			)

			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapStale(context.Background())
	require.EqualValues(t, 2,
		transitioned.Load())
	require.EqualValues(t, 2,
		scheduled.Load(),
	)
}

func TestReaper_ReapStale_CrashesWhenAttemptsExhausted(t *testing.T) {
	t.Parallel()
	runs := make([]domain.JobRun, 1000)
	for i := range runs {
		runs[i] = domain.JobRun{
			ID:      fmt.Sprintf("run-%03d", i),
			JobID:   "job-1",
			Status:  domain.StatusExecuting,
			Attempt: 3,
		}
	}

	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return runs, nil
		},
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{MaxAttempts: 3}, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			require.Equal(t, domain.
				StatusExecuting,
				from)
			require.Equal(t, domain.
				StatusCrashed,
				to)

			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapStale(context.Background())
	require.Equal(t, int32(len(runs)), transitioned.
		Load())
}

func TestReaper_ReapExpired(t *testing.T) {
	t.Parallel()
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusQueued},
			}, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _ domain.RunStatus, to domain.RunStatus, _ map[string]any) error {
			assert.Equal(t, domain.
				StatusExpired,
				to)

			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpired(context.Background())
	require.EqualValues(t, 1,
		transitioned.Load())
}

func TestReaper_ReapStaleDequeued(t *testing.T) {
	t.Parallel()
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusDequeued},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			assert.Equal(t, domain.
				StatusDequeued,
				from)
			assert.Equal(t, domain.
				StatusQueued,
				to)

			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapStaleDequeued(context.Background())
	require.EqualValues(t, 1,
		transitioned.Load())
}

func TestReaper_NoStaleRuns(t *testing.T) {
	t.Parallel()
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapStale(context.Background())
	r.reapExpired(context.Background())
	r.reapStaleDequeued(context.Background())
	require.EqualValues(t, 0,
		transitioned.Load())
}

func TestReaper_RunLoop(t *testing.T) {
	t.Parallel()
	var ticked atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			ticked.Add(1)
			return nil, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
	}

	r := NewReaper(ms, 50*time.Millisecond, 30*time.Second, 0, 0, false, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	r.Run(ctx)
	require.GreaterOrEqual(t, ticked.Load(), int32(1))
}

func TestReaper_ReapOldWorkflowRuns(t *testing.T) {
	t.Parallel()
	var deleted atomic.Int64
	ms := &mockReaperStore{
		deleteOldWorkflowRunsFn: func(_ context.Context, before time.Time, limit int) (int64, error) {
			require.Positive(t, limit)
			require.False(t, before.
				IsZero())

			deleted.Store(3)
			return 3, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapOldWorkflowRuns(context.Background())
	require.EqualValues(t, 3,
		deleted.Load())
}

func TestReaper_ReapTimedOutWorkflows(t *testing.T) {
	t.Parallel()
	var wfUpdates atomic.Int32
	var stepCancels atomic.Int32
	var jobRunCancels atomic.Int32

	ms := &mockReaperStore{
		listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
			return []domain.WorkflowRun{{ID: "wr-1", Status: domain.WfStatusRunning}}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
			require.Equal(t, "wr-1",
				id)
			require.False(t, from !=
				domain.WfStatusRunning ||
				to !=
					domain.WfStatusTimedOut,
			)

			wfUpdates.Add(1)
			return nil
		},
		cancelNonTerminalStepRunsFn: func(_ context.Context, workflowRunID string, _ time.Time, _ string) (int64, error) {
			require.Equal(t, "wr-1",
				workflowRunID,
			)

			stepCancels.Add(1)
			return 1, nil
		},
		cancelJobRunsByWorkflowRunFn: func(_ context.Context, workflowRunID string, _ time.Time, _ string) (int64, error) {
			require.Equal(t, "wr-1",
				workflowRunID,
			)

			jobRunCancels.Add(1)
			return 1, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapTimedOutWorkflows(context.Background())
	require.EqualValues(t, 1,
		wfUpdates.Load(),
	)
	require.EqualValues(t, 1,
		stepCancels.Load())
	require.EqualValues(t, 1,
		jobRunCancels.Load())
}

func TestReaper_ReapStale_ListError(t *testing.T) {
	t.Parallel()
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, errors.New("list stale failed")
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapStale(context.Background())
	require.EqualValues(t, 0,
		transitioned.Load())
}

func TestReaper_ReapStale_UpdateError(t *testing.T) {
	t.Parallel()
	var transitioned atomic.Int32
	var updateCalls atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusExecuting},
				{ID: "run-2", JobID: "job-2", Status: domain.StatusExecuting},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _, _ domain.RunStatus, _ map[string]any) error {
			updateCalls.Add(1)
			if id == "run-1" {
				return errors.New("update failed")
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapStale(context.Background())
	require.EqualValues(t, 2,
		updateCalls.Load())
	require.EqualValues(t, 1,
		transitioned.Load())
}

func TestReaper_ReapExpired_ListError(t *testing.T) {
	t.Parallel()
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, errors.New("list expired failed")
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpired(context.Background())
	require.EqualValues(t, 0,
		transitioned.Load())
}

func TestReaper_ReapExpired_UpdateError(t *testing.T) {
	t.Parallel()
	var transitioned atomic.Int32
	var updateCalls atomic.Int32
	ms := &mockReaperStore{
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusQueued},
				{ID: "run-2", JobID: "job-2", Status: domain.StatusExecuting},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _, _ domain.RunStatus, _ map[string]any) error {
			updateCalls.Add(1)
			if id == "run-1" {
				return errors.New("update failed")
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpired(context.Background())
	require.EqualValues(t, 2,
		updateCalls.Load())
	require.EqualValues(t, 1,
		transitioned.Load())
}

func TestReaper_ReapStaleDequeued_ListError(t *testing.T) {
	t.Parallel()
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, errors.New("list stale dequeued failed")
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapStaleDequeued(context.Background())
	require.EqualValues(t, 0,
		transitioned.Load())
}

func TestReaper_ReapStaleDequeued_UpdateError(t *testing.T) {
	t.Parallel()
	var transitioned atomic.Int32
	var updateCalls atomic.Int32
	ms := &mockReaperStore{
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusDequeued},
				{ID: "run-2", JobID: "job-2", Status: domain.StatusDequeued},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _, _ domain.RunStatus, _ map[string]any) error {
			updateCalls.Add(1)
			if id == "run-1" {
				return errors.New("update failed")
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapStaleDequeued(context.Background())
	require.EqualValues(t, 2,
		updateCalls.Load())
	require.EqualValues(t, 1,
		transitioned.Load())
}

func TestReaper_ReapExpiredApprovals(t *testing.T) {
	t.Parallel()
	t.Run("success_single_approval", func(t *testing.T) {
		t.Parallel()
		var approvalUpdates atomic.Int32
		var stepUpdates atomic.Int32
		var workflowUpdates atomic.Int32

		ms := &mockReaperStore{
			listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
				return []domain.WorkflowStepApproval{{
					ID:                "ap-1",
					WorkflowRunID:     "wr-1",
					WorkflowStepRunID: "sr-1",
				}}, nil
			},
			updateWorkflowApprovalFn: func(_ context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error {
				require.False(t, id !=
					"ap-1" || status !=
					"timed_out" ||
					approvedBy !=
						"" || approvedAt !=
					nil || errMsg != "approval timed out",
				)

				approvalUpdates.Add(1)
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				require.False(t, id !=
					"sr-1" || status !=
					domain.
						StepFailed,
				)

				stepUpdates.Add(1)
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				require.False(t, id !=
					"wr-1" || from !=
					domain.
						WfStatusRunning ||
					to !=
						domain.
							WfStatusFailed,
				)

				workflowUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())
		require.EqualValues(t, 1,
			approvalUpdates.
				Load())
		require.EqualValues(t, 1,
			stepUpdates.Load())
		require.EqualValues(t, 1,
			workflowUpdates.
				Load())
	})

	t.Run("list_error", func(t *testing.T) {
		t.Parallel()
		var approvalUpdates atomic.Int32
		var stepUpdates atomic.Int32
		var workflowUpdates atomic.Int32

		ms := &mockReaperStore{
			listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
				return nil, errors.New("list failed")
			},
			updateWorkflowApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				approvalUpdates.Add(1)
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				stepUpdates.Add(1)
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				workflowUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())
		require.EqualValues(t, 0,
			approvalUpdates.
				Load())
		require.EqualValues(t, 0,
			stepUpdates.Load())
		require.EqualValues(t, 0,
			workflowUpdates.
				Load())
	})

	t.Run("update_approval_error_continues", func(t *testing.T) {
		t.Parallel()
		var stepUpdates atomic.Int32
		var workflowUpdates atomic.Int32

		ms := &mockReaperStore{
			listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
				return []domain.WorkflowStepApproval{
					{ID: "ap-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1"},
					{ID: "ap-2", WorkflowRunID: "wr-2", WorkflowStepRunID: "sr-2"},
				}, nil
			},
			updateWorkflowApprovalFn: func(_ context.Context, id string, _ string, _ string, _ *time.Time, _ string) error {
				if id == "ap-1" {
					return errors.New("approval update failed")
				}
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				require.False(t, id !=
					"sr-2" || status !=
					domain.
						StepFailed,
				)

				stepUpdates.Add(1)
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				require.False(t, id !=
					"wr-2" || from !=
					domain.
						WfStatusRunning ||
					to !=
						domain.
							WfStatusFailed,
				)

				workflowUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())
		require.EqualValues(t, 1,
			stepUpdates.Load())
		require.EqualValues(t, 1,
			workflowUpdates.
				Load())
	})

	t.Run("update_step_error_continues", func(t *testing.T) {
		t.Parallel()
		var workflowUpdates atomic.Int32
		var stepUpdates atomic.Int32

		ms := &mockReaperStore{
			listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
				return []domain.WorkflowStepApproval{{
					ID:                "ap-1",
					WorkflowRunID:     "wr-1",
					WorkflowStepRunID: "sr-1",
				}}, nil
			},
			updateWorkflowApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				stepUpdates.Add(1)
				return errors.New("step update failed")
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				require.False(t, from !=
					domain.WfStatusRunning ||
					to !=
						domain.WfStatusFailed,
				)

				workflowUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())
		require.EqualValues(t, 1,
			stepUpdates.Load())
		require.EqualValues(t, 1,
			workflowUpdates.
				Load())
	})

	t.Run("workflow_running_to_failed", func(t *testing.T) {
		t.Parallel()
		var workflowUpdates atomic.Int32

		ms := &mockReaperStore{
			listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
				return []domain.WorkflowStepApproval{{ID: "ap-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1"}}, nil
			},
			updateWorkflowApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				require.False(t, from !=
					domain.WfStatusRunning ||
					to !=
						domain.WfStatusFailed,
				)

				workflowUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())
		require.EqualValues(t, 1,
			workflowUpdates.
				Load())
	})

	t.Run("workflow_paused_fallback", func(t *testing.T) {
		t.Parallel()
		var workflowUpdates atomic.Int32

		ms := &mockReaperStore{
			listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
				return []domain.WorkflowStepApproval{{ID: "ap-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1"}}, nil
			},
			updateWorkflowApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				workflowUpdates.Add(1)
				if from == domain.WfStatusRunning && to == domain.WfStatusFailed {
					return errors.New("running transition failed")
				}
				require.False(t, from !=
					domain.WfStatusPaused ||
					to !=
						domain.WfStatusFailed,
				)

				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())
		require.EqualValues(t, 2,
			workflowUpdates.
				Load())
	})

	t.Run("both_workflow_updates_fail", func(t *testing.T) {
		t.Parallel()
		var approvalUpdates atomic.Int32
		var stepUpdates atomic.Int32
		var workflowUpdates atomic.Int32

		ms := &mockReaperStore{
			listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
				return []domain.WorkflowStepApproval{
					{ID: "ap-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1"},
					{ID: "ap-2", WorkflowRunID: "wr-2", WorkflowStepRunID: "sr-2"},
				}, nil
			},
			updateWorkflowApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				approvalUpdates.Add(1)
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				stepUpdates.Add(1)
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				require.Equal(t, domain.
					WfStatusFailed,
					to)

				if id == "wr-1" {
					workflowUpdates.Add(1)
					return errors.New("workflow transition failed")
				}
				if id == "wr-2" && from == domain.WfStatusRunning {
					workflowUpdates.Add(1)
					return nil
				}
				require.NotEqual(t,
					"wr-2", id)

				workflowUpdates.Add(1)
				return errors.New("unexpected")
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())
		require.EqualValues(t, 2,
			approvalUpdates.
				Load())
		require.EqualValues(t, 2,
			stepUpdates.Load())
		require.EqualValues(t, 3,
			workflowUpdates.
				Load())
	})

	t.Run("multiple_approvals", func(t *testing.T) {
		t.Parallel()
		var approvalUpdates atomic.Int32
		var stepUpdates atomic.Int32
		var workflowUpdates atomic.Int32

		ms := &mockReaperStore{
			listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
				return []domain.WorkflowStepApproval{
					{ID: "ap-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1"},
					{ID: "ap-2", WorkflowRunID: "wr-2", WorkflowStepRunID: "sr-2"},
					{ID: "ap-3", WorkflowRunID: "wr-3", WorkflowStepRunID: "sr-3"},
				}, nil
			},
			updateWorkflowApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				approvalUpdates.Add(1)
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, _ map[string]any) error {
				require.Equal(t, domain.
					StepFailed,
					status)

				stepUpdates.Add(1)
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				require.False(t, from !=
					domain.WfStatusRunning ||
					to !=
						domain.WfStatusFailed,
				)

				workflowUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())
		require.EqualValues(t, 3,
			approvalUpdates.
				Load())
		require.EqualValues(t, 3,
			stepUpdates.Load())
		require.EqualValues(t, 3,
			workflowUpdates.
				Load())
	})
}

func TestReaper_ReapOldWorkflowRuns_EdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("retention_disabled", func(t *testing.T) {
		t.Parallel()
		var deleteCalls atomic.Int32

		ms := &mockReaperStore{
			deleteOldWorkflowRunsFn: func(_ context.Context, _ time.Time, _ int) (int64, error) {
				deleteCalls.Add(1)
				return 0, nil
			},
		}

		r := &Reaper{store: ms, workflowRetention: 0, deleteBatchLimit: 100}
		r.reapOldWorkflowRuns(context.Background())
		require.EqualValues(t, 0,
			deleteCalls.Load())
	})

	t.Run("delete_error", func(t *testing.T) {
		t.Parallel()
		var deleteCalls atomic.Int32

		ms := &mockReaperStore{
			deleteOldWorkflowRunsFn: func(_ context.Context, _ time.Time, _ int) (int64, error) {
				deleteCalls.Add(1)
				return 0, errors.New("delete failed")
			},
		}

		r := &Reaper{store: ms, workflowRetention: time.Hour, deleteBatchLimit: 100}
		r.reapOldWorkflowRuns(context.Background())
		require.EqualValues(t, 1,
			deleteCalls.Load())
	})

	t.Run("delete_zero_count", func(t *testing.T) {
		t.Parallel()
		var deleteCalls atomic.Int32

		ms := &mockReaperStore{
			deleteOldWorkflowRunsFn: func(_ context.Context, _ time.Time, _ int) (int64, error) {
				deleteCalls.Add(1)
				return 0, nil
			},
		}

		r := &Reaper{store: ms, workflowRetention: time.Hour, deleteBatchLimit: 100}
		r.reapOldWorkflowRuns(context.Background())
		require.EqualValues(t, 1,
			deleteCalls.Load())
	})

	t.Run("negative_retention", func(t *testing.T) {
		t.Parallel()
		var deleteCalls atomic.Int32

		ms := &mockReaperStore{
			deleteOldWorkflowRunsFn: func(_ context.Context, _ time.Time, _ int) (int64, error) {
				deleteCalls.Add(1)
				return 0, nil
			},
		}

		r := &Reaper{store: ms, workflowRetention: -time.Second, deleteBatchLimit: 100}
		r.reapOldWorkflowRuns(context.Background())
		require.EqualValues(t, 0,
			deleteCalls.Load())
	})
}

func TestReaper_ReapTimedOutWorkflows_EdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("list_error", func(t *testing.T) {
		t.Parallel()
		var wfUpdates atomic.Int32
		var stepCancels atomic.Int32
		var jobRunCancels atomic.Int32

		ms := &mockReaperStore{
			listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
				return nil, errors.New("list failed")
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				wfUpdates.Add(1)
				return nil
			},
			cancelNonTerminalStepRunsFn: func(_ context.Context, _ string, _ time.Time, _ string) (int64, error) {
				stepCancels.Add(1)
				return 0, nil
			},
			cancelJobRunsByWorkflowRunFn: func(_ context.Context, _ string, _ time.Time, _ string) (int64, error) {
				jobRunCancels.Add(1)
				return 0, nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapTimedOutWorkflows(context.Background())
		require.False(t, wfUpdates.
			Load() !=
			0 || stepCancels.
			Load() != 0 ||
			jobRunCancels.
				Load() != 0)
	})

	t.Run("workflow_update_error_continues", func(t *testing.T) {
		t.Parallel()
		var wfUpdates atomic.Int32
		var stepCancels atomic.Int32
		var jobRunCancels atomic.Int32

		ms := &mockReaperStore{
			listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
				return []domain.WorkflowRun{
					{ID: "wr-1", Status: domain.WfStatusRunning},
					{ID: "wr-2", Status: domain.WfStatusRunning},
				}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, id string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				wfUpdates.Add(1)
				if id == "wr-1" {
					return errors.New("workflow update failed")
				}
				return nil
			},
			cancelNonTerminalStepRunsFn: func(_ context.Context, workflowRunID string, _ time.Time, _ string) (int64, error) {
				require.Equal(t, "wr-2",
					workflowRunID,
				)

				stepCancels.Add(1)
				return 1, nil
			},
			cancelJobRunsByWorkflowRunFn: func(_ context.Context, workflowRunID string, _ time.Time, _ string) (int64, error) {
				require.Equal(t, "wr-2",
					workflowRunID,
				)

				jobRunCancels.Add(1)
				return 1, nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapTimedOutWorkflows(context.Background())
		require.EqualValues(t, 2,
			wfUpdates.Load(),
		)
		require.EqualValues(t, 1,
			stepCancels.Load())
		require.EqualValues(t, 1,
			jobRunCancels.Load())
	})
}

func TestReaper_WithWorkflowRetention(t *testing.T) {
	t.Parallel()
	t.Run("sets_positive_retention", func(t *testing.T) {
		t.Parallel()
		ms := &mockReaperStore{}
		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.WithWorkflowRetention(7 * 24 * time.Hour)
		require.Equal(t, 7*
			24*time.Hour,
			r.workflowRetention,
		)
	})

	t.Run("ignores_zero_retention", func(t *testing.T) {
		t.Parallel()
		ms := &mockReaperStore{}
		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.WithWorkflowRetention(0)
		require.Equal(t, defaultWorkflowRetention,

			r.workflowRetention,
		)
	})

	t.Run("ignores_negative_retention", func(t *testing.T) {
		t.Parallel()
		ms := &mockReaperStore{}
		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.WithWorkflowRetention(-time.Hour)
		require.Equal(t, defaultWorkflowRetention,

			r.workflowRetention,
		)
	})

	t.Run("custom_retention_used_in_reap", func(t *testing.T) {
		t.Parallel()
		var deletedBefore time.Time
		var deleteCalls atomic.Int32

		ms := &mockReaperStore{
			deleteOldWorkflowRunsFn: func(_ context.Context, before time.Time, limit int) (int64, error) {
				deletedBefore = before
				deleteCalls.Add(1)
				require.Equal(t, defaultDeleteBatchLimit,

					limit,
				)

				return 5, nil
			},
		}

		customRetention := 3 * 24 * time.Hour
		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).
			WithWorkflowRetention(customRetention)
		r.reapOldWorkflowRuns(context.Background())
		require.EqualValues(t, 1,
			deleteCalls.Load())

		// The before time should be approximately now - 3 days
		expectedBefore := time.Now().Add(-customRetention)
		diff := expectedBefore.Sub(deletedBefore)
		require.False(t, diff <
			-time.Minute ||
			diff >
				time.Minute,
		)
	})
}

func TestReaper_ReapTerminalRetention(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	ms := &mockReaperStore{
		deleteRetentionFn: func(_ context.Context, shortRetention, longRetention time.Duration) (int64, error) {
			require.Equal(t, 30*
				24*time.Hour,
				shortRetention,
			)
			require.Equal(t, 90*
				24*time.Hour,
				longRetention,
			)

			called.Add(1)
			return 2, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, true, nil)
	r.reapTerminalRetention(context.Background())
	require.EqualValues(t, 1,
		called.Load())
}

func TestReaper_RetentionDisabled_SkipsRetention(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		deleteRetentionFn: func(_ context.Context, _, _ time.Duration) (int64, error) {
			called.Add(1)
			return 0, nil
		},
	}

	r := NewReaper(ms, 50*time.Millisecond, 30*time.Second, 0, 0, false, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	r.Run(ctx)
	require.EqualValues(t, 0,
		called.Load())
}

func TestReaper_RetentionEnabled_CallsRetention(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		deleteRetentionFn: func(_ context.Context, _, _ time.Duration) (int64, error) {
			called.Add(1)
			return 0, nil
		},
	}

	r := NewReaper(ms, 50*time.Millisecond, 30*time.Second, 0, 0, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	r.Run(ctx)
	require.GreaterOrEqual(t, called.Load(), int32(1))
}

func TestReaper_CustomRetentionPeriods(t *testing.T) {
	t.Parallel()
	customShort := 7 * 24 * time.Hour
	customLong := 14 * 24 * time.Hour
	var called atomic.Int32
	ms := &mockReaperStore{
		deleteRetentionFn: func(_ context.Context, shortRetention, longRetention time.Duration) (int64, error) {
			require.Equal(t, customShort,
				shortRetention,
			)
			require.Equal(t, customLong,
				longRetention,
			)

			called.Add(1)
			return 0, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, customShort, customLong, true, nil)
	r.reapTerminalRetention(context.Background())
	require.EqualValues(t, 1,
		called.Load())
}

func TestReaper_DefaultRetentionPeriodsWhenZero(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	ms := &mockReaperStore{
		deleteRetentionFn: func(_ context.Context, shortRetention, longRetention time.Duration) (int64, error) {
			require.Equal(t, 30*
				24*time.Hour,
				shortRetention,
			)
			require.Equal(t, 90*
				24*time.Hour,
				longRetention,
			)

			called.Add(1)
			return 0, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, true, nil)
	r.reapTerminalRetention(context.Background())
	require.EqualValues(t, 1,
		called.Load())
}

func TestReapExpiredEventTriggers_WorkflowStep_TimesOut(t *testing.T) {
	t.Parallel()

	var triggerTimedOut, stepFailed, workflowFailed bool

	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:                "evt-1",
					EventKey:          "aml:app-1",
					SourceType:        "workflow_step",
					WorkflowRunID:     "wr-1",
					WorkflowStepRunID: "sr-1",
					Status:            domain.EventTriggerStatusWaiting,
				},
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, id string, status string, _ json.RawMessage, _ *time.Time, _ string) error {
			if id == "evt-1" && status == domain.EventTriggerStatusTimedOut {
				triggerTimedOut = true
			}
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if id == "sr-1" && status == domain.StepFailed {
				stepFailed = true
			}
			return nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
			if id == "wr-1" && to == domain.WfStatusFailed {
				workflowFailed = true
			}
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpiredEventTriggers(context.Background())
	require.True(t, triggerTimedOut)
	require.True(t, stepFailed)
	require.True(t, workflowFailed)
}

func TestReapExpiredEventTriggers_JobRun_TimesOut(t *testing.T) {
	t.Parallel()

	var triggerTimedOut, runTimedOut bool

	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:         "evt-2",
					EventKey:   "agent:run-1:confirm",
					SourceType: "job_run",
					JobRunID:   "run-1",
					Status:     domain.EventTriggerStatusWaiting,
				},
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, id string, status string, _ json.RawMessage, _ *time.Time, _ string) error {
			if id == "evt-2" && status == domain.EventTriggerStatusTimedOut {
				triggerTimedOut = true
			}
			return nil
		},
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id == "run-1" {
				return &domain.JobRun{ID: "run-1", Status: domain.StatusWaiting}, nil
			}
			return nil, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _, to domain.RunStatus, _ map[string]any) error {
			if id == "run-1" && to == domain.StatusTimedOut {
				runTimedOut = true
			}
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpiredEventTriggers(context.Background())
	require.True(t, triggerTimedOut)
	require.True(t, runTimedOut)
}

func TestReapExpiredEventTriggers_NoExpired(t *testing.T) {
	t.Parallel()

	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return nil, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpiredEventTriggers(context.Background())
}

func TestReapExpiredEventTriggers_StoreError(t *testing.T) {
	t.Parallel()

	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return nil, errors.New("db down")
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpiredEventTriggers(context.Background())
}

func TestReapExpiredEventTriggers_JobRunAlreadyTerminal(t *testing.T) {
	t.Parallel()

	var updateRunCalled bool

	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:         "evt-3",
					EventKey:   "payment:order-1",
					SourceType: "job_run",
					JobRunID:   "run-1",
					Status:     domain.EventTriggerStatusWaiting,
				},
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			updateRunCalled = true
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpiredEventTriggers(context.Background())
	require.False(t, updateRunCalled)
}

func TestReapExpiredEventTriggers_SleepCompletesStep(t *testing.T) {
	t.Parallel()

	var updatedTriggerStatus string
	var updatedStepStatus domain.StepRunStatus

	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:                "slp:sr-1",
					EventKey:          "sleep:wr-1:wait-step",
					SourceType:        domain.EventSourceWorkflowStep,
					WorkflowRunID:     "wr-1",
					WorkflowStepRunID: "sr-1",
					Status:            domain.EventTriggerStatusWaiting,
					TriggerType:       domain.TriggerTypeSleep,
				},
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, status string, _ json.RawMessage, _ *time.Time, _ string) error {
			updatedTriggerStatus = status
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, _ map[string]any) error {
			updatedStepStatus = status
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.ReapOnce(context.Background())
	require.Equal(t, domain.
		EventTriggerStatusReceived,

		updatedTriggerStatus,
	)
	require.Equal(t, domain.
		StepCompleted,
		updatedStepStatus,
	)
}

func TestReapExpiredEventTriggers_SleepCallsOnStepCompleted(t *testing.T) {
	t.Parallel()

	var callbackCalled bool
	var callbackRunID, callbackStepID string

	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:                "slp:sr-1",
					EventKey:          "sleep:wr-1:wait-step",
					SourceType:        domain.EventSourceWorkflowStep,
					WorkflowRunID:     "wr-1",
					WorkflowStepRunID: "sr-1",
					Status:            domain.EventTriggerStatusWaiting,
					TriggerType:       domain.TriggerTypeSleep,
				},
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := &mockWorkflowCallback{
		onStepCompletedFn: func(_ context.Context, wfRunID string, stepRunID string) {
			callbackCalled = true
			callbackRunID = wfRunID
			callbackStepID = stepRunID
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, cb)
	r.ReapOnce(context.Background())
	require.True(t, callbackCalled)
	require.Equal(t, "wr-1",
		callbackRunID,
	)
	require.Equal(t, "sr-1",
		callbackStepID,
	)
}

func TestReapExpiredEventTriggers_DelegatesOnStepFailed(t *testing.T) {
	t.Parallel()

	var failedCallbackCalled bool
	var failedRunID, failedStepID string

	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:                "evt:sr-1",
					EventKey:          "approval:wr-1:check",
					SourceType:        domain.EventSourceWorkflowStep,
					WorkflowRunID:     "wr-1",
					WorkflowStepRunID: "sr-1",
					Status:            domain.EventTriggerStatusWaiting,
					TriggerType:       domain.TriggerTypeEvent,
				},
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := &mockWorkflowCallback{
		onStepFailedFn: func(_ context.Context, wfRunID string, stepRunID string) {
			failedCallbackCalled = true
			failedRunID = wfRunID
			failedStepID = stepRunID
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, cb)
	r.ReapOnce(context.Background())
	require.True(t, failedCallbackCalled)
	require.Equal(t, "wr-1",
		failedRunID,
	)
	require.Equal(t, "sr-1",
		failedStepID,
	)
}

func TestReapExpiredEventTriggers_NilCallbackFallback(t *testing.T) {
	t.Parallel()

	var wfStatusUpdated bool

	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:                "evt:sr-1",
					EventKey:          "approval:wr-1:check",
					SourceType:        domain.EventSourceWorkflowStep,
					WorkflowRunID:     "wr-1",
					WorkflowStepRunID: "sr-1",
					Status:            domain.EventTriggerStatusWaiting,
					TriggerType:       domain.TriggerTypeEvent,
				},
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			wfStatusUpdated = true
			return nil
		},
	}

	// nil callback → direct workflow failure
	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.ReapOnce(context.Background())
	require.True(t, wfStatusUpdated)
}

func TestReapExpiredEventTriggers_NilCallbackFallbackPausedWorkflow(t *testing.T) {
	t.Parallel()

	var runningAttempted atomic.Bool
	var pausedFailed atomic.Bool

	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{{
				ID:                "evt-paused",
				EventKey:          "approval:wr-paused:check",
				SourceType:        domain.EventSourceWorkflowStep,
				WorkflowRunID:     "wr-paused",
				WorkflowStepRunID: "sr-paused",
				Status:            domain.EventTriggerStatusWaiting,
				TriggerType:       domain.TriggerTypeEvent,
			}}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, id string, status string, _ json.RawMessage, _ *time.Time, errMsg string) error {
			require.Equal(t, "evt-paused", id)
			require.Equal(t, domain.EventTriggerStatusTimedOut, status)
			require.Contains(t, errMsg, "timed out")
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
			require.Equal(t, "sr-paused", id)
			require.Equal(t, domain.StepFailed, status)
			require.Equal(t, "event trigger timed out", fields["error"])
			return nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
			require.Equal(t, "wr-paused", id)
			require.Equal(t, domain.WfStatusFailed, to)
			require.Equal(t, "event trigger timed out", fields["error"])
			switch from {
			case domain.WfStatusRunning:
				runningAttempted.Store(true)
				return errors.New("workflow is paused")
			case domain.WfStatusPaused:
				pausedFailed.Store(true)
				return nil
			default:
				require.Failf(t, "unexpected workflow status", "from=%s", from)
				return nil
			}
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpiredEventTriggers(context.Background())
	require.True(t, runningAttempted.Load())
	require.True(t, pausedFailed.Load())
}

func TestReapInconsistentEventTriggers_WorkflowStepReconciled(t *testing.T) {
	t.Parallel()

	var onEventCalled bool
	ms := &mockReaperStore{
		listReceivedEventTriggersWithStaleStepsFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:                "evt-stale",
					SourceType:        domain.EventSourceWorkflowStep,
					TriggerType:       domain.TriggerTypeEvent,
					WorkflowRunID:     "wfr-1",
					WorkflowStepRunID: "wsr-1",
					Status:            domain.EventTriggerStatusReceived,
				},
			}, nil
		},
	}

	wfCb := &mockWorkflowCallback{
		onEventReceivedFn: func(_ context.Context, _ *domain.EventTrigger) error {
			onEventCalled = true
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.workflowCallback = wfCb
	r.ReapOnce(context.Background())
	require.True(t, onEventCalled)
}

func TestReapInconsistentEventTriggers_SleepReconciled(t *testing.T) {
	t.Parallel()

	var onStepCompletedCalled bool
	ms := &mockReaperStore{
		listReceivedEventTriggersWithStaleStepsFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:                "evt-sleep-stale",
					SourceType:        domain.EventSourceWorkflowStep,
					TriggerType:       domain.TriggerTypeSleep,
					WorkflowRunID:     "wfr-1",
					WorkflowStepRunID: "wsr-1",
					Status:            domain.EventTriggerStatusReceived,
				},
			}, nil
		},
	}

	wfCb := &mockWorkflowCallback{
		onStepCompletedFn: func(_ context.Context, _ string, _ string) {
			onStepCompletedCalled = true
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.workflowCallback = wfCb
	r.ReapOnce(context.Background())
	require.True(t, onStepCompletedCalled)
}

func TestReapInconsistentEventTriggers_JobRunReconciled(t *testing.T) {
	t.Parallel()

	var requeuedRunID string
	ms := &mockReaperStore{
		listReceivedEventTriggersWithStaleStepsFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:         "evt-jr-stale",
					SourceType: domain.EventSourceJobRun,
					JobRunID:   "run-stale",
					Status:     domain.EventTriggerStatusReceived,
				},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			requeuedRunID = id
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.ReapOnce(context.Background())
	require.Equal(t, "run-stale",
		requeuedRunID,
	)
}

func TestReapInconsistentEventTriggers_ListError(t *testing.T) {
	t.Parallel()

	ms := &mockReaperStore{
		listReceivedEventTriggersWithStaleStepsFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return nil, errors.New("db error")
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.ReapOnce(context.Background()) // should not panic
}

func TestCompleteSleepTrigger_StepUpdateError(t *testing.T) {
	t.Parallel()

	var triggerUpdated bool
	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:                "evt-sleep-err",
					SourceType:        domain.EventSourceWorkflowStep,
					TriggerType:       domain.TriggerTypeSleep,
					WorkflowRunID:     "wfr-1",
					WorkflowStepRunID: "wsr-1",
					Status:            domain.EventTriggerStatusWaiting,
					RequestedAt:       time.Now().Add(-10 * time.Minute),
				},
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			triggerUpdated = true
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return errors.New("step update failed")
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.ReapOnce(context.Background())
	require.True(t, triggerUpdated)
}

func TestReapOldEventTriggers_RetentionDisabled(t *testing.T) {
	t.Parallel()

	ms := &mockReaperStore{}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	// eventTriggerRetention defaults to 0 → skip
	r.ReapOnce(context.Background()) // should not panic or call delete
}

func TestReapOldEventTriggers_DeleteError(t *testing.T) {
	t.Parallel()

	ms := &mockReaperStore{
		deleteEventTriggersFinishedBeforeFn: func(_ context.Context, _ time.Time, _ int) (int64, error) {
			return 0, errors.New("delete failed")
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.eventTriggerRetention = 24 * time.Hour
	r.ReapOnce(context.Background()) // should not panic
}

// completeSleepTrigger: no step run ID skips step update, still completes trigger.
func TestCompleteSleepTrigger_NoStepRunID(t *testing.T) {
	t.Parallel()

	var triggerUpdated bool
	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:          "evt-sleep-nostep",
					SourceType:  domain.EventSourceJobRun,
					TriggerType: domain.TriggerTypeSleep,
					JobRunID:    "run-1",
					Status:      domain.EventTriggerStatusWaiting,
					RequestedAt: time.Now().Add(-5 * time.Minute),
				},
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			triggerUpdated = true
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.ReapOnce(context.Background())
	require.True(t, triggerUpdated)
}

// completeSleepTrigger: nil callback skips OnStepCompleted call.
func TestCompleteSleepTrigger_NilCallback(t *testing.T) {
	t.Parallel()

	var stepUpdated bool
	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:                "evt-sleep-nocb",
					SourceType:        domain.EventSourceWorkflowStep,
					TriggerType:       domain.TriggerTypeSleep,
					WorkflowRunID:     "wfr-1",
					WorkflowStepRunID: "wsr-1",
					Status:            domain.EventTriggerStatusWaiting,
					RequestedAt:       time.Now().Add(-5 * time.Minute),
				},
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			stepUpdated = true
			return nil
		},
	}

	// nil callback — should not panic.
	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.ReapOnce(context.Background())
	require.True(t, stepUpdated)
}

// completeSleepTrigger: trigger status update error returns early.
func TestCompleteSleepTrigger_TriggerUpdateError(t *testing.T) {
	t.Parallel()

	var stepUpdated bool
	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:                "evt-sleep-trigerr",
					SourceType:        domain.EventSourceWorkflowStep,
					TriggerType:       domain.TriggerTypeSleep,
					WorkflowStepRunID: "wsr-1",
					Status:            domain.EventTriggerStatusWaiting,
					RequestedAt:       time.Now().Add(-5 * time.Minute),
				},
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return errors.New("trigger update failed")
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			stepUpdated = true
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.ReapOnce(context.Background())
	require.False(t, stepUpdated)
}

// reapInconsistentEventTriggers: OnEventReceived error continues to next trigger.
func TestReapInconsistentEventTriggers_EventReceivedError(t *testing.T) {
	t.Parallel()

	var requeuedRunID string
	ms := &mockReaperStore{
		listReceivedEventTriggersWithStaleStepsFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:                "evt-err",
					SourceType:        domain.EventSourceWorkflowStep,
					TriggerType:       domain.TriggerTypeEvent,
					WorkflowRunID:     "wfr-1",
					WorkflowStepRunID: "wsr-1",
				},
				{
					ID:         "evt-jr",
					SourceType: domain.EventSourceJobRun,
					JobRunID:   "run-2",
				},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			requeuedRunID = id
			return nil
		},
	}

	wfCb := &mockWorkflowCallback{
		onEventReceivedFn: func(_ context.Context, _ *domain.EventTrigger) error {
			return errors.New("callback failed")
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.workflowCallback = wfCb
	r.ReapOnce(context.Background())
	require.Equal(t, "run-2",
		requeuedRunID,
	)

	// The second trigger (job run) should still be processed despite the first failing.
}

// reapInconsistentEventTriggers: empty job run ID is skipped.
func TestReapInconsistentEventTriggers_EmptyJobRunID(t *testing.T) {
	t.Parallel()

	var updateCalled bool
	ms := &mockReaperStore{
		listReceivedEventTriggersWithStaleStepsFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{
					ID:         "evt-nojr",
					SourceType: domain.EventSourceJobRun,
					JobRunID:   "", // empty — should be skipped
				},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			updateCalled = true
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.ReapOnce(context.Background())
	require.False(t, updateCalled)
}

func TestReaper_ReapStalledWorkflows_Reconcile(t *testing.T) {
	t.Parallel()

	var resumed atomic.Int32
	ms := &mockReaperStore{
		listStalledWorkflowRunsFn: func(_ context.Context, _ time.Duration) ([]domain.WorkflowRun, error) {
			return []domain.WorkflowRun{{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}}, nil
		},
	}
	cb := &mockWorkflowCallback{
		resumeWorkflowFn: func(_ context.Context, workflowRunID string) error {
			require.Equal(t, "wr-1",
				workflowRunID,
			)

			resumed.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, cb).WithStalledAction("reconcile")
	r.reapStalledWorkflows(context.Background())
	require.EqualValues(t, 1,
		resumed.Load())
}

func TestReaper_ReapStalledWorkflows_DefaultReconciles(t *testing.T) {
	t.Parallel()

	var resumed atomic.Int32
	ms := &mockReaperStore{
		listStalledWorkflowRunsFn: func(_ context.Context, _ time.Duration) ([]domain.WorkflowRun, error) {
			return []domain.WorkflowRun{{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}}, nil
		},
	}
	cb := &mockWorkflowCallback{
		resumeWorkflowFn: func(_ context.Context, workflowRunID string) error {
			require.Equal(t, "wr-1",
				workflowRunID,
			)

			resumed.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, cb)
	require.Equal(t, "reconcile",
		r.stalledAction,
	)

	r.reapStalledWorkflows(context.Background())
	require.EqualValues(t, 1,
		resumed.Load())
}

func TestReaper_WithStalledActionEmptyUsesReconcile(t *testing.T) {
	t.Parallel()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil).WithStalledAction("")
	require.Equal(t, "reconcile",
		r.stalledAction,
	)
}

func TestReaper_ReapStalledWorkflows_FailWorkflow(t *testing.T) {
	t.Parallel()

	var failed atomic.Int32
	ms := &mockReaperStore{
		listStalledWorkflowRunsFn: func(_ context.Context, _ time.Duration) ([]domain.WorkflowRun, error) {
			return []domain.WorkflowRun{{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
			require.Equal(t, "wr-1",
				id)
			require.False(t, from !=
				domain.WfStatusRunning ||
				to !=
					domain.WfStatusFailed,
			)
			require.NotNil(t, fields["finished_at"])

			failed.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).WithStalledAction("fail_workflow")
	r.reapStalledWorkflows(context.Background())
	require.EqualValues(t, 1,
		failed.Load())
}

// mockNotifierReaperStore composes mockReaperStore with ApprovalNotifierStore and ApprovalReminderStore.
type mockNotifierReaperStore struct {
	mockReaperStore
	listEnabledNotificationChannelsFn             func(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	listEnabledNotificationChannelsByProjectIDsFn func(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error)
	createNotificationDeliveryFn                  func(ctx context.Context, d *domain.NotificationDelivery) error
	getWorkflowRunFn                              func(ctx context.Context, id string) (*domain.WorkflowRun, error)
	listApprovalsPastReminderPointFn              func(ctx context.Context) ([]domain.WorkflowStepApproval, error)
}

func (m *mockNotifierReaperStore) ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error) {
	if m.listEnabledNotificationChannelsFn != nil {
		return m.listEnabledNotificationChannelsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockNotifierReaperStore) ListEnabledNotificationChannelsByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error) {
	if m.listEnabledNotificationChannelsByProjectIDsFn != nil {
		return m.listEnabledNotificationChannelsByProjectIDsFn(ctx, projectIDs)
	}
	result := make(map[string][]domain.NotificationChannel)
	for _, projectID := range projectIDs {
		channels, err := m.ListEnabledNotificationChannels(ctx, projectID)
		if err != nil {
			return nil, err
		}
		result[projectID] = channels
	}
	return result, nil
}

func (m *mockNotifierReaperStore) CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	if m.createNotificationDeliveryFn != nil {
		return m.createNotificationDeliveryFn(ctx, d)
	}
	return nil
}

func (m *mockNotifierReaperStore) GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	if m.getWorkflowRunFn != nil {
		return m.getWorkflowRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockNotifierReaperStore) ListApprovalsPastReminderPoint(ctx context.Context) ([]domain.WorkflowStepApproval, error) {
	if m.listApprovalsPastReminderPointFn != nil {
		return m.listApprovalsPastReminderPointFn(ctx)
	}
	return nil, nil
}

func TestReaper_ReapExpiredApprovals_SendsExpiredNotification(t *testing.T) {
	t.Parallel()
	var deliveries []*domain.NotificationDelivery
	expires := time.Now().Add(-1 * time.Minute)
	ms := &mockNotifierReaperStore{
		mockReaperStore: mockReaperStore{
			listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
				return []domain.WorkflowStepApproval{
					{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: "pending", ExpiresAt: &expires},
				}, nil
			},
			updateWorkflowApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", WorkflowID: "wf-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpiredApprovals(context.Background())
	require.Len(t, deliveries,
		1)
	assert.Equal(t, domain.
		NotificationEventApprovalExpired,

		deliveries[0].EventType,
	)
}

func TestReaper_SendApprovalNotificationSkipsMissingWorkflowRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{name: "lookup error", err: errors.New("workflow run lookup failed")},
		{name: "missing workflow run"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ms := &mockNotifierReaperStore{
				getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
					require.Equal(t, "wr-1", id)

					return nil, tt.err
				},
				listEnabledNotificationChannelsFn: func(context.Context, string) ([]domain.NotificationChannel, error) {
					require.Fail(t, "ListEnabledNotificationChannels must not run without a workflow run")
					return nil, nil
				},
				createNotificationDeliveryFn: func(context.Context, *domain.NotificationDelivery) error {
					require.Fail(t, "CreateNotificationDelivery must not run without a workflow run")
					return nil
				},
			}
			r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)

			r.sendApprovalNotification(context.Background(), "wr-1", domain.NotificationEventApprovalExpired, map[string]any{
				"approval_id": "appr-1",
			})
		})
	}
}

func TestReaper_ReapExpiredApprovals_NoNotificationWithoutInterface(t *testing.T) {
	t.Parallel()
	approvalReaped := false
	ms := &mockReaperStore{
		listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return []domain.WorkflowStepApproval{
				{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: "pending"},
			}, nil
		},
		updateWorkflowApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
			approvalReaped = true
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpiredApprovals(context.Background())
	require.True(t, approvalReaped)
}

func TestReaper_ReapExpiredApprovals_IgnoresRejectedApprovals(t *testing.T) {
	t.Parallel()
	updateCalled := false
	ms := &mockReaperStore{
		listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			// Rejected approvals are filtered out by the query (WHERE status = 'pending'),
			// so the store returns empty.
			return nil, nil
		},
		updateWorkflowApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
			updateCalled = true
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			updateCalled = true
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpiredApprovals(context.Background())
	require.False(t, updateCalled)
}

func TestReaper_ReapExpiredApprovals_MixedApprovals(t *testing.T) {
	t.Parallel()
	var reapedIDs []string
	expires := time.Now().Add(-1 * time.Minute)
	ms := &mockNotifierReaperStore{
		mockReaperStore: mockReaperStore{
			listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
				// Only the still-pending one is returned by the query.
				return []domain.WorkflowStepApproval{
					{ID: "appr-2", WorkflowRunID: "wr-2", WorkflowStepRunID: "sr-2", Status: "pending", ExpiresAt: &expires},
				}, nil
			},
			updateWorkflowApprovalFn: func(_ context.Context, id string, _ string, _ string, _ *time.Time, _ string) error {
				reapedIDs = append(reapedIDs, id)
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-2", ProjectID: "proj-1", WorkflowID: "wf-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return nil, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpiredApprovals(context.Background())
	require.False(t, len(reapedIDs) != 1 ||
		reapedIDs[0] !=
			"appr-2")
}

func TestSkipThenReap_ApprovalNotReaped(t *testing.T) {
	t.Parallel()
	// Simulates the full skip-then-reap lifecycle:
	// 1. An approval starts as "pending"
	// 2. SkipStep transitions it to "rejected"
	// 3. The reaper's ListExpiredWorkflowStepApprovals (WHERE status = 'pending')
	//    no longer matches it, so it is never reaped.
	expires := time.Now().Add(-1 * time.Minute) // already expired

	// After skip, the approval status is "rejected" (was "pending" before skip).
	approvalStatus := domain.ApprovalStatusRejected

	// The reaper query only returns pending approvals.
	updateCalled := false
	ms := &mockReaperStore{
		listExpiredApprovalsFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			// The real query filters WHERE status = 'pending'.
			if approvalStatus == domain.ApprovalStatusPending {
				return []domain.WorkflowStepApproval{
					{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: approvalStatus, ExpiresAt: &expires},
				}, nil
			}
			return nil, nil
		},
		updateWorkflowApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
			updateCalled = true
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			updateCalled = true
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpiredApprovals(context.Background())
	require.False(t, updateCalled)
}

func TestReaper_ReapApprovalReminders_SendsReminder(t *testing.T) {
	t.Parallel()
	var deliveries []*domain.NotificationDelivery
	// 60 min total timeout, 58% elapsed (past 50% threshold).
	now := time.Now()
	requested := now.Add(-35 * time.Minute)
	expires := now.Add(25 * time.Minute)
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return []domain.WorkflowStepApproval{
				{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: "pending", RequestedAt: requested, ExpiresAt: &expires},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", WorkflowID: "wf-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.Len(t, deliveries,
		1)
	assert.Equal(t, domain.
		NotificationEventApprovalReminder,

		deliveries[0].EventType,
	)

	var payload map[string]any
	require.NoError(t,
		json.Unmarshal(deliveries[0].Payload,
			&payload))
	require.False(t, payload["approval_id"] != "appr-1" ||
		payload["workflow_run_id"] != "wr-1" ||
		payload["workflow_id"] !=
			"wf-1" || payload["step_run_id"] != "sr-1")
}

func TestReaper_ReapApprovalReminders_BeforeHalfway_NoReminder(t *testing.T) {
	t.Parallel()
	deliveryCalled := false
	// 60 min total, only 17% elapsed -- DB would filter this out.
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return nil, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.False(t, deliveryCalled)
}

func TestReaper_ReapApprovalReminders_ExactlyHalfway(t *testing.T) {
	t.Parallel()
	var deliveries []*domain.NotificationDelivery
	now := time.Now()
	requested := now.Add(-30 * time.Minute)
	expires := now.Add(30 * time.Minute)
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return []domain.WorkflowStepApproval{
				{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: "pending", RequestedAt: requested, ExpiresAt: &expires},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", WorkflowID: "wf-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.Len(t, deliveries,
		1)
}

func TestReaper_ReapApprovalReminders_ShortTimeout(t *testing.T) {
	t.Parallel()
	var deliveries []*domain.NotificationDelivery
	// 10 min total, 60% elapsed.
	now := time.Now()
	requested := now.Add(-6 * time.Minute)
	expires := now.Add(4 * time.Minute)
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return []domain.WorkflowStepApproval{
				{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: "pending", RequestedAt: requested, ExpiresAt: &expires},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", WorkflowID: "wf-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.Len(t, deliveries,
		1)
}

func TestReaper_ReapApprovalReminders_LongTimeout(t *testing.T) {
	t.Parallel()
	var deliveries []*domain.NotificationDelivery
	// 4h total, 62.5% elapsed.
	now := time.Now()
	requested := now.Add(-150 * time.Minute)
	expires := now.Add(90 * time.Minute)
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return []domain.WorkflowStepApproval{
				{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: "pending", RequestedAt: requested, ExpiresAt: &expires},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", WorkflowID: "wf-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.Len(t, deliveries,
		1)
}

func TestReaper_ReapApprovalReminders_StoreError(t *testing.T) {
	t.Parallel()
	deliveryCalled := false
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return nil, errors.New("db down")
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.False(t, deliveryCalled)
}

func TestReaper_ReapApprovalReminders_Dedup(t *testing.T) {
	t.Parallel()
	var deliveryCount int
	expires := time.Now().Add(10 * time.Minute)
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return []domain.WorkflowStepApproval{
				{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: "pending", ExpiresAt: &expires},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", WorkflowID: "wf-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCount++
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	r.reapApprovalReminders(context.Background())
	require.Equal(t, 1,
		deliveryCount)
}

func TestReaper_ReapApprovalReminders_CachesWorkflowAndChannelsPerPoll(t *testing.T) {
	t.Parallel()
	expires := time.Now().Add(10 * time.Minute)
	approvals := []domain.WorkflowStepApproval{
		{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: domain.ApprovalStatusPending, ExpiresAt: &expires},
		{ID: "appr-2", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-2", Status: domain.ApprovalStatusPending, ExpiresAt: &expires},
		{ID: "appr-3", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-3", Status: domain.ApprovalStatusPending, ExpiresAt: &expires},
	}
	var workflowLookups atomic.Int32
	var channelLookups atomic.Int32
	var deliveries atomic.Int32
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return approvals, nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			require.Equal(t, "wr-1",
				id)

			workflowLookups.Add(1)
			return &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", WorkflowID: "wf-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, projectID string) ([]domain.NotificationChannel, error) {
			require.Equal(t, "proj-1",
				projectID,
			)

			channelLookups.Add(1)
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveries.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.Equal(t, int32(len(approvals)), deliveries.
		Load())
	require.EqualValues(t, 1,
		workflowLookups.
			Load())
	require.EqualValues(t, 1,
		channelLookups.Load())
}

func TestReaper_ReapApprovalReminders_BulkListsChannelsForMultipleProjects(t *testing.T) {
	t.Parallel()
	expires := time.Now().Add(10 * time.Minute)
	approvals := []domain.WorkflowStepApproval{
		{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: domain.ApprovalStatusPending, ExpiresAt: &expires},
		{ID: "appr-2", WorkflowRunID: "wr-2", WorkflowStepRunID: "sr-2", Status: domain.ApprovalStatusPending, ExpiresAt: &expires},
		{ID: "appr-3", WorkflowRunID: "wr-3", WorkflowStepRunID: "sr-3", Status: domain.ApprovalStatusPending, ExpiresAt: &expires},
	}
	var bulkLookups atomic.Int32
	var singleLookups atomic.Int32
	var deliveries atomic.Int32
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return approvals, nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, ProjectID: "proj-" + id, WorkflowID: "wf-" + id}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			singleLookups.Add(1)
			return nil, nil
		},
		listEnabledNotificationChannelsByProjectIDsFn: func(_ context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error) {
			bulkLookups.Add(1)
			require.Len(t, projectIDs,
				3)

			result := make(map[string][]domain.NotificationChannel, len(projectIDs))
			for _, projectID := range projectIDs {
				result[projectID] = []domain.NotificationChannel{{ID: "ch-" + projectID, ProjectID: projectID}}
			}
			return result, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveries.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.Equal(t, int32(len(approvals)), deliveries.
		Load())
	require.EqualValues(t, 1,
		bulkLookups.Load())
	require.EqualValues(t, 0,
		singleLookups.Load())
}

func TestReaper_ReapApprovalReminders_NoApprovals(t *testing.T) {
	t.Parallel()
	deliveryCalled := false
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return nil, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.False(t, deliveryCalled)
}

func TestReaper_ReapApprovalReminders_SkipsWhenChannelLookupFails(t *testing.T) {
	t.Parallel()

	expires := time.Now().Add(10 * time.Minute)
	var deliveryCalled atomic.Bool
	var singleLookups atomic.Int32
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return []domain.WorkflowStepApproval{{
				ID:                "appr-channel-error",
				WorkflowRunID:     "wr-channel-error",
				WorkflowStepRunID: "sr-channel-error",
				Status:            domain.ApprovalStatusPending,
				ExpiresAt:         &expires,
			}}, nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			require.Equal(t, "wr-channel-error", id)
			return &domain.WorkflowRun{
				ID:         id,
				ProjectID:  "proj-channel-error",
				WorkflowID: "wf-channel-error",
			}, nil
		},
		listEnabledNotificationChannelsByProjectIDsFn: func(_ context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error) {
			require.Equal(t, []string{"proj-channel-error"}, projectIDs)
			return map[string][]domain.NotificationChannel{}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, projectID string) ([]domain.NotificationChannel, error) {
			require.Equal(t, "proj-channel-error", projectID)
			singleLookups.Add(1)
			return nil, errors.New("channel lookup failed")
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCalled.Store(true)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.EqualValues(t, 1, singleLookups.Load())
	require.False(t, deliveryCalled.Load())
}

func BenchmarkReaper_ReapApprovalReminders_ManyProjects(b *testing.B) {
	expires := time.Now().Add(10 * time.Minute)
	const approvalCount = 128
	approvals := make([]domain.WorkflowStepApproval, approvalCount)
	workflowRuns := make(map[string]*domain.WorkflowRun, approvalCount)
	for i := range approvalCount {
		workflowRunID := fmt.Sprintf("wr-%03d", i)
		approvals[i] = domain.WorkflowStepApproval{
			ID:                fmt.Sprintf("appr-%03d", i),
			WorkflowRunID:     workflowRunID,
			WorkflowStepRunID: fmt.Sprintf("sr-%03d", i),
			Status:            domain.ApprovalStatusPending,
			RequestedAt:       time.Now().Add(-time.Hour),
			ExpiresAt:         &expires,
		}
		workflowRuns[workflowRunID] = &domain.WorkflowRun{
			ID:         workflowRunID,
			ProjectID:  fmt.Sprintf("proj-%03d", i),
			WorkflowID: fmt.Sprintf("wf-%03d", i),
		}
	}

	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return approvals, nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return workflowRuns[id], nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, projectID string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-" + projectID, ProjectID: projectID}}, nil
		},
		listEnabledNotificationChannelsByProjectIDsFn: func(_ context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error) {
			result := make(map[string][]domain.NotificationChannel, len(projectIDs))
			for _, projectID := range projectIDs {
				result[projectID] = []domain.NotificationChannel{{ID: "ch-" + projectID, ProjectID: projectID}}
			}
			return result, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			return nil
		},
	}

	b.ReportAllocs()
	for b.Loop() {
		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapApprovalReminders(context.Background())
	}
}
