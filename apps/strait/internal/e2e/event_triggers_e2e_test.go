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
	if err != nil {
		t.Fatalf("list step runs: %v", err)
	}
	waitStep := findStepRunByRef(t, stepRuns, "wait_step")
	if waitStep.Status != domain.StepWaiting {
		t.Fatalf("expected wait_step status waiting, got %s", waitStep.Status)
	}

	// Verify event trigger exists via API.
	getResp := wfDoReq(t, srv, http.MethodGet, "/v1/events/e2e-check:app-e2e-123", "")
	if getResp.Code != http.StatusOK {
		t.Fatalf("get event trigger status = %d, body = %s", getResp.Code, getResp.Body.String())
	}
	triggerObj := mustDecodeObject(t, getResp)
	if asString(t, triggerObj, "status") != "waiting" {
		t.Fatalf("expected trigger status waiting, got %s", asString(t, triggerObj, "status"))
	}

	// Send event via API.
	sendResp := wfDoReq(t, srv, http.MethodPost, "/v1/events/e2e-check:app-e2e-123/send", `{"payload":{"result":"clear"}}`)
	if sendResp.Code != http.StatusOK {
		t.Fatalf("send event status = %d, body = %s", sendResp.Code, sendResp.Body.String())
	}

	// Verify step completed with event payload.
	stepRuns, err = testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	if err != nil {
		t.Fatalf("list step runs after event: %v", err)
	}
	waitStep = findStepRunByRef(t, stepRuns, "wait_step")
	if waitStep.Status != domain.StepCompleted {
		t.Fatalf("expected wait_step completed, got %s", waitStep.Status)
	}

	// Verify output contains event payload.
	var output map[string]any
	if err := json.Unmarshal(waitStep.Output, &output); err != nil {
		t.Fatalf("unmarshal step output: %v", err)
	}
	if output["result"] != "clear" {
		t.Fatalf("expected output result=clear, got %v", output["result"])
	}

	// Verify workflow completed.
	run, err := testStore.GetWorkflowRun(ctx, runID)
	if err != nil {
		t.Fatalf("get workflow run: %v", err)
	}
	if run.Status != domain.WfStatusCompleted {
		t.Fatalf("expected workflow completed, got %s", run.Status)
	}

	// Verify event trigger is now received.
	getResp2 := wfDoReq(t, srv, http.MethodGet, "/v1/events/e2e-check:app-e2e-123", "")
	if getResp2.Code != http.StatusOK {
		t.Fatalf("get event trigger after send status = %d", getResp2.Code)
	}
	triggerObj2 := mustDecodeObject(t, getResp2)
	if asString(t, triggerObj2, "status") != "received" {
		t.Fatalf("expected trigger status received, got %s", asString(t, triggerObj2, "status"))
	}
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
	if err != nil {
		t.Fatalf("list step runs: %v", err)
	}
	waitStep := findStepRunByRef(t, stepRuns, "wait_step")
	if waitStep.Status != domain.StepWaiting {
		t.Fatalf("expected wait_step status waiting, got %s", waitStep.Status)
	}

	// Force the trigger to expire by updating expires_at in the past.
	trigger, err := testStore.GetEventTriggerByStepRunID(ctx, waitStep.ID)
	if err != nil || trigger == nil {
		t.Fatalf("get event trigger: %v (trigger=%v)", err, trigger)
	}
	pastExpiry := time.Now().Add(-time.Minute)
	if _, dbErr := testEnv.DB.Pool.Exec(ctx, "UPDATE event_triggers SET expires_at = $1 WHERE id = $2", pastExpiry, trigger.ID); dbErr != nil {
		t.Fatalf("force expire trigger: %v", dbErr)
	}

	// Run the reaper once.
	engine := workflow.NewWorkflowEngine(testStore, testQueue, slog.Default())
	callback := workflow.NewStepCallback(testStore, engine, slog.Default())
	reaper := scheduler.NewReaper(testStore, time.Second, 30*time.Second, 0, 0, false, callback)
	reaper.ReapOnce(ctx)

	// Verify trigger timed out.
	updatedTrigger, err := testStore.GetEventTriggerByStepRunID(ctx, waitStep.ID)
	if err != nil || updatedTrigger == nil {
		t.Fatalf("get updated trigger: %v", err)
	}
	if updatedTrigger.Status != domain.EventTriggerStatusTimedOut {
		t.Fatalf("expected trigger timed_out, got %s", updatedTrigger.Status)
	}

	// Verify step failed.
	stepRuns, err = testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	if err != nil {
		t.Fatalf("list step runs after reap: %v", err)
	}
	waitStep = findStepRunByRef(t, stepRuns, "wait_step")
	if waitStep.Status != domain.StepFailed {
		t.Fatalf("expected wait_step failed, got %s", waitStep.Status)
	}

	// Verify workflow failed.
	run, err := testStore.GetWorkflowRun(ctx, runID)
	if err != nil {
		t.Fatalf("get workflow run: %v", err)
	}
	if run.Status != domain.WfStatusFailed {
		t.Fatalf("expected workflow failed, got %s", run.Status)
	}
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
	if err != nil {
		t.Fatalf("list step runs: %v", err)
	}
	waitStep := findStepRunByRef(t, stepRuns, "wait_step")
	jobStep := findStepRunByRef(t, stepRuns, "job_step")
	if waitStep.Status != domain.StepWaiting {
		t.Fatalf("expected wait_step waiting, got %s", waitStep.Status)
	}
	if jobStep.Status != domain.StepWaiting {
		t.Fatalf("expected job_step waiting (deps unmet), got %s", jobStep.Status)
	}

	// Get event key from trigger.
	trigger, err := testStore.GetEventTriggerByStepRunID(ctx, waitStep.ID)
	if err != nil || trigger == nil {
		t.Fatalf("get event trigger: %v", err)
	}

	// Send event.
	sendResp := wfDoReq(t, srv, http.MethodPost, "/v1/events/"+trigger.EventKey+"/send", `{"payload":{"done":true}}`)
	if sendResp.Code != http.StatusOK {
		t.Fatalf("send event status = %d, body = %s", sendResp.Code, sendResp.Body.String())
	}

	// Verify wait_step completed and job_step was started (running or pending with job_run).
	stepRuns, err = testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	if err != nil {
		t.Fatalf("list step runs after event: %v", err)
	}
	waitStep = findStepRunByRef(t, stepRuns, "wait_step")
	jobStep = findStepRunByRef(t, stepRuns, "job_step")
	if waitStep.Status != domain.StepCompleted {
		t.Fatalf("expected wait_step completed, got %s", waitStep.Status)
	}
	// job_step should be running (has job_run_id) since its dependency completed.
	if jobStep.Status != domain.StepRunning {
		t.Fatalf("expected job_step running (fan-in triggered), got %s", jobStep.Status)
	}
	if jobStep.JobRunID == "" {
		t.Fatal("expected job_step to have a job_run_id after fan-in")
	}
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
	if sendResp1.Code != http.StatusOK {
		t.Fatalf("first send status = %d, body = %s", sendResp1.Code, sendResp1.Body.String())
	}

	// Second send returns 409.
	sendResp2 := wfDoReq(t, srv, http.MethodPost, "/v1/events/"+eventKey+"/send", `{"payload":{"second":true}}`)
	if sendResp2.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body = %s", sendResp2.Code, sendResp2.Body.String())
	}
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
	if err != nil {
		t.Fatalf("list step runs: %v", err)
	}
	approvalStep := findStepRunByRef(t, stepRuns, "approve_step")
	if approvalStep.Status != domain.StepWaiting {
		t.Fatalf("expected approval step waiting, got %s", approvalStep.Status)
	}

	// Check for parallel event trigger (non-fatal if not created, but should exist).
	trigger, err := testStore.GetEventTriggerByStepRunID(ctx, approvalStep.ID)
	if err != nil {
		t.Fatalf("get event trigger by step run: %v", err)
	}
	if trigger == nil {
		t.Log("no parallel event trigger found for approval step (non-fatal creation)")
	} else if trigger.Status != domain.EventTriggerStatusWaiting {
		t.Fatalf("expected trigger status waiting, got %s", trigger.Status)
	}

	// Approve the step.
	approveResp := wfDoReq(t, srv, http.MethodPost, "/v1/workflow-runs/"+runID+"/steps/approve_step/approve", `{"approver":"admin@example.com"}`)
	if approveResp.Code != http.StatusOK {
		t.Fatalf("approve step status = %d, body = %s", approveResp.Code, approveResp.Body.String())
	}

	// Verify step completed.
	stepRuns, err = testStore.ListStepRunsByWorkflowRun(ctx, runID, 100, nil)
	if err != nil {
		t.Fatalf("list step runs after approve: %v", err)
	}
	approvalStep = findStepRunByRef(t, stepRuns, "approve_step")
	if approvalStep.Status != domain.StepCompleted {
		t.Fatalf("expected approval step completed, got %s", approvalStep.Status)
	}

	// Verify workflow completed.
	run, err := testStore.GetWorkflowRun(ctx, runID)
	if err != nil {
		t.Fatalf("get workflow run: %v", err)
	}
	if run.Status != domain.WfStatusCompleted {
		t.Fatalf("expected workflow completed, got %s", run.Status)
	}

	// If trigger existed, verify it's now received.
	if trigger != nil {
		updatedTrigger, getErr := testStore.GetEventTriggerByStepRunID(ctx, approvalStep.ID)
		if getErr == nil && updatedTrigger != nil {
			if updatedTrigger.Status != domain.EventTriggerStatusReceived {
				t.Fatalf("expected trigger received after approval, got %s", updatedTrigger.Status)
			}
		}
	}
}
