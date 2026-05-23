package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
)

// continueTestSteps returns a standard 2-step DAG (a -> b) used by the
// continue-as-new engine tests.
func continueTestSteps() []domain.WorkflowStep {
	return []domain.WorkflowStep{
		{ID: "step-a", JobID: "job-a", StepRef: "a"},
		{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
	}
}

func TestContinueWorkflowRunAsNew(t *testing.T) {
	t.Parallel()

	t.Run("happy path completes predecessor and starts successor", func(t *testing.T) {
		t.Parallel()

		var bootstrapPredID string
		var bootstrapFromStatus domain.WorkflowRunStatus
		var successorRun *domain.WorkflowRun
		var successorStepRuns []domain.WorkflowStepRun
		enqueued := make([]string, 0)

		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID:              id,
					WorkflowID:      "wf-1",
					ProjectID:       "proj-1",
					Status:          domain.WfStatusRunning,
					TriggeredBy:     domain.TriggerManual,
					WorkflowVersion: 1,
					LineageDepth:    0,
					Tags:            map[string]string{"run": "pred"},
				}, nil
			},
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{
					ID:               id,
					ProjectID:        "proj-1",
					Enabled:          true,
					Version:          1,
					VersionID:        "wf-v1",
					MaxParallelSteps: 4,
					Tags:             map[string]string{"team": "core"},
				}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return continueTestSteps(), nil
			},
			continueWorkflowRunBootstrapFn: func(_ context.Context, predecessorID string, fromStatus domain.WorkflowRunStatus, successor *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, now time.Time) error {
				bootstrapPredID = predecessorID
				bootstrapFromStatus = fromStatus
				successorRun = successor
				successorStepRuns = append(successorStepRuns[:0], stepRuns...)
				if now.IsZero() {
					t.Fatal("expected non-zero bootstrap time")
				}
				if successor.ID == "" {
					t.Fatal("successor ID must be set before bootstrap")
				}
				for _, sr := range stepRuns {
					if sr.WorkflowRunID != successor.ID {
						t.Fatalf("step run %s workflow_run_id = %q, want %q", sr.StepRef, sr.WorkflowRunID, successor.ID)
					}
				}
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = append(enqueued, run.JobID)
			run.ID = "jr-" + run.JobID
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		successor, err := engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", json.RawMessage(`{"cursor":42}`))
		if err != nil {
			t.Fatalf("ContinueWorkflowRunAsNew() error = %v", err)
		}

		if bootstrapPredID != "pred-run-1" {
			t.Fatalf("bootstrap predecessor = %q, want pred-run-1", bootstrapPredID)
		}
		if bootstrapFromStatus != domain.WfStatusRunning {
			t.Fatalf("bootstrap fromStatus = %q, want running", bootstrapFromStatus)
		}
		if successor == nil || successor != successorRun {
			t.Fatal("returned successor should be the bootstrapped run")
		}
		if successor.Status != domain.WfStatusRunning {
			t.Fatalf("successor status = %q, want running", successor.Status)
		}
		if successor.StartedAt == nil {
			t.Fatal("successor StartedAt should be set")
		}
		if successor.ContinuedFromWorkflowRunID != "pred-run-1" {
			t.Fatalf("successor ContinuedFrom = %q, want pred-run-1", successor.ContinuedFromWorkflowRunID)
		}
		if successor.LineageDepth != 1 {
			t.Fatalf("successor LineageDepth = %d, want 1", successor.LineageDepth)
		}
		if successor.TriggeredBy != domain.TriggerContinuation {
			t.Fatalf("successor TriggeredBy = %q, want continuation", successor.TriggeredBy)
		}
		if string(successor.Payload) != `{"cursor":42}` {
			t.Fatalf("successor Payload = %q, want carry-over input", string(successor.Payload))
		}
		// Tags: workflow tags overlaid by predecessor run tags.
		if successor.Tags["team"] != "core" || successor.Tags["run"] != "pred" {
			t.Fatalf("successor Tags = %v, want merged workflow+run tags", successor.Tags)
		}
		// Fresh, empty step history: only the DAG's step runs, all new.
		if len(successorStepRuns) != 2 {
			t.Fatalf("successor step runs = %d, want 2", len(successorStepRuns))
		}
		// Only the root job should be enqueued.
		if len(enqueued) != 1 || enqueued[0] != "job-a" {
			t.Fatalf("enqueued = %v, want [job-a]", enqueued)
		}
	})

	t.Run("re-resolves a newer published version via canary routing", func(t *testing.T) {
		t.Parallel()

		var listedVersion int
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID:                id,
					WorkflowID:        "wf-1",
					ProjectID:         "proj-1",
					Status:            domain.WfStatusRunning,
					WorkflowVersion:   1,
					WorkflowVersionID: "wf-v1",
				}, nil
			},
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1, VersionID: "wf-v1"}, nil
			},
			getActiveCanaryDeploymentFn: func(_ context.Context, _ string) (*domain.CanaryDeployment, error) {
				return &domain.CanaryDeployment{
					WorkflowID:    "wf-1",
					ProjectID:     "proj-1",
					SourceVersion: 1,
					TargetVersion: 2,
					TrafficPct:    100,
					Status:        "active",
				}, nil
			},
			getWorkflowVersionFn: func(_ context.Context, _ string, version int) (*domain.WorkflowVersion, error) {
				if version != 2 {
					t.Fatalf("GetWorkflowVersion version = %d, want 2", version)
				}
				return &domain.WorkflowVersion{
					WorkflowID:       "wf-1",
					ProjectID:        "proj-1",
					Version:          2,
					VersionID:        "wf-v2",
					Name:             "Workflow v2",
					Slug:             "workflow",
					Enabled:          true,
					MaxParallelSteps: 3,
				}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, version int) ([]domain.WorkflowStep, error) {
				listedVersion = version
				return []domain.WorkflowStep{{ID: "step-v2", JobID: "job-v2", StepRef: "root"}}, nil
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
		successor, err := engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", nil)
		if err != nil {
			t.Fatalf("ContinueWorkflowRunAsNew() error = %v", err)
		}
		if listedVersion != 2 {
			t.Fatalf("listedVersion = %d, want canary target version 2", listedVersion)
		}
		if successor.WorkflowVersion != 2 || successor.WorkflowVersionID != "wf-v2" {
			t.Fatalf("successor version = %d/%q, want 2/wf-v2", successor.WorkflowVersion, successor.WorkflowVersionID)
		}
	})

	t.Run("rejects continuation when depth cap is exceeded", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID:           id,
					WorkflowID:   "wf-1",
					ProjectID:    "proj-1",
					Status:       domain.WfStatusRunning,
					LineageDepth: 5,
				}, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default()).WithMaxContinueDepth(5)
		_, err := engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", nil)
		if err == nil || !strings.Contains(err.Error(), "exceeds max") {
			t.Fatalf("expected depth-cap error, got %v", err)
		}
	})

	t.Run("allows continuation exactly at the depth boundary", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID:           id,
					WorkflowID:   "wf-1",
					ProjectID:    "proj-1",
					Status:       domain.WfStatusRunning,
					LineageDepth: 4,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "s1", JobID: "job-1", StepRef: "a"}}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-" + run.JobID
			return nil
		}}
		engine := NewWorkflowEngine(ms, mq, slog.Default()).WithMaxContinueDepth(5)
		successor, err := engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", nil)
		if err != nil {
			t.Fatalf("ContinueWorkflowRunAsNew() error = %v", err)
		}
		if successor.LineageDepth != 5 {
			t.Fatalf("successor LineageDepth = %d, want 5", successor.LineageDepth)
		}
	})

	t.Run("rejects continuation of a terminal run", func(t *testing.T) {
		t.Parallel()
		for _, st := range []domain.WorkflowRunStatus{
			domain.WfStatusCompleted,
			domain.WfStatusFailed,
			domain.WfStatusCanceled,
			domain.WfStatusContinued,
		} {
			ms := &mockEngineStore{
				getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
					return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: st}, nil
				},
			}
			engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
			_, err := engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", nil)
			if err == nil || !strings.Contains(err.Error(), "must be running or paused") {
				t.Fatalf("status %s: expected non-terminal precondition error, got %v", st, err)
			}
		}
	})

	t.Run("continues a paused run", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusPaused}, nil
			},
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "s1", JobID: "job-1", StepRef: "a"}}, nil
			},
			continueWorkflowRunBootstrapFn: func(_ context.Context, _ string, fromStatus domain.WorkflowRunStatus, _ *domain.WorkflowRun, _ []domain.WorkflowStepRun, _ time.Time) error {
				if fromStatus != domain.WfStatusPaused {
					t.Fatalf("bootstrap fromStatus = %q, want paused", fromStatus)
				}
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error { run.ID = "jr"; return nil }}
		engine := NewWorkflowEngine(ms, mq, slog.Default())
		if _, err := engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", nil); err != nil {
			t.Fatalf("ContinueWorkflowRunAsNew() error = %v", err)
		}
	})

	t.Run("run not found", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.ContinueWorkflowRunAsNew(context.Background(), "missing", nil)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("rejects when workflow is disabled", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: false}, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", nil)
		if err == nil || !strings.Contains(err.Error(), "disabled") {
			t.Fatalf("expected disabled error, got %v", err)
		}
	})

	t.Run("rejects when project is not runnable", func(t *testing.T) {
		t.Parallel()
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			isProjectRunnableFn: func(_ context.Context, _ string) (bool, error) {
				return false, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", nil)
		if err == nil || !strings.Contains(err.Error(), "not active") {
			t.Fatalf("expected inactive project error, got %v", err)
		}
	})
}

