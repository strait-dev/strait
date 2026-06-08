package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	otelTrace "go.opentelemetry.io/otel/trace"
	noopTrace "go.opentelemetry.io/otel/trace/noop"
)

type mockEngineStore struct {
	getWorkflowFn                     func(ctx context.Context, id string) (*domain.Workflow, error)
	getActiveCanaryDeploymentFn       func(ctx context.Context, workflowID string) (*domain.CanaryDeployment, error)
	getWorkflowVersionFn              func(ctx context.Context, workflowID string, version int) (*domain.WorkflowVersion, error)
	listStepsByWorkflowVerFn          func(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	countRunningWorkflowRunsFn        func(ctx context.Context, workflowID string) (int, error)
	createWorkflowRunFn               func(ctx context.Context, run *domain.WorkflowRun) error
	createWorkflowRunBootstrapFn      func(ctx context.Context, run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, startedAt time.Time) error
	isProjectRunnableFn               func(ctx context.Context, projectID string) (bool, error)
	createWorkflowStepRunFn           func(ctx context.Context, sr *domain.WorkflowStepRun) error
	createWorkflowStepApprovalFn      func(ctx context.Context, approval *domain.WorkflowStepApproval) error
	createEventTriggerFn              func(ctx context.Context, trigger *domain.EventTrigger) error
	updateWorkflowRunStatusFn         func(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	updateStepRunStatusFn             func(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	getStepOutputsFn                  func(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error)
	getWorkflowRunFn                  func(ctx context.Context, id string) (*domain.WorkflowRun, error)
	listStepRunsByWorkflowRunFn       func(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
	getWorkflowRunsByParentFn         func(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error)
	listEnabledNotificationChannelsFn func(projectID string) ([]domain.NotificationChannel, error)
	createNotificationDeliveryFn      func(d *domain.NotificationDelivery) error
}

func (m *mockEngineStore) GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error) {
	if m.getWorkflowFn != nil {
		return m.getWorkflowFn(ctx, id)
	}
	return nil, nil
}

func (m *mockEngineStore) GetActiveCanaryDeployment(ctx context.Context, workflowID string) (*domain.CanaryDeployment, error) {
	if m.getActiveCanaryDeploymentFn != nil {
		return m.getActiveCanaryDeploymentFn(ctx, workflowID)
	}
	return nil, domain.ErrCanaryNotFound
}

func (m *mockEngineStore) GetWorkflowVersion(ctx context.Context, workflowID string, version int) (*domain.WorkflowVersion, error) {
	if m.getWorkflowVersionFn != nil {
		return m.getWorkflowVersionFn(ctx, workflowID, version)
	}
	return nil, nil
}

func (m *mockEngineStore) IsProjectRunnable(ctx context.Context, projectID string) (bool, error) {
	if m.isProjectRunnableFn != nil {
		return m.isProjectRunnableFn(ctx, projectID)
	}
	return true, nil
}

func (m *mockEngineStore) ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
	if m.listStepsByWorkflowVerFn != nil {
		return m.listStepsByWorkflowVerFn(ctx, workflowID, version)
	}
	return nil, nil
}

func (m *mockEngineStore) CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error) {
	if m.countRunningWorkflowRunsFn != nil {
		return m.countRunningWorkflowRunsFn(ctx, workflowID)
	}
	return 0, nil
}

func (m *mockEngineStore) CreateWorkflowRun(ctx context.Context, run *domain.WorkflowRun) error {
	if m.createWorkflowRunFn != nil {
		return m.createWorkflowRunFn(ctx, run)
	}
	return nil
}

func (m *mockEngineStore) CreateWorkflowRunBootstrap(ctx context.Context, run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, startedAt time.Time) error {
	if m.createWorkflowRunBootstrapFn != nil {
		return m.createWorkflowRunBootstrapFn(ctx, run, stepRuns, startedAt)
	}
	if err := m.CreateWorkflowRun(ctx, run); err != nil {
		return err
	}
	if err := m.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": startedAt}); err != nil {
		return err
	}
	for i := range stepRuns {
		if err := m.CreateWorkflowStepRun(ctx, &stepRuns[i]); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockEngineStore) CreateWorkflowStepRun(ctx context.Context, sr *domain.WorkflowStepRun) error {
	if m.createWorkflowStepRunFn != nil {
		return m.createWorkflowStepRunFn(ctx, sr)
	}
	return nil
}

func (m *mockEngineStore) CreateWorkflowStepApproval(ctx context.Context, approval *domain.WorkflowStepApproval) error {
	if m.createWorkflowStepApprovalFn != nil {
		return m.createWorkflowStepApprovalFn(ctx, approval)
	}
	return nil
}

func (m *mockEngineStore) CreateEventTrigger(ctx context.Context, trigger *domain.EventTrigger) error {
	if m.createEventTriggerFn != nil {
		return m.createEventTriggerFn(ctx, trigger)
	}
	return nil
}

func (m *mockEngineStore) UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
	if m.updateWorkflowRunStatusFn != nil {
		return m.updateWorkflowRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockEngineStore) UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
	if m.updateStepRunStatusFn != nil {
		return m.updateStepRunStatusFn(ctx, id, status, fields)
	}
	return nil
}

func (m *mockEngineStore) GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error) {
	if m.getStepOutputsFn != nil {
		return m.getStepOutputsFn(ctx, workflowRunID, stepRefs)
	}
	return nil, nil
}

func (m *mockEngineStore) GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	if m.getWorkflowRunFn != nil {
		return m.getWorkflowRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockEngineStore) ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error) {
	if m.listStepRunsByWorkflowRunFn != nil {
		return m.listStepRunsByWorkflowRunFn(ctx, workflowRunID, limit, cursor)
	}
	return nil, nil
}

func (m *mockEngineStore) GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error) {
	if m.getWorkflowRunsByParentFn != nil {
		return m.getWorkflowRunsByParentFn(ctx, parentWorkflowRunID)
	}
	return nil, nil
}

func (m *mockEngineStore) GetOrCreateWorkflowSnapshot(_ context.Context, _ *domain.Workflow, _ []domain.WorkflowStep) (*domain.WorkflowSnapshot, error) {
	return &domain.WorkflowSnapshot{ID: "snap-test"}, nil
}

func (m *mockEngineStore) CopyRunState(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockEngineStore) GetJobCostEstimate(_ context.Context, _ string) (*domain.JobCostEstimate, error) {
	return nil, nil
}

func (m *mockEngineStore) ListEnabledNotificationChannels(_ context.Context, projectID string) ([]domain.NotificationChannel, error) {
	if m.listEnabledNotificationChannelsFn != nil {
		return m.listEnabledNotificationChannelsFn(projectID)
	}
	return nil, nil
}

func (m *mockEngineStore) CreateNotificationDelivery(_ context.Context, d *domain.NotificationDelivery) error {
	if m.createNotificationDeliveryFn != nil {
		return m.createNotificationDeliveryFn(d)
	}
	return nil
}

type mockEngineQueue struct {
	enqueueFn           func(ctx context.Context, run *domain.JobRun) error
	requeuePausedRunsFn func(ctx context.Context, workflowRunID string) (int64, error)
}

func (m *mockEngineQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, run)
	}
	return nil
}

func (m *mockEngineQueue) RequeuePausedJobRuns(ctx context.Context, workflowRunID string) (int64, error) {
	if m.requeuePausedRunsFn != nil {
		return m.requeuePausedRunsFn(ctx, workflowRunID)
	}
	return 0, nil
}

func (m *mockEngineQueue) Dequeue(context.Context) (*domain.JobRun, error) {
	return nil, nil
}

func (m *mockEngineQueue) DequeueN(context.Context, int) ([]domain.JobRun, error) {
	return nil, nil
}

func (m *mockEngineQueue) DequeueNByProject(context.Context, int, string) ([]domain.JobRun, error) {
	return nil, nil
}

func buildWfCtx(run *domain.WorkflowRun, steps []domain.WorkflowStep) *wfCtx {
	byRef := make(map[string]domain.WorkflowStep, len(steps))
	stepIndex := make(map[string]int, len(steps))
	for i, s := range steps {
		byRef[s.StepRef] = s
		stepIndex[s.StepRef] = i
	}
	return &wfCtx{run: run, steps: steps, stepByRef: byRef, stepIndex: stepIndex}
}

func TestTriggerWorkflow(t *testing.T) {
	t.Parallel()
	t.Run("happy path starts root steps only", func(t *testing.T) {
		t.Parallel()
		stepRunsCreated := make(map[string]domain.WorkflowStepRun)
		enqueued := 0
		updateStepCalls := 0
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
					{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-1"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				require.False(t,
					from != domain.
						WfStatusPending ||
						to !=
							domain.
								WfStatusRunning,
				)

				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = *sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id == "sr-a" {
					require.Equal(t,
						domain.StepRunning,
						status)

					updateStepCalls++
				}
				return nil
			},
		}
		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueued++
				run.ID = "run-a"
				require.False(t,
					run.JobID !=
						"job-a" || run.
						WorkflowStepRunID !=
						"sr-a")

				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		wfRun, err := engine.TriggerWorkflow(context.Background(), "wf-1", "proj-1", json.RawMessage(`{"k":"v"}`), "manual", nil, nil)
		require.NoError(
			t, err)
		require.False(t,
			wfRun == nil ||
				wfRun.ID !=
					"wr-1" ||
				wfRun.
					Status != domain.
					WfStatusRunning,
		)
		require.Equal(t, 1, enqueued)
		require.NotEqual(t, 0, updateStepCalls)
		require.Equal(t,
			domain.StepWaiting,
			stepRunsCreated["b"].Status,
		)
	})

	t.Run("root steps with same concurrency_key do not run in parallel", func(t *testing.T) {
		t.Parallel()
		started := make(map[string]struct{})
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "s1", JobID: "job-1", StepRef: "a", ConcurrencyKey: "db"},
					{ID: "s2", JobID: "job-2", StepRef: "b", ConcurrencyKey: "db"},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-1"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error { return nil },
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if status == domain.StepRunning {
					started[id] = struct{}{}
				}
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error { run.ID = "jr-" + run.JobID; return nil }}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "manual", nil, nil)
		require.NoError(
			t, err)
		require.Len(t, started,
			1)
	})

	t.Run("disabled workflow", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf", ProjectID: "proj-1", Enabled: false}, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "", nil, nil)
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "disabled")
	})

	t.Run("empty steps", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf", ProjectID: "proj-1", Enabled: true}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return nil, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "", nil, nil)
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "at least one step",
		)
	})

	t.Run("project mismatch", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf", ProjectID: "proj-a", Enabled: true}, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-b", nil, "", nil, nil)
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "does not belong",
		)
	})

	t.Run("inactive project", func(t *testing.T) {
		t.Parallel()
		var listedSteps bool
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf", ProjectID: "proj-1", Enabled: true}, nil
			},
			isProjectRunnableFn: func(_ context.Context, projectID string) (bool, error) {
				require.Equal(t,
					"proj-1",
					projectID)

				return false, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				listedSteps = true
				return nil, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "", nil, nil)
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "not active",
		)
		require.False(t,
			listedSteps,
		)
	})

	t.Run("GetWorkflow error", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return nil, errors.New("db get workflow failed")
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "", nil, nil)
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "get workflow",
		)
	})

	t.Run("ListStepsByWorkflowVersion error", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf", ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return nil, errors.New("db list steps failed")
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "", nil, nil)
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "list workflow steps by version",
		)
	})

	t.Run("CreateWorkflowStepRun error", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf", ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-a", JobID: "job-a", StepRef: "a"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-1"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, _ *domain.WorkflowStepRun) error {
				return errors.New("db create step run failed")
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "", nil, nil)
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "create step run",
		)
	})
	t.Run("bootstrap path sets workflow_run_id on step runs", func(t *testing.T) {
		t.Parallel()

		capturedRunID := ""
		capturedStepRuns := make([]domain.WorkflowStepRun, 0, 2)
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
					{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
				}, nil
			},
			createWorkflowRunBootstrapFn: func(_ context.Context, run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, startedAt time.Time) error {
				require.NotEmpty(t, run.
					ID)
				require.False(t,
					startedAt.
						IsZero())

				capturedRunID = run.ID
				capturedStepRuns = append(capturedStepRuns[:0], stepRuns...)
				for _, sr := range stepRuns {
					require.Equal(t,
						run.ID, sr.
							WorkflowRunID)
					require.NotEmpty(t, sr.
						ID)
				}
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-" + run.JobID
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		wfRun, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "manual", nil, nil)
		require.NoError(
			t, err)
		require.NotEmpty(t, wfRun.
			ID)
		require.Equal(t,
			capturedRunID,
			wfRun.ID)
		require.Len(t, capturedStepRuns,

			2)
	})
}

func TestTriggerWorkflow_AppliesActiveCanaryRouting(t *testing.T) {
	var listedVersion int
	var createdVersion int
	var createdVersionID string
	ms := &mockEngineStore{
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{
				ID:        id,
				ProjectID: "proj-1",
				Enabled:   true,
				Version:   1,
				VersionID: "wf-v1",
			}, nil
		},
		getActiveCanaryDeploymentFn: func(_ context.Context, workflowID string) (*domain.CanaryDeployment, error) {
			require.Equal(t,
				"wf-1", workflowID,
			)

			return &domain.CanaryDeployment{
				WorkflowID:    "wf-1",
				ProjectID:     "proj-1",
				SourceVersion: 1,
				TargetVersion: 2,
				TrafficPct:    100,
				Status:        "active",
			}, nil
		},
		getWorkflowVersionFn: func(_ context.Context, workflowID string, version int) (*domain.WorkflowVersion, error) {
			require.False(t,
				workflowID !=
					"wf-1" || version !=
					2)

			return &domain.WorkflowVersion{
				WorkflowID:        "wf-1",
				ProjectID:         "proj-1",
				Version:           2,
				VersionID:         "wf-v2",
				Name:              "Workflow v2",
				Slug:              "workflow",
				Enabled:           true,
				MaxConcurrentRuns: 4,
				MaxParallelSteps:  3,
			}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
			require.Equal(t,
				"wf-1", workflowID,
			)

			listedVersion = version
			return []domain.WorkflowStep{{ID: "step-v2", JobID: "job-v2", StepRef: "root"}}, nil
		},
		createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
			run.ID = "wr-canary"
			createdVersion = run.WorkflowVersion
			createdVersionID = run.WorkflowVersionID
			return nil
		},
	}
	mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		run.ID = "run-v2"
		return nil
	}}

	engine := NewWorkflowEngine(ms, mq, slog.Default())
	wfRun, err := engine.TriggerWorkflow(context.Background(), "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	require.Equal(t, 2, listedVersion)
	require.False(t,
		createdVersion !=
			2 || createdVersionID !=
			"wf-v2",
	)
	require.False(t,
		wfRun.WorkflowVersion !=
			2 ||
			wfRun.WorkflowVersionID !=
				"wf-v2")
}

func TestTriggerWorkflow_SnapshotIDPopulated(t *testing.T) {
	t.Parallel()

	var capturedRun *domain.WorkflowRun
	ms := &mockEngineStore{
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, VersionID: "vid-1"}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{ID: "s1", JobID: "job-1", StepRef: "a"},
			}, nil
		},
		createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
			capturedRun = run
			run.ID = "wr-1"
			return nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
			sr.ID = "sr-" + sr.StepRef
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}
	mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error { run.ID = "jr-1"; return nil }}

	engine := NewWorkflowEngine(ms, mq, slog.Default())
	wfRun, err := engine.TriggerWorkflow(context.Background(), "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	assert.Equal(t,
		"snap-test",
		wfRun.WorkflowSnapshotID,
	)
	assert.False(t,
		capturedRun !=
			nil && capturedRun.
			WorkflowSnapshotID !=
			"snap-test",
	)
}

func TestTriggerWorkflow_SnapshotFailureIsFatal(t *testing.T) {
	t.Parallel()

	snapshotCalled := false
	ms := &mockEngineStore{
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "s1", JobID: "j1", StepRef: "a"}}, nil
		},
	}
	// Override the mock snapshot to return an error.
	origGetOrCreate := ms.GetOrCreateWorkflowSnapshot
	_ = origGetOrCreate

	engine := NewWorkflowEngine(&snapshotFailStore{mockEngineStore: ms}, &mockEngineQueue{}, slog.Default())
	_, err := engine.TriggerWorkflow(context.Background(), "wf-1", "proj-1", nil, "manual", nil, nil)
	require.Error(t,
		err)
	assert.Contains(t, err.Error(), "create workflow snapshot")

	_ = snapshotCalled
}

// snapshotFailStore wraps mockEngineStore but fails GetOrCreateWorkflowSnapshot.
type snapshotFailStore struct {
	*mockEngineStore
}

func (s *snapshotFailStore) GetOrCreateWorkflowSnapshot(_ context.Context, _ *domain.Workflow, _ []domain.WorkflowStep) (*domain.WorkflowSnapshot, error) {
	return nil, fmt.Errorf("database connection failed")
}

func TestTriggerWorkflow_NestingDepthExceeded(t *testing.T) {
	t.Parallel()

	ms := &mockEngineStore{
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			switch id {
			case "wf-parent", "wf-child":
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			default:
				return nil, nil
			}
		},
		listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
			if workflowID == "wf-parent" {
				return []domain.WorkflowStep{{
					ID:              "step-sub",
					StepRef:         "sub",
					StepType:        domain.WorkflowStepTypeSubWorkflow,
					SubWorkflowID:   "wf-child",
					MaxNestingDepth: 1,
				}}, nil
			}
			return []domain.WorkflowStep{{ID: "child-root", StepRef: "child-root", JobID: "job-child"}}, nil
		},
		createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
			run.ID = "wr-" + run.WorkflowID
			return nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
			sr.ID = "sr-" + sr.StepRef
			return nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			if id == "parent-run" {
				return &domain.WorkflowRun{ID: "parent-run"}, nil
			}
			return nil, nil
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
	_, err := engine.TriggerSubWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, "parent-run", "")
	require.Error(t,
		err)
	require.Contains(t,
		err.Error(), "nesting depth")
}

func TestTriggerWorkflow_ConcurrencyLimitReached(t *testing.T) {
	t.Parallel()

	ms := &mockEngineStore{
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1, MaxConcurrentRuns: 1}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "step-a", JobID: "job-a", StepRef: "a"}}, nil
		},
		countRunningWorkflowRunsFn: func(_ context.Context, _ string) (int, error) {
			return 1, nil
		},
	}
	mq := &mockEngineQueue{}

	engine := NewWorkflowEngine(ms, mq, slog.Default())
	_, err := engine.TriggerWorkflow(context.Background(), "wf-1", "proj-1", nil, domain.TriggerWorkflow, nil, nil)
	require.Error(t,
		err)
	require.Contains(t,
		err.Error(), "max concurrent runs")
}

