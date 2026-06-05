//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestListRunningStepRunsByWorkflowRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-running-step-runs", domain.StepRunning)

	// Create a second step run in pending status.
	pendingSR := testutil.BuildWorkflowStepRun(wfRun.ID, stepRun.WorkflowStepID, &testutil.WorkflowStepRunOpts{
		Status:  testutil.Ptr(domain.StepPending),
		StepRef: new("pending-step"),
	})
	require.NoError(t, q.CreateWorkflowStepRun(ctx,
		pendingSR))

	running, err := q.ListRunningStepRunsByWorkflowRun(ctx, wfRun.ID, 100)
	require.NoError(t, err)
	require.Len(t, running,

		1)
	require.Equal(t, stepRun.
		ID, running[0].ID)

	// Empty case.
	empty, err := q.ListRunningStepRunsByWorkflowRun(ctx, newID(), 100)
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestListStepRunStatusesByWorkflowRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-step-run-statuses", domain.StepRunning)

	statuses, err := q.ListStepRunStatusesByWorkflowRun(ctx, wfRun.ID)
	require.NoError(t, err)
	require.Len(t, statuses,

		1)
	require.Equal(t, domain.
		StepRunning,
		statuses[stepRun.StepRef])

	// Empty case.
	empty, err := q.ListStepRunStatusesByWorkflowRun(ctx, newID())
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestListStepRunsByWorkflowRun_CursorMovesForward(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	wf, wfRun, first := mustCreateWorkflowStepFixture(t, ctx, q, "project-step-run-cursor-"+newID(), domain.StepPending)
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(wf.ProjectID)})
	secondStep := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   new(stepJob.ID),
		StepRef: new("cursor-second-" + newID()),
	})
	second := testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, secondStep.ID, &testutil.WorkflowStepRunOpts{
		Status:  new(domain.StepPending),
		StepRef: new(secondStep.StepRef),
	})
	thirdStep := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   new(stepJob.ID),
		StepRef: new("cursor-third-" + newID()),
	})
	third := testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, thirdStep.ID, &testutil.WorkflowStepRunOpts{
		Status:  new(domain.StepPending),
		StepRef: new(thirdStep.StepRef),
	})

	baseTime := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	for i, stepRunID := range []string{first.ID, second.ID, third.ID} {
		if _, err := testDB.Pool.Exec(ctx, `
			UPDATE workflow_step_runs
			SET created_at = $2
			WHERE id = $1
		`, stepRunID, baseTime.Add(time.Duration(i)*time.Minute)); err != nil {
			require.Failf(t, "test failure",

				"set step run created_at(%d): %v", i, err)
		}
	}

	page1, err := q.ListStepRunsByWorkflowRun(ctx, wfRun.ID, 2, nil)
	require.NoError(t, err)
	require.False(t, len(page1) != 2 ||
		page1[0].
			ID != first.ID ||
		page1[1].
			ID != second.
			ID)

	cursor := page1[1].CreatedAt
	page2, err := q.ListStepRunsByWorkflowRun(ctx, wfRun.ID, 2, &cursor)
	require.NoError(t, err)
	require.False(t, len(page2) != 1 ||
		page2[0].
			ID != third.ID)

}

func stepRunIDs(stepRuns []domain.WorkflowStepRun) []string {
	ids := make([]string, 0, len(stepRuns))
	for _, stepRun := range stepRuns {
		ids = append(ids, stepRun.ID)
	}
	return ids
}