// TestContinueWorkflowRunAsNew_BootstrapError verifies that an error from the
// atomic store handoff surfaces and that the successor is not started: the
// engine never reaches startRootSteps, so no job is enqueued. This is the
// engine-layer crash-mid-continue case (predecessor untouched by the engine).
func TestContinueWorkflowRunAsNew_BootstrapError(t *testing.T) {
	t.Parallel()
	enqueued := 0
	ms := &mockEngineStore{
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
		},
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "s1", JobID: "job-1", StepRef: "a"}}, nil
		},
		continueWorkflowRunBootstrapFn: func(_ context.Context, _ string, _ domain.WorkflowRunStatus, _ *domain.WorkflowRun, _ []domain.WorkflowStepRun, _ time.Time) error {
			return errors.New("tx rolled back: successor insert failed")
		},
	}
	mq := &mockEngineQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued++
		return nil
	}}
	engine := NewWorkflowEngine(ms, mq, slog.Default())
	_, err := engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", nil)
	if err == nil || !strings.Contains(err.Error(), "continue workflow run bootstrap") {
		t.Fatalf("expected bootstrap error, got %v", err)
	}
	if enqueued != 0 {
		t.Fatalf("enqueued = %d, want 0 (successor must not start when bootstrap fails)", enqueued)
	}
}

