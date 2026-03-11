package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestReaper_ReapStale(t *testing.T) {
	t.Parallel()
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusExecuting},
				{ID: "run-2", JobID: "job-2", Status: domain.StatusExecuting},
			}, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			if from != domain.StatusExecuting {
				t.Errorf("expected from=executing, got %s", from)
			}
			if to != domain.StatusCrashed {
				t.Errorf("expected to=crashed, got %s", to)
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapStale(context.Background())

	if transitioned.Load() != 2 {
		t.Fatalf("expected 2 status transitions, got %d", transitioned.Load())
	}
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
			if to != domain.StatusExpired {
				t.Errorf("expected to=expired, got %s", to)
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapExpired(context.Background())

	if transitioned.Load() != 1 {
		t.Fatalf("expected 1 status transition, got %d", transitioned.Load())
	}
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
			if from != domain.StatusDequeued {
				t.Errorf("expected from=dequeued, got %s", from)
			}
			if to != domain.StatusQueued {
				t.Errorf("expected to=queued, got %s", to)
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapStaleDequeued(context.Background())

	if transitioned.Load() != 1 {
		t.Fatalf("expected 1 status transition, got %d", transitioned.Load())
	}
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

	if transitioned.Load() != 0 {
		t.Fatalf("expected 0 status transitions, got %d", transitioned.Load())
	}
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

	if ticked.Load() < 1 {
		t.Fatalf("expected at least 1 tick, got %d", ticked.Load())
	}
}

func TestReaper_ReapOldWorkflowRuns(t *testing.T) {
	t.Parallel()
	var deleted atomic.Int64
	ms := &mockReaperStore{
		deleteOldWorkflowRunsFn: func(_ context.Context, before time.Time, limit int) (int64, error) {
			if limit <= 0 {
				t.Fatalf("expected positive limit, got %d", limit)
			}
			if before.IsZero() {
				t.Fatal("expected non-zero before timestamp")
			}
			deleted.Store(3)
			return 3, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapOldWorkflowRuns(context.Background())

	if deleted.Load() != 3 {
		t.Fatalf("expected deleted count 3, got %d", deleted.Load())
	}
}

func TestReaper_ReapTimedOutWorkflows(t *testing.T) {
	t.Parallel()
	var wfUpdates atomic.Int32
	var stepUpdates atomic.Int32
	var runUpdates atomic.Int32

	ms := &mockReaperStore{
		listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
			return []domain.WorkflowRun{{ID: "wr-1", Status: domain.WfStatusRunning}}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
			if id != "wr-1" {
				t.Fatalf("unexpected workflow run id %q", id)
			}
			if from != domain.WfStatusRunning || to != domain.WfStatusTimedOut {
				t.Fatalf("unexpected workflow transition %s -> %s", from, to)
			}
			wfUpdates.Add(1)
			return nil
		},
		listStepRunsByWfRunFn: func(_ context.Context, workflowRunID string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			if workflowRunID != "wr-1" {
				t.Fatalf("unexpected workflowRunID %q", workflowRunID)
			}
			return []domain.WorkflowStepRun{{ID: "sr-1", Status: domain.StepRunning, JobRunID: "run-1"}}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if id != "sr-1" || status != domain.StepCanceled {
				t.Fatalf("unexpected step update %s -> %s", id, status)
			}
			stepUpdates.Add(1)
			return nil
		},
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id != "run-1" {
				t.Fatalf("unexpected run id %q", id)
			}
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, _ map[string]any) error {
			if id != "run-1" || from != domain.StatusExecuting || to != domain.StatusCanceled {
				t.Fatalf("unexpected run update %s %s -> %s", id, from, to)
			}
			runUpdates.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapTimedOutWorkflows(context.Background())

	if wfUpdates.Load() != 1 {
		t.Fatalf("expected 1 workflow update, got %d", wfUpdates.Load())
	}
	if stepUpdates.Load() != 1 {
		t.Fatalf("expected 1 step update, got %d", stepUpdates.Load())
	}
	if runUpdates.Load() != 1 {
		t.Fatalf("expected 1 job run update, got %d", runUpdates.Load())
	}
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

	if transitioned.Load() != 0 {
		t.Fatalf("expected 0 status transitions, got %d", transitioned.Load())
	}
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

	if updateCalls.Load() != 2 {
		t.Fatalf("expected 2 update attempts, got %d", updateCalls.Load())
	}
	if transitioned.Load() != 1 {
		t.Fatalf("expected 1 successful transition, got %d", transitioned.Load())
	}
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

	if transitioned.Load() != 0 {
		t.Fatalf("expected 0 status transitions, got %d", transitioned.Load())
	}
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

	if updateCalls.Load() != 2 {
		t.Fatalf("expected 2 update attempts, got %d", updateCalls.Load())
	}
	if transitioned.Load() != 1 {
		t.Fatalf("expected 1 successful transition, got %d", transitioned.Load())
	}
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

	if transitioned.Load() != 0 {
		t.Fatalf("expected 0 status transitions, got %d", transitioned.Load())
	}
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

	if updateCalls.Load() != 2 {
		t.Fatalf("expected 2 update attempts, got %d", updateCalls.Load())
	}
	if transitioned.Load() != 1 {
		t.Fatalf("expected 1 successful transition, got %d", transitioned.Load())
	}
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
				if id != "ap-1" || status != "timed_out" || approvedBy != "" || approvedAt != nil || errMsg != "approval timed out" {
					t.Fatalf("unexpected approval update payload id=%s status=%s approvedBy=%s approvedAtNil=%v err=%s", id, status, approvedBy, approvedAt == nil, errMsg)
				}
				approvalUpdates.Add(1)
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id != "sr-1" || status != domain.StepFailed {
					t.Fatalf("unexpected step update id=%s status=%s", id, status)
				}
				stepUpdates.Add(1)
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				if id != "wr-1" || from != domain.WfStatusRunning || to != domain.WfStatusFailed {
					t.Fatalf("unexpected workflow update id=%s %s->%s", id, from, to)
				}
				workflowUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())

		if approvalUpdates.Load() != 1 {
			t.Fatalf("expected 1 approval update, got %d", approvalUpdates.Load())
		}
		if stepUpdates.Load() != 1 {
			t.Fatalf("expected 1 step update, got %d", stepUpdates.Load())
		}
		if workflowUpdates.Load() != 1 {
			t.Fatalf("expected 1 workflow update, got %d", workflowUpdates.Load())
		}
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

		if approvalUpdates.Load() != 0 {
			t.Fatalf("expected 0 approval updates, got %d", approvalUpdates.Load())
		}
		if stepUpdates.Load() != 0 {
			t.Fatalf("expected 0 step updates, got %d", stepUpdates.Load())
		}
		if workflowUpdates.Load() != 0 {
			t.Fatalf("expected 0 workflow updates, got %d", workflowUpdates.Load())
		}
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
				if id != "sr-2" || status != domain.StepFailed {
					t.Fatalf("unexpected step update id=%s status=%s", id, status)
				}
				stepUpdates.Add(1)
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				if id != "wr-2" || from != domain.WfStatusRunning || to != domain.WfStatusFailed {
					t.Fatalf("unexpected workflow update id=%s %s->%s", id, from, to)
				}
				workflowUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())

		if stepUpdates.Load() != 1 {
			t.Fatalf("expected 1 step update, got %d", stepUpdates.Load())
		}
		if workflowUpdates.Load() != 1 {
			t.Fatalf("expected 1 workflow update, got %d", workflowUpdates.Load())
		}
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
				if from != domain.WfStatusRunning || to != domain.WfStatusFailed {
					t.Fatalf("unexpected workflow transition %s->%s", from, to)
				}
				workflowUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())

		if stepUpdates.Load() != 1 {
			t.Fatalf("expected 1 step update, got %d", stepUpdates.Load())
		}
		if workflowUpdates.Load() != 1 {
			t.Fatalf("expected 1 workflow update, got %d", workflowUpdates.Load())
		}
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
				if from != domain.WfStatusRunning || to != domain.WfStatusFailed {
					t.Fatalf("unexpected workflow transition %s->%s", from, to)
				}
				workflowUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())

		if workflowUpdates.Load() != 1 {
			t.Fatalf("expected 1 workflow update, got %d", workflowUpdates.Load())
		}
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
				if from != domain.WfStatusPaused || to != domain.WfStatusFailed {
					t.Fatalf("unexpected fallback workflow transition %s->%s", from, to)
				}
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())

		if workflowUpdates.Load() != 2 {
			t.Fatalf("expected 2 workflow update attempts, got %d", workflowUpdates.Load())
		}
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
				if to != domain.WfStatusFailed {
					t.Fatalf("unexpected workflow target status %s", to)
				}
				if id == "wr-1" {
					workflowUpdates.Add(1)
					return errors.New("workflow transition failed")
				}
				if id == "wr-2" && from == domain.WfStatusRunning {
					workflowUpdates.Add(1)
					return nil
				}
				if id == "wr-2" {
					t.Fatalf("did not expect fallback for wr-2")
				}
				workflowUpdates.Add(1)
				return errors.New("unexpected")
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())

		if approvalUpdates.Load() != 2 {
			t.Fatalf("expected 2 approval updates, got %d", approvalUpdates.Load())
		}
		if stepUpdates.Load() != 2 {
			t.Fatalf("expected 2 step updates, got %d", stepUpdates.Load())
		}
		if workflowUpdates.Load() != 3 {
			t.Fatalf("expected 3 workflow update attempts, got %d", workflowUpdates.Load())
		}
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
				if status != domain.StepFailed {
					t.Fatalf("unexpected step status %s", status)
				}
				stepUpdates.Add(1)
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				if from != domain.WfStatusRunning || to != domain.WfStatusFailed {
					t.Fatalf("unexpected workflow transition %s->%s", from, to)
				}
				workflowUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapExpiredApprovals(context.Background())

		if approvalUpdates.Load() != 3 {
			t.Fatalf("expected 3 approval updates, got %d", approvalUpdates.Load())
		}
		if stepUpdates.Load() != 3 {
			t.Fatalf("expected 3 step updates, got %d", stepUpdates.Load())
		}
		if workflowUpdates.Load() != 3 {
			t.Fatalf("expected 3 workflow updates, got %d", workflowUpdates.Load())
		}
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

		if deleteCalls.Load() != 0 {
			t.Fatalf("expected 0 delete calls, got %d", deleteCalls.Load())
		}
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

		if deleteCalls.Load() != 1 {
			t.Fatalf("expected 1 delete call, got %d", deleteCalls.Load())
		}
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

		if deleteCalls.Load() != 1 {
			t.Fatalf("expected 1 delete call, got %d", deleteCalls.Load())
		}
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

		if deleteCalls.Load() != 0 {
			t.Fatalf("expected 0 delete calls, got %d", deleteCalls.Load())
		}
	})
}