func TestMergePayloads(t *testing.T) {
	t.Parallel()
	t.Run("object merge with parent outputs", func(t *testing.T) {
		t.Parallel()
		out := mergePayloads(
			json.RawMessage(`{"a":1,"shared":"trigger"}`),
			json.RawMessage(`{"b":2,"shared":"step"}`),
			json.RawMessage(`{"p":{"ok":true}}`),
		)

		var got map[string]any
		require.NoError(
			t, json.Unmarshal(out, &got),
		)
		require.Equal(t,
			"step", got["shared"])
		require.False(t,
			got["a"] !=
				float64(1) || got["b"] !=
				float64(2))

		if _, ok := got["parent_outputs"]; !ok {
			require.Failf(t, "test failure",

				"missing parent_outputs: %+v", got)
		}
	})

	t.Run("step payload overrides non-object fallback", func(t *testing.T) {
		t.Parallel()
		out := mergePayloads(json.RawMessage(`{"a":1}`), json.RawMessage(`"step"`), nil)
		require.Equal(t,
			`"step"`,
			string(out))
	})

	t.Run("empty step payload keeps trigger payload", func(t *testing.T) {
		t.Parallel()
		out := mergePayloads(json.RawMessage(`{"a":1}`), nil, nil)
		require.Equal(t,
			`{"a":1}`,
			string(out))
	})

	t.Run("empty trigger payload keeps step payload", func(t *testing.T) {
		t.Parallel()
		out := mergePayloads(nil, json.RawMessage(`{"step":true}`), nil)
		require.Equal(t,
			`{"step":true}`,
			string(out))
	})

	t.Run("parent outputs added when trigger has payload and step is empty", func(t *testing.T) {
		t.Parallel()
		out := mergePayloads(json.RawMessage(`{"a":1}`), nil, json.RawMessage(`{"p":true}`))

		var got map[string]any
		require.NoError(
			t, json.Unmarshal(out, &got),
		)
		require.InDelta(t,
			float64(1),
			got["a"], 1e-9)

		if _, ok := got["parent_outputs"]; !ok {
			require.Failf(t, "test failure",

				"missing parent_outputs: %+v", got)
		}
	})

	t.Run("duplicate keys keep step payload value", func(t *testing.T) {
		t.Parallel()
		out := mergePayloads(json.RawMessage(`{"a":1,"keep":true}`), json.RawMessage(`{"a":2}`), nil)

		var got map[string]any
		require.NoError(
			t, json.Unmarshal(out, &got),
		)
		require.InDelta(t,
			float64(2),
			got["a"], 1e-9)
		require.Equal(t,
			true, got["keep"])
	})

	t.Run("duplicate keys keep last value within each payload", func(t *testing.T) {
		t.Parallel()
		out := mergePayloads(json.RawMessage(`{"a":1,"a":2}`), json.RawMessage(`{"b":1,"b":2}`), nil)

		var got map[string]any
		require.NoError(
			t, json.Unmarshal(out, &got),
		)
		require.InDelta(t,
			float64(2),
			got["a"], 1e-9)
		require.InDelta(t,
			float64(2),
			got["b"], 1e-9)
	})

	t.Run("escaped keys still merge", func(t *testing.T) {
		t.Parallel()
		out := mergePayloads(json.RawMessage(`{"tenant\u002did":"trigger"}`), json.RawMessage(`{"tenant\u002did":"step"}`), nil)

		var got map[string]any
		require.NoError(
			t, json.Unmarshal(out, &got),
		)
		require.Equal(t,
			"step", got["tenant-id"])
	})
}

func BenchmarkMergePayloads(b *testing.B) {
	triggerPayload := json.RawMessage(`{"account_id":"acct-123","region":"us-east-1","attempt":1,"flags":{"dry_run":false}}`)
	stepPayload := json.RawMessage(`{"step":"validate","attempt":2,"limits":{"cpu":"500m","memory":"512Mi"}}`)
	parentOutputs := json.RawMessage(`{"extract":{"rows":1000},"normalize":{"status":"completed"}}`)
	nonObjectStepPayload := json.RawMessage(`"step"`)

	b.Run("trigger_only", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = mergePayloads(triggerPayload, nil, nil)
		}
	})
	b.Run("step_only", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = mergePayloads(nil, stepPayload, nil)
		}
	})
	b.Run("object_merge", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = mergePayloads(triggerPayload, stepPayload, nil)
		}
	})
	b.Run("object_merge_with_parent_outputs", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = mergePayloads(triggerPayload, stepPayload, parentOutputs)
		}
	})
	b.Run("non_object_step_fallback", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = mergePayloads(triggerPayload, nonObjectStepPayload, nil)
		}
	})
}

type mockCallbackStore struct {
	getStepRunByJobRunIDFn              func(ctx context.Context, jobRunID string) (*domain.WorkflowStepRun, error)
	getWorkflowStepRunFn                func(ctx context.Context, id string) (*domain.WorkflowStepRun, error)
	listWorkflowStepRunsByIDsFn         func(ctx context.Context, ids []string) ([]domain.WorkflowStepRun, error)
	updateStepRunStatusFn               func(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	incrementStepDepsFn                 func(ctx context.Context, workflowRunID string, completedStepRef string) ([]store.StepDepResult, error)
	incrementStepDepsBatchFn            func(ctx context.Context, workflowRunID string, completedStepRefs []string) ([]store.StepDepResult, error)
	incrementStepRunAttemptFn           func(ctx context.Context, id string, newAttempt int) error
	getWorkflowRunFn                    func(ctx context.Context, id string) (*domain.WorkflowRun, error)
	updateWorkflowRunStatusFn           func(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	listStepRunsByWorkflowRun           func(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
	listRunnableStepRunsByWorkflowRunFn func(ctx context.Context, workflowRunID string, limit int) ([]domain.WorkflowStepRun, error)
	listRunningStepRunsByWorkflowRunFn  func(ctx context.Context, workflowRunID string, limit int) ([]domain.WorkflowStepRun, error)
	listStepRunStatusesByWorkflowRunFn  func(ctx context.Context, workflowRunID string) (map[string]domain.StepRunStatus, error)
	countNonTerminalStepRunsFn          func(ctx context.Context, workflowRunID string) (int, error)
	listFailedStepRunRefsFn             func(ctx context.Context, workflowRunID string) ([]string, error)
	getWorkflowStepCompletionSummaryFn  func(ctx context.Context, workflowRunID string) (store.WorkflowStepCompletionSummary, error)
	cancelNonTerminalStepRunsFn         func(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
	skipStepRunsByRefsFn                func(ctx context.Context, workflowRunID string, refs []string, finishedAt time.Time) (int64, error)
	getStepOutputsFn                    func(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error)
	listStepsByWorkflowVerFn            func(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	getWorkflowFn                       func(ctx context.Context, id string) (*domain.Workflow, error)
	getStepRunByRunAndRefFn             func(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	createWorkflowStepApprovalFn        func(ctx context.Context, approval *domain.WorkflowStepApproval) error
	getWorkflowStepApprovalFn           func(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	updateWorkflowStepApprovalFn        func(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	createAuditEventFn                  func(ctx context.Context, ev *domain.AuditEvent) error
	updateRunStatusFn                   func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	listDependentsByDependencyJobFn     func(ctx context.Context, dependsOnJobID string) ([]domain.JobDependency, error)
	listWaitingRunsByJobIDsFn           func(ctx context.Context, jobIDs []string, limit int) ([]domain.JobRun, error)
	areJobDependenciesSatisfiedFn       func(ctx context.Context, run *domain.JobRun) (bool, error)
	getWorkflowRunsByParentFn           func(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error)
	getEventTriggerByStepRunIDFn        func(ctx context.Context, stepRunID string) (*domain.EventTrigger, error)
	getEventTriggerByEventKeyFn         func(ctx context.Context, eventKey string) (*domain.EventTrigger, error)
	getEventTriggerByEventKeyProjectFn  func(ctx context.Context, eventKey, projectID string) (*domain.EventTrigger, error)
	updateEventTriggerStatusFn          func(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error
	advisoryXactLockFn                  func(ctx context.Context, lockID int64) error
	createWorkflowStepDecisionFn        func(ctx context.Context, d *domain.WorkflowStepDecision) error
	markCompensationRunTerminalFn       func(ctx context.Context, jobRunID string, status string, output json.RawMessage, errMsg string, finishedAt time.Time) (*domain.CompensationRun, error)
	countIncompleteCompensationRunsFn   func(ctx context.Context, workflowRunID string) (int, error)
	requeuePausedJobRunsFn              func(ctx context.Context, workflowRunID string) (int64, error)
	createWorkflowProgressionEventFn    func(ctx context.Context, workflowRunID, stepRunID, stepRef, status string) error
}

func (m *mockCallbackStore) GetEventTriggerByStepRunID(ctx context.Context, stepRunID string) (*domain.EventTrigger, error) {
	if m.getEventTriggerByStepRunIDFn != nil {
		return m.getEventTriggerByStepRunIDFn(ctx, stepRunID)
	}
	return nil, nil
}

func (m *mockCallbackStore) CreateWorkflowProgressionEvent(ctx context.Context, workflowRunID, stepRunID, stepRef, status string) error {
	if m.createWorkflowProgressionEventFn != nil {
		return m.createWorkflowProgressionEventFn(ctx, workflowRunID, stepRunID, stepRef, status)
	}
	return nil
}

func (m *mockCallbackStore) GetEventTriggerByEventKey(ctx context.Context, eventKey string) (*domain.EventTrigger, error) {
	if m.getEventTriggerByEventKeyFn != nil {
		return m.getEventTriggerByEventKeyFn(ctx, eventKey)
	}
	return nil, nil
}

func (m *mockCallbackStore) GetEventTriggerByEventKeyForProject(ctx context.Context, eventKey, projectID string) (*domain.EventTrigger, error) {
	if m.getEventTriggerByEventKeyProjectFn != nil {
		return m.getEventTriggerByEventKeyProjectFn(ctx, eventKey, projectID)
	}
	if m.getEventTriggerByEventKeyFn != nil {
		return m.getEventTriggerByEventKeyFn(ctx, eventKey)
	}
	return nil, nil
}

func (m *mockCallbackStore) UpdateEventTriggerStatus(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error {
	if m.updateEventTriggerStatusFn != nil {
		return m.updateEventTriggerStatusFn(ctx, id, status, responsePayload, receivedAt, errMsg)
	}
	return nil
}

func (m *mockCallbackStore) AdvisoryXactLock(ctx context.Context, lockID int64) error {
	if m.advisoryXactLockFn != nil {
		return m.advisoryXactLockFn(ctx, lockID)
	}
	return nil
}

func (m *mockCallbackStore) CreateWorkflowStepDecision(ctx context.Context, d *domain.WorkflowStepDecision) error {
	if m.createWorkflowStepDecisionFn != nil {
		return m.createWorkflowStepDecisionFn(ctx, d)
	}
	return nil
}

func (m *mockCallbackStore) MarkCompensationRunTerminalByJobRunID(ctx context.Context, jobRunID string, status string, output json.RawMessage, errMsg string, finishedAt time.Time) (*domain.CompensationRun, error) {
	if m.markCompensationRunTerminalFn != nil {
		return m.markCompensationRunTerminalFn(ctx, jobRunID, status, output, errMsg, finishedAt)
	}
	return nil, nil
}

func (m *mockCallbackStore) CountIncompleteCompensationRuns(ctx context.Context, workflowRunID string) (int, error) {
	if m.countIncompleteCompensationRunsFn != nil {
		return m.countIncompleteCompensationRunsFn(ctx, workflowRunID)
	}
	return 0, nil
}

func (m *mockCallbackStore) GetStepRunByJobRunID(ctx context.Context, jobRunID string) (*domain.WorkflowStepRun, error) {
	if m.getStepRunByJobRunIDFn != nil {
		return m.getStepRunByJobRunIDFn(ctx, jobRunID)
	}
	return nil, nil
}

func (m *mockCallbackStore) GetWorkflowStepRun(ctx context.Context, id string) (*domain.WorkflowStepRun, error) {
	if m.getWorkflowStepRunFn != nil {
		return m.getWorkflowStepRunFn(ctx, id)
	}
	if m.listStepRunsByWorkflowRun != nil {
		runs, err := m.listStepRunsByWorkflowRun(ctx, "", 10000, nil)
		if err != nil {
			return nil, err
		}
		for i := range runs {
			if runs[i].ID == id {
				return &runs[i], nil
			}
		}
	}
	return nil, nil
}

func (m *mockCallbackStore) ListWorkflowStepRunsByIDs(ctx context.Context, ids []string) ([]domain.WorkflowStepRun, error) {
	if m.listWorkflowStepRunsByIDsFn != nil {
		return m.listWorkflowStepRunsByIDsFn(ctx, ids)
	}
	out := make([]domain.WorkflowStepRun, 0, len(ids))
	for _, id := range ids {
		sr, err := m.GetWorkflowStepRun(ctx, id)
		if err != nil {
			return nil, err
		}
		if sr != nil {
			out = append(out, *sr)
		}
	}
	return out, nil
}

func (m *mockCallbackStore) UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
	if m.updateStepRunStatusFn != nil {
		return m.updateStepRunStatusFn(ctx, id, status, fields)
	}
	return nil
}

func (m *mockCallbackStore) UpdateStepRunStatusFrom(ctx context.Context, id string, _ domain.StepRunStatus, to domain.StepRunStatus, fields map[string]any) error {
	if m.updateStepRunStatusFn != nil {
		return m.updateStepRunStatusFn(ctx, id, to, fields)
	}
	return nil
}

func (m *mockCallbackStore) IncrementStepDeps(ctx context.Context, workflowRunID string, completedStepRef string) ([]store.StepDepResult, error) {
	if m.incrementStepDepsFn != nil {
		return m.incrementStepDepsFn(ctx, workflowRunID, completedStepRef)
	}
	return nil, nil
}

func (m *mockCallbackStore) IncrementStepDepsBatch(ctx context.Context, workflowRunID string, completedStepRefs []string) ([]store.StepDepResult, error) {
	if m.incrementStepDepsBatchFn != nil {
		return m.incrementStepDepsBatchFn(ctx, workflowRunID, completedStepRefs)
	}
	var out []store.StepDepResult
	for _, ref := range completedStepRefs {
		results, err := m.IncrementStepDeps(ctx, workflowRunID, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, results...)
	}
	return out, nil
}

func (m *mockCallbackStore) GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	if m.getWorkflowRunFn != nil {
		return m.getWorkflowRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockCallbackStore) UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
	if m.updateWorkflowRunStatusFn != nil {
		return m.updateWorkflowRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockCallbackStore) ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error) {
	if m.listStepRunsByWorkflowRun != nil {
		return m.listStepRunsByWorkflowRun(ctx, workflowRunID, limit, cursor)
	}
	return nil, nil
}

func (m *mockCallbackStore) ListRunnableStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int) ([]domain.WorkflowStepRun, error) {
	if m.listRunnableStepRunsByWorkflowRunFn != nil {
		return m.listRunnableStepRunsByWorkflowRunFn(ctx, workflowRunID, limit)
	}
	if m.listStepRunsByWorkflowRun == nil {
		return nil, nil
	}
	runs, err := m.listStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
	if err != nil {
		return nil, err
	}
	runnable := make([]domain.WorkflowStepRun, 0, len(runs))
	for _, sr := range runs {
		if sr.Status == domain.StepRunning || sr.Status.IsTerminal() {
			continue
		}
		if sr.DepsCompleted == sr.DepsRequired {
			runnable = append(runnable, sr)
		}
	}
	return runnable, nil
}

func (m *mockCallbackStore) ListRunningStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int) ([]domain.WorkflowStepRun, error) {
	if m.listRunningStepRunsByWorkflowRunFn != nil {
		return m.listRunningStepRunsByWorkflowRunFn(ctx, workflowRunID, limit)
	}
	if m.listStepRunsByWorkflowRun == nil {
		return nil, nil
	}
	runs, err := m.listStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
	if err != nil {
		return nil, err
	}
	running := make([]domain.WorkflowStepRun, 0, len(runs))
	for _, sr := range runs {
		if sr.Status == domain.StepRunning {
			running = append(running, sr)
		}
	}
	return running, nil
}

func (m *mockCallbackStore) ListStepRunStatusesByWorkflowRun(ctx context.Context, workflowRunID string) (map[string]domain.StepRunStatus, error) {
	if m.listStepRunStatusesByWorkflowRunFn != nil {
		return m.listStepRunStatusesByWorkflowRunFn(ctx, workflowRunID)
	}
	if m.listStepRunsByWorkflowRun == nil {
		return map[string]domain.StepRunStatus{}, nil
	}
	runs, err := m.listStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
	if err != nil {
		return nil, err
	}
	statuses := make(map[string]domain.StepRunStatus, len(runs))
	for _, sr := range runs {
		statuses[sr.StepRef] = sr.Status
	}
	return statuses, nil
}

func (m *mockCallbackStore) CountNonTerminalStepRuns(ctx context.Context, workflowRunID string) (int, error) {
	if m.countNonTerminalStepRunsFn != nil {
		return m.countNonTerminalStepRunsFn(ctx, workflowRunID)
	}
	if m.listStepRunsByWorkflowRun != nil {
		runs, err := m.listStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
		if err != nil {
			return 0, err
		}
		count := 0
		for _, sr := range runs {
			if !sr.Status.IsTerminal() {
				count++
			}
		}
		return count, nil
	}
	return 0, nil
}

func (m *mockCallbackStore) ListFailedStepRunRefs(ctx context.Context, workflowRunID string) ([]string, error) {
	if m.listFailedStepRunRefsFn != nil {
		return m.listFailedStepRunRefsFn(ctx, workflowRunID)
	}
	if m.listStepRunsByWorkflowRun != nil {
		runs, err := m.listStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
		if err != nil {
			return nil, err
		}
		refs := make([]string, 0, len(runs))
		for _, sr := range runs {
			if sr.Status == domain.StepFailed {
				refs = append(refs, sr.StepRef)
			}
		}
		return refs, nil
	}
	return nil, nil
}

func (m *mockCallbackStore) GetWorkflowStepCompletionSummary(ctx context.Context, workflowRunID string) (store.WorkflowStepCompletionSummary, error) {
	if m.getWorkflowStepCompletionSummaryFn != nil {
		return m.getWorkflowStepCompletionSummaryFn(ctx, workflowRunID)
	}
	count, err := m.CountNonTerminalStepRuns(ctx, workflowRunID)
	if err != nil {
		return store.WorkflowStepCompletionSummary{}, err
	}
	refs, err := m.ListFailedStepRunRefs(ctx, workflowRunID)
	if err != nil {
		return store.WorkflowStepCompletionSummary{}, err
	}
	return store.WorkflowStepCompletionSummary{NonTerminalCount: count, FailedStepRefs: refs}, nil
}

func (m *mockCallbackStore) CancelNonTerminalStepRuns(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
	if m.cancelNonTerminalStepRunsFn != nil {
		return m.cancelNonTerminalStepRunsFn(ctx, workflowRunID, finishedAt, reason)
	}
	if m.listStepRunsByWorkflowRun == nil {
		return 0, nil
	}
	runs, err := m.listStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
	if err != nil {
		return 0, err
	}
	if m.updateStepRunStatusFn == nil {
		return 0, nil
	}
	var count int64
	for _, sr := range runs {
		if sr.Status.IsTerminal() {
			continue
		}
		fields := map[string]any{"finished_at": finishedAt}
		if reason != "" {
			fields["error"] = reason
		}
		if err := m.updateStepRunStatusFn(ctx, sr.ID, domain.StepCanceled, fields); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (m *mockCallbackStore) SkipStepRunsByRefs(ctx context.Context, workflowRunID string, refs []string, finishedAt time.Time) (int64, error) {
	if m.skipStepRunsByRefsFn != nil {
		return m.skipStepRunsByRefsFn(ctx, workflowRunID, refs, finishedAt)
	}
	if m.listStepRunsByWorkflowRun == nil {
		return 0, nil
	}
	want := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		want[ref] = struct{}{}
	}
	runs, err := m.listStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
	if err != nil {
		return 0, err
	}
	if m.updateStepRunStatusFn == nil {
		return 0, nil
	}
	var count int64
	for _, sr := range runs {
		if _, ok := want[sr.StepRef]; !ok || sr.Status.IsTerminal() {
			continue
		}
		if err := m.updateStepRunStatusFn(ctx, sr.ID, domain.StepSkipped, map[string]any{"finished_at": finishedAt}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (m *mockCallbackStore) GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error) {
	if m.getStepOutputsFn != nil {
		return m.getStepOutputsFn(ctx, workflowRunID, stepRefs)
	}
	return nil, nil
}

func (m *mockCallbackStore) ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
	if m.listStepsByWorkflowVerFn != nil {
		return m.listStepsByWorkflowVerFn(ctx, workflowID, version)
	}
	return nil, nil
}

func (m *mockCallbackStore) GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error) {
	if m.getWorkflowFn != nil {
		return m.getWorkflowFn(ctx, id)
	}
	return nil, nil
}

func (m *mockCallbackStore) GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
	if m.getStepRunByRunAndRefFn != nil {
		return m.getStepRunByRunAndRefFn(ctx, workflowRunID, stepRef)
	}
	return nil, nil
}

func (m *mockCallbackStore) CreateWorkflowStepApproval(ctx context.Context, approval *domain.WorkflowStepApproval) error {
	if m.createWorkflowStepApprovalFn != nil {
		return m.createWorkflowStepApprovalFn(ctx, approval)
	}
	return nil
}

func (m *mockCallbackStore) GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error) {
	if m.getWorkflowStepApprovalFn != nil {
		return m.getWorkflowStepApprovalFn(ctx, stepRunID)
	}
	return nil, nil
}

func (m *mockCallbackStore) UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error {
	if m.updateWorkflowStepApprovalFn != nil {
		return m.updateWorkflowStepApprovalFn(ctx, id, status, approvedBy, approvedAt, errMsg)
	}
	return nil
}

func (m *mockCallbackStore) CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error {
	if m.createAuditEventFn != nil {
		return m.createAuditEventFn(ctx, ev)
	}
	return nil
}

func (m *mockCallbackStore) IncrementStepRunAttempt(ctx context.Context, id string, newAttempt int) error {
	if m.incrementStepRunAttemptFn != nil {
		return m.incrementStepRunAttemptFn(ctx, id, newAttempt)
	}
	return nil
}

func (m *mockCallbackStore) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	if m.updateRunStatusFn != nil {
		return m.updateRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockCallbackStore) ListDependentsByDependencyJob(ctx context.Context, dependsOnJobID string) ([]domain.JobDependency, error) {
	if m.listDependentsByDependencyJobFn != nil {
		return m.listDependentsByDependencyJobFn(ctx, dependsOnJobID)
	}
	return nil, nil
}

func (m *mockCallbackStore) ListWaitingRunsByJobIDs(ctx context.Context, jobIDs []string, limit int) ([]domain.JobRun, error) {
	if m.listWaitingRunsByJobIDsFn != nil {
		return m.listWaitingRunsByJobIDsFn(ctx, jobIDs, limit)
	}
	return nil, nil
}

func (m *mockCallbackStore) AreJobDependenciesSatisfied(ctx context.Context, run *domain.JobRun) (bool, error) {
	if m.areJobDependenciesSatisfiedFn != nil {
		return m.areJobDependenciesSatisfiedFn(ctx, run)
	}
	return true, nil
}

func (m *mockCallbackStore) GetWorkflowSnapshot(_ context.Context, _, _ string) (*domain.WorkflowSnapshot, error) {
	return nil, nil // Fallback to live table by default in tests.
}

func (m *mockCallbackStore) RequeuePausedJobRuns(ctx context.Context, workflowRunID string) (int64, error) {
	if m.requeuePausedJobRunsFn != nil {
		return m.requeuePausedJobRunsFn(ctx, workflowRunID)
	}
	return 0, nil
}

func (m *mockCallbackStore) ScheduleRetry(_ context.Context, _ string, _ time.Time, _ int) error {
	return nil
}

func (m *mockCallbackStore) GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error) {
	if m.getWorkflowRunsByParentFn != nil {
		return m.getWorkflowRunsByParentFn(ctx, parentWorkflowRunID)
	}
	return nil, nil
}

func TestStepCallback_OnJobRunTerminal(t *testing.T) {
	t.Parallel()
	t.Run("nil run no-op", func(t *testing.T) {
		t.Parallel()
		cb := NewStepCallback(&mockCallbackStore{}, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.OnJobRunTerminal(context.
				Background(),
				nil))
	})

	t.Run("missing workflow step run id no-op", func(t *testing.T) {
		t.Parallel()
		cb := NewStepCallback(&mockCallbackStore{}, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted})
		require.NoError(
			t, err)
	})

	t.Run("already terminal step no-op", func(t *testing.T) {
		t.Parallel()
		getCalled := 0
		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				getCalled++
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepCompleted}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted})
		require.NoError(
			t, err)
		require.Equal(t, 1, getCalled)
	})

	t.Run("completed run updates step and workflow", func(t *testing.T) {
		t.Parallel()
		progressionCreated := false
		stepUpdated := false
		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
				if id == "sr-1" {
					require.Equal(t,
						domain.StepCompleted,
						status,
					)

					if _, ok := fields["output"]; !ok {
						require.Failf(t, "test failure",

							"expected output field: %+v", fields)
					}
					stepUpdated = true
				}
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", StepRef: "s1", Status: domain.StepCompleted}}, nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "s1", StepRef: "s1"}}, nil
			},
			createWorkflowProgressionEventFn: func(_ context.Context, workflowRunID, stepRunID, stepRef, status string) error {
				require.False(t,
					workflowRunID !=
						"wr-1" ||
						stepRunID !=
							"sr-1" ||
						stepRef !=
							"s1" ||
						status !=
							string(domain.StepCompleted))

				progressionCreated = true
				return nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted, Result: json.RawMessage(`{"ok":true}`)})
		require.NoError(
			t, err)
		require.False(t,
			!stepUpdated ||
				!progressionCreated,
		)
	})

	t.Run("failed run applies fail_workflow policy", func(t *testing.T) {
		t.Parallel()
		workflowFailed := false
		canceledDependents := 0
		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-fail", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				require.False(t,
					id == "sr-fail" &&
						status !=
							domain.StepFailed,
				)

				if id == "sr-other" && status == domain.StepCanceled {
					canceledDependents++
				}
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{StepRef: "s1", OnFailure: domain.FailWorkflow},
					{StepRef: "s2"},
				}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				require.False(t,
					from != domain.
						WfStatusRunning ||
						to !=
							domain.
								WfStatusFailed,
				)

				workflowFailed = true
				return nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-fail", StepRef: "s1", Status: domain.StepFailed},
					{ID: "sr-other", StepRef: "s2", Status: domain.StepWaiting},
				}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-fail", Status: domain.StatusFailed, Error: "boom"})
		require.NoError(
			t, err)
		require.True(t,
			workflowFailed,
		)
		require.Equal(t, 1, canceledDependents)
	})

	t.Run("canceled run maps to step canceled", func(t *testing.T) {
		t.Parallel()
		statusSeen := domain.StepPending
		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, _ map[string]any) error {
				statusSeen = status
				return nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", StepRef: "s1", Status: domain.StepCanceled}}, nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCanceled})
		require.NoError(
			t, err)
		require.Equal(t,
			domain.StepCanceled,
			statusSeen,
		)
	})
}

