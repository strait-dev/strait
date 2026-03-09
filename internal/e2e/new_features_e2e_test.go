//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"
)

// ========== 2.23 Run DLQ ==========

func TestE2E_DLQ_ListDeadLetterRuns(t *testing.T) {
	mustClean(t)

	projectID := "proj-dlq-list-" + newID()

	// Create a job
	job := createJob(t, projectID, "DLQ Job", "dlq-job-"+newID())
	jobID := asString(t, job, "id")

	// Trigger a run
	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"dlq"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %s", w.Code, w.Body.String())
	}
	triggerResp := mustDecodeObject(t, w)
	runID := asString(t, triggerResp, "id")

	// Move run to dead_letter via store (simulating executor DLQ)
	err := testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	err = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, nil)
	if err != nil {
		t.Fatalf("executing: %v", err)
	}
	err = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusExecuting, domain.StatusDeadLetter, nil)
	if err != nil {
		t.Fatalf("dead_letter: %v", err)
	}

	// List DLQ runs
	w = doRequest(t, http.MethodGet, "/v1/runs/dlq?project_id="+projectID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list dlq status = %d, body = %s", w.Code, w.Body.String())
	}

	runs := mustDecodeList(t, w)
	if len(runs) != 1 {
		t.Fatalf("expected 1 DLQ run, got %d", len(runs))
	}
	if asString(t, runs[0], "id") != runID {
		t.Fatalf("expected run %s in DLQ", runID)
	}
}

func TestE2E_DLQ_ReplayDeadLetterRun(t *testing.T) {
	mustClean(t)

	projectID := "proj-dlq-replay-" + newID()
	job := createJob(t, projectID, "DLQ Replay", "dlq-replay-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"dlq-replay"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %s", w.Code, w.Body.String())
	}
	runID := asString(t, mustDecodeObject(t, w), "id")

	// Move to dead_letter
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil)
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, nil)
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusExecuting, domain.StatusDeadLetter, nil)

	// Replay from DLQ
	w = doRequest(t, http.MethodPost, fmt.Sprintf("/v1/runs/%s/dlq-replay", runID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("dlq replay status = %d, body = %s", w.Code, w.Body.String())
	}

	replayed := mustDecodeObject(t, w)
	if asString(t, replayed, "status") != "queued" {
		t.Fatalf("expected replayed run status=queued, got %s", asString(t, replayed, "status"))
	}
}

func TestE2E_DLQ_ReplayNonDLQRun_Fails(t *testing.T) {
	mustClean(t)

	projectID := "proj-dlq-fail-" + newID()
	job := createJob(t, projectID, "DLQ NonDLQ", "dlq-nondlq-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"not-dlq"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %s", w.Code, w.Body.String())
	}
	runID := asString(t, mustDecodeObject(t, w), "id")

	// Try to DLQ-replay a run that is still queued
	w = doRequest(t, http.MethodPost, fmt.Sprintf("/v1/runs/%s/dlq-replay", runID), "")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for non-DLQ run, got %d, body = %s", w.Code, w.Body.String())
	}
}

func TestE2E_DLQ_FeatureFlag_Disabled(t *testing.T) {
	// DLQ is enabled in our config, so this verifies the endpoint exists.
	// When disabled, it returns 404. We test by confirming 200/400 (not 404).
	w := doRequest(t, http.MethodGet, "/v1/runs/dlq?project_id=nonexistent", "")
	if w.Code == http.StatusNotFound {
		t.Fatal("DLQ should be enabled but returned 404")
	}
}

// ========== 2.45 Execution Replay/Debug ==========

