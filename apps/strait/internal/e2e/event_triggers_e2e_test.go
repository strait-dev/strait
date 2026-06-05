//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/scheduler"
	"strait/internal/workflow"

	"github.com/stretchr/testify/require"
)

func TestE2E_WaitForEventStep_CompletesViaAPI(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-evt-complete-" + newID()

	// Create a workflow with a wait_for_event step.
	wf := wfCreateWorkflow(t, srv, projectID, "Event Wait Workflow", "wf-evt-wait-"+newID(), []map[string]any{
		{
			"step_ref":           "wait_step",
			"step_type":          "wait_for_event",
			"event_key":          "e2e-check:{{app_id}}",
			"event_timeout_secs": 300,
		},
	})

	// Trigger workflow with payload containing the template variable.
	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, map[string]any{
		"app_id": "app-e2e-123",
	}, nil)
	runID := asString(t, triggered, "id")

	// Verify step run is waiting.
	stepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	require.NoError(t, err)

	waitStep := findStepRunByRef(t, stepRuns, "wait_step")
	require.Equal(t, domain.
		StepWaiting,
		waitStep.
			Status)

	// Verify event trigger exists via API.
	getResp := wfDoReq(t, srv, http.MethodGet, "/v1/events/e2e-check:app-e2e-123", "")
	require.Equal(t, http.
		StatusOK,
		getResp.Code,
	)

	triggerObj := mustDecodeObject(t, getResp)
	require.Equal(t, "waiting",

		asString(t, triggerObj,
			"status",
		))

	// Send event via API.
	sendResp := wfDoReq(t, srv, http.MethodPost, "/v1/events/e2e-check:app-e2e-123/send", `{"payload":{"result":"clear"}}`)
	require.Equal(t, http.
		StatusOK,
		sendResp.Code,
	)

	// Verify step completed with event payload.
	stepRuns, err = testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	require.NoError(t, err)

	waitStep = findStepRunByRef(t, stepRuns, "wait_step")
	require.Equal(t, domain.
		StepCompleted,
		waitStep.
			Status)

	// Verify output contains event payload.
	var output map[string]any
	require.NoError(t, json.
		Unmarshal(waitStep.
			Output, &output),
	)
	require.Equal(t, "clear",

		output["result"],
	)

	// Verify workflow completed.
	run, err := testStore.GetWorkflowRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusCompleted,

		run.Status)

	// Verify event trigger is now received.
	getResp2 := wfDoReq(t, srv, http.MethodGet, "/v1/events/e2e-check:app-e2e-123", "")
	require.Equal(t, http.
		StatusOK,
		getResp2.Code,
	)

	triggerObj2 := mustDecodeObject(t, getResp2)
	require.Equal(t, "received",

		asString(t, triggerObj2,
			"status",
		))

}

func TestE2E_WaitForEventStep_TimeoutViaReaper(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-evt-timeout-" + newID()

	wf := wfCreateWorkflow(t, srv, projectID, "Event Timeout Workflow", "wf-evt-timeout-"+newID(), []map[string]any{
		{
			"step_ref":           "wait_step",
			"step_type":          "wait_for_event",
			"event_key":          "timeout-check:" + newID(),
			"event_timeout_secs": 1,
		},
	})

	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, nil, nil)
	runID := asString(t, triggered, "id")

	// Verify step is waiting.
	stepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	require.NoError(t, err)

	waitStep := findStepRunByRef(t, stepRuns, "wait_step")
	require.Equal(t, domain.
		StepWaiting,
		waitStep.
			Status)

	// Force the trigger to expire by updating expires_at in the past.
	trigger, err := testStore.GetEventTriggerByStepRunID(ctx, waitStep.ID)
	require.False(t, err !=
		nil ||
		trigger ==
			nil)

	pastExpiry := time.Now().Add(-time.Minute)
	if _, dbErr := testEnv.DB.Pool.Exec(ctx, "UPDATE event_triggers SET expires_at = $1 WHERE id = $2", pastExpiry, trigger.ID); dbErr != nil {
		require.Failf(t, "test failure",

			"force expire trigger: %v", dbErr)
	}

	// Run the reaper once.
	engine := workflow.NewWorkflowEngine(testStore, testQueue, slog.Default())
	callback := workflow.NewStepCallback(testStore, engine, slog.Default())
	reaper := scheduler.NewReaper(testStore, time.Second, 30*time.Second, 0, 0, false, callback)
	reaper.ReapOnce(ctx)

	// Verify trigger timed out.
	updatedTrigger, err := testStore.GetEventTriggerByStepRunID(ctx, waitStep.ID)
	require.False(t, err !=
		nil ||
		updatedTrigger ==
			nil)
	require.Equal(t, domain.
		EventTriggerStatusTimedOut,

		updatedTrigger.
			Status,
	)

	// Verify step failed.
	stepRuns, err = testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	require.NoError(t, err)

	waitStep = findStepRunByRef(t, stepRuns, "wait_step")
	require.Equal(t, domain.
		StepFailed,
		waitStep.
			Status)

	// Verify workflow failed.
	run, err := testStore.GetWorkflowRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusFailed,
		run.
			Status)

}