func TestStepCallback_OnJobRunTerminal_PausedWorkflowDoesNotScheduleChildren(t *testing.T) {
	t.Parallel()
	enqueueCalled := false
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-parent", WorkflowRunID: "wr-1", StepRef: "parent", Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			require.False(t,
				id == "sr-parent" &&
					status !=
						domain.
							StepCompleted,
			)

			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return []store.StepDepResult{{StepRunID: "sr-child", StepRef: "child", DepsCompleted: 1, DepsRequired: 1}}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusPaused}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "step-parent", StepRef: "parent"}, {ID: "step-child", StepRef: "child", JobID: "job-1", DependsOn: []string{"parent"}}}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-parent", StepRef: "parent", Status: domain.StepCompleted},
				{ID: "sr-child", StepRef: "child", Status: domain.StepWaiting},
			}, nil
		},
	}
	mq := &mockEngineQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueueCalled = true
		return nil
	}}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, mq, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-parent", Status: domain.StatusCompleted})
	require.NoError(
		t, err)
	require.False(t,
		enqueueCalled,
	)
}

func TestMapRunStatusToStepStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		runStatus domain.RunStatus
		want      domain.StepRunStatus
	}{
		{name: "completed", runStatus: domain.StatusCompleted, want: domain.StepCompleted},
		{name: "canceled", runStatus: domain.StatusCanceled, want: domain.StepCanceled},
		{name: "failed", runStatus: domain.StatusFailed, want: domain.StepFailed},
		{name: "timed_out", runStatus: domain.StatusTimedOut, want: domain.StepFailed},
		{name: "crashed", runStatus: domain.StatusCrashed, want: domain.StepFailed},
		{name: "system_failed", runStatus: domain.StatusSystemFailed, want: domain.StepFailed},
		{name: "expired", runStatus: domain.StatusExpired, want: domain.StepFailed},
		{name: "unexpected queued", runStatus: domain.StatusQueued, want: domain.StepFailed},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, _ := mapRunStatusToStepStatus(&domain.JobRun{Status: tt.runStatus, Error: "err", Result: json.RawMessage(`{"ok":true}`)})
			require.Equal(t,
				tt.want, status,
			)
		})
	}
}

func TestMapRunStatusToStepStatus_Exhaustive(t *testing.T) {
	t.Parallel()
	t.Run("StatusCompleted includes output when result present", func(t *testing.T) {
		t.Parallel()
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{
			Status: domain.StatusCompleted,
			Result: json.RawMessage(`{"ok":true}`),
		})
		require.Equal(t,
			domain.StepCompleted,
			status,
		)

		output, ok := fields["output"]
		require.True(t,
			ok)

		raw, ok := output.(json.RawMessage)
		require.False(t,
			!ok || string(raw) != `{"ok":true}`,
		)
	})

	t.Run("StatusCompleted with empty result has no output", func(t *testing.T) {
		t.Parallel()
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusCompleted})
		require.Equal(t,
			domain.StepCompleted,
			status,
		)

		if _, ok := fields["output"]; ok {
			require.Failf(t, "test failure",

				"did not expect output field, got %+v", fields)
		}
	})

	t.Run("StatusCanceled maps to StepCanceled", func(t *testing.T) {
		t.Parallel()
		status, _ := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusCanceled})
		require.Equal(t,
			domain.StepCanceled,
			status,
		)
	})

	t.Run("StatusFailed maps to StepFailed and sets error", func(t *testing.T) {
		t.Parallel()
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusFailed})
		require.Equal(t,
			domain.StepFailed,
			status)

		errVal, ok := fields["error"].(string)
		require.False(t,
			!ok || !strings.Contains(errVal,
				"job run ended with status",
			))
	})

	t.Run("StatusTimedOut maps to StepFailed and sets error", func(t *testing.T) {
		t.Parallel()
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusTimedOut})
		require.Equal(t,
			domain.StepFailed,
			status)

		if _, ok := fields["error"]; !ok {
			require.Failf(t, "test failure",

				"expected error field, got %+v", fields)
		}
	})

	t.Run("StatusCrashed maps to StepFailed and sets error", func(t *testing.T) {
		t.Parallel()
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusCrashed})
		require.Equal(t,
			domain.StepFailed,
			status)

		if _, ok := fields["error"]; !ok {
			require.Failf(t, "test failure",

				"expected error field, got %+v", fields)
		}
	})

	t.Run("StatusSystemFailed maps to StepFailed and sets error", func(t *testing.T) {
		t.Parallel()
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusSystemFailed})
		require.Equal(t,
			domain.StepFailed,
			status)

		if _, ok := fields["error"]; !ok {
			require.Failf(t, "test failure",

				"expected error field, got %+v", fields)
		}
	})

	t.Run("StatusExpired maps to StepFailed and sets error", func(t *testing.T) {
		t.Parallel()
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusExpired})
		require.Equal(t,
			domain.StepFailed,
			status)

		if _, ok := fields["error"]; !ok {
			require.Failf(t, "test failure",

				"expected error field, got %+v", fields)
		}
	})

	t.Run("StatusFailed with explicit Error uses that string", func(t *testing.T) {
		t.Parallel()
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusFailed, Error: "boom"})
		require.Equal(t,
			domain.StepFailed,
			status)
		require.Equal(t,
			"boom", fields["error"])
	})

	t.Run("StatusFailed with empty Error uses fallback message", func(t *testing.T) {
		t.Parallel()
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusFailed})
		require.Equal(t,
			domain.StepFailed,
			status)

		errVal, ok := fields["error"].(string)
		require.False(t,
			!ok || !strings.Contains(errVal,
				"job run ended with status",
			))
	})

	t.Run("unknown status defaults to StepFailed", func(t *testing.T) {
		t.Parallel()
		status, _ := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.RunStatus("bogus")})
		require.Equal(t,
			domain.StepFailed,
			status)
	})
}

func TestCancelRemainingSteps_Engine(t *testing.T) {
	t.Parallel()
	t.Run("cancels non-terminal steps", func(t *testing.T) {
		t.Parallel()
		updated := make(map[string]domain.StepRunStatus)
		ms := &mockCallbackStore{
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-completed", Status: domain.StepCompleted},
					{ID: "sr-running", Status: domain.StepRunning},
					{ID: "sr-pending", Status: domain.StepPending},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				updated[id] = status
				return nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.cancelRemainingSteps(context.Background(), "wr-1")
		require.NoError(
			t, err)

		testutil.AssertEqual(t, updated, map[string]domain.StepRunStatus{
			"sr-running": domain.StepCanceled,
			"sr-pending": domain.StepCanceled,
		})
		if _, ok := updated["sr-completed"]; ok {
			require.Fail(t,

				"completed step should not be canceled")
		}
	})

	t.Run("skips all terminal", func(t *testing.T) {
		t.Parallel()
		updateCalls := 0
		ms := &mockCallbackStore{
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-completed", Status: domain.StepCompleted},
					{ID: "sr-failed", Status: domain.StepFailed},
					{ID: "sr-skipped", Status: domain.StepSkipped},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				updateCalls++
				return nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.cancelRemainingSteps(context.Background(), "wr-1")
		require.NoError(
			t, err)
		require.Equal(t, 0, updateCalls)
	})

	t.Run("store list error", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, errors.New("list failed")
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.cancelRemainingSteps(context.Background(), "wr-1")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "cancel non-terminal step runs",
		)
	})

	t.Run("store update error", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-running", Status: domain.StepRunning}}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return errors.New("update failed")
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.cancelRemainingSteps(context.Background(), "wr-1")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "cancel non-terminal step runs",
		)
	})
}

func TestStepCallback_OnJobRunTerminal_GetStepRunError(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return nil, errors.New("boom")
		},
	}
	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted})
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "get step run by job run id",
	)
}

func TestStepCallback_OnJobRunTerminal_UpdateStepRunStatusErrorWrapped(t *testing.T) {
	t.Parallel()
	baseErr := errors.New("write failed")
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "s1"}}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return baseErr
		},
	}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted})
	require.Error(t,
		err)
	assert.Contains(t, err.Error(), "update step run terminal status")
	assert.ErrorIs(t, err, baseErr)
}

func TestStepCallback_OnJobRunTerminal_CheckStepRetryErrorWrapped(t *testing.T) {
	t.Parallel()
	baseErr := errors.New("workflow lookup failed")
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Attempt: 1, Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return nil, baseErr
		},
	}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusFailed, Error: "boom"})
	require.Error(t,
		err)
	assert.Contains(t, err.Error(), "load workflow context")
	assert.ErrorIs(t, err, baseErr)
}

func TestStepCallback_OnJobRunTerminal_ProcessCompletedStepErrorWrapped(t *testing.T) {
	t.Parallel()
	baseErr := errors.New("deps update failed")
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "s1"}}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		createWorkflowProgressionEventFn: func(_ context.Context, _, _, _, _ string) error {
			return baseErr
		},
	}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted})
	require.Error(t,
		err)
	assert.Contains(t, err.Error(), "create workflow progression event")
	assert.ErrorIs(t, err, baseErr)
}

func TestStepCallback_OnJobRunTerminal_ProcessFailedStepErrorWrapped(t *testing.T) {
	t.Parallel()
	baseErr := errors.New("update workflow status failed")
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Attempt: 1, Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "s1", RetryMaxAttempts: 0, OnFailure: domain.FailWorkflow}}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return baseErr
		},
	}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusFailed, Error: "boom"})
	require.Error(t,
		err)
	assert.Contains(t, err.Error(), "process failed step s1")
	assert.ErrorIs(t, err, baseErr)
}

func TestStepCallback_OnJobRunTerminal_FanInStartsChildren(t *testing.T) {
	t.Parallel()
	progressionCreated := false
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-a", WorkflowRunID: "wr-1", StepRef: "a", Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			require.False(t,
				id == "sr-a" &&
					status != domain.
						StepCompleted,
			)

			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return []store.StepDepResult{{StepRunID: "sr-b", StepRef: "b", DepsCompleted: 1, DepsRequired: 1}}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning, ProjectID: "proj-1"}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "step-a", StepRef: "a", JobID: "job-a"}, {ID: "step-b", StepRef: "b", JobID: "job-b", DependsOn: []string{"a"}}}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted}, {ID: "sr-b", StepRef: "b", Status: domain.StepWaiting, WorkflowStepID: "step-b"}}, nil
		},
		getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
			return map[string]json.RawMessage{"a": json.RawMessage(`{"ok":true}`)}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		createWorkflowProgressionEventFn: func(_ context.Context, workflowRunID, stepRunID, stepRef, status string) error {
			require.False(t,
				workflowRunID !=
					"wr-1" ||
					stepRunID !=
						"sr-a" ||
					stepRef !=
						"a" ||
					status !=
						string(
							domain.StepCompleted,
						))

			progressionCreated = true
			return nil
		},
	}
	mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		run.ID = "job-run-b"
		if run.JobID != "job-b" {
			return fmt.Errorf("unexpected job id %s", run.JobID)
		}
		return nil
	}}
	engine := NewWorkflowEngine(&mockEngineStore{}, mq, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-a", WorkflowStepRunID: "sr-a", Status: domain.StatusCompleted})
	require.NoError(
		t, err)
	require.True(t,
		progressionCreated,
	)
}

func TestStepCallback_checkStepRetry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		stepRun           *domain.WorkflowStepRun
		getWorkflowRunFn  func(ctx context.Context, id string) (*domain.WorkflowRun, error)
		listStepsFn       func(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
		wantShouldRetry   bool
		wantNewAttempt    int
		wantErrContains   string
		assertNextRetryAt func(t *testing.T, got time.Time, before, after time.Time)
	}{
		{
			name: "no_retry_policy",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       1,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1", RetryMaxAttempts: 0}}, nil
			},
			wantShouldRetry: false,
			wantNewAttempt:  0,
		},
		{
			name: "first_attempt_with_retry_policy",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       1,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{
					StepRef:               "s1",
					RetryMaxAttempts:      3,
					RetryBackoff:          domain.RetryBackoffExponential,
					RetryInitialDelaySecs: 2,
					RetryMaxDelaySecs:     120,
				}}, nil
			},
			wantShouldRetry: true,
			wantNewAttempt:  2,
			assertNextRetryAt: func(t *testing.T, got time.Time, before, after time.Time) {
				t.Helper()
				require.False(t,
					got.IsZero())
				require.True(t,
					got.After(before))
				require.True(t,
					got.After(after))
			},
		},
		{
			name: "exhausted_attempts",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       2,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1", RetryMaxAttempts: 2}}, nil
			},
			wantShouldRetry: false,
			wantNewAttempt:  0,
		},
		{
			name: "step_not_found",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "missing",
				Attempt:       1,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1", RetryMaxAttempts: 3}}, nil
			},
			wantErrContains: "step definition not found",
		},
		{
			name: "exponential_backoff_delay",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       1,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{
					StepRef:               "s1",
					RetryMaxAttempts:      4,
					RetryBackoff:          domain.RetryBackoffExponential,
					RetryInitialDelaySecs: 10,
					RetryMaxDelaySecs:     120,
				}}, nil
			},
			wantShouldRetry: true,
			wantNewAttempt:  2,
			assertNextRetryAt: func(t *testing.T, got time.Time, before, _ time.Time) {
				t.Helper()
				delay := got.Sub(before)
				require.False(t,
					delay < 15*
						time.Second || delay >
						25*
							time.Second,
				)
			},
		},
		{
			name: "fixed_backoff_delay",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       4,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{
					StepRef:               "s1",
					RetryMaxAttempts:      10,
					RetryBackoff:          domain.RetryBackoffFixed,
					RetryInitialDelaySecs: 8,
					RetryMaxDelaySecs:     120,
				}}, nil
			},
			wantShouldRetry: true,
			wantNewAttempt:  5,
			assertNextRetryAt: func(t *testing.T, got time.Time, before, _ time.Time) {
				t.Helper()
				delay := got.Sub(before)
				require.False(t,
					delay < 6*
						time.Second || delay >
						10*time.
							Second,
				)
			},
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockCallbackStore{}
			cb := NewStepCallback(store, nil, slog.Default())

			run, _ := tt.getWorkflowRunFn(context.Background(), "")
			steps, _ := tt.listStepsFn(context.Background(), "", 0)
			wc := testWfCtx(run, steps)

			before := time.Now()
			shouldRetry, nextRetryAt, newAttempt, err := cb.checkStepRetry(context.Background(), tt.stepRun, &domain.JobRun{}, wc)
			after := time.Now()

			if tt.wantErrContains != "" {
				require.Error(t,
					err)
				assert.Contains(
					t, err.Error(), tt.wantErrContains,
				)

				return
			}
			require.NoError(
				t, err)
			require.Equal(t,
				tt.wantShouldRetry,
				shouldRetry,
			)
			require.Equal(t,
				tt.wantNewAttempt,
				newAttempt,
			)

			if tt.assertNextRetryAt != nil {
				tt.assertNextRetryAt(t, nextRetryAt, before, after)
			}
		})
	}
}