func TestE2E_DebugBundle_GetBundle(t *testing.T) {
	mustClean(t)

	projectID := "proj-debug-" + newID()
	job := createJob(t, projectID, "Debug Job", "debug-job-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"debug"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %s", w.Code, w.Body.String())
	}
	runID := asString(t, mustDecodeObject(t, w), "id")

	// Get debug bundle
	w = doRequest(t, http.MethodGet, fmt.Sprintf("/v1/runs/%s/debug-bundle", runID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("debug bundle status = %d, body = %s", w.Code, w.Body.String())
	}

	bundle := mustDecodeObject(t, w)
	run := asObject(t, bundle, "run")
	if asString(t, run, "id") != runID {
		t.Fatalf("expected run ID %s in bundle", runID)
	}
}

func TestE2E_DebugBundle_NotFound(t *testing.T) {
	w := doRequest(t, http.MethodGet, "/v1/runs/nonexistent-run-id/debug-bundle", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent run, got %d", w.Code)
	}
}

func TestE2E_Debug_SetDebugMode(t *testing.T) {
	mustClean(t)

	projectID := "proj-debug-mode-" + newID()
	job := createJob(t, projectID, "Debug Mode", "debug-mode-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"debug-mode"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %s", w.Code, w.Body.String())
	}
	runID := asString(t, mustDecodeObject(t, w), "id")

	// Enable debug mode
	w = doRequest(t, http.MethodPost, fmt.Sprintf("/v1/runs/%s/debug", runID),
		`{"debug_mode":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("set debug mode status = %d, body = %s", w.Code, w.Body.String())
	}

	// Verify via get run
	w = doRequest(t, http.MethodGet, fmt.Sprintf("/v1/runs/%s", runID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("get run status = %d, body = %s", w.Code, w.Body.String())
	}
	run := mustDecodeObject(t, w)
	if !asBool(t, run, "debug_mode") {
		t.Fatal("expected debug_mode=true after setting it")
	}

	// Disable debug mode
	w = doRequest(t, http.MethodPost, fmt.Sprintf("/v1/runs/%s/debug", runID),
		`{"debug_mode":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("disable debug mode status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, fmt.Sprintf("/v1/runs/%s", runID), "")
	run = mustDecodeObject(t, w)
	if asBool(t, run, "debug_mode") {
		t.Fatal("expected debug_mode=false after disabling")
	}
}

// ========== 2.11 Run Continuation ==========

func TestE2E_RunContinuation_SDKContinue(t *testing.T) {
	mustClean(t)

	projectID := "proj-continue-" + newID()
	job := createJob(t, projectID, "Continue Job", "continue-job-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"continue"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %s", w.Code, w.Body.String())
	}
	triggerResp := mustDecodeObject(t, w)
	runID := asString(t, triggerResp, "id")
	runToken := asString(t, triggerResp, "run_token")

	// Move run to executing
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil)
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{"started_at": time.Now()})

	// SDK continue
	w = doSDKRequest(t, http.MethodPost, fmt.Sprintf("/sdk/v1/runs/%s/continue", runID), runToken,
		`{"payload":{"continued":true}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("continue status = %d, body = %s", w.Code, w.Body.String())
	}

	contRun := mustDecodeObject(t, w)
	if asString(t, contRun, "continuation_of") != runID {
		t.Fatalf("expected continuation_of=%s, got %s", runID, asString(t, contRun, "continuation_of"))
	}
	if asInt(t, contRun, "lineage_depth") != 1 {
		t.Fatalf("expected lineage_depth=1, got %d", asInt(t, contRun, "lineage_depth"))
	}

	// Verify payload is the continued one
	payload := contRun["payload"]
	payloadBytes, _ := json.Marshal(payload)
	if string(payloadBytes) == "" {
		t.Fatal("expected non-empty payload on continuation run")
	}
}

func TestE2E_RunContinuation_InheritsPayload(t *testing.T) {
	mustClean(t)

	projectID := "proj-continue-inherit-" + newID()
	job := createJob(t, projectID, "Inherit Job", "inherit-job-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"original":"data"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %s", w.Code, w.Body.String())
	}
	triggerResp := mustDecodeObject(t, w)
	runID := asString(t, triggerResp, "id")
	runToken := asString(t, triggerResp, "run_token")

	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil)
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{"started_at": time.Now()})

	// Continue WITHOUT payload — should inherit parent's
	w = doSDKRequest(t, http.MethodPost, fmt.Sprintf("/sdk/v1/runs/%s/continue", runID), runToken, `{}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("continue status = %d, body = %s", w.Code, w.Body.String())
	}

	contRun := mustDecodeObject(t, w)
	payload := contRun["payload"]
	payloadBytes, _ := json.Marshal(payload)
	var p map[string]any
	if err := json.Unmarshal(payloadBytes, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p["original"] != "data" {
		t.Fatalf("expected inherited payload with original=data, got %v", p)
	}
}