func TestReaper_ReapTimedOutWorkflows_EdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("list_error", func(t *testing.T) {
		t.Parallel()
		var wfUpdates atomic.Int32
		var stepLists atomic.Int32
		var stepUpdates atomic.Int32
		var runGets atomic.Int32
		var runUpdates atomic.Int32

		ms := &mockReaperStore{
			listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
				return nil, errors.New("list failed")
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				wfUpdates.Add(1)
				return nil
			},
			listStepRunsByWfRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				stepLists.Add(1)
				return nil, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				stepUpdates.Add(1)
				return nil
			},
			getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
				runGets.Add(1)
				return nil, nil
			},
			updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
				runUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapTimedOutWorkflows(context.Background())

		if wfUpdates.Load() != 0 || stepLists.Load() != 0 || stepUpdates.Load() != 0 || runGets.Load() != 0 || runUpdates.Load() != 0 {
			t.Fatalf("expected no update/list calls after list error, got wf=%d stepLists=%d stepUpdates=%d runGets=%d runUpdates=%d", wfUpdates.Load(), stepLists.Load(), stepUpdates.Load(), runGets.Load(), runUpdates.Load())
		}
	})

	t.Run("workflow_update_error_continues", func(t *testing.T) {
		t.Parallel()
		var wfUpdates atomic.Int32
		var stepLists atomic.Int32

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
			listStepRunsByWfRunFn: func(_ context.Context, workflowRunID string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				stepLists.Add(1)
				if workflowRunID != "wr-2" {
					t.Fatalf("expected step listing only for wr-2, got %s", workflowRunID)
				}
				return nil, nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapTimedOutWorkflows(context.Background())

		if wfUpdates.Load() != 2 {
			t.Fatalf("expected 2 workflow update attempts, got %d", wfUpdates.Load())
		}
		if stepLists.Load() != 1 {
			t.Fatalf("expected 1 step listing call, got %d", stepLists.Load())
		}
	})

	t.Run("step_listing_error_continues", func(t *testing.T) {
		t.Parallel()
		var stepLists atomic.Int32

		ms := &mockReaperStore{
			listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
				return []domain.WorkflowRun{
					{ID: "wr-1", Status: domain.WfStatusRunning},
					{ID: "wr-2", Status: domain.WfStatusRunning},
				}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepRunsByWfRunFn: func(_ context.Context, workflowRunID string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				stepLists.Add(1)
				if workflowRunID == "wr-1" {
					return nil, errors.New("list steps failed")
				}
				if workflowRunID != "wr-2" {
					t.Fatalf("unexpected workflowRunID %s", workflowRunID)
				}
				return nil, nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapTimedOutWorkflows(context.Background())

		if stepLists.Load() != 2 {
			t.Fatalf("expected 2 step listing attempts, got %d", stepLists.Load())
		}
	})

	t.Run("step_already_terminal_skipped", func(t *testing.T) {
		t.Parallel()
		var stepUpdates atomic.Int32

		ms := &mockReaperStore{
			listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
				return []domain.WorkflowRun{{ID: "wr-1", Status: domain.WfStatusRunning}}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepRunsByWfRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", Status: domain.StepCompleted}}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				stepUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapTimedOutWorkflows(context.Background())

		if stepUpdates.Load() != 0 {
			t.Fatalf("expected 0 step updates, got %d", stepUpdates.Load())
		}
	})

	t.Run("step_no_job_run_skipped", func(t *testing.T) {
		t.Parallel()
		var stepUpdates atomic.Int32
		var runGets atomic.Int32

		ms := &mockReaperStore{
			listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
				return []domain.WorkflowRun{{ID: "wr-1", Status: domain.WfStatusRunning}}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepRunsByWfRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", Status: domain.StepRunning, JobRunID: ""}}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id != "sr-1" || status != domain.StepCanceled {
					t.Fatalf("unexpected step update id=%s status=%s", id, status)
				}
				stepUpdates.Add(1)
				return nil
			},
			getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
				runGets.Add(1)
				return nil, nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapTimedOutWorkflows(context.Background())

		if stepUpdates.Load() != 1 {
			t.Fatalf("expected 1 step update, got %d", stepUpdates.Load())
		}
		if runGets.Load() != 0 {
			t.Fatalf("expected 0 get run calls, got %d", runGets.Load())
		}
	})

	t.Run("get_job_run_error_continues", func(t *testing.T) {
		t.Parallel()
		var runGets atomic.Int32
		var runUpdates atomic.Int32

		ms := &mockReaperStore{
			listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
				return []domain.WorkflowRun{{ID: "wr-1", Status: domain.WfStatusRunning}}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepRunsByWfRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-1", Status: domain.StepRunning, JobRunID: "run-1"},
					{ID: "sr-2", Status: domain.StepRunning, JobRunID: "run-2"},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
				runGets.Add(1)
				if id == "run-1" {
					return nil, errors.New("get run failed")
				}
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			},
			updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
				runUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapTimedOutWorkflows(context.Background())

		if runGets.Load() != 2 {
			t.Fatalf("expected 2 get run calls, got %d", runGets.Load())
		}
		if runUpdates.Load() != 1 {
			t.Fatalf("expected 1 run update call, got %d", runUpdates.Load())
		}
	})

	t.Run("job_run_already_terminal_skipped", func(t *testing.T) {
		t.Parallel()
		var runUpdates atomic.Int32

		ms := &mockReaperStore{
			listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
				return []domain.WorkflowRun{{ID: "wr-1", Status: domain.WfStatusRunning}}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepRunsByWfRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", Status: domain.StepRunning, JobRunID: "run-1"}}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
				return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
			},
			updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
				runUpdates.Add(1)
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapTimedOutWorkflows(context.Background())

		if runUpdates.Load() != 0 {
			t.Fatalf("expected 0 run updates, got %d", runUpdates.Load())
		}
	})

	t.Run("job_run_update_error_continues", func(t *testing.T) {
		t.Parallel()
		var runUpdates atomic.Int32

		ms := &mockReaperStore{
			listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
				return []domain.WorkflowRun{{ID: "wr-1", Status: domain.WfStatusRunning}}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepRunsByWfRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-1", Status: domain.StepRunning, JobRunID: "run-1"},
					{ID: "sr-2", Status: domain.StepRunning, JobRunID: "run-2"},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			},
			updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, _ map[string]any) error {
				runUpdates.Add(1)
				if from != domain.StatusExecuting || to != domain.StatusCanceled {
					t.Fatalf("unexpected run transition %s->%s", from, to)
				}
				if id == "run-1" {
					return errors.New("update run failed")
				}
				return nil
			},
		}

		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.reapTimedOutWorkflows(context.Background())

		if runUpdates.Load() != 2 {
			t.Fatalf("expected 2 run update attempts, got %d", runUpdates.Load())
		}
	})
}