func TestStepCallback_scheduleStepRetry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                 string
		incrementErr         error
		updateRunStatusErr   error
		wantErrContains      string
		wantUpdateRunInvoked bool
	}{
		{
			name:                 "success",
			wantUpdateRunInvoked: true,
		},
		{
			name:                 "increment_attempt_error",
			incrementErr:         errors.New("boom"),
			wantErrContains:      "increment step run attempt",
			wantUpdateRunInvoked: false,
		},
		{
			name:                 "update_run_status_error",
			updateRunStatusErr:   errors.New("boom"),
			wantErrContains:      "update job run status for retry",
			wantUpdateRunInvoked: true,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			incrementCalled := 0
			updateRunCalled := 0

			store := &mockCallbackStore{
				incrementStepRunAttemptFn: func(_ context.Context, id string, newAttempt int) error {
					incrementCalled++
					require.False(t,
						id != "sr-1" ||
							newAttempt !=
								2)

					return tt.incrementErr
				},
				updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
					updateRunCalled++
					require.Equal(t,
						"run-1", id,
					)
					require.False(t,
						from != domain.
							StatusFailed ||
							to !=
								domain.
									StatusDelayed,
					)
					require.EqualValues(t, 2, fields["attempt"])

					if _, ok := fields["next_retry_at"]; ok {
						require.Failf(t, "test failure",

							"next_retry_at must not be in UpdateRunStatus fields (side-table only), got %+v", fields)
					}
					return tt.updateRunStatusErr
				},
			}

			cb := NewStepCallback(store, nil, slog.Default())
			err := cb.scheduleStepRetry(
				context.Background(),
				&domain.JobRun{ID: "run-1", Status: domain.StatusFailed},
				&domain.WorkflowStepRun{ID: "sr-1"},
				time.Now().Add(2*time.Second),
				2,
			)

			if tt.wantErrContains != "" {
				require.Error(t,
					err)
				assert.Contains(
					t, err.Error(), tt.wantErrContains,
				)
			} else if err != nil {
				require.Failf(t, "test failure",

					"scheduleStepRetry() error = %v", err)
			}
			require.Equal(t, 1, incrementCalled)
			require.False(t,
				tt.wantUpdateRunInvoked &&
					updateRunCalled !=
						1)
			require.False(t,
				!tt.wantUpdateRunInvoked &&
					updateRunCalled !=
						0)
		})
	}
}

func TestStepCallback_OnJobRunTerminal_RetryIntegration(t *testing.T) {
	t.Parallel()
	t.Run("failed_run_triggers_retry", func(t *testing.T) {
		t.Parallel()
		incrementCalled := 0
		updateRunCalled := 0

		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{
					ID:            "sr-1",
					WorkflowRunID: "wr-1",
					StepRef:       "s1",
					Attempt:       1,
					Status:        domain.StepRunning,
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				require.False(t,
					id == "sr-1" &&
						status != domain.
							StepFailed,
				)

				return nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{
					StepRef:               "s1",
					OnFailure:             domain.FailWorkflow,
					RetryMaxAttempts:      3,
					RetryBackoff:          domain.RetryBackoffFixed,
					RetryInitialDelaySecs: 1,
					RetryMaxDelaySecs:     5,
				}}, nil
			},
			incrementStepRunAttemptFn: func(_ context.Context, id string, newAttempt int) error {
				incrementCalled++
				require.False(t,
					id != "sr-1" ||
						newAttempt !=
							2)

				return nil
			},
			updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
				updateRunCalled++
				require.False(t,
					id != "run-1" ||
						from != domain.
							StatusFailed ||
						to != domain.
							StatusDelayed,
				)
				require.EqualValues(t, 2, fields["attempt"])

				if _, ok := fields["next_retry_at"]; ok {
					require.Failf(t, "test failure",

						"next_retry_at must not be in UpdateRunStatus fields (side-table only), got %+v", fields)
				}
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				require.Fail(t,

					"UpdateWorkflowRunStatus should not be called when retry is scheduled")
				return nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				require.Fail(t,

					"ListStepRunsByWorkflowRun should not be called when retry is scheduled")
				return nil, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusFailed, Error: "boom"})
		require.NoError(
			t, err)
		require.Equal(t, 1, incrementCalled)
		require.Equal(t, 1, updateRunCalled)
	})

	t.Run("failed_run_no_retry_falls_through", func(t *testing.T) {
		t.Parallel()
		workflowFailed := 0
		canceledDependents := 0

		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{
					ID:            "sr-fail",
					WorkflowRunID: "wr-1",
					StepRef:       "s1",
					Attempt:       1,
					Status:        domain.StepRunning,
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				require.False(t,
					id == "sr-fail" &&
						status !=
							domain.StepFailed,
				)

				if id == "sr-other" && status == domain.StepCanceled {
					canceledDependents++
				}
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{
					StepRef:          "s1",
					OnFailure:        domain.FailWorkflow,
					RetryMaxAttempts: 0,
				}}, nil
			},
			incrementStepRunAttemptFn: func(_ context.Context, _ string, _ int) error {
				require.Fail(t,

					"IncrementStepRunAttempt should not be called when retry is disabled")
				return nil
			},
			updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
				require.Fail(t,

					"UpdateRunStatus should not be called when retry is disabled")
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				require.False(t,
					from != domain.
						WfStatusRunning ||
						to !=
							domain.
								WfStatusFailed,
				)

				workflowFailed++
				return nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-fail", StepRef: "s1", Status: domain.StepFailed},
					{ID: "sr-other", StepRef: "s2", Status: domain.StepWaiting},
				}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-fail", Status: domain.StatusFailed, Error: "boom"})
		require.NoError(
			t, err)
		require.Equal(t, 1, workflowFailed)
		require.Equal(t, 1, canceledDependents)
	})
}

func TestStepCallback_skipDependentSteps(t *testing.T) {
	t.Parallel()
	t.Run("chain_A_B_C", func(t *testing.T) {
		t.Parallel()
		skipCalls := make(map[string]domain.StepRunStatus)
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{StepRef: "a"},
					{StepRef: "b", DependsOn: []string{"a"}},
					{StepRef: "c", DependsOn: []string{"b"}},
				}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
					{ID: "sr-b", StepRef: "b", Status: domain.StepPending},
					{ID: "sr-c", StepRef: "c", Status: domain.StepPending},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				skipCalls[id] = status
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		wc := buildWfCtx(
			&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1},
			[]domain.WorkflowStep{
				{StepRef: "a"},
				{StepRef: "b", DependsOn: []string{"a"}},
				{StepRef: "c", DependsOn: []string{"b"}},
			},
		)
		require.NoError(
			t, cb.skipDependentSteps(context.
				Background(),
				"wr-1", wc,
				"a"))
		require.Len(t, skipCalls,
			2,
		)
		require.Equal(t,
			domain.StepSkipped,
			skipCalls["sr-b"],
		)
		require.Equal(t,
			domain.StepSkipped,
			skipCalls["sr-c"],
		)
	})

	t.Run("diamond_A_BC_D", func(t *testing.T) {
		t.Parallel()
		skipCalls := make(map[string]domain.StepRunStatus)
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{StepRef: "a"},
					{StepRef: "b", DependsOn: []string{"a"}},
					{StepRef: "c", DependsOn: []string{"a"}},
					{StepRef: "d", DependsOn: []string{"b", "c"}},
				}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
					{ID: "sr-b", StepRef: "b", Status: domain.StepPending},
					{ID: "sr-c", StepRef: "c", Status: domain.StepPending},
					{ID: "sr-d", StepRef: "d", Status: domain.StepPending},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				skipCalls[id] = status
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		wc := buildWfCtx(
			&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1},
			[]domain.WorkflowStep{
				{StepRef: "a"},
				{StepRef: "b", DependsOn: []string{"a"}},
				{StepRef: "c", DependsOn: []string{"a"}},
				{StepRef: "d", DependsOn: []string{"b", "c"}},
			},
		)
		require.NoError(
			t, cb.skipDependentSteps(context.
				Background(),
				"wr-1", wc,
				"a"))
		require.Len(t, skipCalls,
			3,
		)

		for _, id := range []string{"sr-b", "sr-c", "sr-d"} {
			require.Equal(t,
				domain.StepSkipped,
				skipCalls[id])
		}
	})

	t.Run("leaf_node_no_dependents", func(t *testing.T) {
		t.Parallel()
		updateCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{StepRef: "a"},
					{StepRef: "leaf", DependsOn: []string{"a"}},
				}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
					{ID: "sr-leaf", StepRef: "leaf", Status: domain.StepPending},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				updateCalled = true
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		// Fail "leaf" which has no dependents
		wc := buildWfCtx(
			&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1},
			[]domain.WorkflowStep{
				{StepRef: "a"},
				{StepRef: "leaf", DependsOn: []string{"a"}},
			},
		)
		require.NoError(
			t, cb.skipDependentSteps(context.
				Background(),
				"wr-1", wc,
				"leaf"))
		require.False(t,
			updateCalled,
		)
	})

	t.Run("already_terminal_not_skipped", func(t *testing.T) {
		t.Parallel()
		skipCalls := make(map[string]domain.StepRunStatus)
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{StepRef: "a"},
					{StepRef: "b", DependsOn: []string{"a"}},
					{StepRef: "c", DependsOn: []string{"a"}},
				}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
					{ID: "sr-b", StepRef: "b", Status: domain.StepCompleted},
					{ID: "sr-c", StepRef: "c", Status: domain.StepPending},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				skipCalls[id] = status
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		wc := buildWfCtx(
			&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1},
			[]domain.WorkflowStep{
				{StepRef: "a"},
				{StepRef: "b", DependsOn: []string{"a"}},
				{StepRef: "c", DependsOn: []string{"a"}},
			},
		)
		require.NoError(
			t, cb.skipDependentSteps(context.
				Background(),
				"wr-1", wc,
				"a"))
		require.Len(t, skipCalls,
			1,
		)

		if _, ok := skipCalls["sr-b"]; ok {
			require.Fail(t,

				"sr-b is already terminal and should not be skipped")
		}
		require.Equal(t,
			domain.StepSkipped,
			skipCalls["sr-c"],
		)
	})

	t.Run("skip_step_runs_by_refs_error", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			skipStepRunsByRefsFn: func(_ context.Context, _ string, _ []string, _ time.Time) (int64, error) {
				return 0, errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		wc := buildWfCtx(
			&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1},
			[]domain.WorkflowStep{{StepRef: "a"}, {StepRef: "b", DependsOn: []string{"a"}}},
		)
		err := cb.skipDependentSteps(context.Background(), "wr-1", wc, "a")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "skip step runs by refs",
		)
	})
}

func TestStepCallback_ApproveStep(t *testing.T) {
	t.Parallel()
	t.Run("empty_approver", func(t *testing.T) {
		t.Parallel()
		cb := NewStepCallback(&mockCallbackStore{}, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "approver is required",
		)
	})

	t.Run("get_step_run_error", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return nil, errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "get step run",
		)
	})

	t.Run("step_not_found", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "step run not found",
		)
	})

	t.Run("step_already_terminal", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepCompleted}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "already in terminal state",
		)
	})

	t.Run("approval_not_found", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return nil, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "approval not found",
		)
	})

	t.Run("approval_already_approved", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: "approved", Approvers: []string{"alice"}}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "already approved",
		)
	})

	t.Run("unauthorized_approver", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: "pending", Approvers: []string{"alice"}}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "bob")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "not allowed",
		)
	})

	t.Run("update_approval_error", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: "pending", Approvers: []string{"alice"}}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "update approval",
		)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		approvalUpdated := false
		stepCompleted := false
		var capturedAudit *domain.AuditEvent
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: "pending", Approvers: []string{"alice", "bob"}}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, id string, status string, approvedBy string, _ *time.Time, _ string) error {
				require.False(t,
					id != "apr-1" ||
						status !=
							"approved" ||
						approvedBy !=
							"alice",
				)

				approvalUpdated = true
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id == "sr-1" && status == domain.StepCompleted {
					stepCompleted = true
				}
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", StepRef: "s1", Status: domain.StepCompleted}}, nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			createAuditEventFn: func(_ context.Context, ev *domain.AuditEvent) error {
				capturedAudit = ev
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		require.NoError(
			t, err)
		require.True(t,
			approvalUpdated,
		)
		require.True(t,
			stepCompleted,
		)
		require.NotNil(t,
			capturedAudit,
		)
		require.Equal(t,
			"workflow.step.approved",
			capturedAudit.
				Action,
		)
		require.False(t,
			capturedAudit.
				ActorID != "alice" ||
				capturedAudit.
					ActorType !=
					"user",
		)
		require.False(t,
			capturedAudit.
				ResourceType !=
				"workflow_step_approval" ||
				capturedAudit.
					ResourceID !=
					"apr-1")

		var details map[string]any
		require.NoError(
			t, json.Unmarshal(capturedAudit.
				Details,
				&details,
			))
		require.Equal(t,
			"approved",
			details["decision"])
		require.Equal(t,
			"sr-1", details["step_run_id"])
	})

	t.Run("audit failure is non-fatal", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: "pending", Approvers: []string{"alice"}}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", StepRef: "s1", Status: domain.StepCompleted}}, nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			createAuditEventFn: func(_ context.Context, _ *domain.AuditEvent) error {
				return errors.New("audit down")
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, logger), logger)
		require.NoError(
			t, cb.ApproveStep(context.Background(),
				"wr-1",
				"s1", "alice",
			))
		require.Contains(t,
			logs.String(), "failed to create approval audit event")
	})

	t.Run("cost gate timeout approvals are audited as system decisions", func(t *testing.T) {
		t.Parallel()
		var capturedAudit *domain.AuditEvent
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: "pending"}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", StepRef: "s1", Status: domain.StepCompleted}}, nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			createAuditEventFn: func(_ context.Context, ev *domain.AuditEvent) error {
				capturedAudit = ev
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.ApproveStep(context.Background(),
				"wr-1",
				"s1", "system:cost-gate-timeout",
			))
		require.NotNil(t,
			capturedAudit,
		)
		require.False(t,
			capturedAudit.
				ActorID != "system:cost-gate-timeout" ||
				capturedAudit.
					ActorType !=
					"system",
		)
	})

	t.Run("success emits approval completed notification with approved decision", func(t *testing.T) {
		t.Parallel()
		var deliveries []*domain.NotificationDelivery
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: "pending", Approvers: []string{"alice"}}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", StepRef: "s1", Status: domain.StepCompleted}}, nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
		}
		engineStore := &mockEngineStore{
			listEnabledNotificationChannelsFn: func(_ string) ([]domain.NotificationChannel, error) {
				return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
			},
			createNotificationDeliveryFn: func(d *domain.NotificationDelivery) error {
				deliveries = append(deliveries, d)
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(engineStore, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.ApproveStep(context.Background(),
				"wr-1",
				"s1", "alice",
			))
		require.Len(t, deliveries,

			1)
		require.Equal(t,
			domain.NotificationEventApprovalCompleted,

			deliveries[0].
				EventType,
		)

		var payload map[string]any
		require.NoError(
			t, json.Unmarshal(deliveries[0].Payload,
				&payload,
			))
		require.Equal(t,
			"approved",
			payload["decision"])
		require.Equal(t,
			"alice", payload["approved_by"])

		if _, ok := payload["approved_at"]; !ok {
			require.Fail(t,

				"expected approved_at in payload")
		}
	})
}

func TestStepCallback_SkipStep(t *testing.T) {
	t.Parallel()
	t.Run("step in pending status succeeds", func(t *testing.T) {
		t.Parallel()
		updated := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}, {StepRef: "child", DependsOn: []string{"s1"}}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
				require.False(t,
					workflowRunID !=
						"wr-1" ||
						stepRef !=
							"s1")

				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: workflowRunID, StepRef: stepRef, Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
				require.False(t,
					id != "sr-1" ||
						status != domain.
							StepSkipped,
				)

				if _, ok := fields["finished_at"]; !ok {
					require.Failf(t, "test failure",

						"expected finished_at field, got %+v", fields)
				}
				require.Equal(t,
					"manual",
					fields["error"])

				updated = true
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-child", StepRef: "child", Status: domain.StepWaiting}}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.SkipStep(context.Background(), "wr-1",

				"s1", "manual",
				""))
		require.True(t,
			updated)
	})

	t.Run("step in waiting status succeeds", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}, {StepRef: "child", DependsOn: []string{"s1"}}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepWaiting}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, _ map[string]any) error {
				require.Equal(t,
					domain.StepSkipped,
					status)

				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-child", StepRef: "child", Status: domain.StepWaiting}}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.SkipStep(context.Background(), "wr-1",

				"s1", "", "",
			))
	})

	t.Run("step in running status returns error", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepRunning}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.SkipStep(context.Background(), "wr-1", "s1", "", "")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "cannot skip step in running status",
		)
	})

	t.Run("step in completed status returns error", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepCompleted}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.SkipStep(context.Background(), "wr-1", "s1", "", "")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "cannot skip step in completed status",
		)
	})

	t.Run("skip step with pending approval rejects the approval", func(t *testing.T) {
		t.Parallel()
		var approvalUpdateArgs struct {
			id, status, approvedBy, errMsg string
			approvedAt                     *time.Time
		}
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: domain.ApprovalStatusPending}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error {
				approvalUpdateArgs.id = id
				approvalUpdateArgs.status = status
				approvalUpdateArgs.approvedBy = approvedBy
				approvalUpdateArgs.approvedAt = approvedAt
				approvalUpdateArgs.errMsg = errMsg
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.SkipStep(context.Background(), "wr-1",

				"s1", "rejected by admin",

				"user_admin",
			))
		require.Equal(t,
			"apr-1", approvalUpdateArgs.
				id)
		require.Equal(t,
			domain.ApprovalStatusRejected,

			approvalUpdateArgs.
				status,
		)
		require.Equal(t,
			"user_admin",
			approvalUpdateArgs.
				approvedBy,
		)
		require.NotNil(t,
			approvalUpdateArgs.
				approvedAt,
		)
		require.Equal(t,
			"rejected by admin",
			approvalUpdateArgs.
				errMsg,
		)
	})

	t.Run("skip step with pending approval and empty reason", func(t *testing.T) {
		t.Parallel()
		updateCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: domain.ApprovalStatusPending}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, errMsg string) error {
				updateCalled = true
				require.Empty(t,
					errMsg,
				)

				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.SkipStep(context.Background(), "wr-1",

				"s1", "", "",
			))
		require.True(t,
			updateCalled,
		)
	})

	t.Run("skip step with pending approval — approval update fails returns error", func(t *testing.T) {
		t.Parallel()
		stepUpdated := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				stepUpdated = true
				return nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: domain.ApprovalStatusPending}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return errors.New("db down")
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.SkipStep(context.Background(), "wr-1", "s1", "reason", "")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "reject approval on skip",
		)
		require.False(t,
			stepUpdated,
		)
	})

	t.Run("skip step with pending approval — approval lookup failure aborts skip", func(t *testing.T) {
		t.Parallel()
		updateCalled := false
		stepUpdated := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				stepUpdated = true
				return nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return nil, errors.New("db error")
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				updateCalled = true
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.SkipStep(context.Background(), "wr-1", "s1", "reason", "")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "get workflow step approval",
		)
		require.False(t,
			updateCalled,
		)
		require.False(t,
			stepUpdated,
		)
	})

	t.Run("skip step without approval skips normally", func(t *testing.T) {
		t.Parallel()
		updateCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return nil, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				updateCalled = true
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.SkipStep(context.Background(), "wr-1",

				"s1", "manual",
				""))
		require.False(t,
			updateCalled,
		)
	})

	t.Run("skip step with already-approved approval does not re-reject", func(t *testing.T) {
		t.Parallel()
		updateCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: domain.ApprovalStatusApproved}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				updateCalled = true
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.SkipStep(context.Background(), "wr-1",

				"s1", "manual",
				""))
		require.False(t,
			updateCalled,
		)
	})

	t.Run("skip step with already-rejected approval does not double-reject", func(t *testing.T) {
		t.Parallel()
		updateCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: domain.ApprovalStatusRejected}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				updateCalled = true
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.SkipStep(context.Background(), "wr-1",

				"s1", "manual",
				""))
		require.False(t,
			updateCalled,
		)
	})

	t.Run("skip step with pending approval emits rejection notification", func(t *testing.T) {
		t.Parallel()
		var deliveries []*domain.NotificationDelivery
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: domain.ApprovalStatusPending}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}

		engineStore := &mockEngineStore{
			listEnabledNotificationChannelsFn: func(_ string) ([]domain.NotificationChannel, error) {
				return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
			},
			createNotificationDeliveryFn: func(d *domain.NotificationDelivery) error {
				deliveries = append(deliveries, d)
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(engineStore, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.SkipStep(context.Background(), "wr-1",

				"s1", "rejected by admin",

				"user_admin",
			))
		require.Len(t, deliveries,

			1)
		assert.Equal(t,
			domain.NotificationEventApprovalCompleted,

			deliveries[0].
				EventType)

		var payload map[string]any
		require.NoError(
			t, json.Unmarshal(deliveries[0].Payload,
				&payload,
			))
		require.Equal(t,
			"rejected",
			payload["decision"])
		assert.Equal(t,
			"user_admin",
			payload["rejected_by"])

		if _, ok := payload["rejected_at"]; !ok {
			require.Fail(t,

				"expected rejected_at in payload")
		}
		assert.Equal(t,
			"rejected by admin",
			payload["reason"],
		)
	})

	t.Run("skip step emits approval rejection audit event", func(t *testing.T) {
		t.Parallel()
		var capturedAudit *domain.AuditEvent
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: domain.ApprovalStatusPending}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return nil
			},
			createAuditEventFn: func(_ context.Context, ev *domain.AuditEvent) error {
				capturedAudit = ev
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.SkipStep(context.Background(), "wr-1",

				"s1", "rejected by admin",

				"user_admin",
			))
		require.NotNil(t,
			capturedAudit,
		)
		require.Equal(t,
			"workflow.step.rejected",
			capturedAudit.
				Action,
		)
		require.False(t,
			capturedAudit.
				ActorID != "user_admin" ||
				capturedAudit.
					ActorType !=
					"user")

		var details map[string]any
		require.NoError(
			t, json.Unmarshal(capturedAudit.
				Details,
				&details,
			))
		require.Equal(t,
			"rejected",
			details["decision"])
		require.Equal(t,
			"rejected by admin",
			details["reason"])
	})

	t.Run("skip step falls back to skip actor on reject persistence", func(t *testing.T) {
		t.Parallel()
		var approvedBy string
		var approvedAt *time.Time
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: domain.ApprovalStatusPending}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, incomingApprovedBy string, incomingApprovedAt *time.Time, _ string) error {
				approvedBy = incomingApprovedBy
				approvedAt = incomingApprovedAt
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.SkipStep(context.Background(), "wr-1",

				"s1", "", "",
			))
		require.Equal(t,
			"skip", approvedBy,
		)
		require.NotNil(t,
			approvedAt,
		)
	})

	t.Run("reject audit failure is non-fatal", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: domain.ApprovalStatusPending}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return nil
			},
			createAuditEventFn: func(_ context.Context, _ *domain.AuditEvent) error {
				return errors.New("audit down")
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, logger), logger)
		require.NoError(
			t, cb.SkipStep(context.Background(), "wr-1",

				"s1", "manual",
				"user_admin",
			))
		require.Contains(t,
			logs.String(), "failed to create approval audit event")
	})
}