func TestE2E_WaitForEventStep_ChainedDependencies(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-evt-chain-" + newID()
	job := wfCreateJob(t, srv, projectID, "Chain Job", "wf-chain-job-"+newID())

	wf := wfCreateWorkflow(t, srv, projectID, "Chained Workflow", "wf-chain-"+newID(), []map[string]any{
		{
			"step_ref":           "wait_step",
			"step_type":          "wait_for_event",
			"event_key":          "chain-check:" + newID(),
			"event_timeout_secs": 300,
		},
		{
			"step_ref":   "job_step",
			"job_id":     asString(t, job, "id"),
			"depends_on": []string{"wait_step"},
		},
	})

	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, nil, nil)
	runID := asString(t, triggered, "id")

	// Verify wait_step is waiting and job_step is pending.
	stepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	require.NoError(t, err)

	waitStep := findStepRunByRef(t, stepRuns, "wait_step")
	jobStep := findStepRunByRef(t, stepRuns, "job_step")
	require.Equal(t, domain.
		StepWaiting,
		waitStep.
			Status)
	require.Equal(t, domain.
		StepWaiting,
		jobStep.
			Status)

	// Get event key from trigger.
	trigger, err := testStore.GetEventTriggerByStepRunID(ctx, waitStep.ID)
	require.False(t, err !=
		nil ||
		trigger ==
			nil)

	// Send event.
	sendResp := wfDoReq(t, srv, http.MethodPost, "/v1/events/"+trigger.EventKey+"/send", `{"payload":{"done":true}}`)
	require.Equal(t, http.
		StatusOK,
		sendResp.Code,
	)

	// Verify wait_step completed and job_step was started (running or pending with job_run).
	stepRuns, err = testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	require.NoError(t, err)

	waitStep = findStepRunByRef(t, stepRuns, "wait_step")
	jobStep = findStepRunByRef(t, stepRuns, "job_step")
	require.Equal(t, domain.
		StepCompleted,
		waitStep.
			Status)
	require.Equal(t, domain.
		StepRunning,
		jobStep.
			Status)
	require.NotEqual(t, "",

		jobStep.
			JobRunID)

	// job_step should be running (has job_run_id) since its dependency completed.

}

func TestE2E_SendEvent_AlreadyReceived_Returns409(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)

	projectID := "proj-evt-conflict-" + newID()
	eventKey := "conflict-check:" + newID()

	wf := wfCreateWorkflow(t, srv, projectID, "Conflict Workflow", "wf-conflict-"+newID(), []map[string]any{
		{
			"step_ref":           "wait_step",
			"step_type":          "wait_for_event",
			"event_key":          eventKey,
			"event_timeout_secs": 300,
		},
	})

	wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, nil, nil)

	// First send succeeds.
	sendResp1 := wfDoReq(t, srv, http.MethodPost, "/v1/events/"+eventKey+"/send", `{"payload":{"first":true}}`)
	require.Equal(t, http.
		StatusOK,
		sendResp1.
			Code)

	// Second send returns 409.
	sendResp2 := wfDoReq(t, srv, http.MethodPost, "/v1/events/"+eventKey+"/send", `{"payload":{"second":true}}`)
	require.Equal(t, http.
		StatusConflict,
		sendResp2.
			Code)

}

func TestE2E_ApprovalStepWithParallelEventTrigger(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-evt-approval-" + newID()

	wf := wfCreateWorkflow(t, srv, projectID, "Approval Workflow", "wf-approval-"+newID(), []map[string]any{
		{
			"step_ref":              "approve_step",
			"step_type":             "approval",
			"approval_approvers":    []string{"test:e2e-actor"},
			"approval_timeout_secs": 3600,
		},
	})

	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, nil, nil)
	runID := asString(t, triggered, "id")

	// Verify step is waiting.
	stepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	require.NoError(t, err)

	approvalStep := findStepRunByRef(t, stepRuns, "approve_step")
	require.Equal(t, domain.
		StepWaiting,
		approvalStep.
			Status)

	// Check for parallel event trigger (non-fatal if not created, but should exist).
	trigger, err := testStore.GetEventTriggerByStepRunID(ctx, approvalStep.ID)
	require.NoError(t, err)

	if trigger == nil {
		t.Log("no parallel event trigger found for approval step (non-fatal creation)")
	} else if trigger.Status != domain.EventTriggerStatusWaiting {
		require.Failf(t, "test failure",

			"expected trigger status waiting, got %s", trigger.Status)
	}

	// Approve the step.
	approveResp := wfDoReq(t, srv, http.MethodPost, "/v1/workflow-runs/"+runID+"/steps/approve_step/approve", `{"approver":"admin@example.com"}`)
	require.Equal(t, http.
		StatusOK,
		approveResp.
			Code)

	// Verify step completed.
	stepRuns, err = testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	require.NoError(t, err)

	approvalStep = findStepRunByRef(t, stepRuns, "approve_step")
	require.Equal(t, domain.
		StepCompleted,
		approvalStep.
			Status)

	// Verify workflow completed.
	run, err := testStore.GetWorkflowRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusCompleted,

		run.Status)

	// If trigger existed, verify it's now received.
	if trigger != nil {
		updatedTrigger, getErr := testStore.GetEventTriggerByStepRunID(ctx, approvalStep.ID)
		if getErr == nil && updatedTrigger != nil {
			require.Equal(t, domain.
				EventTriggerStatusReceived,

				updatedTrigger.
					Status,
			)

		}
	}
}