func TestUpdateStepRunStatusFrom(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, _, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-step-run-status-from", domain.StepPending)
	require.NoError(t, q.UpdateStepRunStatusFrom(ctx, stepRun.ID,
		domain.StepPending,

		domain.StepRunning,

		map[string]any{"started_at": time.Now().UTC().Truncate(time.Microsecond)}))

	// Transition from pending to running.

	got, err := q.GetWorkflowStepRun(ctx, stepRun.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StepRunning,
		got.Status,
	)
	require.NotNil(t, got.StartedAt)

	var xminBeforeNoOp string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM workflow_step_runs
		WHERE id = $1`,

		stepRun.ID).Scan(
		&xminBeforeNoOp))
	require.NoError(t, q.UpdateStepRunStatusFrom(ctx, stepRun.ID,
		domain.StepRunning,

		domain.StepRunning,

		map[string]any{"started_at": *got.StartedAt}))

	var xminAfterNoOp string
	var statusAfterNoOp domain.StepRunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, status
		FROM workflow_step_runs
		WHERE id = $1`,

		stepRun.ID,
	).Scan(&xminAfterNoOp, &statusAfterNoOp))
	require.Equal(t, xminBeforeNoOp,

		xminAfterNoOp,
	)
	require.Equal(t, domain.
		StepRunning,
		statusAfterNoOp,
	)
	require.NoError(t, q.UpdateStepRunStatusFrom(ctx, stepRun.ID,
		domain.StepRunning,

		domain.StepRunning,

		map[string]any{"attempt": 2}))

	var xminAfterChange string
	var attemptAfterChange int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, attempt
		FROM workflow_step_runs
		WHERE id = $1`,

		stepRun.
			ID).Scan(&xminAfterChange,
		&attemptAfterChange))
	require.NotEqual(t, xminAfterNoOp,

		xminAfterChange,
	)
	require.EqualValues(t, 2, attemptAfterChange)

	// Conflict: try from pending again (already running).
	err = q.UpdateStepRunStatusFrom(ctx, stepRun.ID, domain.StepPending, domain.StepCompleted, nil)
	require.Error(t, err)

	// Invalid field.
	err = q.UpdateStepRunStatusFrom(ctx, stepRun.ID, domain.StepRunning, domain.StepCompleted, map[string]any{
		"bad_field": "x",
	})
	require.Error(t, err)

}

func TestCountNonTerminalStepRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, _ := mustCreateWorkflowStepFixture(t, ctx, q, "project-count-non-terminal", domain.StepPending)

	count, err := q.CountNonTerminalStepRuns(ctx, wfRun.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	// Empty case.
	zeroCount, err := q.CountNonTerminalStepRuns(ctx, newID())
	require.NoError(t, err)
	require.EqualValues(t, 0, zeroCount)

}

func TestListFailedStepRunRefs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-failed-step-refs", domain.StepFailed)

	refs, err := q.ListFailedStepRunRefs(ctx, wfRun.ID)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.Equal(t, stepRun.
		StepRef,
		refs[0])

	// Empty case.
	empty, err := q.ListFailedStepRunRefs(ctx, newID())
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestCancelNonTerminalStepRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, _ := mustCreateWorkflowStepFixture(t, ctx, q, "project-cancel-step-runs", domain.StepPending)

	now := time.Now().UTC()
	affected, err := q.CancelNonTerminalStepRuns(ctx, wfRun.ID, now, "workflow canceled")
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)

	// Calling again should affect 0.
	affected2, err := q.CancelNonTerminalStepRuns(ctx, wfRun.ID, now, "workflow canceled")
	require.NoError(t, err)
	require.EqualValues(t, 0, affected2)

}

func TestSkipStepRunsByRefs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-skip-step-runs", domain.StepPending)

	now := time.Now().UTC()
	affected, err := q.SkipStepRunsByRefs(ctx, wfRun.ID, []string{stepRun.StepRef}, now)
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)

	got, err := q.GetWorkflowStepRun(ctx, stepRun.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StepSkipped,
		got.Status,
	)

	// Empty refs returns 0.
	zero, err := q.SkipStepRunsByRefs(ctx, wfRun.ID, []string{}, now)
	require.NoError(t, err)
	require.EqualValues(t, 0, zero)

}

func TestGetStepOutputs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-step-outputs", domain.StepCompleted)
	require.NoError(t, q.UpdateStepRunStatus(ctx,
		stepRun.ID, domain.
			StepCompleted,
		map[string]any{"output": json.RawMessage(`{"result":"ok"}`)}))

	// Set output on the step run.

	outputs, err := q.GetStepOutputs(ctx, wfRun.ID, []string{stepRun.StepRef})
	require.NoError(t, err)
	require.Len(t, outputs,

		1)

	outStr := string(outputs[stepRun.StepRef])
	require.False(t, !strings.Contains(outStr, `"result"`) || !strings.Contains(outStr,
		`"ok"`))

	// Unknown step ref.
	empty, err := q.GetStepOutputs(ctx, wfRun.ID, []string{"nonexistent"})
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestListRunnableStepRunsByWorkflowRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-runnable-step-runs"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: new(stepJob.ID), StepRef: new("runnable-step")})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})

	// Create a step run with deps_completed == deps_required (runnable).
	sr := &domain.WorkflowStepRun{
		WorkflowRunID:  wfRun.ID,
		WorkflowStepID: step.ID,
		StepRef:        step.StepRef,
		Status:         domain.StepPending,
		DepsCompleted:  1,
		DepsRequired:   1,
	}
	require.NoError(t, q.CreateWorkflowStepRun(ctx,
		sr))

	runnable, err := q.ListRunnableStepRunsByWorkflowRun(ctx, wfRun.ID, 100)
	require.NoError(t, err)
	require.Len(t, runnable,

		1)
	require.Equal(t, sr.ID,

		runnable[0].ID)

	// Empty case.
	empty, err := q.ListRunnableStepRunsByWorkflowRun(ctx, newID(), 100)
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestGetCostGateDefaultAction(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Empty result for a nonexistent step run.
	action, err := q.GetCostGateDefaultAction(ctx, newID())
	require.NoError(t, err)
	require.Equal(t, "", action)

}

func TestGetCostGateDefaultAction_UsesVersionSnapshot(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-cost-gate-snapshot-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("workflow-" + newID()),
		Slug:      new("workflow-slug-" + newID()),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	stepRef := "cost-gate-" + newID()
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   new(stepJob.ID),
		StepRef: new(stepRef),
	})
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE workflow_steps
		SET cost_gate_default_action = 'reject'
		WHERE id = $1
	`, step.ID); err != nil {
		require.Failf(t, "test failure",

			"set snapshot cost gate action: %v", err)
	}
	require.NoError(t, q.CreateWorkflowVersionSnapshot(ctx, wf.ID,
		1))

	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	stepRun := testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		Status:  new(domain.StepWaiting),
		StepRef: new(stepRef),
	})
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE workflow_steps
		SET cost_gate_default_action = 'approve'
		WHERE id = $1
	`, step.ID); err != nil {
		require.Failf(t, "test failure",

			"mutate live cost gate action: %v", err)
	}

	action, err := q.GetCostGateDefaultAction(ctx, stepRun.ID)
	require.NoError(t, err)
	require.Equal(t, "reject",

		action,
	)

}

func TestGetCostGateDefaultAction_UsesWorkflowRunVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-cost-gate-version-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("workflow-" + newID()),
		Slug:      new("workflow-slug-" + newID()),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	stepRef := "cost-gate-version-" + newID()
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   new(stepJob.ID),
		StepRef: new(stepRef),
	})
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE workflow_steps
		SET cost_gate_default_action = 'reject'
		WHERE id = $1
	`, step.ID); err != nil {
		require.Failf(t, "test failure",

			"set v1 cost gate action: %v", err)
	}
	require.NoError(t, q.CreateWorkflowVersionSnapshot(ctx, wf.ID,
		1))

	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflows SET version = 2 WHERE id = $1`, wf.ID); err != nil {
		require.Failf(t, "test failure",

			"set workflow version 2: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE workflow_steps
		SET cost_gate_default_action = 'approve'
		WHERE id = $1
	`, step.ID); err != nil {
		require.Failf(t, "test failure",

			"set v2 cost gate action: %v", err)
	}
	require.NoError(t, q.CreateWorkflowVersionSnapshot(ctx, wf.ID,
		2))

	v1Run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})
	v1Run.WorkflowVersion = 1
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_runs SET workflow_version = 1 WHERE id = $1`, v1Run.ID); err != nil {
		require.Failf(t, "test failure",

			"pin workflow run to v1: %v", err)
	}
	v1StepRun := testutil.MustCreateWorkflowStepRun(t, ctx, q, v1Run.ID, step.ID, &testutil.WorkflowStepRunOpts{
		Status:  new(domain.StepWaiting),
		StepRef: new(stepRef),
	})

	v2Run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})
	v2Run.WorkflowVersion = 2
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_runs SET workflow_version = 2 WHERE id = $1`, v2Run.ID); err != nil {
		require.Failf(t, "test failure",

			"pin workflow run to v2: %v", err)
	}
	v2StepRun := testutil.MustCreateWorkflowStepRun(t, ctx, q, v2Run.ID, step.ID, &testutil.WorkflowStepRunOpts{
		Status:  new(domain.StepWaiting),
		StepRef: new(stepRef),
	})

	v1Action, err := q.GetCostGateDefaultAction(ctx, v1StepRun.ID)
	require.NoError(t, err)
	require.Equal(t, "reject",

		v1Action,
	)

	v2Action, err := q.GetCostGateDefaultAction(ctx, v2StepRun.ID)
	require.NoError(t, err)
	require.Equal(t, "approve",

		v2Action,
	)

}

func TestListOrphanedStepRuns_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orphans, err := q.ListOrphanedStepRuns(ctx)
	require.NoError(t, err)
	require.Len(t, orphans,

		0)

}

func TestGetWorkflowStepRun_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetWorkflowStepRun(ctx, newID())
	require.True(t, errors.Is(err, store.
		ErrWorkflowStepRunNotFound,
	))

}

func TestIncrementStepRunAttempt_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.IncrementStepRunAttempt(ctx, newID(), 2)
	require.Error(t, err)
	require.True(t, errors.Is(err, store.
		ErrWorkflowStepRunNotFound,
	))

}