func TestStepCallback_ForceCompleteStep(t *testing.T) {
	t.Parallel()
	t.Run("step in pending status succeeds", func(t *testing.T) {
		t.Parallel()
		updated := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}, {StepRef: "child", DependsOn: []string{"s1"}}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
				require.False(t,
					workflowRunID !=
						"wr-1" ||
						stepRef !=
							"s1")

				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: workflowRunID, StepRef: stepRef, Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
				require.False(t,
					id != "sr-1" ||
						status != domain.
							StepCompleted,
				)

				if _, ok := fields["finished_at"]; !ok {
					require.Failf(t, "test failure",

						"expected finished_at field, got %+v", fields)
				}
				require.Equal(t,
					`{"ok":true}`,
					string(fields["output"].(json.
						RawMessage),
					))

				updated = true
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-child", StepRef: "child", Status: domain.StepWaiting}}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.ForceCompleteStep(context.
				Background(),
				"wr-1", "s1",
				json.RawMessage(`{"ok":true}`)))
		require.True(t,
			updated)
	})

	t.Run("step in waiting status succeeds", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}, {StepRef: "child", DependsOn: []string{"s1"}}}, nil
			},
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepWaiting}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, _ map[string]any) error {
				require.Equal(t,
					domain.StepCompleted,
					status,
				)

				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-child", StepRef: "child", Status: domain.StepWaiting}}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		require.NoError(
			t, cb.ForceCompleteStep(context.
				Background(),
				"wr-1", "s1",
				nil))
	})

	t.Run("step in running status returns error", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepRunning}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ForceCompleteStep(context.Background(), "wr-1", "s1", nil)
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "cannot force-complete step in running status",
		)
	})

	t.Run("step in completed status returns error", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepCompleted}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ForceCompleteStep(context.Background(), "wr-1", "s1", nil)
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "cannot force-complete step in completed status",
		)
	})
}

func TestStepCallback_ResumeWorkflowRun(t *testing.T) {
	t.Parallel()
	t.Run("workflow_run_not_found", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "workflow run not found",
		)
	})

	t.Run("workflow_run_not_paused", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", Status: domain.WfStatusRunning}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "not paused",
		)
	})

	t.Run("get_workflow_run_error", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "get workflow run",
		)
	})

	t.Run("update_workflow_run_status_error", func(t *testing.T) {
		t.Parallel()
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "resume workflow run",
		)
	})

	t.Run("queue_requeue_preferred_when_available", func(t *testing.T) {
		t.Parallel()
		var queueRequeueCalled, storeRequeueCalled bool
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			requeuePausedJobRunsFn: func(_ context.Context, _ string) (int64, error) {
				storeRequeueCalled = true
				return 0, nil
			},
		}
		mq := &mockEngineQueue{
			requeuePausedRunsFn: func(_ context.Context, workflowRunID string) (int64, error) {
				require.Equal(t,
					"wr-1", workflowRunID,
				)

				queueRequeueCalled = true
				return 2, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, mq, slog.Default()), slog.Default())
		require.NoError(
			t, cb.ResumeWorkflowRun(context.
				Background(),
				"wr-1"))
		require.True(t,
			queueRequeueCalled,
		)
		require.False(t,
			storeRequeueCalled,
		)
	})

	t.Run("success_starts_ready_steps", func(t *testing.T) {
		t.Parallel()
		enqueueCalled := false
		engStepUpdated := false
		engStore := &mockEngineStore{
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id == "sr-root" && status == domain.StepRunning {
					engStepUpdated = true
				}
				return nil
			},
		}
		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				run.ID = "jr-1"
				require.Equal(t,
					"job-root",
					run.JobID)

				enqueueCalled = true
				return nil
			},
		}
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, ProjectID: "proj-1", Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-root", StepRef: "root", JobID: "job-root"}}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-root", StepRef: "root", WorkflowStepID: "step-root", Status: domain.StepPending, DepsCompleted: 0, DepsRequired: 0}}, nil
			},
		}
		engine := NewWorkflowEngine(engStore, mq, slog.Default())
		cb := NewStepCallback(ms, engine, slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		require.NoError(
			t, err)
		require.True(t,
			enqueueCalled,
		)
		require.True(t,
			engStepUpdated,
		)
	})

	t.Run("skips_terminal_steps", func(t *testing.T) {
		t.Parallel()
		enqueueCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-a", StepRef: "a", JobID: "job-a"}}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted, DepsCompleted: 0, DepsRequired: 0}}, nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCalled = true
			return nil
		}}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, mq, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		require.NoError(
			t, err)
		require.False(t,
			enqueueCalled,
		)
	})

	t.Run("skips_deps_not_met", func(t *testing.T) {
		t.Parallel()
		enqueueCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-b", StepRef: "b", JobID: "job-b", DependsOn: []string{"a"}}}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-b", StepRef: "b", Status: domain.StepPending, DepsCompleted: 0, DepsRequired: 1}}, nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCalled = true
			return nil
		}}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, mq, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		require.NoError(
			t, err)
		require.False(t,
			enqueueCalled,
		)
	})

	t.Run("respects_max_parallel_steps", func(t *testing.T) {
		t.Parallel()
		enqueueCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, MaxParallelSteps: 1, Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", StepRef: "a", JobID: "job-a"},
					{ID: "step-b", StepRef: "b", JobID: "job-b"},
				}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-a", StepRef: "a", Status: domain.StepRunning, DepsCompleted: 0, DepsRequired: 0},
					{ID: "sr-b", StepRef: "b", Status: domain.StepPending, DepsCompleted: 0, DepsRequired: 0},
				}, nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCalled = true
			return nil
		}}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, mq, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		require.NoError(
			t, err)
		require.False(t,
			enqueueCalled,
		)
	})
}

func TestStepCallback_FanInStartsWaitingRootsWithoutDependents(t *testing.T) {
	t.Parallel()

	enqueueCalled := false
	stepRunningUpdated := false
	ms := &mockCallbackStore{
		incrementStepDepsFn: func(_ context.Context, workflowRunID, completedStepRef string) ([]store.StepDepResult, error) {
			require.False(t,
				workflowRunID !=
					"wr-1" ||
					completedStepRef !=
						"a")

			return nil, nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			require.Equal(t,
				"wr-1", id,
			)

			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
			require.False(t,
				workflowID !=
					"wf-1" || version !=
					1)

			return []domain.WorkflowStep{
				{ID: "step-a", StepRef: "a", JobID: "job-a", ConcurrencyKey: "db"},
				{ID: "step-b", StepRef: "b", JobID: "job-b", ConcurrencyKey: "db"},
			}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, workflowRunID string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			require.Equal(t,
				"wr-1", workflowRunID,
			)

			return []domain.WorkflowStepRun{
				{ID: "sr-a", WorkflowRunID: "wr-1", WorkflowStepID: "step-a", StepRef: "a", Status: domain.StepCompleted, DepsCompleted: 0, DepsRequired: 0},
				{ID: "sr-b", WorkflowRunID: "wr-1", WorkflowStepID: "step-b", StepRef: "b", Status: domain.StepWaiting, DepsCompleted: 0, DepsRequired: 0},
			}, nil
		},
	}
	engStore := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if id == "sr-b" && status == domain.StepRunning {
				stepRunningUpdated = true
			}
			return nil
		},
	}
	mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		enqueueCalled = true
		run.ID = "jr-b"
		require.False(t,
			run.JobID !=
				"job-b" || run.
				WorkflowStepRunID !=
				"sr-b")

		return nil
	}}

	cb := NewStepCallback(ms, NewWorkflowEngine(engStore, mq, slog.Default()), slog.Default())
	wc := buildWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, ProjectID: "proj-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{ID: "step-a", StepRef: "a", JobID: "job-a", ConcurrencyKey: "db"},
			{ID: "step-b", StepRef: "b", JobID: "job-b", ConcurrencyKey: "db"},
		},
	)
	err := cb.fanInAndStartReadyChildren(context.Background(), &domain.WorkflowStepRun{ID: "sr-a", WorkflowRunID: "wr-1", StepRef: "a", Status: domain.StepCompleted}, wc)
	require.NoError(
		t, err)
	require.True(t,
		enqueueCalled,
	)
	require.True(t,
		stepRunningUpdated,
	)
}

func TestRetryWorkflowRun(t *testing.T) {
	t.Parallel()
	// Helper: build a standard 3-step DAG (a -> b -> c) for retry tests.
	buildSteps := func() []domain.WorkflowStep {
		return []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
			{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
			{ID: "step-c", JobID: "job-c", StepRef: "c", DependsOn: []string{"b"}},
		}
	}

	t.Run("retry from failed step b in a->b->c DAG", func(t *testing.T) {
		t.Parallel()
		stepRunsCreated := make(map[string]*domain.WorkflowStepRun)
		enqueuedJobs := make([]string, 0)
		steps := buildSteps()

		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID:              "orig-run-1",
					WorkflowID:      "wf-1",
					ProjectID:       "proj-1",
					Status:          domain.WfStatusFailed,
					TriggeredBy:     domain.TriggerManual,
					WorkflowVersion: 1,
					Payload:         json.RawMessage(`{"input":"data"}`),
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return steps, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepCompleted, Output: json.RawMessage(`{"result":"ok"}`)},
					{ID: "orig-sr-b", StepRef: "b", Status: domain.StepFailed, Error: "timeout"},
					{ID: "orig-sr-c", StepRef: "c", Status: domain.StepCanceled},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run-1"
				require.Equal(t,
					"orig-run-1",
					run.RetryOfRunID,
				)
				require.Equal(t,
					domain.TriggerRetry,
					run.TriggeredBy,
				)

				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return map[string]json.RawMessage{"a": json.RawMessage(`{"result":"ok"}`)}, nil
			},
		}

		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueuedJobs = append(enqueuedJobs, run.JobID)
				run.ID = "job-run-" + run.JobID
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		newRun, err := engine.RetryWorkflowRun(context.Background(), "orig-run-1")
		require.NoError(
			t, err)
		require.Equal(t,
			"retry-run-1",
			newRun.ID)
		require.Equal(t,
			domain.WfStatusRunning,
			newRun.
				Status,
		)
		require.Equal(t,
			"orig-run-1",
			newRun.RetryOfRunID,
		)

		// Verify retry run properties.

		// Step a should be pre-completed (copied from original).
		if sr, ok := stepRunsCreated["a"]; !ok {
			require.Fail(t,

				"step run 'a' not created")
		} else {
			require.Equal(t,
				domain.StepCompleted,
				sr.Status,
			)
			require.JSONEq(t,
				`{"result":"ok"}`,
				string(sr.Output))
		}

		// Step b should be fresh (was failed, now re-executed).
		if sr, ok := stepRunsCreated["b"]; !ok {
			require.Fail(t,

				"step run 'b' not created")
		} else if sr.DepsCompleted != 1 || sr.DepsRequired != 1 {
			require.Failf(t, "test failure",

				// Step b deps are all complete (a is pre-completed), so it should be started.
				"step b deps: completed=%d required=%d, want 1/1", sr.DepsCompleted, sr.DepsRequired)
		}

		// Step c should be waiting (its dep b was not completed in original).
		if sr, ok := stepRunsCreated["c"]; !ok {
			require.Fail(t,

				"step run 'c' not created")
		} else if sr.Status != domain.StepWaiting {
			require.Failf(t, "test failure",

				"step c status = %q, want waiting", sr.Status)
		}
		require.False(t,
			len(enqueuedJobs) != 1 || enqueuedJobs[0] !=
				"job-b")

		// Only job-b should be enqueued (step a pre-completed, step c waiting).
	})

	t.Run("cannot retry non-terminal run", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "run-1", Status: domain.WfStatusRunning}, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "run-1")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "must be terminal",
		)
	})

	t.Run("cannot retry when workflow is disabled", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "run-1", WorkflowID: "wf-1", Status: domain.WfStatusFailed, WorkflowVersion: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: false}, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "run-1")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "disabled")
	})

	t.Run("retry run not found", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "no-such-run")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "not found")
	})

	t.Run("retry all-completed run re-starts root steps", func(t *testing.T) {
		t.Parallel()
		// If the original run completed successfully but user wants to retry,
		// all steps should be re-executed since there's no failed step.
		stepRunsCreated := make(map[string]*domain.WorkflowStepRun)
		enqueueCount := 0

		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusCompleted, WorkflowVersion: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
					{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepCompleted, Output: json.RawMessage(`{"x":1}`)},
					{ID: "orig-sr-b", StepRef: "b", Status: domain.StepCompleted, Output: json.RawMessage(`{"y":2}`)},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return nil, nil
			},
		}

		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueueCount++
				run.ID = "job-run-" + run.JobID
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		newRun, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		require.NoError(
			t, err)
		require.Equal(t,
			"retry-run",
			newRun.ID)

		// All steps completed in original — both should be pre-completed.
		for _, ref := range []string{"a", "b"} {
			sr, ok := stepRunsCreated[ref]
			require.True(t,
				ok)
			require.Equal(t,
				domain.StepCompleted,
				sr.Status,
			)
		}
		require.Equal(t, 0, enqueueCount)

		// No new jobs should be enqueued since all steps were pre-completed.
	})

	t.Run("retry respects max parallel steps", func(t *testing.T) {
		t.Parallel()
		enqueueCount := 0
		stepRunsCreated := make(map[string]*domain.WorkflowStepRun)

		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusFailed, WorkflowVersion: 1,
					MaxParallelSteps: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				// Two independent root steps (no deps on each other).
				return []domain.WorkflowStep{
					{ID: "step-x", JobID: "job-x", StepRef: "x"},
					{ID: "step-y", JobID: "job-y", StepRef: "y"},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-x", StepRef: "x", Status: domain.StepFailed},
					{ID: "orig-sr-y", StepRef: "y", Status: domain.StepCanceled},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return nil, nil
			},
		}

		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueueCount++
				run.ID = "job-run-" + run.JobID
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		newRun, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		require.NoError(
			t, err)
		require.NotNil(t,
			newRun)
		require.Equal(t, 1, enqueueCount)

		// With max_parallel_steps=1, only 1 step should be enqueued.
	})

	t.Run("retry with timeout sets expires_at", func(t *testing.T) {
		t.Parallel()
		var createdRun *domain.WorkflowRun
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusTimedOut, WorkflowVersion: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1, TimeoutSecs: 300}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepFailed},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				createdRun = run
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return nil, nil
			},
		}

		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		require.NoError(
			t, err)
		require.False(t,
			createdRun ==
				nil || createdRun.
				ExpiresAt ==
				nil)
	})

	t.Run("retry preserves original payload", func(t *testing.T) {
		t.Parallel()
		var capturedPayload json.RawMessage
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusFailed, WorkflowVersion: 1,
					Payload: json.RawMessage(`{"env":"prod","batch_id":42}`),
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepFailed},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				capturedPayload = run.Payload
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return nil, nil
			},
		}

		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		require.NoError(
			t, err)
		require.JSONEq(t,
			`{"env":"prod","batch_id":42}`,

			string(capturedPayload))
	})

	t.Run("retry canceled run with all steps completed", func(t *testing.T) {
		t.Parallel()
		stepRunsCreated := make(map[string]*domain.WorkflowStepRun)
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusCanceled, WorkflowVersion: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					// Canceled run but step completed before cancellation
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepCompleted, Output: json.RawMessage(`{"v":1}`)},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return nil, nil
			},
		}

		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		newRun, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		require.NoError(
			t, err)
		require.NotNil(t,
			newRun)

		// Step a was completed, so should be pre-completed.
		if sr, ok := stepRunsCreated["a"]; !ok {
			require.Fail(t,

				"step a not created")
		} else if sr.Status != domain.StepCompleted {
			require.Failf(t, "test failure",

				"step a status = %q, want completed", sr.Status)
		}
	})

	t.Run("retry store error on get workflow run", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, fmt.Errorf("database connection error")
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "run-1")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "database connection error",
		)
	})

	t.Run("retry with fan-out DAG: a->{b,c} where c failed", func(t *testing.T) {
		t.Parallel()
		stepRunsCreated := make(map[string]*domain.WorkflowStepRun)
		enqueuedJobs := make([]string, 0)

		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusFailed, WorkflowVersion: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
					{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
					{ID: "step-c", JobID: "job-c", StepRef: "c", DependsOn: []string{"a"}},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepCompleted, Output: json.RawMessage(`{"a":1}`)},
					{ID: "orig-sr-b", StepRef: "b", Status: domain.StepCompleted, Output: json.RawMessage(`{"b":2}`)},
					{ID: "orig-sr-c", StepRef: "c", Status: domain.StepFailed, Error: "oom"},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return map[string]json.RawMessage{"a": json.RawMessage(`{"a":1}`)}, nil
			},
		}

		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueuedJobs = append(enqueuedJobs, run.JobID)
				run.ID = "job-run-" + run.JobID
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		require.NoError(
			t, err)
		require.Equal(t,
			domain.StepCompleted,
			stepRunsCreated["a"].Status,
		)
		require.Equal(t,
			domain.StepCompleted,
			stepRunsCreated["b"].Status,
		)
		require.False(t,
			len(enqueuedJobs) != 1 || enqueuedJobs[0] !=
				"job-c")

		// Step a: pre-completed. Step b: pre-completed. Step c: re-executed.

		// Only step c should be enqueued.
	})
}

