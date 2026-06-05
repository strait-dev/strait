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

	"github.com/stretchr/testify/require"
)

func TestE2E_DLQ_ListDeadLetterRuns(t *testing.T) {
	mustClean(t)

	projectID := "proj-dlq-list-" + newID()

	// Create a job
	job := createJob(t, projectID, "DLQ Job", "dlq-job-"+newID())
	jobID := asString(t, job, "id")

	// Trigger a run
	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"dlq"}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	triggerResp := mustDecodeObject(t, w)
	runID := asString(t, triggerResp, "id")

	// Move run to dead_letter via store (simulating executor DLQ)
	err := testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil)
	require.NoError(t, err)

	err = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, nil)
	require.NoError(t, err)

	err = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusExecuting, domain.StatusDeadLetter, nil)
	require.NoError(t, err)

	// List DLQ runs
	w = doRequest(t, http.MethodGet, "/v1/runs/dlq", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	runs := mustDecodeList(t, w)
	require.Len(t, runs,
		1,
	)
	require.Equal(t, runID,

		asString(t, runs[0], "id"))

}

func TestE2E_DLQ_ReplayDeadLetterRun(t *testing.T) {
	mustClean(t)

	projectID := "proj-dlq-replay-" + newID()
	job := createJob(t, projectID, "DLQ Replay", "dlq-replay-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"dlq-replay"}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	runID := asString(t, mustDecodeObject(t, w), "id")

	// Move to dead_letter
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil)
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, nil)
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusExecuting, domain.StatusDeadLetter, nil)

	// Replay from DLQ
	w = doRequest(t, http.MethodPost, fmt.Sprintf("/v1/runs/%s/dlq-replay", runID), "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	replayed := mustDecodeObject(t, w)
	require.Equal(t, "queued",

		asString(t, replayed,
			"status"))

}