// TestContinueWorkflowRunAsNew_ExpiryAnchoredToStart verifies the successor's
// expires_at and started_at derive from a single wall-clock reading, so their
// difference is exactly the configured timeout. Two separate time.Now() calls
// would make expires_at - started_at drift below the timeout.
func TestContinueWorkflowRunAsNew_ExpiryAnchoredToStart(t *testing.T) {
	t.Parallel()
	const timeoutSecs = 3600
	ms := &mockEngineStore{
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning, WorkflowVersion: 1}, nil
		},
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1, TimeoutSecs: timeoutSecs}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return continueTestSteps(), nil
		},
		continueWorkflowRunBootstrapFn: func(_ context.Context, _ string, _ domain.WorkflowRunStatus, _ *domain.WorkflowRun, _ []domain.WorkflowStepRun, _ time.Time) error {
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
	successor, err := engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", nil)
	if err != nil {
		t.Fatalf("ContinueWorkflowRunAsNew() error = %v", err)
	}
	if successor.StartedAt == nil || successor.ExpiresAt == nil {
		t.Fatalf("StartedAt/ExpiresAt must be set: started=%v expires=%v", successor.StartedAt, successor.ExpiresAt)
	}
	if got := successor.ExpiresAt.Sub(*successor.StartedAt); got != timeoutSecs*time.Second {
		t.Fatalf("expires_at - started_at = %v, want exactly %v (single time.Now anchor)", got, time.Duration(timeoutSecs)*time.Second)
	}
}

// TestContinueWorkflowRunAsNew_StartRootStepsFailsAfterCommit verifies that when
// root-step start fails after the handoff has already committed, the engine logs
// the durable committed lineage (predecessor continued, successor running) before
// surfacing the error, so the partial failure is not mistaken for a no-op.
func TestContinueWorkflowRunAsNew_StartRootStepsFailsAfterCommit(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))

	var successorID string
	ms := &mockEngineStore{
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning, WorkflowVersion: 1}, nil
		},
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "s1", JobID: "job-1", StepRef: "a"}}, nil
		},
		continueWorkflowRunBootstrapFn: func(_ context.Context, _ string, _ domain.WorkflowRunStatus, successor *domain.WorkflowRun, _ []domain.WorkflowStepRun, _ time.Time) error {
			successorID = successor.ID
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}
	// The handoff commits, then enqueueing the root job fails.
	mq := &mockEngineQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		return errors.New("queue unavailable")
	}}
	engine := NewWorkflowEngine(ms, mq, logger)

	if _, err := engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", nil); err == nil {
		t.Fatal("expected error when root step start fails after commit")
	}

	logged := buf.String()
	if !strings.Contains(logged, "continue-as-new committed but root step start failed") {
		t.Fatalf("expected committed-but-failed error log, got: %s", logged)
	}
	if successorID == "" || !strings.Contains(logged, successorID) {
		t.Fatalf("log must reference committed successor %q, got: %s", successorID, logged)
	}
	if !strings.Contains(logged, "pred-run-1") {
		t.Fatalf("log must reference predecessor pred-run-1, got: %s", logged)
	}
}