func TestTriggerSubWorkflow(t *testing.T) {
	t.Parallel()
	t.Run("happy path triggers child workflow", func(t *testing.T) {
		t.Parallel()
		var createdRun *domain.WorkflowRun
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-a", JobID: "job-a", StepRef: "a"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "child-run-1"
				createdRun = run
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		wfRun, err := engine.TriggerSubWorkflow(context.Background(), "wf-child", "proj-1", json.RawMessage(`{"from":"parent"}`), domain.TriggerWorkflow, "parent-run-1", "")
		require.NoError(
			t, err)
		require.False(t,
			wfRun == nil ||
				wfRun.ID !=
					"child-run-1",
		)
		require.NotNil(t,
			createdRun,
		)
		require.Equal(t,
			"parent-run-1",
			createdRun.
				ParentWorkflowRunID,
		)
	})

	t.Run("inherits project ID from parent", func(t *testing.T) {
		t.Parallel()
		parentRun := &domain.WorkflowRun{ID: "parent-run-1", ProjectID: "proj-parent"}
		var createdRun *domain.WorkflowRun

		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: parentRun.ProjectID, Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-a", JobID: "job-a", StepRef: "a"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "child-run-2"
				createdRun = run
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-child", parentRun.ProjectID, json.RawMessage(`{"from":"parent"}`), domain.TriggerWorkflow, parentRun.ID, "")
		require.NoError(
			t, err)
		require.NotNil(t,
			createdRun,
		)
		require.Equal(t,
			parentRun.
				ProjectID, createdRun.
				ProjectID,
		)
	})
}

func TestStartSubWorkflowStep(t *testing.T) {
	t.Parallel()
	t.Run("triggers sub-workflow and sets step running", func(t *testing.T) {
		t.Parallel()
		stepRunningUpdated := false
		var parentRunID string
		childTriggered := false

		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				switch id {
				case "wf-parent":
					return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
				case "wf-child":
					return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
				default:
					return nil, nil
				}
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:            "step-sub",
						StepRef:       "sub",
						StepType:      domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID: "wf-child",
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				if run.WorkflowID == "wf-parent" {
					run.ID = "wr-parent"
					parentRunID = run.ID
					return nil
				}
				if run.WorkflowID == "wf-child" {
					run.ID = "wr-child"
					if run.ParentWorkflowRunID == parentRunID {
						childTriggered = true
					}
					return nil
				}
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef + "-" + sr.WorkflowRunID
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if strings.Contains(id, "sr-sub-") && status == domain.StepRunning {
					stepRunningUpdated = true
				}
				return nil
			},
		}

		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-child"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf-parent", "proj-1", json.RawMessage(`{"hello":"world"}`), "manual", nil, nil)
		require.NoError(
			t, err)
		require.True(t,
			stepRunningUpdated,
		)
		require.True(t,
			childTriggered,
		)
	})

	t.Run("fails when nesting depth exceeded", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:              "step-sub",
						StepRef:         "sub",
						StepType:        domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID:   "wf-child",
						MaxNestingDepth: 1,
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				if id == "ancestor-run" {
					return &domain.WorkflowRun{ID: "ancestor-run", ParentWorkflowRunID: ""}, nil
				}
				return nil, nil
			},
		}

		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, "ancestor-run", "")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "nesting depth",
		)
	})

	t.Run("fails when sub-workflow is disabled", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				if id == "wf-parent" {
					return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
				}
				if id == "wf-child" {
					return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: false, Version: 1}, nil
				}
				return nil, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:            "step-sub",
						StepRef:       "sub",
						StepType:      domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID: "wf-child",
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, nil, nil)
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "disabled")
	})
}

func TestGetNestingDepth(t *testing.T) {
	t.Parallel()
	t.Run("depth 0 for root workflow", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:              "step-sub",
						StepRef:         "sub",
						StepType:        domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID:   "wf-child",
						MaxNestingDepth: 2,
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, nil, nil)
		require.NoError(
			t, err)
	})

	t.Run("depth 1 for single parent", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:              "step-sub",
						StepRef:         "sub",
						StepType:        domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID:   "wf-child",
						MaxNestingDepth: 2,
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				if id == "p1" {
					return &domain.WorkflowRun{ID: "p1", ParentWorkflowRunID: ""}, nil
				}
				return nil, nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, "p1", "")
		require.NoError(
			t, err)
	})

	t.Run("depth 2 for nested chain", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:              "step-sub",
						StepRef:         "sub",
						StepType:        domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID:   "wf-child",
						MaxNestingDepth: 3,
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				switch id {
				case "p2":
					return &domain.WorkflowRun{ID: "p2", ParentWorkflowRunID: "p1"}, nil
				case "p1":
					return &domain.WorkflowRun{ID: "p1", ParentWorkflowRunID: ""}, nil
				default:
					return nil, nil
				}
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, "p2", "")
		require.NoError(
			t, err)
	})

	t.Run("circular reference detected", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:            "step-sub",
						StepRef:       "sub",
						StepType:      domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID: "wf-child",
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				switch id {
				case "B":
					return &domain.WorkflowRun{ID: "B", ParentWorkflowRunID: "A"}, nil
				case "A":
					return &domain.WorkflowRun{ID: "A", ParentWorkflowRunID: "B"}, nil
				default:
					return nil, nil
				}
			},
		}

		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, "B", "")
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "circular")
	})

	t.Run("parent not found returns depth so far", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:            "step-sub",
						StepRef:       "sub",
						StepType:      domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID: "wf-child",
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, "missing-parent", "")
		require.NoError(
			t, err)
	})
}

func TestGetNestingDepth_Direct(t *testing.T) {
	t.Parallel()
	t.Run("no parent", func(t *testing.T) {
		t.Parallel()
		engine := NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default())
		depth, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "run-a"})
		require.NoError(
			t, err)
		require.Equal(t, 0, depth)
	})

	t.Run("single parent", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				if id == "parent" {
					return &domain.WorkflowRun{ID: "parent"}, nil
				}
				return nil, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		depth, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "child", ParentWorkflowRunID: "parent"})
		require.NoError(
			t, err)
		require.Equal(t, 1, depth)
	})

	t.Run("three levels deep", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				switch id {
				case "parent":
					return &domain.WorkflowRun{ID: "parent", ParentWorkflowRunID: "grandparent"}, nil
				case "grandparent":
					return &domain.WorkflowRun{ID: "grandparent"}, nil
				default:
					return nil, nil
				}
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		depth, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "child", ParentWorkflowRunID: "parent"})
		require.NoError(
			t, err)
		require.Equal(t, 2, depth)
	})

	t.Run("circular reference", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				switch id {
				case "run-b":
					return &domain.WorkflowRun{ID: "run-b", ParentWorkflowRunID: "run-a"}, nil
				case "run-a":
					return &domain.WorkflowRun{ID: "run-a", ParentWorkflowRunID: "run-b"}, nil
				default:
					return nil, nil
				}
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "run-a", ParentWorkflowRunID: "run-b"})
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "circular parent reference",
		)
	})

	t.Run("parent not found", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		depth, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "child", ParentWorkflowRunID: "missing"})
		require.NoError(
			t, err)
		require.Equal(t, 1, depth)
	})

	t.Run("store error", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, errors.New("db error")
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "child", ParentWorkflowRunID: "parent"})
		require.Error(t,
			err)
	})
}

// propagateToParent tests — exercised indirectly through OnJobRunTerminal.

func TestPropagateToParent_ChildCompleted(t *testing.T) {
	t.Parallel()
	// Flow: job run completed → step completed → fanIn (no children) →
	// checkWorkflowCompletion (all terminal) → mark child completed →
	// propagateToParent → find parent step → mark parent step completed →
	// fanIn on parent (no deps) → checkWorkflowCompletion on parent.

	progressionCreated := false

	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, jobRunID string) (*domain.WorkflowStepRun, error) {
			if jobRunID != "jr-child-1" {
				return nil, nil
			}
			return &domain.WorkflowStepRun{
				ID:            "sr-child-root",
				WorkflowRunID: "child-run-1",
				StepRef:       "child-root",
				Status:        domain.StepRunning,
				JobRunID:      "jr-child-1",
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
			require.False(t,
				id == "sr-child-root" &&
					status !=
						domain.
							StepCompleted,
			)

			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return nil, nil // no deps to fan-in
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			switch id {
			case "child-run-1":
				return &domain.WorkflowRun{
					ID:                  "child-run-1",
					WorkflowID:          "wf-child",
					WorkflowVersion:     1,
					Status:              domain.WfStatusRunning,
					ParentWorkflowRunID: "parent-run-1",
				}, nil
			case "parent-run-1":
				return &domain.WorkflowRun{
					ID:              "parent-run-1",
					WorkflowID:      "wf-parent",
					WorkflowVersion: 1,
					Status:          domain.WfStatusRunning,
				}, nil
			default:
				return nil, nil
			}
		},
		listStepRunsByWorkflowRun: func(_ context.Context, workflowRunID string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			switch workflowRunID {
			case "child-run-1":
				return []domain.WorkflowStepRun{
					{ID: "sr-child-root", WorkflowRunID: "child-run-1", StepRef: "child-root", Status: domain.StepCompleted, Output: json.RawMessage(`{"result":"ok"}`)},
				}, nil
			case "parent-run-1":
				return []domain.WorkflowStepRun{
					{ID: "sr-parent-sub", WorkflowRunID: "parent-run-1", StepRef: "sub-step", Status: domain.StepCompleted},
				}, nil
			default:
				return nil, nil
			}
		},
		listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
			switch workflowID {
			case "wf-child":
				return []domain.WorkflowStep{
					{ID: "step-child-root", StepRef: "child-root", JobID: "job-c1"},
				}, nil
			case "wf-parent":
				return []domain.WorkflowStep{
					{ID: "step-parent-sub", StepRef: "sub-step", StepType: domain.WorkflowStepTypeSubWorkflow, SubWorkflowID: "wf-child"},
				}, nil
			default:
				return nil, nil
			}
		},
		getStepRunByRunAndRefFn: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
			if workflowRunID == "parent-run-1" && stepRef == "sub-step" {
				return &domain.WorkflowStepRun{
					ID:            "sr-parent-sub",
					WorkflowRunID: "parent-run-1",
					StepRef:       "sub-step",
					Status:        domain.StepRunning,
				}, nil
			}
			return nil, nil
		},
		createWorkflowProgressionEventFn: func(_ context.Context, workflowRunID, stepRunID, stepRef, status string) error {
			require.False(t,
				workflowRunID !=
					"child-run-1" ||
					stepRunID !=
						"sr-child-root" ||
					stepRef !=
						"child-root" ||
					status !=
						string(domain.StepCompleted),
			)

			progressionCreated = true
			return nil
		},
	}

	engine := NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:                "jr-child-1",
		Status:            domain.StatusCompleted,
		Result:            json.RawMessage(`{"result":"ok"}`),
		WorkflowStepRunID: "sr-child-root",
	})
	require.NoError(
		t, err)
	require.True(t,
		progressionCreated,
	)
}

func TestPropagateToParent_ChildFailed(t *testing.T) {
	t.Parallel()
	// Flow: job run fails → step fails → handleFailedStep (fail_workflow) →
	// mark child workflow failed → cancelRemainingSteps → propagateToParent →
	// mark parent step failed → handleFailedStep on parent.

	parentStepFailed := false
	childWfMarkedFailed := false
	parentWfMarkedFailed := false

	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, jobRunID string) (*domain.WorkflowStepRun, error) {
			if jobRunID != "jr-child-1" {
				return nil, nil
			}
			return &domain.WorkflowStepRun{
				ID:            "sr-child-root",
				WorkflowRunID: "child-run-1",
				StepRef:       "child-root",
				Status:        domain.StepRunning,
				JobRunID:      "jr-child-1",
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if id == "sr-parent-sub" && status == domain.StepFailed {
				parentStepFailed = true
			}
			return nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			switch id {
			case "child-run-1":
				return &domain.WorkflowRun{
					ID:                  "child-run-1",
					WorkflowID:          "wf-child",
					WorkflowVersion:     1,
					Status:              domain.WfStatusRunning,
					ParentWorkflowRunID: "parent-run-1",
				}, nil
			case "parent-run-1":
				return &domain.WorkflowRun{
					ID:              "parent-run-1",
					WorkflowID:      "wf-parent",
					WorkflowVersion: 1,
					Status:          domain.WfStatusRunning,
				}, nil
			default:
				return nil, nil
			}
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
			if id == "child-run-1" && to == domain.WfStatusFailed {
				childWfMarkedFailed = true
			}
			if id == "parent-run-1" && to == domain.WfStatusFailed {
				parentWfMarkedFailed = true
			}
			return nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, workflowRunID string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			switch workflowRunID {
			case "child-run-1":
				// All step runs already terminal (the one step is failed)
				return []domain.WorkflowStepRun{
					{ID: "sr-child-root", WorkflowRunID: "child-run-1", StepRef: "child-root", Status: domain.StepFailed, Error: "job failed"},
				}, nil
			case "parent-run-1":
				return []domain.WorkflowStepRun{
					{ID: "sr-parent-sub", WorkflowRunID: "parent-run-1", StepRef: "sub-step", Status: domain.StepFailed},
				}, nil
			default:
				return nil, nil
			}
		},
		listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
			switch workflowID {
			case "wf-child":
				return []domain.WorkflowStep{
					{ID: "step-child-root", StepRef: "child-root", JobID: "job-c1", OnFailure: domain.FailWorkflow},
				}, nil
			case "wf-parent":
				return []domain.WorkflowStep{
					{ID: "step-parent-sub", StepRef: "sub-step", StepType: domain.WorkflowStepTypeSubWorkflow, SubWorkflowID: "wf-child", OnFailure: domain.FailWorkflow},
				}, nil
			default:
				return nil, nil
			}
		},
		getStepRunByRunAndRefFn: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
			if workflowRunID == "parent-run-1" && stepRef == "sub-step" {
				return &domain.WorkflowStepRun{
					ID:            "sr-parent-sub",
					WorkflowRunID: "parent-run-1",
					StepRef:       "sub-step",
					Status:        domain.StepRunning,
				}, nil
			}
			return nil, nil
		},
	}

	engine := NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:                "jr-child-1",
		Status:            domain.StatusFailed,
		Error:             "job failed",
		WorkflowStepRunID: "sr-child-root",
	})
	require.NoError(
		t, err)
	require.True(t,
		childWfMarkedFailed,
	)
	require.True(t,
		parentStepFailed,
	)
	require.True(t,
		parentWfMarkedFailed,
	)
}

func TestPropagateToParent_NoParent(t *testing.T) {
	t.Parallel()
	// When ParentWorkflowRunID is empty, propagateToParent is a no-op.
	// The parent's GetWorkflowRun should never be called.

	parentLookedUp := false
	getRunCalls := 0

	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID:            "sr-1",
				WorkflowRunID: "child-run-1",
				StepRef:       "root",
				Status:        domain.StepRunning,
				JobRunID:      "jr-1",
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			getRunCalls++
			if id == "child-run-1" {
				return &domain.WorkflowRun{
					ID:                  "child-run-1",
					WorkflowID:          "wf-child",
					WorkflowVersion:     1,
					Status:              domain.WfStatusRunning,
					ParentWorkflowRunID: "", // No parent
				}, nil
			}
			// Any other call means we tried to look up a parent
			parentLookedUp = true
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", WorkflowRunID: "child-run-1", StepRef: "root", Status: domain.StepCompleted},
			}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{ID: "step-1", StepRef: "root", JobID: "job-1"},
			}, nil
		},
	}

	engine := NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:                "jr-1",
		Status:            domain.StatusCompleted,
		Result:            json.RawMessage(`{"ok":true}`),
		WorkflowStepRunID: "sr-1",
	})
	require.NoError(
		t, err)
	require.False(t,
		parentLookedUp,
	)
}

func TestPropagateToParent_ParentAlreadyTerminal(t *testing.T) {
	t.Parallel()
	// When the parent workflow run is already terminal, propagation stops.
	// GetStepRunByWorkflowRunAndRef should NOT be called.

	stepRunLookedUp := false

	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID:            "sr-1",
				WorkflowRunID: "child-run-1",
				StepRef:       "root",
				Status:        domain.StepRunning,
				JobRunID:      "jr-1",
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			switch id {
			case "child-run-1":
				return &domain.WorkflowRun{
					ID:                  "child-run-1",
					WorkflowID:          "wf-child",
					WorkflowVersion:     1,
					Status:              domain.WfStatusRunning,
					ParentWorkflowRunID: "parent-run-1",
				}, nil
			case "parent-run-1":
				return &domain.WorkflowRun{
					ID:              "parent-run-1",
					WorkflowID:      "wf-parent",
					WorkflowVersion: 1,
					Status:          domain.WfStatusCompleted, // Already terminal
				}, nil
			default:
				return nil, nil
			}
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", WorkflowRunID: "child-run-1", StepRef: "root", Status: domain.StepCompleted},
			}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{ID: "step-1", StepRef: "root", JobID: "job-1"},
			}, nil
		},
		getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
			stepRunLookedUp = true
			return nil, nil
		},
	}

	engine := NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:                "jr-1",
		Status:            domain.StatusCompleted,
		Result:            json.RawMessage(`{"ok":true}`),
		WorkflowStepRunID: "sr-1",
	})
	require.NoError(
		t, err)
	require.False(t,
		stepRunLookedUp,
	)
}

func TestApplyStepOverrides(t *testing.T) {
	t.Parallel()
	t.Run("no overrides returns original steps", func(t *testing.T) {
		t.Parallel()
		steps := []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
			{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
		}

		gotNil, err := applyStepOverrides(steps, nil)
		require.NoError(
			t, err)
		require.Len(t, gotNil,
			len(
				steps))
		require.False(t,
			gotNil[0].
				StepRef != "a" ||
				gotNil[1].
					StepRef !=
					"b")
		require.False(t,
			len(gotNil[1].DependsOn) !=
				1 || gotNil[1].DependsOn[0] !=
				"a")

		gotEmpty, err := applyStepOverrides(steps, []domain.StepOverride{})
		require.NoError(
			t, err)
		require.Len(t, gotEmpty,
			len(steps))
		require.False(t,
			gotEmpty[0].StepRef != "a" ||
				gotEmpty[1].StepRef !=
					"b",
		)
		require.False(t,
			len(gotEmpty[1].DependsOn) !=
				1 || gotEmpty[1].DependsOn[0] != "a",
		)
	})

	t.Run("disable one step", func(t *testing.T) {
		t.Parallel()
		steps := []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
			{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
			{ID: "step-c", JobID: "job-c", StepRef: "c", DependsOn: []string{"b"}},
		}

		got, err := applyStepOverrides(steps, []domain.StepOverride{{StepRef: "b", Enabled: false}})
		require.NoError(
			t, err)
		require.Len(t, got,
			2)
		require.False(t,
			got[0].StepRef !=
				"a" || got[1].StepRef !=
				"c",
		)
		require.Empty(t, got[1].DependsOn)
	})

	t.Run("unknown step_ref returns error", func(t *testing.T) {
		t.Parallel()
		steps := []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
		}

		_, err := applyStepOverrides(steps, []domain.StepOverride{{StepRef: "nonexistent", Enabled: false}})
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "unknown step_ref",
		)
	})

	t.Run("unknown enabled step_ref returns error", func(t *testing.T) {
		t.Parallel()
		steps := []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
		}

		_, err := applyStepOverrides(steps, []domain.StepOverride{{StepRef: "nonexistent", Enabled: true}})
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "unknown step_ref",
		)
	})

	t.Run("all steps disabled returns error", func(t *testing.T) {
		t.Parallel()
		steps := []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
			{ID: "step-b", JobID: "job-b", StepRef: "b"},
		}

		_, err := applyStepOverrides(steps, []domain.StepOverride{
			{StepRef: "a", Enabled: false},
			{StepRef: "b", Enabled: false},
		})
		require.Error(t,
			err)
		assert.Contains(
			t, err.Error(), "all steps disabled",
		)
	})

	t.Run("prunes depends_on for disabled step", func(t *testing.T) {
		t.Parallel()
		steps := []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
			{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
			{ID: "step-c", JobID: "job-c", StepRef: "c", DependsOn: []string{"a", "b"}},
		}

		got, err := applyStepOverrides(steps, []domain.StepOverride{{StepRef: "b", Enabled: false}})
		require.NoError(
			t, err)
		require.Len(t, got,
			2)
		require.Equal(t,
			"c", got[1].StepRef)
		require.False(t,
			len(got[1].
				DependsOn) != 1 ||
				got[1].
					DependsOn[0] != "a",
		)
	})
}