func TestReaper_WithWorkflowRetention(t *testing.T) {
	t.Parallel()
	t.Run("sets_positive_retention", func(t *testing.T) {
		t.Parallel()
		ms := &mockReaperStore{}
		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.WithWorkflowRetention(7 * 24 * time.Hour)

		if r.workflowRetention != 7*24*time.Hour {
			t.Fatalf("expected 7d retention, got %v", r.workflowRetention)
		}
	})

	t.Run("ignores_zero_retention", func(t *testing.T) {
		t.Parallel()
		ms := &mockReaperStore{}
		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.WithWorkflowRetention(0)

		if r.workflowRetention != defaultWorkflowRetention {
			t.Fatalf("expected default retention, got %v", r.workflowRetention)
		}
	})

	t.Run("ignores_negative_retention", func(t *testing.T) {
		t.Parallel()
		ms := &mockReaperStore{}
		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
		r.WithWorkflowRetention(-time.Hour)

		if r.workflowRetention != defaultWorkflowRetention {
			t.Fatalf("expected default retention, got %v", r.workflowRetention)
		}
	})

	t.Run("custom_retention_used_in_reap", func(t *testing.T) {
		t.Parallel()
		var deletedBefore time.Time
		var deleteCalls atomic.Int32

		ms := &mockReaperStore{
			deleteOldWorkflowRunsFn: func(_ context.Context, before time.Time, limit int) (int64, error) {
				deletedBefore = before
				deleteCalls.Add(1)
				if limit != defaultDeleteBatchLimit {
					t.Fatalf("expected batch limit %d, got %d", defaultDeleteBatchLimit, limit)
				}
				return 5, nil
			},
		}

		customRetention := 3 * 24 * time.Hour
		r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).
			WithWorkflowRetention(customRetention)
		r.reapOldWorkflowRuns(context.Background())

		if deleteCalls.Load() != 1 {
			t.Fatalf("expected 1 delete call, got %d", deleteCalls.Load())
		}

		// The before time should be approximately now - 3 days
		expectedBefore := time.Now().Add(-customRetention)
		diff := expectedBefore.Sub(deletedBefore)
		if diff < -time.Minute || diff > time.Minute {
			t.Fatalf("expected before time near %v, got %v (diff: %v)", expectedBefore, deletedBefore, diff)
		}
	})
}