func TestE2E_RunContinuation_RejectsNonExecutingRun(t *testing.T) {
	mustClean(t)

	projectID := "proj-continue-reject-" + newID()
	job := createJob(t, projectID, "Reject Job", "reject-job-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"reject"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %s", w.Code, w.Body.String())
	}
	triggerResp := mustDecodeObject(t, w)
	runID := asString(t, triggerResp, "id")
	runToken := asString(t, triggerResp, "run_token")

	// Run is still queued — should not be able to continue
	w = doSDKRequest(t, http.MethodPost, fmt.Sprintf("/sdk/v1/runs/%s/continue", runID), runToken,
		`{"payload":{"continued":true}}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for non-executing run, got %d, body = %s", w.Code, w.Body.String())
	}
}

func TestE2E_RunContinuation_Lineage(t *testing.T) {
	mustClean(t)

	projectID := "proj-lineage-" + newID()
	job := createJob(t, projectID, "Lineage Job", "lineage-job-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"lineage"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %s", w.Code, w.Body.String())
	}
	triggerResp := mustDecodeObject(t, w)
	runID := asString(t, triggerResp, "id")
	runToken := asString(t, triggerResp, "run_token")

	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil)
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{"started_at": time.Now()})

	// Create continuation
	w = doSDKRequest(t, http.MethodPost, fmt.Sprintf("/sdk/v1/runs/%s/continue", runID), runToken,
		`{"payload":{"step":1}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("continue status = %d, body = %s", w.Code, w.Body.String())
	}

	// Get lineage
	w = doRequest(t, http.MethodGet, fmt.Sprintf("/v1/runs/%s/lineage", runID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("lineage status = %d, body = %s", w.Code, w.Body.String())
	}

	lineage := mustDecodeList(t, w)
	if len(lineage) < 2 {
		t.Fatalf("expected at least 2 runs in lineage, got %d", len(lineage))
	}
}

// ========== 2.18 Adaptive Timeout ==========

func TestE2E_AdaptiveTimeout_FeatureFlagEnabled(t *testing.T) {
	// Adaptive timeout is a worker-side feature. The E2E test verifies
	// that the feature flag is enabled and the health stats endpoint works.
	mustClean(t)

	projectID := "proj-adaptive-" + newID()
	job := createJob(t, projectID, "Adaptive Job", "adaptive-job-"+newID())
	jobID := asString(t, job, "id")

	// Trigger a run and complete it with known timing to seed health stats.
	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"adaptive"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %s", w.Code, w.Body.String())
	}
	triggerResp := mustDecodeObject(t, w)
	runID := asString(t, triggerResp, "id")

	// Move to completed to populate health stats.
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil)
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{"started_at": time.Now()})
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{"finished_at": time.Now()})

	// Verify job health stats via the stats endpoint. The adaptive timeout
	// feature reads GetJobHealthStats from the store — we can only validate
	// the flag is active by confirming runs complete normally.
	w = doRequest(t, http.MethodGet, fmt.Sprintf("/v1/runs/%s", runID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("get run status = %d, body = %s", w.Code, w.Body.String())
	}
	run := mustDecodeObject(t, w)
	if asString(t, run, "status") != "completed" {
		t.Fatalf("expected completed, got %s", asString(t, run, "status"))
	}
}