func BenchmarkApplyStepOverrides(b *testing.B) {
	steps := make([]domain.WorkflowStep, 100)
	for i := range steps {
		steps[i] = domain.WorkflowStep{
			ID:      fmt.Sprintf("step-%03d", i),
			JobID:   fmt.Sprintf("job-%03d", i),
			StepRef: fmt.Sprintf("step-%03d", i),
		}
		if i > 0 {
			steps[i].DependsOn = []string{steps[i-1].StepRef}
		}
	}

	b.Run("none", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			got, err := applyStepOverrides(steps, nil)
			if err != nil {
				b.Fatal(err)
			}
			if len(got) != len(steps) {
				b.Fatalf("len(got) = %d", len(got))
			}
		}
	})
	b.Run("all_enabled", func(b *testing.B) {
		overrides := []domain.StepOverride{{StepRef: "step-050", Enabled: true}}
		b.ReportAllocs()
		for b.Loop() {
			got, err := applyStepOverrides(steps, overrides)
			if err != nil {
				b.Fatal(err)
			}
			if len(got) != len(steps) {
				b.Fatalf("len(got) = %d", len(got))
			}
		}
	})
	b.Run("disable_middle", func(b *testing.B) {
		overrides := []domain.StepOverride{{StepRef: "step-050", Enabled: false}}
		b.ReportAllocs()
		for b.Loop() {
			got, err := applyStepOverrides(steps, overrides)
			if err != nil {
				b.Fatal(err)
			}
			if len(got) != len(steps)-1 {
				b.Fatalf("len(got) = %d", len(got))
			}
		}
	})
	b.Run("disable_many", func(b *testing.B) {
		overrides := []domain.StepOverride{
			{StepRef: "step-020", Enabled: false},
			{StepRef: "step-040", Enabled: false},
			{StepRef: "step-060", Enabled: false},
			{StepRef: "step-080", Enabled: false},
		}
		b.ReportAllocs()
		for b.Loop() {
			got, err := applyStepOverrides(steps, overrides)
			if err != nil {
				b.Fatal(err)
			}
			if len(got) != len(steps)-4 {
				b.Fatalf("len(got) = %d", len(got))
			}
		}
	})
}

func TestApplyStepOverrides_DoesNotMutateInputDependsOn(t *testing.T) {
	t.Parallel()

	steps := []domain.WorkflowStep{
		{ID: "step-a", JobID: "job-a", StepRef: "a"},
		{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
		{ID: "step-c", JobID: "job-c", StepRef: "c", DependsOn: []string{"a", "b"}},
	}

	got, err := applyStepOverrides(steps, []domain.StepOverride{{StepRef: "b", Enabled: false}})
	require.NoError(
		t, err)
	require.Len(t, got,
		2)
	require.False(t,
		len(got[1].
			DependsOn) != 1 ||
			got[1].
				DependsOn[0] != "a",
	)
	require.False(t,
		len(steps[2].DependsOn) !=
			2 || steps[2].DependsOn[0] !=
			"a" || steps[2].DependsOn[1] !=
			"b")
}

func TestTriggerWorkflowWithStepOverrides(t *testing.T) {
	t.Parallel()
	t.Run("overrides filter steps at trigger", func(t *testing.T) {
		t.Parallel()
		createdStepRefs := make([]string, 0)
		enqueueCount := 0

		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
					{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-override"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				createdStepRefs = append(createdStepRefs, sr.StepRef)
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, _ domain.StepRunStatus, _ map[string]any) error {
				require.Equal(t,
					"sr-a", id,
				)

				return nil
			},
		}

		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueueCount++
				run.ID = "jr-a"
				require.Equal(t,
					"job-a", run.
						JobID)
				require.Equal(t,
					"sr-a", run.
						WorkflowStepRunID,
				)

				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		wfRun, err := engine.TriggerWorkflow(
			context.Background(),
			"wf-1",
			"proj-1",
			json.RawMessage(`{"k":"v"}`),
			"manual",
			[]domain.StepOverride{{StepRef: "b", Enabled: false}},
			nil,
		)
		require.NoError(
			t, err)
		require.False(t,
			wfRun == nil ||
				wfRun.ID !=
					"wr-override" ||
				wfRun.Status !=
					domain.
						WfStatusRunning,
		)
		require.False(t,
			len(createdStepRefs) != 1 ||
				createdStepRefs[0] != "a")
		require.Equal(t, 1, enqueueCount)
	})

	t.Run("unknown override step_ref returns error", func(t *testing.T) {
		t.Parallel()
		createWorkflowRunCalled := false

		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-a", JobID: "job-a", StepRef: "a"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, _ *domain.WorkflowRun) error {
				createWorkflowRunCalled = true
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(
			context.Background(),
			"wf-1",
			"proj-1",
			nil,
			"manual",
			[]domain.StepOverride{{StepRef: "nonexistent", Enabled: false}},
			nil,
		)
		require.Error(t,
			err)
		require.Contains(t,
			err.Error(), "unknown step_ref")
		require.False(t,
			createWorkflowRunCalled,
		)
	})
}

func TestStartStep_WaitForEvent_CreatesEventTrigger(t *testing.T) {
	t.Parallel()

	var capturedTrigger *domain.EventTrigger
	var capturedStepStatus domain.StepRunStatus

	ms := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, _ map[string]any) error {
			capturedStepStatus = status
			return nil
		},
		createEventTriggerFn: func(_ context.Context, trigger *domain.EventTrigger) error {
			capturedTrigger = trigger
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())

	stepRun := &domain.WorkflowStepRun{
		ID:            "sr-1",
		WorkflowRunID: "wr-1",
		StepRef:       "wait_aml",
		Status:        domain.StepPending,
	}
	step := &domain.WorkflowStep{
		StepRef:          "wait_aml",
		StepType:         domain.WorkflowStepTypeWaitForEvent,
		EventKey:         "aml-check:app-123",
		EventTimeoutSecs: 7200,
	}
	wfRun := &domain.WorkflowRun{
		ID:        "wr-1",
		ProjectID: "proj-1",
		Payload:   json.RawMessage(`{}`),
	}
	require.NoError(
		t, engine.startStep(context.
			Background(), stepRun,
			step,
			wfRun, nil,
		))
	require.Equal(t,
		domain.StepWaiting,
		capturedStepStatus,
	)
	require.Equal(t,
		domain.StepWaiting,
		stepRun.
			Status)
	require.NotNil(t,
		stepRun.StartedAt,
	)
	require.NotNil(t,
		capturedTrigger,
	)
	require.Equal(t,
		"aml-check:app-123",
		capturedTrigger.
			EventKey,
	)
	require.Equal(t,
		"workflow_step",
		capturedTrigger.
			SourceType,
	)
	require.Equal(t,
		"wr-1", capturedTrigger.
			WorkflowRunID,
	)
	require.Equal(t,
		"sr-1", capturedTrigger.
			WorkflowStepRunID,
	)
	require.Equal(t,
		domain.EventTriggerStatusWaiting,

		capturedTrigger.
			Status,
	)
	require.Equal(t, 7200, capturedTrigger.
		TimeoutSecs,
	)
	require.Equal(t,
		"proj-1",
		capturedTrigger.ProjectID,
	)
}

func TestStartStep_WaitForEvent_RendersTemplateKey(t *testing.T) {
	t.Parallel()

	var capturedTrigger *domain.EventTrigger

	ms := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		createEventTriggerFn: func(_ context.Context, trigger *domain.EventTrigger) error {
			capturedTrigger = trigger
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())

	step := &domain.WorkflowStep{
		StepRef:          "wait_aml",
		StepType:         domain.WorkflowStepTypeWaitForEvent,
		EventKey:         "aml:{{app_id}}",
		EventTimeoutSecs: 3600,
	}
	wfRun := &domain.WorkflowRun{
		ID:        "wr-1",
		ProjectID: "proj-1",
		Payload:   json.RawMessage(`{"app_id":"app-456"}`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "wait_aml"}
	require.NoError(
		t, engine.startStep(context.
			Background(), stepRun,
			step,
			wfRun, nil,
		))
	require.NotNil(t,
		capturedTrigger,
	)
	require.Equal(t,
		"aml:app-456",
		capturedTrigger.
			EventKey,
	)
}

func TestStartStep_WaitForEvent_DefaultTimeout(t *testing.T) {
	t.Parallel()

	var capturedTrigger *domain.EventTrigger

	ms := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		createEventTriggerFn: func(_ context.Context, trigger *domain.EventTrigger) error {
			capturedTrigger = trigger
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())

	step := &domain.WorkflowStep{
		StepRef:          "wait_step",
		StepType:         domain.WorkflowStepTypeWaitForEvent,
		EventKey:         "some-key",
		EventTimeoutSecs: 0, // should use default
	}
	wfRun := &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", Payload: json.RawMessage(`{}`)}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "wait_step"}
	require.NoError(
		t, engine.startStep(context.
			Background(), stepRun,
			step,
			wfRun, nil,
		))
	require.Equal(t,
		domain.DefaultEventTimeoutSecs,

		capturedTrigger.
			TimeoutSecs,
	)
}

func TestStartStep_WaitForEvent_StoreError(t *testing.T) {
	t.Parallel()

	ms := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		createEventTriggerFn: func(_ context.Context, _ *domain.EventTrigger) error {
			return errors.New("db connection failed")
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())

	step := &domain.WorkflowStep{
		StepRef:  "wait_step",
		StepType: domain.WorkflowStepTypeWaitForEvent,
		EventKey: "some-key",
	}
	wfRun := &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", Payload: json.RawMessage(`{}`)}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "wait_step"}

	err := engine.startStep(context.Background(), stepRun, step, wfRun, nil)
	require.Error(t,
		err)
	require.Contains(t,
		err.Error(), "create event trigger")
}

func TestStartStep_WaitForEvent_EmptyEventKey(t *testing.T) {
	t.Parallel()

	var stepStatusUpdated bool
	ms := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			stepStatusUpdated = true
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())

	step := &domain.WorkflowStep{
		StepRef:  "wait_step",
		StepType: domain.WorkflowStepTypeWaitForEvent,
		EventKey: "", // empty
	}
	wfRun := &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", Payload: json.RawMessage(`{}`)}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "wait_step"}

	err := engine.startStep(context.Background(), stepRun, step, wfRun, nil)
	require.Error(t,
		err)
	require.Contains(t,
		err.Error(), "event_key is empty")
	require.False(t,
		stepStatusUpdated,
	)

	// Step status should NOT have been updated — fail fast before DB writes.
}

func TestTriggerWorkflow_WaitForEventStep_RootStep(t *testing.T) {
	t.Parallel()

	var capturedTrigger *domain.EventTrigger
	stepRunsCreated := make(map[string]*domain.WorkflowStepRun)

	ms := &mockEngineStore{
		getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", ProjectID: "proj-1", Enabled: true, Version: 1}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{
					ID:               "step-1",
					StepRef:          "wait_aml",
					StepType:         domain.WorkflowStepTypeWaitForEvent,
					EventKey:         "aml-check:{{id}}",
					EventTimeoutSecs: 86400,
				},
			}, nil
		},
		createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
			run.ID = "wr-1"
			return nil
		},
		createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
			sr.ID = "sr-" + sr.StepRef
			stepRunsCreated[sr.StepRef] = sr
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		createEventTriggerFn: func(_ context.Context, trigger *domain.EventTrigger) error {
			capturedTrigger = trigger
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())

	wfRun, err := engine.TriggerWorkflow(
		context.Background(),
		"wf-1", "proj-1",
		json.RawMessage(`{"id":"app-789"}`),
		"manual",
		nil,
		nil,
	)
	require.NoError(
		t, err)
	require.NotNil(t,
		wfRun)
	require.NotNil(t,
		capturedTrigger,
	)
	require.Equal(t,
		"aml-check:app-789",
		capturedTrigger.
			EventKey,
	)
	require.Equal(t, 86400, capturedTrigger.
		TimeoutSecs,
	)
}

func TestStartStep_Approval_CreatesParallelEventTrigger(t *testing.T) {
	t.Parallel()

	var capturedApproval *domain.WorkflowStepApproval
	var capturedTrigger *domain.EventTrigger

	ms := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		createWorkflowStepApprovalFn: func(_ context.Context, approval *domain.WorkflowStepApproval) error {
			capturedApproval = approval
			return nil
		},
		createEventTriggerFn: func(_ context.Context, trigger *domain.EventTrigger) error {
			capturedTrigger = trigger
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())

	stepRun := &domain.WorkflowStepRun{
		ID:            "sr-1",
		WorkflowRunID: "wr-1",
		StepRef:       "approval_step",
		Status:        domain.StepPending,
	}
	step := &domain.WorkflowStep{
		StepRef:             "approval_step",
		StepType:            domain.WorkflowStepTypeApproval,
		ApprovalApprovers:   []string{"admin@example.com"},
		ApprovalTimeoutSecs: 86400,
	}
	wfRun := &domain.WorkflowRun{
		ID:        "wr-1",
		ProjectID: "proj-1",
		Payload:   json.RawMessage(`{}`),
	}
	require.NoError(
		t, engine.startStep(context.
			Background(), stepRun,
			step,
			wfRun, nil,
		))
	require.NotNil(t,
		capturedApproval,
	)
	require.NotNil(t,
		capturedTrigger,
	)
	require.Equal(t,
		"approval:wr-1:approval_step",

		capturedTrigger.
			EventKey)
	require.Equal(t,
		"workflow_step",
		capturedTrigger.
			SourceType,
	)
	require.Equal(t, 86400, capturedTrigger.
		TimeoutSecs,
	)
}

func TestStartStep_Approval_EventTriggerFailureNonFatal(t *testing.T) {
	t.Parallel()

	var approvalCreated bool

	ms := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		createWorkflowStepApprovalFn: func(_ context.Context, _ *domain.WorkflowStepApproval) error {
			approvalCreated = true
			return nil
		},
		createEventTriggerFn: func(_ context.Context, _ *domain.EventTrigger) error {
			return errors.New("unique constraint violation")
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())

	stepRun := &domain.WorkflowStepRun{
		ID:            "sr-1",
		WorkflowRunID: "wr-1",
		StepRef:       "approval_step",
		Status:        domain.StepPending,
	}
	step := &domain.WorkflowStep{
		StepRef:           "approval_step",
		StepType:          domain.WorkflowStepTypeApproval,
		ApprovalApprovers: []string{"admin@example.com"},
	}
	wfRun := &domain.WorkflowRun{
		ID:        "wr-1",
		ProjectID: "proj-1",
		Payload:   json.RawMessage(`{}`),
	}
	require.NoError(
		t, engine.startStep(context.
			Background(), stepRun,
			step,
			wfRun, nil,
		))
	require.True(t,
		approvalCreated,
	)
	require.Equal(t,
		domain.StepWaiting,
		stepRun.
			Status)

	// Should not error even though event trigger creation fails.
}

func TestApproveStep_SyncsEventTrigger(t *testing.T) {
	t.Parallel()

	var triggerSynced bool

	ms := &mockCallbackStore{
		getStepRunByRunAndRefFn: func(_ context.Context, _ string, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID:            "sr-1",
				WorkflowRunID: "wr-1",
				StepRef:       "approval_step",
				Status:        domain.StepWaiting,
			}, nil
		},
		getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
			return &domain.WorkflowStepApproval{
				ID:            "approval:sr-1",
				WorkflowRunID: "wr-1",
				Approvers:     []string{"admin@example.com"},
				Status:        domain.ApprovalStatusPending,
			}, nil
		},
		updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		getEventTriggerByStepRunIDFn: func(_ context.Context, stepRunID string) (*domain.EventTrigger, error) {
			if stepRunID == "sr-1" {
				return &domain.EventTrigger{
					ID:     "evt:approval:sr-1",
					Status: domain.EventTriggerStatusWaiting,
				}, nil
			}
			return nil, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, id string, status string, _ json.RawMessage, _ *time.Time, _ string) error {
			if id == "evt:approval:sr-1" && status == domain.EventTriggerStatusReceived {
				triggerSynced = true
			}
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:              "wr-1",
				Status:          domain.WfStatusRunning,
				WorkflowID:      "wf-1",
				WorkflowVersion: 1,
			}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", StepRef: "approval_step", Status: domain.StepCompleted},
			}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "approval_step", StepType: domain.WorkflowStepTypeApproval},
			}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := NewStepCallback(ms, nil, slog.Default())
	require.NoError(
		t, cb.ApproveStep(context.Background(),
			"wr-1",
			"approval_step",
			"admin@example.com",
		))
	require.True(t,
		triggerSynced,
	)
}

func TestApproveStep_NoEventTrigger_StillSucceeds(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{
		getStepRunByRunAndRefFn: func(_ context.Context, _ string, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID:            "sr-1",
				WorkflowRunID: "wr-1",
				StepRef:       "approval_step",
				Status:        domain.StepWaiting,
			}, nil
		},
		getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
			return &domain.WorkflowStepApproval{
				ID:            "approval:sr-1",
				WorkflowRunID: "wr-1",
				Approvers:     []string{"admin@example.com"},
				Status:        domain.ApprovalStatusPending,
			}, nil
		},
		updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		getEventTriggerByStepRunIDFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, nil // No event trigger
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:              "wr-1",
				Status:          domain.WfStatusRunning,
				WorkflowID:      "wf-1",
				WorkflowVersion: 1,
			}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", StepRef: "approval_step", Status: domain.StepCompleted},
			}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "approval_step", StepType: domain.WorkflowStepTypeApproval},
			}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := NewStepCallback(ms, nil, slog.Default())
	err := cb.ApproveStep(context.Background(), "wr-1", "approval_step", "admin@example.com")
	require.NoError(
		t, err)
}

func TestStartStep_Sleep_CreatesTrigger(t *testing.T) {
	t.Parallel()

	var captured *domain.EventTrigger

	ms := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		createEventTriggerFn: func(_ context.Context, trigger *domain.EventTrigger) error {
			captured = trigger
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, nil, slog.Default())

	step := &domain.WorkflowStep{
		StepRef:           "sleep-step",
		StepType:          domain.WorkflowStepTypeSleep,
		SleepDurationSecs: 300,
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-sleep-1", StepRef: "sleep-step"}
	wfRun := &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1"}

	err := engine.startStep(context.Background(), stepRun, step, wfRun, nil)
	require.NoError(
		t, err)
	require.NotNil(t,
		captured)
	require.Equal(t,
		domain.TriggerTypeSleep,
		captured.
			TriggerType,
	)
	require.Equal(t, 300, captured.
		TimeoutSecs)
	require.Equal(t,
		domain.StepWaiting,
		stepRun.
			Status)
}

func TestStartStep_Sleep_RejectsDurationAboveCap(t *testing.T) {
	t.Parallel()

	ms := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			require.Fail(t,

				"oversized sleep step must not update step status")
			return nil
		},
		createEventTriggerFn: func(_ context.Context, _ *domain.EventTrigger) error {
			require.Fail(t,

				"oversized sleep step must not create an event trigger")
			return nil
		},
	}
	engine := NewWorkflowEngine(ms, nil, slog.Default())

	step := &domain.WorkflowStep{
		StepRef:           "sleep-too-long",
		StepType:          domain.WorkflowStepTypeSleep,
		SleepDurationSecs: domain.MaxSleepDurationSecs + 1,
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-sleep-too-long", StepRef: "sleep-too-long"}
	wfRun := &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1"}

	err := engine.startStep(context.Background(), stepRun, step, wfRun, nil)
	require.Error(t,
		err)
	require.Contains(t,
		err.Error(), "exceeds maximum")
}