func TestReaper_ReapTerminalRetention(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	ms := &mockReaperStore{
		deleteRetentionFn: func(_ context.Context, shortRetention, longRetention time.Duration) (int64, error) {
			if shortRetention != 30*24*time.Hour {
				t.Fatalf("short retention = %v, want %v", shortRetention, 30*24*time.Hour)
			}
			if longRetention != 90*24*time.Hour {
				t.Fatalf("long retention = %v, want %v", longRetention, 90*24*time.Hour)
			}
			called.Add(1)
			return 2, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, true, nil)
	r.reapTerminalRetention(context.Background())

	if called.Load() != 1 {
		t.Fatalf("retention call count = %d, want 1", called.Load())
	}
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

	if called.Load() != 0 {
		t.Fatalf("retention should not be called when disabled, got %d calls", called.Load())
	}
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

	if called.Load() < 1 {
		t.Fatalf("retention should be called when enabled, got %d calls", called.Load())
	}
}

func TestReaper_CustomRetentionPeriods(t *testing.T) {
	t.Parallel()
	customShort := 7 * 24 * time.Hour
	customLong := 14 * 24 * time.Hour
	var called atomic.Int32
	ms := &mockReaperStore{
		deleteRetentionFn: func(_ context.Context, shortRetention, longRetention time.Duration) (int64, error) {
			if shortRetention != customShort {
				t.Fatalf("short retention = %v, want %v", shortRetention, customShort)
			}
			if longRetention != customLong {
				t.Fatalf("long retention = %v, want %v", longRetention, customLong)
			}
			called.Add(1)
			return 0, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, customShort, customLong, true, nil)
	r.reapTerminalRetention(context.Background())

	if called.Load() != 1 {
		t.Fatalf("retention call count = %d, want 1", called.Load())
	}
}

func TestReaper_DefaultRetentionPeriodsWhenZero(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	ms := &mockReaperStore{
		deleteRetentionFn: func(_ context.Context, shortRetention, longRetention time.Duration) (int64, error) {
			if shortRetention != 30*24*time.Hour {
				t.Fatalf("default short retention = %v, want %v", shortRetention, 30*24*time.Hour)
			}
			if longRetention != 90*24*time.Hour {
				t.Fatalf("default long retention = %v, want %v", longRetention, 90*24*time.Hour)
			}
			called.Add(1)
			return 0, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, true, nil)
	r.reapTerminalRetention(context.Background())

	if called.Load() != 1 {
		t.Fatalf("retention call count = %d, want 1", called.Load())
	}
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

	if !triggerTimedOut {
		t.Fatal("expected event trigger to be marked timed out")
	}
	if !stepFailed {
		t.Fatal("expected step run to be marked failed")
	}
	if !workflowFailed {
		t.Fatal("expected workflow run to be marked failed")
	}
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

	if !triggerTimedOut {
		t.Fatal("expected event trigger to be marked timed out")
	}
	if !runTimedOut {
		t.Fatal("expected job run to be marked timed out")
	}
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

	if updateRunCalled {
		t.Fatal("should not update already-terminal job run")
	}
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

	if updatedTriggerStatus != domain.EventTriggerStatusReceived {
		t.Fatalf("expected trigger status=received, got %s", updatedTriggerStatus)
	}
	if updatedStepStatus != domain.StepCompleted {
		t.Fatalf("expected step status=completed, got %s", updatedStepStatus)
	}
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

	if !callbackCalled {
		t.Fatal("expected OnStepCompleted callback to be called")
	}
	if callbackRunID != "wr-1" {
		t.Fatalf("expected workflow run ID wr-1, got %s", callbackRunID)
	}
	if callbackStepID != "sr-1" {
		t.Fatalf("expected step run ID sr-1, got %s", callbackStepID)
	}
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

	if !failedCallbackCalled {
		t.Fatal("expected OnStepFailed callback to be called")
	}
	if failedRunID != "wr-1" {
		t.Fatalf("expected workflow run ID wr-1, got %s", failedRunID)
	}
	if failedStepID != "sr-1" {
		t.Fatalf("expected step run ID sr-1, got %s", failedStepID)
	}
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

	if !wfStatusUpdated {
		t.Fatal("expected direct workflow run status update when callback is nil")
	}
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

	if !onEventCalled {
		t.Fatal("expected OnEventReceived to be called for inconsistent event trigger")
	}
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

	if !onStepCompletedCalled {
		t.Fatal("expected OnStepCompleted for inconsistent sleep trigger")
	}
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

	if requeuedRunID != "run-stale" {
		t.Fatalf("expected run-stale to be requeued, got %q", requeuedRunID)
	}
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

	if !triggerUpdated {
		t.Fatal("expected trigger status to be updated even if step update fails")
	}
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

	if !triggerUpdated {
		t.Fatal("expected trigger status to be updated for sleep trigger without step")
	}
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

	if !stepUpdated {
		t.Fatal("expected step to be updated even with nil callback")
	}
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

	if stepUpdated {
		t.Fatal("step should NOT be updated when trigger update fails")
	}
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

	// The second trigger (job run) should still be processed despite the first failing.
	if requeuedRunID != "run-2" {
		t.Fatalf("expected run-2 to be requeued after first trigger error, got %q", requeuedRunID)
	}
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

	if updateCalled {
		t.Fatal("expected empty job run ID to be skipped")
	}
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
			if workflowRunID != "wr-1" {
				t.Fatalf("workflowRunID = %s, want wr-1", workflowRunID)
			}
			resumed.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, cb).WithStalledAction("reconcile")
	r.reapStalledWorkflows(context.Background())

	if resumed.Load() != 1 {
		t.Fatalf("resumed count = %d, want 1", resumed.Load())
	}
}

func TestReaper_ReapStalledWorkflows_FailWorkflow(t *testing.T) {
	t.Parallel()

	var failed atomic.Int32
	ms := &mockReaperStore{
		listStalledWorkflowRunsFn: func(_ context.Context, _ time.Duration) ([]domain.WorkflowRun, error) {
			return []domain.WorkflowRun{{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
			if id != "wr-1" {
				t.Fatalf("id = %s, want wr-1", id)
			}
			if from != domain.WfStatusRunning || to != domain.WfStatusFailed {
				t.Fatalf("unexpected transition %s -> %s", from, to)
			}
			if fields["finished_at"] == nil {
				t.Fatal("expected finished_at field")
			}
			failed.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).WithStalledAction("fail_workflow")
	r.reapStalledWorkflows(context.Background())

	if failed.Load() != 1 {
		t.Fatalf("failed transition count = %d, want 1", failed.Load())
	}
}