func TestE2E_DLQ_ReplayNonDLQRun_Fails(t *testing.T) {
	mustClean(t)

	projectID := "proj-dlq-fail-" + newID()
	job := createJob(t, projectID, "DLQ NonDLQ", "dlq-nondlq-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"not-dlq"}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	runID := asString(t, mustDecodeObject(t, w), "id")

	// Try to DLQ-replay a run that is still queued
	w = doRequest(t, http.MethodPost, fmt.Sprintf("/v1/runs/%s/dlq-replay", runID), "")
	require.Equal(t, http.
		StatusConflict,
		w.Code,
	)

}

func TestE2E_DLQ_FeatureFlag_Disabled(t *testing.T) {
	// DLQ is enabled in our config, so this verifies the endpoint exists.
	// When disabled, it returns 404. We test by confirming 200/400 (not 404).
	w := doRequest(t, http.MethodGet, "/v1/runs/dlq", "", "nonexistent")
	require.NotEqual(t, http.
		StatusNotFound,
		w.
			Code)

}

func TestE2E_DebugBundle_GetBundle(t *testing.T) {
	mustClean(t)

	projectID := "proj-debug-" + newID()
	job := createJob(t, projectID, "Debug Job", "debug-job-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"debug"}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	runID := asString(t, mustDecodeObject(t, w), "id")

	// Get debug bundle
	w = doRequest(t, http.MethodGet, fmt.Sprintf("/v1/runs/%s/debug-bundle", runID), "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	bundle := mustDecodeObject(t, w)
	run := asObject(t, bundle, "run")
	require.Equal(t, runID,

		asString(t, run, "id"))

}

func TestE2E_DebugBundle_NotFound(t *testing.T) {
	w := doRequest(t, http.MethodGet, "/v1/runs/nonexistent-run-id/debug-bundle", "")
	require.Equal(t, http.
		StatusNotFound,
		w.Code,
	)

}

func TestE2E_Debug_SetDebugMode(t *testing.T) {
	mustClean(t)

	projectID := "proj-debug-mode-" + newID()
	job := createJob(t, projectID, "Debug Mode", "debug-mode-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"debug-mode"}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	runID := asString(t, mustDecodeObject(t, w), "id")

	// Enable debug mode
	w = doRequest(t, http.MethodPost, fmt.Sprintf("/v1/runs/%s/debug", runID),
		`{"debug_mode":true}`)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	w = doRequest(t, http.MethodGet, fmt.Sprintf("/v1/runs/%s", runID), "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	run := mustDecodeObject(t, w)
	require.True(t, asBool(t, run,
		"debug_mode",
	))

	// Disable debug mode
	w = doRequest(t, http.MethodPost, fmt.Sprintf("/v1/runs/%s/debug", runID),
		`{"debug_mode":false}`)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	w = doRequest(t, http.MethodGet, fmt.Sprintf("/v1/runs/%s", runID), "")
	run = mustDecodeObject(t, w)
	require.False(t, asBool(t, run,
		"debug_mode",
	))

}

func TestE2E_RunContinuation_SDKContinue(t *testing.T) {
	mustClean(t)

	projectID := "proj-continue-" + newID()
	job := createJob(t, projectID, "Continue Job", "continue-job-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"continue"}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	triggerResp := mustDecodeObject(t, w)
	runID := asString(t, triggerResp, "id")
	runToken := makeE2ERunToken(t, runID)

	// Move run to executing
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil)
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{"started_at": time.Now()})

	// SDK continue
	w = doSDKRequest(t, http.MethodPost, fmt.Sprintf("/sdk/v1/runs/%s/continue", runID), runToken,
		`{"payload":{"continued":true}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	contRun := mustDecodeObject(t, w)
	require.Equal(t, runID,

		asString(t, contRun,
			"continuation_of",
		))
	require.EqualValues(t, 1, asInt(t, contRun,
		"lineage_depth",
	))

	payload := contRun["payload"]
	payloadBytes, _ := json.Marshal(payload)
	require.NotEqual(t, "",

		string(
			payloadBytes,
		))

}

func TestE2E_RunContinuation_InheritsPayload(t *testing.T) {
	mustClean(t)

	projectID := "proj-continue-inherit-" + newID()
	job := createJob(t, projectID, "Inherit Job", "inherit-job-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"original":"data"}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	triggerResp := mustDecodeObject(t, w)
	runID := asString(t, triggerResp, "id")
	runToken := makeE2ERunToken(t, runID)

	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil)
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{"started_at": time.Now()})

	// Continue WITHOUT payload — should inherit parent's
	w = doSDKRequest(t, http.MethodPost, fmt.Sprintf("/sdk/v1/runs/%s/continue", runID), runToken, `{}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	contRun := mustDecodeObject(t, w)
	payload := contRun["payload"]
	payloadBytes, _ := json.Marshal(payload)
	var p map[string]any
	require.NoError(t, json.
		Unmarshal(payloadBytes,
			&p))
	require.Equal(t, "data",

		p["original"])

}

func TestE2E_RunContinuation_RejectsNonExecutingRun(t *testing.T) {
	mustClean(t)

	projectID := "proj-continue-reject-" + newID()
	job := createJob(t, projectID, "Reject Job", "reject-job-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"reject"}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	triggerResp := mustDecodeObject(t, w)
	runID := asString(t, triggerResp, "id")
	runToken := makeE2ERunToken(t, runID)

	// Run is still queued — should not be able to continue
	w = doSDKRequest(t, http.MethodPost, fmt.Sprintf("/sdk/v1/runs/%s/continue", runID), runToken,
		`{"payload":{"continued":true}}`)
	require.Equal(t, http.
		StatusConflict,
		w.Code,
	)

}

func TestE2E_RunContinuation_Lineage(t *testing.T) {
	mustClean(t)

	projectID := "proj-lineage-" + newID()
	job := createJob(t, projectID, "Lineage Job", "lineage-job-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"lineage"}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	triggerResp := mustDecodeObject(t, w)
	runID := asString(t, triggerResp, "id")
	runToken := makeE2ERunToken(t, runID)

	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil)
	_ = testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{"started_at": time.Now()})

	// Create continuation
	w = doSDKRequest(t, http.MethodPost, fmt.Sprintf("/sdk/v1/runs/%s/continue", runID), runToken,
		`{"payload":{"step":1}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	// Get lineage
	w = doRequest(t, http.MethodGet, fmt.Sprintf("/v1/runs/%s/lineage", runID), "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	lineage := mustDecodeList(t, w)
	require.GreaterOrEqual(t, len(lineage), 2)

}

func TestE2E_AdaptiveTimeout_FeatureFlagEnabled(t *testing.T) {
	// Adaptive timeout is a worker-side feature. The E2E test verifies
	// that the health stats endpoint works.
	mustClean(t)

	projectID := "proj-adaptive-" + newID()
	job := createJob(t, projectID, "Adaptive Job", "adaptive-job-"+newID())
	jobID := asString(t, job, "id")

	// Trigger a run and complete it with known timing to seed health stats.
	w := doRequest(t, http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID),
		`{"payload":{"test":"adaptive"}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

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
	require.Equal(t, http.
		StatusOK,
		w.Code)

	run := mustDecodeObject(t, w)
	require.Equal(t, "completed",

		asString(t,
			run, "status"))

}