// Scheduling semantics regression tests.

func TestEffectiveResourceClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"", "small"},
		{"small", "small"},
		{"medium", "medium"},
		{"large", "large"},
	}
	for _, tt := range tests {
		got := effectiveResourceClass(tt.input)
		assert.Equal(t,
			tt.want, got,
		)
	}
}

func TestHasResourceClassCapacity(t *testing.T) {
	t.Parallel()

	t.Run("empty running map allows all classes", func(t *testing.T) {
		t.Parallel()
		running := map[string]int{}
		for _, class := range []string{"small", "medium", "large", ""} {
			assert.True(t, hasResourceClassCapacity(running,
				class,
			))
		}
	})

	t.Run("small limit 50", func(t *testing.T) {
		t.Parallel()
		running := map[string]int{"small": 49}
		assert.True(t, hasResourceClassCapacity(running,
			"small",
		))

		running["small"] = 50
		assert.False(t,
			hasResourceClassCapacity(running,
				"small",
			))
	})

	t.Run("medium limit 20", func(t *testing.T) {
		t.Parallel()
		running := map[string]int{"medium": 19}
		assert.True(t, hasResourceClassCapacity(running,
			"medium",
		))

		running["medium"] = 20
		assert.False(t,
			hasResourceClassCapacity(running,
				"medium",
			))
	})

	t.Run("large limit 5", func(t *testing.T) {
		t.Parallel()
		running := map[string]int{"large": 4}
		assert.True(t, hasResourceClassCapacity(running,
			"large",
		))

		running["large"] = 5
		assert.False(t,
			hasResourceClassCapacity(running,
				"large",
			))
	})

	t.Run("unknown class falls back to small limit", func(t *testing.T) {
		t.Parallel()
		running := map[string]int{"small": 50}
		assert.False(t,
			hasResourceClassCapacity(running,
				"unknown",
			))
	})

	t.Run("classes are independent", func(t *testing.T) {
		t.Parallel()
		running := map[string]int{"small": 50, "medium": 0, "large": 0}
		assert.False(t,
			hasResourceClassCapacity(running,
				"small",
			))
		assert.True(t, hasResourceClassCapacity(running,
			"medium",
		))
		assert.True(t, hasResourceClassCapacity(running,
			"large",
		))
	})
}

func BenchmarkHasResourceClassCapacity(b *testing.B) {
	running := map[string]int{
		"small":  12,
		"medium": 8,
		"large":  2,
	}

	b.Run("empty_class", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if !hasResourceClassCapacity(running, "") {
				b.Fatal("expected capacity")
			}
		}
	})
	b.Run("known_class", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if !hasResourceClassCapacity(running, "medium") {
				b.Fatal("expected capacity")
			}
		}
	})
	b.Run("unknown_class", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if !hasResourceClassCapacity(running, "gpu") {
				b.Fatal("expected fallback capacity")
			}
		}
	})
}

func TestScheduleRunnableSteps_ConcurrencyKeySerialization(t *testing.T) {
	t.Parallel()

	// Two steps share the same concurrency_key. Only one should start.
	ms := &mockCallbackStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
			return nil, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return nil, nil
		},
	}

	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return nil
		},
	}
	meSt := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		createWorkflowStepRunFn: func(_ context.Context, _ *domain.WorkflowStepRun) error { return nil },
	}

	engine := NewWorkflowEngine(meSt, mq, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	wfRun := &domain.WorkflowRun{
		ID:         "wr-1",
		WorkflowID: "wf-1",
		ProjectID:  "proj-1",
	}

	steps := []domain.WorkflowStep{
		{StepRef: "a", ConcurrencyKey: "deploy", JobID: "job-a"},
		{StepRef: "b", ConcurrencyKey: "deploy", JobID: "job-b"},
	}

	runnableStepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", WorkflowRunID: "wr-1", DepsRequired: 0, DepsCompleted: 0, Status: domain.StepPending},
		{ID: "sr-b", StepRef: "b", WorkflowRunID: "wr-1", DepsRequired: 0, DepsCompleted: 0, Status: domain.StepPending},
	}

	statuses := map[string]domain.StepRunStatus{}

	err := cb.scheduleRunnableSteps(context.Background(), wfRun, steps, statuses, nil, runnableStepRuns)
	require.NoError(
		t, err)

	// Only one should have transitioned to running since they share a concurrency key.
	runningCount := 0
	for _, s := range statuses {
		if s == domain.StepRunning {
			runningCount++
		}
	}
	require.LessOrEqual(t, runningCount,
		1)
}

func TestScheduleRunnableSteps_MaxParallelSteps(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
			return nil, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return nil, nil
		},
	}

	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return nil
		},
	}
	meSt := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		createWorkflowStepRunFn: func(_ context.Context, _ *domain.WorkflowStepRun) error { return nil },
	}

	engine := NewWorkflowEngine(meSt, mq, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	wfRun := &domain.WorkflowRun{
		ID:               "wr-1",
		WorkflowID:       "wf-1",
		ProjectID:        "proj-1",
		MaxParallelSteps: 1,
	}

	steps := []domain.WorkflowStep{
		{StepRef: "a", JobID: "job-a"},
		{StepRef: "b", JobID: "job-b"},
		{StepRef: "c", JobID: "job-c"},
	}

	runnableStepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", WorkflowRunID: "wr-1", DepsRequired: 0, DepsCompleted: 0, Status: domain.StepPending},
		{ID: "sr-b", StepRef: "b", WorkflowRunID: "wr-1", DepsRequired: 0, DepsCompleted: 0, Status: domain.StepPending},
		{ID: "sr-c", StepRef: "c", WorkflowRunID: "wr-1", DepsRequired: 0, DepsCompleted: 0, Status: domain.StepPending},
	}

	statuses := map[string]domain.StepRunStatus{}

	err := cb.scheduleRunnableSteps(context.Background(), wfRun, steps, statuses, nil, runnableStepRuns)
	require.NoError(
		t, err)

	runningCount := 0
	for _, s := range statuses {
		if s == domain.StepRunning {
			runningCount++
		}
	}
	require.LessOrEqual(t, runningCount,
		1)
}

func TestEnqueueApprovalNotification_CreatesDeliveries(t *testing.T) {
	t.Parallel()
	var deliveries []*domain.NotificationDelivery
	ms := &mockEngineStore{
		listEnabledNotificationChannelsFn: func(_ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{
				{ID: "ch-1", ProjectID: "proj-1"},
				{ID: "ch-2", ProjectID: "proj-1"},
			}, nil
		},
		createNotificationDeliveryFn: func(d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
	engine.enqueueApprovalNotification(context.Background(), "proj-1", domain.NotificationEventApprovalCompleted, map[string]any{
		"approval_id": "appr-1",
	})
	require.Len(t, deliveries,

		2)

	for _, d := range deliveries {
		assert.Equal(t,
			domain.NotificationEventApprovalCompleted,

			d.
				EventType)
		assert.Equal(t,
			"proj-1", d.
				ProjectID)
	}
}

func TestEnqueueApprovalNotification_NoChannels(t *testing.T) {
	t.Parallel()
	deliveryCalled := false
	ms := &mockEngineStore{
		listEnabledNotificationChannelsFn: func(_ string) ([]domain.NotificationChannel, error) {
			return nil, nil
		},
		createNotificationDeliveryFn: func(_ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
	engine.enqueueApprovalNotification(context.Background(), "proj-1", domain.NotificationEventApprovalExpired, map[string]any{})
	require.False(t,
		deliveryCalled,
	)
}

func TestEnqueueApprovalNotification_StoreError(t *testing.T) {
	t.Parallel()
	deliveryCalled := false
	ms := &mockEngineStore{
		listEnabledNotificationChannelsFn: func(_ string) ([]domain.NotificationChannel, error) {
			return nil, errors.New("db down")
		},
		createNotificationDeliveryFn: func(_ *domain.NotificationDelivery) error {
			deliveryCalled = true
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
	engine.enqueueApprovalNotification(context.Background(), "proj-1", domain.NotificationEventApprovalExpired, map[string]any{})
	require.False(t,
		deliveryCalled,
	)
}

// 4f. Workflow engine trace capture.

// traceTestSetup installs an in-memory TracerProvider into the global OTel
// state and returns a cleanup function that restores the previous provider.
// Tests that mutate global OTel state must not run in parallel.
func traceTestSetup() (cleanup func()) {
	prev := otel.GetTracerProvider()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)
	return func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prev)
	}
}

// traceTestStore returns a minimal mockEngineStore that satisfies TriggerWorkflow.
// capturedRun receives the WorkflowRun created during bootstrap.
func traceTestStore(capturedRun **domain.WorkflowRun) *mockEngineStore {
	return &mockEngineStore{
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{
				ID:        id,
				ProjectID: "proj-1",
				Enabled:   true,
				Version:   1,
			}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{ID: "step-1", JobID: "job-1", StepRef: "s1", StepType: domain.WorkflowStepTypeJob},
			}, nil
		},
		createWorkflowRunBootstrapFn: func(_ context.Context, run *domain.WorkflowRun, _ []domain.WorkflowStepRun, _ time.Time) error {
			if capturedRun != nil {
				*capturedRun = run
			}
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}
}

func TestTriggerWorkflow_CapturesTraceContext(t *testing.T) {
	cleanup := traceTestSetup()
	defer cleanup()

	var capturedRun *domain.WorkflowRun
	ms := traceTestStore(&capturedRun)
	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, mq, slog.Default())

	ctx := context.Background()
	ctx, span := otel.Tracer("test").Start(ctx, "test-trigger")
	inputTraceID := span.SpanContext().TraceID().String()
	defer span.End()

	_, err := engine.TriggerWorkflow(ctx, "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	require.NotNil(t,
		capturedRun,
	)

	tp, ok := capturedRun.TraceContext["traceparent"]
	require.True(t,
		ok)
	require.Contains(t,
		tp, inputTraceID)
}

func TestTriggerWorkflow_CapturesTraceState(t *testing.T) {
	cleanup := traceTestSetup()
	defer cleanup()

	var capturedRun *domain.WorkflowRun
	ms := traceTestStore(&capturedRun)
	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, mq, slog.Default())

	// Build a remote span context with a tracestate entry.
	traceID := otelTrace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	spanID := otelTrace.SpanID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	ts, _ := otelTrace.ParseTraceState("vendor=opaque")
	sc := otelTrace.NewSpanContext(otelTrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: otelTrace.FlagsSampled,
		TraceState: ts,
		Remote:     true,
	})
	ctx := otelTrace.ContextWithRemoteSpanContext(context.Background(), sc)

	_, err := engine.TriggerWorkflow(ctx, "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	require.NotNil(t,
		capturedRun,
	)

	tsVal, ok := capturedRun.TraceContext["tracestate"]
	require.True(t,
		ok)
	require.Equal(t,
		"vendor=opaque",
		tsVal)
}

func TestTriggerWorkflow_NoActiveSpan(t *testing.T) {
	// Use a no-op tracer provider so no valid spans are created.
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(noopTrace.NewTracerProvider())
	defer otel.SetTracerProvider(prev)

	var capturedRun *domain.WorkflowRun
	ms := traceTestStore(&capturedRun)
	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, mq, slog.Default())

	_, err := engine.TriggerWorkflow(context.Background(), "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	require.NotNil(t,
		capturedRun,
	)
	require.Nil(t, capturedRun.
		TraceContext,
	)
}

func TestTriggerWorkflow_TraceparentFormat(t *testing.T) {
	cleanup := traceTestSetup()
	defer cleanup()

	var capturedRun *domain.WorkflowRun
	ms := traceTestStore(&capturedRun)
	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, mq, slog.Default())

	ctx := context.Background()
	ctx, span := otel.Tracer("test").Start(ctx, "format-test")
	defer span.End()

	_, err := engine.TriggerWorkflow(ctx, "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	require.NotNil(t,
		capturedRun,
	)

	tp := capturedRun.TraceContext["traceparent"]
	pattern := `^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`
	matched, _ := regexp.MatchString(pattern, tp)
	require.True(t,
		matched)
}

func TestTriggerWorkflow_TraceStateTruncation(t *testing.T) {
	cleanup := traceTestSetup()
	defer cleanup()

	var capturedRun *domain.WorkflowRun
	ms := traceTestStore(&capturedRun)
	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, mq, slog.Default())

	// Build a tracestate that exceeds 512 characters by combining multiple
	// members (W3C limits individual member values to 256 chars).
	// Each "kN=<value>," entry is key(2) + "=" + value(250) + "," = 253 chars.
	// Three entries: 253*3 - 1 (no trailing comma) = 758 > 512.
	parts := make([]string, 3)
	for i := range parts {
		parts[i] = fmt.Sprintf("k%d=%s", i, strings.Repeat("a", 250))
	}
	tsStr := strings.Join(parts, ",")
	ts, tsErr := otelTrace.ParseTraceState(tsStr)
	require.NoError(t, tsErr)
	require.Greater(t,
		len(ts.String()), 512)

	traceID := otelTrace.TraceID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}
	spanID := otelTrace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	sc := otelTrace.NewSpanContext(otelTrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: otelTrace.FlagsSampled,
		TraceState: ts,
		Remote:     true,
	})
	ctx := otelTrace.ContextWithRemoteSpanContext(context.Background(), sc)

	_, err := engine.TriggerWorkflow(ctx, "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	require.NotNil(t,
		capturedRun,
	)

	if _, ok := capturedRun.TraceContext["tracestate"]; ok {
		require.Failf(t, "test failure",

			"tracestate should be omitted when length exceeds 512, got length %d", len(capturedRun.TraceContext["tracestate"]))
	}
}

func TestTriggerWorkflow_TraceStateExactly512(t *testing.T) {
	cleanup := traceTestSetup()
	defer cleanup()

	var capturedRun *domain.WorkflowRun
	ms := traceTestStore(&capturedRun)
	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, mq, slog.Default())

	// Build a tracestate that is exactly 512 bytes when serialised.
	// Use two members: "aa=<v1>,bb=<v2>" -- "aa=" (3) + v1(250) + "," (1) + "bb=" (3) + v2(255) = 512.
	tsStr := fmt.Sprintf("aa=%s,bb=%s", strings.Repeat("x", 250), strings.Repeat("y", 255))
	ts, tsErr := otelTrace.ParseTraceState(tsStr)
	require.NoError(t, tsErr)
	require.Len(t, ts.
		String(),
		512)

	traceID := otelTrace.TraceID{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30}
	spanID := otelTrace.SpanID{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28}
	sc := otelTrace.NewSpanContext(otelTrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: otelTrace.FlagsSampled,
		TraceState: ts,
		Remote:     true,
	})
	ctx := otelTrace.ContextWithRemoteSpanContext(context.Background(), sc)

	_, err := engine.TriggerWorkflow(ctx, "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	require.NotNil(t,
		capturedRun,
	)

	tsVal, ok := capturedRun.TraceContext["tracestate"]
	require.True(t,
		ok)
	require.Len(t, tsVal,
		512)
}

// 4g. Workflow step trace propagation.

func TestStartStep_PropagatesTraceContext(t *testing.T) {
	cleanup := traceTestSetup()
	defer cleanup()

	var capturedRun *domain.WorkflowRun
	var capturedJobRun *domain.JobRun
	ms := traceTestStore(&capturedRun)
	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedJobRun = run
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, mq, slog.Default())

	// Inject a remote span context with both traceparent and tracestate.
	traceID := otelTrace.TraceID{0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x40}
	spanID := otelTrace.SpanID{0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38}
	ts, _ := otelTrace.ParseTraceState("vendor=test123")
	sc := otelTrace.NewSpanContext(otelTrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: otelTrace.FlagsSampled,
		TraceState: ts,
		Remote:     true,
	})
	ctx := otelTrace.ContextWithRemoteSpanContext(context.Background(), sc)

	wfRun, err := engine.TriggerWorkflow(ctx, "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	require.NotNil(t,
		capturedJobRun,
	)
	require.NotNil(t,
		capturedJobRun.
			Metadata)

	tp, ok := capturedJobRun.Metadata["_trace_parent"]
	require.True(t,
		ok)

	wfTP := wfRun.TraceContext["traceparent"]
	require.Equal(t,
		wfTP, tp)

	tsVal, ok := capturedJobRun.Metadata["_trace_state"]
	require.True(t,
		ok)

	wfTS := wfRun.TraceContext["tracestate"]
	require.Equal(t,
		wfTS, tsVal,
	)
}

func TestStartStep_NoTraceContext(t *testing.T) {
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(noopTrace.NewTracerProvider())
	defer otel.SetTracerProvider(prev)

	var capturedJobRun *domain.JobRun
	ms := traceTestStore(nil)
	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedJobRun = run
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, mq, slog.Default())

	_, err := engine.TriggerWorkflow(context.Background(), "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	require.NotNil(t,
		capturedJobRun,
	)

	if _, ok := capturedJobRun.Metadata["_trace_parent"]; ok {
		require.Fail(t,

			"_trace_parent should not be present when no trace context exists")
	}
	if _, ok := capturedJobRun.Metadata["_trace_state"]; ok {
		require.Fail(t,

			"_trace_state should not be present when no trace context exists")
	}
}

func TestStartStep_OnlyTraceparent(t *testing.T) {
	cleanup := traceTestSetup()
	defer cleanup()

	var capturedJobRun *domain.JobRun
	ms := traceTestStore(nil)
	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedJobRun = run
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, mq, slog.Default())

	// Remote span context without tracestate.
	traceID := otelTrace.TraceID{0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48, 0x49, 0x4a, 0x4b, 0x4c, 0x4d, 0x4e, 0x4f, 0x50}
	spanID := otelTrace.SpanID{0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48}
	sc := otelTrace.NewSpanContext(otelTrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: otelTrace.FlagsSampled,
		Remote:     true,
	})
	ctx := otelTrace.ContextWithRemoteSpanContext(context.Background(), sc)

	_, err := engine.TriggerWorkflow(ctx, "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	require.NotNil(t,
		capturedJobRun,
	)

	if _, ok := capturedJobRun.Metadata["_trace_parent"]; !ok {
		require.Fail(t,

			"_trace_parent should be present")
	}
	if _, ok := capturedJobRun.Metadata["_trace_state"]; ok {
		require.Fail(t,

			"_trace_state should not be present when tracestate is empty")
	}
}

func TestStartStep_MultipleSteps_SameTraceID(t *testing.T) {
	cleanup := traceTestSetup()
	defer cleanup()

	var mu sync.Mutex
	var capturedJobRuns []*domain.JobRun
	ms := &mockEngineStore{
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{
				ID:        id,
				ProjectID: "proj-1",
				Enabled:   true,
				Version:   1,
			}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{ID: "step-1", JobID: "job-1", StepRef: "s1", StepType: domain.WorkflowStepTypeJob},
				{ID: "step-2", JobID: "job-2", StepRef: "s2", StepType: domain.WorkflowStepTypeJob},
				{ID: "step-3", JobID: "job-3", StepRef: "s3", StepType: domain.WorkflowStepTypeJob},
			}, nil
		},
		createWorkflowRunBootstrapFn: func(_ context.Context, _ *domain.WorkflowRun, _ []domain.WorkflowStepRun, _ time.Time) error {
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}
	mq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			mu.Lock()
			capturedJobRuns = append(capturedJobRuns, run)
			mu.Unlock()
			return nil
		},
	}

	engine := NewWorkflowEngine(ms, mq, slog.Default())

	ctx := context.Background()
	ctx, span := otel.Tracer("test").Start(ctx, "multi-step")
	defer span.End()

	_, err := engine.TriggerWorkflow(ctx, "wf-1", "proj-1", nil, "manual", nil, nil)
	require.NoError(
		t, err)
	require.Len(t, capturedJobRuns,

		3)

	firstTP := capturedJobRuns[0].Metadata["_trace_parent"]
	require.NotEmpty(t, firstTP)

	for _, jr := range capturedJobRuns[1:] {
		tp := jr.Metadata["_trace_parent"]
		require.Equal(t,
			firstTP, tp,
		)
	}
}