// TestContinueWorkflowRunAsNew_ConcurrentSingleWinner verifies that when two
// continuations race the same predecessor, the store's guarded handoff lets at
// most one win; the loser surfaces the conflict and never starts a successor.
func TestContinueWorkflowRunAsNew_ConcurrentSingleWinner(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	consumed := false // simulates the predecessor's running->continued transition
	enqueued := 0

	ms := &mockEngineStore{
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
		},
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "s1", JobID: "job-1", StepRef: "a"}}, nil
		},
		continueWorkflowRunBootstrapFn: func(_ context.Context, _ string, _ domain.WorkflowRunStatus, _ *domain.WorkflowRun, _ []domain.WorkflowStepRun, _ time.Time) error {
			mu.Lock()
			defer mu.Unlock()
			if consumed {
				return errors.New("workflow run continue-as-new conflict: predecessor no longer in expected state")
			}
			consumed = true
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}
	mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		mu.Lock()
		enqueued++
		mu.Unlock()
		run.ID = "jr"
		return nil
	}}
	engine := NewWorkflowEngine(ms, mq, slog.Default())

	const racers = 8
	var wg sync.WaitGroup
	results := make([]error, racers)
	wg.Add(racers)
	for i := range racers {
		go func(idx int) {
			defer wg.Done()
			_, results[idx] = engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", nil)
		}(i)
	}
	wg.Wait()

	wins := 0
	for _, err := range results {
		if err == nil {
			wins++
		} else if !strings.Contains(err.Error(), "conflict") {
			t.Fatalf("loser error = %v, want conflict", err)
		}
	}
	if wins != 1 {
		t.Fatalf("winners = %d, want exactly 1", wins)
	}
	if enqueued != 1 {
		t.Fatalf("enqueued = %d, want exactly 1 (only the winner starts a successor)", enqueued)
	}
}

// TestBuildInitialStepRuns_RootsAndDeps locks the extracted helper's behavior,
// shared by the trigger and continue-as-new paths: roots are Pending with zero
// required deps, dependents are Waiting with their dep count, and every step run
// gets a unique ID bound to the run.
func TestBuildInitialStepRuns_RootsAndDeps(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{ID: "step-a", JobID: "job-a", StepRef: "a"},
		{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
		{ID: "step-c", JobID: "job-c", StepRef: "c", DependsOn: []string{"a", "b"}},
	}
	stepRuns := buildInitialStepRuns("wr-1", steps)
	if len(stepRuns) != 3 {
		t.Fatalf("step runs = %d, want 3", len(stepRuns))
	}
	byRef := make(map[string]domain.WorkflowStepRun, len(stepRuns))
	seenIDs := make(map[string]struct{}, len(stepRuns))
	for i, sr := range stepRuns {
		if sr.WorkflowRunID != "wr-1" {
			t.Fatalf("step run %s workflow_run_id = %q, want wr-1", sr.StepRef, sr.WorkflowRunID)
		}
		if sr.WorkflowStepID != steps[i].ID {
			t.Fatalf("step run %s workflow_step_id = %q, want %q", sr.StepRef, sr.WorkflowStepID, steps[i].ID)
		}
		if sr.ID == "" {
			t.Fatalf("step run %s has empty ID", sr.StepRef)
		}
		if _, dup := seenIDs[sr.ID]; dup {
			t.Fatalf("duplicate step run ID %q", sr.ID)
		}
		seenIDs[sr.ID] = struct{}{}
		byRef[sr.StepRef] = sr
	}
	if byRef["a"].Status != domain.StepPending || byRef["a"].DepsRequired != 0 {
		t.Fatalf("root a = %+v, want pending/0 deps", byRef["a"])
	}
	if byRef["b"].Status != domain.StepWaiting || byRef["b"].DepsRequired != 1 {
		t.Fatalf("step b = %+v, want waiting/1 dep", byRef["b"])
	}
	if byRef["c"].Status != domain.StepWaiting || byRef["c"].DepsRequired != 2 {
		t.Fatalf("step c = %+v, want waiting/2 deps", byRef["c"])
	}
	if got := countRootSteps(steps); got != 1 {
		t.Fatalf("countRootSteps = %d, want 1", got)
	}
}

// FuzzContinueWorkflowRunAsNew feeds arbitrary input payload bytes through the
// engine method: invalid JSON is tolerated as an opaque carry-over blob and the
// method must never panic.
func FuzzContinueWorkflowRunAsNew(f *testing.F) {
	f.Add([]byte(`{"cursor":1}`))
	f.Add([]byte(``))
	f.Add([]byte(`not json`))
	f.Add([]byte(`{"a":`))
	f.Add([]byte("\x00\x01\x02"))

	f.Fuzz(func(t *testing.T, input []byte) {
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "s1", JobID: "job-1", StepRef: "a"}}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error { run.ID = "jr"; return nil }}
		engine := NewWorkflowEngine(ms, mq, slog.Default())
		// Must not panic regardless of payload bytes.
		_, _ = engine.ContinueWorkflowRunAsNew(context.Background(), "pred-run-1", json.RawMessage(input))
	})
}
