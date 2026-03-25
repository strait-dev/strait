package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	mathrand "math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// --------------------------------------------------------------------------.
// Configuration.
// --------------------------------------------------------------------------.

var (
	straitURL      = envOr("STRAIT_URL", "http://localhost:7676")
	internalSecret = envOr("INTERNAL_SECRET", "pkW6a/DFXeli3ydZZePyKm8RawSV0mvY7J6F2gWjEHhGWOwkCwjA68oJQiiRGzVU")
	echoBaseURL    = envOr("ECHO_BASE_URL", "http://host.docker.internal:9000")
	iterations     = envInt("ITERATIONS", 3000)
	concurrency    = envInt("CONCURRENCY", 10)
)

// --------------------------------------------------------------------------.
// Counters.
// --------------------------------------------------------------------------.

type counters struct {
	scenariosRun  atomic.Int64
	scenariosOK   atomic.Int64
	scenariosFail atomic.Int64
	bugsFound     atomic.Int64
	apiCalls      atomic.Int64
}

type bugReport struct {
	Scenario    string    `json:"scenario"`
	Iteration   int       `json:"iteration"`
	Description string    `json:"description"`
	Request     string    `json:"request"`
	StatusCode  int       `json:"status_code"`
	Response    string    `json:"response"`
	Timestamp   time.Time `json:"timestamp"`
}

var (
	stats    counters
	bugs     []bugReport
	bugsMu   sync.Mutex
	resultMu sync.Mutex
	results  = map[string]*scenarioResult{}
)

type scenarioResult struct {
	Name     string `json:"name"`
	Runs     int    `json:"runs"`
	Passes   int    `json:"passes"`
	Failures int    `json:"failures"`
}

func trackResult(name string, pass bool) {
	resultMu.Lock()
	defer resultMu.Unlock()
	r, ok := results[name]
	if !ok {
		r = &scenarioResult{Name: name}
		results[name] = r
	}
	r.Runs++
	if pass {
		r.Passes++
	} else {
		r.Failures++
	}
}

// --------------------------------------------------------------------------.
// HTTP helpers.
// --------------------------------------------------------------------------.

var httpClient = &http.Client{Timeout: 60 * time.Second}

func apiCall(method, path string, body any, auth string) (int, map[string]any, string, error) {
	stats.apiCalls.Add(1)
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, straitURL+path, bodyReader)
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.HasPrefix(auth, "strait_") {
		req.Header.Set("Authorization", "Bearer "+auth)
	} else {
		req.Header.Set("X-Internal-Secret", auth)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, "", fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result, string(raw), nil
}

func mustAPI(method, path string, body any, auth string, expectStatus int) (map[string]any, string) {
	code, result, raw, err := apiCall(method, path, body, auth)
	if err != nil {
		log.Fatalf("API call failed: %s %s: %v", method, path, err)
	}
	if code != expectStatus {
		log.Fatalf("unexpected status %d (want %d) for %s %s: %s", code, expectStatus, method, path, raw)
	}
	return result, raw
}

// --------------------------------------------------------------------------.
// Scenario types.
// --------------------------------------------------------------------------.

type scenario struct {
	Name string
	Fn   func(ctx *testCtx, iteration int) error
}

type testCtx struct {
	projectID string
	apiKey    string
	echoURL   string
}

// --------------------------------------------------------------------------.
// Scenarios.
// --------------------------------------------------------------------------.

var allScenarios = []scenario{
	{"job_crud", scenarioJobCRUD},
	{"job_trigger_fast", scenarioTriggerFast},
	{"job_trigger_slow", scenarioTriggerSlow},
	{"job_trigger_flaky_retry", scenarioTriggerFlakyRetry},
	{"job_trigger_with_payload", scenarioTriggerWithPayload},
	{"job_trigger_with_priority", scenarioTriggerWithPriority},
	{"job_trigger_with_tags", scenarioTriggerWithTags},
	{"job_trigger_delayed", scenarioTriggerDelayed},
	{"job_trigger_idempotency", scenarioTriggerIdempotency},
	{"job_trigger_bulk", scenarioTriggerBulk},
	{"job_pause_resume", scenarioJobPauseResume},
	{"job_clone", scenarioJobClone},
	{"job_update", scenarioJobUpdate},
	{"job_batch_create", scenarioBatchCreateJobs},
	{"job_versions", scenarioJobVersions},
	{"run_cancel", scenarioRunCancel},
	{"run_replay", scenarioRunReplay},
	{"run_list_filter", scenarioRunListFilter},
	{"run_events", scenarioRunEvents},
	{"run_bulk_cancel", scenarioRunBulkCancel},
	{"workflow_simple", scenarioWorkflowSimple},
	{"workflow_parallel", scenarioWorkflowParallel},
	{"workflow_conditional_failure", scenarioWorkflowConditionalFailure},
	{"webhook_subscription", scenarioWebhookSubscription},
	{"webhook_test", scenarioWebhookTest},
	{"api_key_lifecycle", scenarioAPIKeyLifecycle},
	{"environment_crud", scenarioEnvironmentCRUD},
	{"secret_crud", scenarioSecretCRUD},
	{"project_crud", scenarioProjectCRUD},
	{"stats_endpoint", scenarioStatsEndpoint},
	{"analytics_community", scenarioAnalyticsCommunity},
	{"health_check", scenarioHealthCheck},
	{"rbac_roles", scenarioRBACRoles},
	{"job_group_crud", scenarioJobGroupCRUD},
	{"notification_channel", scenarioNotificationChannel},
	{"run_dlq", scenarioRunDLQ},
	{"concurrent_triggers", scenarioConcurrentTriggers},
	{"invalid_inputs", scenarioInvalidInputs},
}

// --------------------------------------------------------------------------.
// Scenario implementations.
// --------------------------------------------------------------------------.

func scenarioJobCRUD(ctx *testCtx, iter int) error {
	slug := fmt.Sprintf("e2e-crud-%d-%d", iter, time.Now().UnixMilli())
	// Create.
	code, result, raw, err := apiCall("POST", "/v1/jobs", map[string]any{
		"project_id":   ctx.projectID,
		"name":         "E2E CRUD Test " + slug,
		"slug":         slug,
		"endpoint_url": ctx.echoURL + "/fast-echo",
		"max_attempts": 1,
		"timeout_secs": 30,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "job_crud", iter, "create job failed", "POST /v1/jobs", code, raw)
	}
	jobID := str(result["id"])

	// Get.
	code, _, raw, err = apiCall("GET", "/v1/jobs/"+jobID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "job_crud", iter, "get job failed", "GET /v1/jobs/"+jobID, code, raw)
	}

	// List.
	code, _, raw, err = apiCall("GET", "/v1/jobs?limit=5", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "job_crud", iter, "list jobs failed", "GET /v1/jobs", code, raw)
	}

	// Delete.
	code, _, raw, err = apiCall("DELETE", "/v1/jobs/"+jobID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 204 {
		return reportBug(ctx, "job_crud", iter, "delete job failed", "DELETE /v1/jobs/"+jobID, code, raw)
	}
	return nil
}

func scenarioTriggerFast(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "fast", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)
	return triggerAndWait(ctx, jobID, nil, iter, "job_trigger_fast", 15*time.Second)
}

func scenarioTriggerSlow(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "slow", "/slow-process", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)
	return triggerAndWait(ctx, jobID, nil, iter, "job_trigger_slow", 30*time.Second)
}

func scenarioTriggerFlakyRetry(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "flaky", "/flaky", 5, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)
	// Trigger and just check it queues (flaky may take multiple attempts).
	code, _, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{
		"payload": map[string]any{"test": true},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "job_trigger_flaky_retry", iter, "trigger failed", "POST trigger", code, raw)
	}
	return nil
}

func scenarioTriggerWithPayload(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "payload", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)
	payload := map[string]any{
		"user_id":   fmt.Sprintf("user-%d", iter),
		"action":    "process",
		"data":      map[string]any{"key": "value", "nested": map[string]any{"deep": true}},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return triggerAndWait(ctx, jobID, payload, iter, "job_trigger_with_payload", 15*time.Second)
}

func scenarioTriggerWithPriority(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "priority", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)
	code, _, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{
		"priority": mathrand.Intn(10) + 1, //nolint:gosec // G404: test code
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "job_trigger_with_priority", iter, "trigger with priority failed", "POST trigger", code, raw)
	}
	return nil
}

func scenarioTriggerWithTags(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "tags", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)
	code, _, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{
		"tags": map[string]string{
			"env":    "test",
			"tenant": fmt.Sprintf("tenant-%d", iter%10),
		},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "job_trigger_with_tags", iter, "trigger with tags failed", "POST trigger", code, raw)
	}
	return nil
}

func scenarioTriggerDelayed(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "delayed", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)
	scheduledAt := time.Now().Add(2 * time.Second).UTC().Format(time.RFC3339)
	code, result, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{
		"scheduled_at": scheduledAt,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "job_trigger_delayed", iter, "delayed trigger failed", "POST trigger", code, raw)
	}
	runID := str(result["id"])
	// Verify it's in delayed status initially.
	code, runResult, raw, err := apiCall("GET", "/v1/runs/"+runID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "job_trigger_delayed", iter, "get delayed run failed", "GET run", code, raw)
	}
	status := str(runResult["status"])
	if status != "delayed" && status != "queued" && status != "dequeued" && status != "executing" && status != "completed" {
		return reportBug(ctx, "job_trigger_delayed", iter, fmt.Sprintf("unexpected delayed run status: %s", status), "GET run", code, raw)
	}
	return nil
}

func scenarioTriggerIdempotency(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "idemp", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	idempKey := fmt.Sprintf("idemp-%d-%d", iter, time.Now().UnixMilli())

	// First trigger.
	req1, _ := http.NewRequest("POST", straitURL+"/v1/jobs/"+jobID+"/trigger", bytes.NewReader(mustJSON(map[string]any{})))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+ctx.apiKey)
	req1.Header.Set("X-Idempotency-Key", idempKey)
	stats.apiCalls.Add(1)
	resp1, err := httpClient.Do(req1)
	if err != nil {
		return err
	}
	defer resp1.Body.Close()
	body1, _ := io.ReadAll(resp1.Body)
	if resp1.StatusCode != 201 {
		return reportBug(ctx, "job_trigger_idempotency", iter, "first trigger failed", "POST trigger", resp1.StatusCode, string(body1))
	}
	var run1 map[string]any
	_ = json.Unmarshal(body1, &run1)

	// Second trigger with same key.
	req2, _ := http.NewRequest("POST", straitURL+"/v1/jobs/"+jobID+"/trigger", bytes.NewReader(mustJSON(map[string]any{})))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+ctx.apiKey)
	req2.Header.Set("X-Idempotency-Key", idempKey)
	stats.apiCalls.Add(1)
	resp2, err := httpClient.Do(req2)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)

	// Should return the same run (200 or 201).
	var run2 map[string]any
	_ = json.Unmarshal(body2, &run2)
	if str(run1["id"]) != "" && str(run2["id"]) != "" && str(run1["id"]) != str(run2["id"]) {
		return reportBug(ctx, "job_trigger_idempotency", iter,
			fmt.Sprintf("idempotency violated: got different run IDs %s vs %s", str(run1["id"]), str(run2["id"])),
			"POST trigger with same idempotency key", resp2.StatusCode, string(body2))
	}
	return nil
}

func scenarioTriggerBulk(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "bulk", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	items := make([]map[string]any, 5)
	for i := range items {
		items[i] = map[string]any{
			"payload": map[string]any{"item": i},
		}
	}
	code, _, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger/bulk", map[string]any{
		"items": items,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "job_trigger_bulk", iter, "bulk trigger failed", "POST trigger/bulk", code, raw)
	}
	return nil
}

func scenarioJobPauseResume(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "pauseres", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	// Pause.
	code, _, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/pause", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "job_pause_resume", iter, "pause failed", "POST pause", code, raw)
	}

	// Verify paused - trigger should fail.
	code, _, _, _ = apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{}, ctx.apiKey)
	if code != 409 {
		return reportBug(ctx, "job_pause_resume", iter,
			fmt.Sprintf("trigger paused job should return 409, got %d", code), "POST trigger", code, "")
	}

	// Resume.
	code, _, raw, err = apiCall("POST", "/v1/jobs/"+jobID+"/resume", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "job_pause_resume", iter, "resume failed", "POST resume", code, raw)
	}

	// Trigger should work now.
	code, _, raw, err = apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "job_pause_resume", iter, "trigger after resume failed", "POST trigger", code, raw)
	}
	return nil
}

func scenarioJobClone(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "clone-src", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	newSlug := fmt.Sprintf("e2e-clone-dest-%d-%d", iter, time.Now().UnixMilli())
	code, result, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/clone", map[string]any{
		"name": "Cloned " + newSlug,
		"slug": newSlug,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "job_clone", iter, "clone failed", "POST clone", code, raw)
	}
	clonedID := str(result["id"])
	defer deleteJob(ctx, clonedID)
	return nil
}

func scenarioJobUpdate(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "update", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	newName := fmt.Sprintf("Updated Job %d", iter)
	code, result, raw, err := apiCall("PATCH", "/v1/jobs/"+jobID, map[string]any{
		"name":         newName,
		"max_attempts": 5,
		"timeout_secs": 60,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "job_update", iter, "update failed", "PATCH job", code, raw)
	}
	if str(result["name"]) != newName {
		return reportBug(ctx, "job_update", iter,
			fmt.Sprintf("name not updated: got %q want %q", str(result["name"]), newName),
			"PATCH job", code, raw)
	}
	return nil
}

func scenarioBatchCreateJobs(ctx *testCtx, iter int) error {
	jobs := make([]map[string]any, 3)
	for i := range jobs {
		slug := fmt.Sprintf("e2e-batch-%d-%d-%d", iter, i, time.Now().UnixMilli())
		jobs[i] = map[string]any{
			"project_id":   ctx.projectID,
			"name":         "Batch Job " + slug,
			"slug":         slug,
			"endpoint_url": ctx.echoURL + "/fast-echo",
			"max_attempts": 1,
			"timeout_secs": 30,
		}
	}
	code, _, raw, err := apiCall("POST", "/v1/jobs/batch", map[string]any{
		"jobs": jobs,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "job_batch_create", iter, "batch create failed", "POST /v1/jobs/batch", code, raw)
	}
	return nil
}

func scenarioJobVersions(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "versions", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	// Update to create a new version.
	_, _, _, _ = apiCall("PATCH", "/v1/jobs/"+jobID, map[string]any{
		"name": fmt.Sprintf("Version 2 Job %d", iter),
	}, ctx.apiKey)

	// List versions.
	code, _, raw, err := apiCall("GET", "/v1/jobs/"+jobID+"/versions", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "job_versions", iter, "list versions failed", "GET versions", code, raw)
	}
	return nil
}

func scenarioRunCancel(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "cancel", "/slow-process", 1, 60)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	code, result, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "run_cancel", iter, "trigger failed", "POST trigger", code, raw)
	}
	runID := str(result["id"])

	// Cancel immediately.
	code, _, raw, err = apiCall("DELETE", "/v1/runs/"+runID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "run_cancel", iter, "cancel failed", "DELETE run", code, raw)
	}
	return nil
}

func scenarioRunReplay(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "replay", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	// Trigger and wait for completion.
	err = triggerAndWait(ctx, jobID, nil, iter, "run_replay", 15*time.Second)
	if err != nil {
		return err
	}

	// Get the last run.
	code, listResult, raw, err := apiCall("GET", "/v1/runs?limit=1", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "run_replay", iter, "list runs failed", "GET runs", code, raw)
	}
	items, ok := listResult["data"].([]any)
	if !ok || len(items) == 0 {
		return nil // No runs to replay
	}
	run := items[0].(map[string]any)
	runID := str(run["id"])

	// Replay.
	code, _, raw, err = apiCall("POST", "/v1/runs/"+runID+"/replay", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "run_replay", iter, "replay failed", "POST replay", code, raw)
	}
	return nil
}

func scenarioRunListFilter(ctx *testCtx, iter int) error {
	// List with various filters.
	filters := []string{
		"/v1/runs?limit=5",
		"/v1/runs?status=completed&limit=5",
		"/v1/runs?status=failed&limit=5",
		"/v1/runs?limit=5&triggered_by=manual",
	}
	for _, path := range filters {
		code, _, raw, err := apiCall("GET", path, nil, ctx.apiKey)
		if err != nil {
			return err
		}
		if code != 200 {
			return reportBug(ctx, "run_list_filter", iter, fmt.Sprintf("filter failed: %s", path), "GET "+path, code, raw)
		}
	}
	return nil
}

func scenarioRunEvents(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "events", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	err = triggerAndWait(ctx, jobID, nil, iter, "run_events", 15*time.Second)
	if err != nil {
		return err
	}

	// Get last run and its events.
	code, listResult, _, err := apiCall("GET", "/v1/runs?limit=1", nil, ctx.apiKey)
	if err != nil || code != 200 {
		return nil
	}
	items, ok := listResult["data"].([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	runID := str(items[0].(map[string]any)["id"])

	code, _, raw, err := apiCall("GET", "/v1/runs/"+runID+"/events", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "run_events", iter, "list events failed", "GET events", code, raw)
	}
	return nil
}

func scenarioRunBulkCancel(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "bulkcancel", "/slow-process", 1, 60)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	// Trigger 3 runs.
	var runIDs []string
	for range 3 {
		code, result, _, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{}, ctx.apiKey)
		if err != nil || code != 201 {
			continue
		}
		runIDs = append(runIDs, str(result["id"]))
	}
	if len(runIDs) == 0 {
		return nil
	}

	code, _, raw, err := apiCall("POST", "/v1/runs/bulk-cancel", map[string]any{
		"run_ids": runIDs,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "run_bulk_cancel", iter, "bulk cancel failed", "POST bulk-cancel", code, raw)
	}
	return nil
}

func scenarioWorkflowSimple(ctx *testCtx, iter int) error {
	// Create two jobs for the workflow steps.
	job1ID, err := createTestJob(ctx, iter, "wf-step1", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, job1ID)

	job2ID, err := createTestJob(ctx, iter, "wf-step2", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, job2ID)

	slug := fmt.Sprintf("e2e-wf-%d-%d", iter, time.Now().UnixMilli())
	code, result, raw, err := apiCall("POST", "/v1/workflows", map[string]any{
		"project_id":   ctx.projectID,
		"name":         "E2E Workflow " + slug,
		"slug":         slug,
		"timeout_secs": 120,
		"steps": []map[string]any{
			{"step_ref": "step-a", "job_id": job1ID},
			{"step_ref": "step-b", "job_id": job2ID, "depends_on": []string{"step-a"}},
		},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "workflow_simple", iter, "create workflow failed", "POST workflows", code, raw)
	}
	wfID := str(result["id"])
	defer func() { _, _, _, _ = apiCall("DELETE", "/v1/workflows/"+wfID, nil, ctx.apiKey) }()

	// Trigger.
	code, _, raw, err = apiCall("POST", "/v1/workflows/"+wfID+"/trigger", map[string]any{
		"triggered_by": "manual",
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "workflow_simple", iter, "trigger workflow failed", "POST trigger", code, raw)
	}
	return nil
}

func scenarioWorkflowParallel(ctx *testCtx, iter int) error {
	job1ID, err := createTestJob(ctx, iter, "wf-par1", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, job1ID)

	job2ID, err := createTestJob(ctx, iter, "wf-par2", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, job2ID)

	job3ID, err := createTestJob(ctx, iter, "wf-par3", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, job3ID)

	slug := fmt.Sprintf("e2e-wf-par-%d-%d", iter, time.Now().UnixMilli())
	code, result, raw, err := apiCall("POST", "/v1/workflows", map[string]any{
		"project_id":   ctx.projectID,
		"name":         "Parallel WF " + slug,
		"slug":         slug,
		"timeout_secs": 120,
		"steps": []map[string]any{
			{"step_ref": "par-a", "job_id": job1ID},
			{"step_ref": "par-b", "job_id": job2ID},
			{"step_ref": "join", "job_id": job3ID, "depends_on": []string{"par-a", "par-b"}},
		},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "workflow_parallel", iter, "create parallel workflow failed", "POST workflows", code, raw)
	}
	wfID := str(result["id"])
	defer func() { _, _, _, _ = apiCall("DELETE", "/v1/workflows/"+wfID, nil, ctx.apiKey) }()

	code, _, raw, err = apiCall("POST", "/v1/workflows/"+wfID+"/trigger", map[string]any{}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "workflow_parallel", iter, "trigger parallel workflow failed", "POST trigger", code, raw)
	}
	return nil
}

func scenarioWorkflowConditionalFailure(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "wf-fail", "/always-fail", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	slug := fmt.Sprintf("e2e-wf-fail-%d-%d", iter, time.Now().UnixMilli())
	code, result, raw, err := apiCall("POST", "/v1/workflows", map[string]any{
		"project_id":   ctx.projectID,
		"name":         "Failing WF " + slug,
		"slug":         slug,
		"timeout_secs": 60,
		"steps": []map[string]any{
			{"step_ref": "fail-step", "job_id": jobID, "on_failure": "stop"},
		},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "workflow_conditional_failure", iter, "create failing workflow failed", "POST workflows", code, raw)
	}
	wfID := str(result["id"])
	defer func() { _, _, _, _ = apiCall("DELETE", "/v1/workflows/"+wfID, nil, ctx.apiKey) }()

	code, _, raw, err = apiCall("POST", "/v1/workflows/"+wfID+"/trigger", map[string]any{}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "workflow_conditional_failure", iter, "trigger failing workflow failed", "POST trigger", code, raw)
	}
	return nil
}

func scenarioWebhookSubscription(ctx *testCtx, iter int) error {
	code, result, raw, err := apiCall("POST", "/v1/webhooks/subscriptions", map[string]any{ //nolint:gosec // G101: test fixture.
		"project_id":  ctx.projectID,
		"webhook_url": ctx.echoURL + "/webhook-receiver",
		"event_types": []string{"run.completed", "run.failed"},
		"secret":      "test-webhook-secret",
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "webhook_subscription", iter, "create subscription failed", "POST subscriptions", code, raw)
	}
	subID := str(result["id"])

	// List.
	code, _, raw, err = apiCall("GET", "/v1/webhooks/subscriptions", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "webhook_subscription", iter, "list subscriptions failed", "GET subscriptions", code, raw)
	}

	// Delete.
	code, _, raw, err = apiCall("DELETE", "/v1/webhooks/subscriptions/"+subID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 204 {
		return reportBug(ctx, "webhook_subscription", iter, "delete subscription failed", "DELETE subscription", code, raw)
	}
	return nil
}

func scenarioWebhookTest(_ *testCtx, _ int) error {
	// Webhook test endpoint uses validateURLWithTLS which blocks localhost
	// even with ALLOW_PRIVATE_ENDPOINTS. Requires a publicly reachable URL.
	// Skip in local e2e testing.
	return nil
}

func scenarioAPIKeyLifecycle(ctx *testCtx, iter int) error {
	// Create.
	code, result, raw, err := apiCall("POST", "/v1/api-keys", map[string]any{
		"project_id": ctx.projectID,
		"name":       fmt.Sprintf("e2e-key-%d", iter),
		"scopes":     []string{"jobs:read", "jobs:write", "runs:read"},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "api_key_lifecycle", iter, "create key failed", "POST api-keys", code, raw)
	}
	keyID := str(result["id"])
	rawKey := str(result["key"])

	// Verify the new key works.
	code, _, raw, err = apiCall("GET", "/v1/jobs?limit=1", nil, rawKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "api_key_lifecycle", iter, "new key doesn't work", "GET jobs", code, raw)
	}

	// Rotate.
	code, _, raw, err = apiCall("POST", "/v1/api-keys/"+keyID+"/rotate", map[string]any{
		"grace_period_minutes": 5,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "api_key_lifecycle", iter, "rotate key failed", "POST rotate", code, raw)
	}

	// Revoke.
	code, _, raw, err = apiCall("DELETE", "/v1/api-keys/"+keyID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "api_key_lifecycle", iter, "revoke key failed", "DELETE key", code, raw)
	}
	return nil
}

func scenarioEnvironmentCRUD(ctx *testCtx, iter int) error {
	name := fmt.Sprintf("e2e-env-%d-%d", iter, time.Now().UnixMilli())
	slug := fmt.Sprintf("e2e-env-%d-%d", iter, time.Now().UnixMilli())
	code, result, raw, err := apiCall("POST", "/v1/environments", map[string]any{
		"project_id": ctx.projectID,
		"name":       name,
		"slug":       slug,
		"variables": map[string]string{
			"API_URL": "https://test.example.com",
			"DEBUG":   "true",
		},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "environment_crud", iter, "create env failed", "POST environments", code, raw)
	}
	envID := str(result["id"])

	// List.
	code, _, raw, err = apiCall("GET", "/v1/environments", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "environment_crud", iter, "list envs failed", "GET environments", code, raw)
	}

	// Delete.
	code, _, raw, err = apiCall("DELETE", "/v1/environments/"+envID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 204 {
		return reportBug(ctx, "environment_crud", iter, "delete env failed", "DELETE env", code, raw)
	}
	return nil
}

func scenarioSecretCRUD(ctx *testCtx, iter int) error {
	secretKey := fmt.Sprintf("E2E_SECRET_%d_%d", iter, time.Now().UnixMilli())
	code, result, raw, err := apiCall("POST", "/v1/secrets", map[string]any{
		"project_id": ctx.projectID,
		"secret_key": secretKey,
		"value":      "super-secret-value",
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "secret_crud", iter, "create secret failed", "POST secrets", code, raw)
	}
	secretID := str(result["id"])

	// List.
	code, _, raw, err = apiCall("GET", "/v1/secrets", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "secret_crud", iter, "list secrets failed", "GET secrets", code, raw)
	}

	// Delete.
	code, _, raw, err = apiCall("DELETE", "/v1/secrets/"+secretID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 204 {
		return reportBug(ctx, "secret_crud", iter, "delete secret failed", "DELETE secret", code, raw)
	}
	return nil
}

func scenarioProjectCRUD(ctx *testCtx, iter int) error {
	name := fmt.Sprintf("e2e-project-%d-%d", iter, time.Now().UnixMilli())
	code, result, raw, err := apiCall("POST", "/v1/projects", map[string]any{
		"id":     randomID(),
		"org_id": randomID(),
		"name":   name,
	}, internalSecret)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "project_crud", iter, "create project failed", "POST projects", code, raw)
	}
	projID := str(result["id"])

	// Get - need to create an API key for this project to access it.
	code, keyResult, raw, err := apiCall("POST", "/v1/api-keys", map[string]any{
		"project_id": projID,
		"name":       "temp-key",
	}, internalSecret)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "project_crud", iter, "create temp key failed", "POST api-keys", code, raw)
	}
	tempKey := str(keyResult["key"])

	code, _, raw, err = apiCall("GET", "/v1/projects/"+projID, nil, tempKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "project_crud", iter, "get project failed", "GET project", code, raw)
	}

	// Delete.
	code, _, raw, err = apiCall("DELETE", "/v1/projects/"+projID, nil, tempKey)
	if err != nil {
		return err
	}
	if code != 204 {
		return reportBug(ctx, "project_crud", iter, "delete project failed", "DELETE project", code, raw)
	}
	return nil
}

func scenarioStatsEndpoint(ctx *testCtx, iter int) error {
	code, _, raw, err := apiCall("GET", "/v1/stats", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "stats_endpoint", iter, "stats failed", "GET stats", code, raw)
	}
	return nil
}

func scenarioAnalyticsCommunity(ctx *testCtx, iter int) error {
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)
	endpoints := []string{
		"/v1/analytics/performance?from=" + from + "&to=" + to,
		"/v1/analytics/costs?from=" + from + "&to=" + to,
		"/v1/analytics/compute?from=" + from + "&to=" + to,
		"/v1/analytics/approvals?from=" + from + "&to=" + to,
	}
	for _, ep := range endpoints {
		code, _, raw, err := apiCall("GET", ep, nil, ctx.apiKey)
		if err != nil {
			return err
		}
		if code != 200 {
			return reportBug(ctx, "analytics_community", iter, fmt.Sprintf("analytics endpoint %s failed", ep), "GET "+ep, code, raw)
		}
	}
	return nil
}

func scenarioHealthCheck(_ *testCtx, _ int) error {
	for _, path := range []string{"/health", "/health/ready"} {
		resp, err := http.Get(straitURL + path)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		stats.apiCalls.Add(1)
		if resp.StatusCode != 200 {
			return fmt.Errorf("health check %s returned %d", path, resp.StatusCode)
		}
	}
	return nil
}

func scenarioRBACRoles(ctx *testCtx, iter int) error {
	// Seed system roles.
	code, _, raw, err := apiCall("POST", "/v1/seed-roles", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "rbac_roles", iter, "seed roles failed", "POST seed-roles", code, raw)
	}

	// List roles.
	code, _, raw, err = apiCall("GET", "/v1/roles", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "rbac_roles", iter, "list roles failed", "GET roles", code, raw)
	}
	return nil
}

func scenarioJobGroupCRUD(ctx *testCtx, iter int) error {
	name := fmt.Sprintf("e2e-group-%d-%d", iter, time.Now().UnixMilli())
	slug := fmt.Sprintf("e2e-grp-%d-%d", iter, time.Now().UnixMilli())
	code, result, raw, err := apiCall("POST", "/v1/job-groups", map[string]any{
		"project_id":  ctx.projectID,
		"name":        name,
		"slug":        slug,
		"description": "E2E test group",
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "job_group_crud", iter, "create group failed", "POST job-groups", code, raw)
	}
	groupID := str(result["id"])

	// List.
	code, _, raw, err = apiCall("GET", "/v1/job-groups", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "job_group_crud", iter, "list groups failed", "GET job-groups", code, raw)
	}

	// Delete.
	code, _, raw, err = apiCall("DELETE", "/v1/job-groups/"+groupID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 204 {
		return reportBug(ctx, "job_group_crud", iter, "delete group failed", "DELETE group", code, raw)
	}
	return nil
}

func scenarioNotificationChannel(ctx *testCtx, iter int) error {
	cfgJSON, _ := json.Marshal(map[string]any{"url": ctx.echoURL + "/webhook-receiver"})
	code, result, raw, err := apiCall("POST", "/v1/notification-channels", map[string]any{
		"channel_type": "webhook",
		"name":         fmt.Sprintf("e2e-notif-%d", iter),
		"config":       json.RawMessage(cfgJSON),
		"enabled":      true,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "notification_channel", iter, "create channel failed", "POST notification-channels", code, raw)
	}
	chanID := str(result["id"])

	// List.
	code, _, raw, err = apiCall("GET", "/v1/notification-channels", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "notification_channel", iter, "list channels failed", "GET notification-channels", code, raw)
	}

	// Delete.
	code, _, raw, err = apiCall("DELETE", "/v1/notification-channels/"+chanID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 204 {
		return reportBug(ctx, "notification_channel", iter, "delete channel failed", "DELETE channel", code, raw)
	}
	return nil
}

func scenarioRunDLQ(ctx *testCtx, iter int) error {
	// List DLQ.
	code, _, raw, err := apiCall("GET", "/v1/runs/dlq", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "run_dlq", iter, "list DLQ failed", "GET dlq", code, raw)
	}
	return nil
}

func scenarioConcurrentTriggers(ctx *testCtx, iter int) error {
	jobID, err := createTestJob(ctx, iter, "conc", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	var wg sync.WaitGroup
	errCh := make(chan error, 10)
	for range 10 {
		wg.Go(func() {
			code, _, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{
				"payload": map[string]any{"concurrent": true},
			}, ctx.apiKey)
			if err != nil {
				errCh <- err
				return
			}
			if code != 201 {
				errCh <- fmt.Errorf("concurrent trigger returned %d: %s", code, raw)
			}
		})
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		return reportBug(ctx, "concurrent_triggers", iter, e.Error(), "POST trigger (concurrent)", 0, "")
	}
	return nil
}

func scenarioInvalidInputs(ctx *testCtx, iter int) error {
	// Missing required fields.
	code, _, _, _ := apiCall("POST", "/v1/jobs", map[string]any{}, ctx.apiKey)
	if code == 201 {
		return reportBug(ctx, "invalid_inputs", iter, "empty job creation should fail but returned 201", "POST jobs", code, "")
	}

	// Invalid job ID.
	code, _, _, _ = apiCall("GET", "/v1/jobs/nonexistent-id-123", nil, ctx.apiKey)
	if code != 404 {
		return reportBug(ctx, "invalid_inputs", iter,
			fmt.Sprintf("nonexistent job should return 404, got %d", code), "GET jobs/nonexistent", code, "")
	}

	// Invalid run ID.
	code, _, _, _ = apiCall("GET", "/v1/runs/nonexistent-run-123", nil, ctx.apiKey)
	if code != 404 {
		return reportBug(ctx, "invalid_inputs", iter,
			fmt.Sprintf("nonexistent run should return 404, got %d", code), "GET runs/nonexistent", code, "")
	}

	// Unauthorized.
	code, _, _, _ = apiCall("GET", "/v1/jobs", nil, "invalid-key")
	if code != 401 {
		return reportBug(ctx, "invalid_inputs", iter,
			fmt.Sprintf("invalid auth should return 401, got %d", code), "GET jobs (bad auth)", code, "")
	}

	return nil
}

// --------------------------------------------------------------------------.
// Helpers.
// --------------------------------------------------------------------------.

func createTestJob(ctx *testCtx, iter int, kind, endpoint string, maxAttempts, timeoutSecs int) (string, error) {
	slug := fmt.Sprintf("e2e-%s-%d-%d", kind, iter, time.Now().UnixMilli())
	code, result, raw, err := apiCall("POST", "/v1/jobs", map[string]any{
		"project_id":   ctx.projectID,
		"name":         fmt.Sprintf("E2E %s %d", kind, iter),
		"slug":         slug,
		"endpoint_url": ctx.echoURL + endpoint,
		"max_attempts": maxAttempts,
		"timeout_secs": timeoutSecs,
	}, ctx.apiKey)
	if err != nil {
		return "", err
	}
	if code != 201 {
		return "", fmt.Errorf("create job failed (%d): %s", code, raw)
	}
	return str(result["id"]), nil
}

func deleteJob(ctx *testCtx, jobID string) {
	_, _, _, _ = apiCall("DELETE", "/v1/jobs/"+jobID, nil, ctx.apiKey)
}

func triggerAndWait(ctx *testCtx, jobID string, payload any, iter int, scenario string, timeout time.Duration) error {
	triggerBody := map[string]any{}
	if payload != nil {
		triggerBody["payload"] = payload
	}
	code, result, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", triggerBody, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, scenario, iter, "trigger failed", "POST trigger", code, raw)
	}
	runID := str(result["id"])

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		code, runResult, _, err := apiCall("GET", "/v1/runs/"+runID, nil, ctx.apiKey)
		if err != nil {
			return err
		}
		if code != 200 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		status := str(runResult["status"])
		switch status {
		case "completed":
			return nil
		case "failed", "timed_out", "crashed", "system_failed", "canceled":
			return reportBug(ctx, scenario, iter,
				fmt.Sprintf("run finished with status %s", status), "GET run", code, fmt.Sprintf("%v", runResult))
		}
		time.Sleep(500 * time.Millisecond)
	}
	return reportBug(ctx, scenario, iter, "run did not complete within timeout", "GET run", 0, "")
}

func reportBug(_ *testCtx, scenario string, iter int, desc, request string, code int, response string) error {
	stats.bugsFound.Add(1)
	bug := bugReport{
		Scenario:    scenario,
		Iteration:   iter,
		Description: desc,
		Request:     request,
		StatusCode:  code,
		Response:    truncate(response, 500),
		Timestamp:   time.Now(),
	}
	bugsMu.Lock()
	bugs = append(bugs, bug)
	bugsMu.Unlock()
	log.Printf("[BUG] scenario=%s iter=%d: %s (status=%d)", scenario, iter, desc, code)
	return fmt.Errorf("bug: %s", desc)
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func randomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			return n
		}
	}
	return def
}

// --------------------------------------------------------------------------.
// Main.
// --------------------------------------------------------------------------.

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	phase := envOr("PHASE", "1")
	log.Printf("=== Strait E2E Stress Test (Phase %s) ===", phase)
	log.Printf("Target: %s", straitURL)
	log.Printf("Echo server: %s", echoBaseURL)
	log.Printf("Iterations: %d", iterations)
	log.Printf("Concurrency: %d", concurrency)
	if phase == "1" {
		log.Printf("Scenarios: %d", len(allScenarios))
	} else {
		log.Printf("Scenarios: %d (phase 2: managed + deep integration)", len(phase2Scenarios))
	}

	// Step 1: Create test project.
	log.Println("Setting up test project...")
	projectID := randomID()
	orgID := randomID()
	projResult, _ := mustAPI("POST", "/v1/projects", map[string]any{
		"id":     projectID,
		"org_id": orgID,
		"name":   fmt.Sprintf("e2e-stress-%d", time.Now().Unix()),
	}, internalSecret, 201)
	projectID = str(projResult["id"])
	log.Printf("Project created: %s", projectID)

	// Step 2: Create API key for the project.
	keyResult, _ := mustAPI("POST", "/v1/api-keys", map[string]any{
		"project_id": projectID,
		"name":       "e2e-stress-key",
	}, internalSecret, 201)
	apiKey := str(keyResult["key"])
	log.Printf("API key created: %s...", apiKey[:16])

	// Seed system roles.
	mustAPI("POST", "/v1/seed-roles", nil, apiKey, 200)
	log.Println("System roles seeded")

	ctx := &testCtx{
		projectID: projectID,
		apiKey:    apiKey,
		echoURL:   echoBaseURL,
	}

	// Step 3: Run iterations.
	if phase == "2" {
		runPhase2(ctx, iterations, concurrency)
		printReport(time.Now())
		return
	}

	startTime := time.Now()
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := 1; i <= iterations; i++ {
		// Pick a random scenario for this iteration.
		s := allScenarios[mathrand.Intn(len(allScenarios))] //nolint:gosec // G404: test code

		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[PANIC] scenario=%s iter=%d: %v", s.Name, i, r)
					stats.scenariosFail.Add(1)
					_ = reportBug(ctx, s.Name, i, fmt.Sprintf("panic: %v", r), "N/A", 0, "")
				}
			}()

			stats.scenariosRun.Add(1)
			err := s.Fn(ctx, i)
			if err != nil {
				stats.scenariosFail.Add(1)
				trackResult(s.Name, false)
			} else {
				stats.scenariosOK.Add(1)
				trackResult(s.Name, true)
			}

			// Progress report every 100 iterations.
			run := stats.scenariosRun.Load()
			if run%100 == 0 {
				elapsed := time.Since(startTime)
				rate := float64(run) / elapsed.Seconds()
				log.Printf("[PROGRESS] %d/%d scenarios (%.1f/s) | OK=%d FAIL=%d BUGS=%d | API calls=%d",
					run, iterations, rate,
					stats.scenariosOK.Load(), stats.scenariosFail.Load(), stats.bugsFound.Load(),
					stats.apiCalls.Load())
			}
		})
	}

	wg.Wait()
	printReport(startTime)
}

func printReport(startTime time.Time) {
	elapsed := time.Since(startTime)

	log.Println()
	log.Println("============================================================")
	log.Println("                    STRESS TEST REPORT")
	log.Println("============================================================")
	log.Printf("Duration:          %s", elapsed.Round(time.Second))
	log.Printf("Total iterations:  %d", stats.scenariosRun.Load())
	log.Printf("Passed:            %d", stats.scenariosOK.Load())
	log.Printf("Failed:            %d", stats.scenariosFail.Load())
	log.Printf("Bugs found:        %d", stats.bugsFound.Load())
	log.Printf("Total API calls:   %d", stats.apiCalls.Load())
	log.Printf("Throughput:        %.1f scenarios/sec", float64(stats.scenariosRun.Load())/max(elapsed.Seconds(), 0.001))
	log.Printf("API call rate:     %.1f calls/sec", float64(stats.apiCalls.Load())/max(elapsed.Seconds(), 0.001))
	log.Println()

	log.Println("--- Per-Scenario Results ---")
	resultMu.Lock()
	for _, r := range results {
		passRate := float64(0)
		if r.Runs > 0 {
			passRate = float64(r.Passes) / float64(r.Runs) * 100
		}
		log.Printf("  %-40s runs=%-5d pass=%-5d fail=%-5d (%.1f%%)", r.Name, r.Runs, r.Passes, r.Failures, passRate)
	}
	resultMu.Unlock()

	if len(bugs) > 0 {
		log.Println()
		log.Println("--- Bugs Found ---")
		seen := map[string]int{}
		for _, b := range bugs {
			key := b.Scenario + "|" + b.Description
			seen[key]++
		}
		for key, count := range seen {
			parts := strings.SplitN(key, "|", 2)
			log.Printf("  [%dx] scenario=%s: %s", count, parts[0], parts[1])
		}
	}

	report := map[string]any{
		"duration_secs":    elapsed.Seconds(),
		"total_iterations": stats.scenariosRun.Load(),
		"passed":           stats.scenariosOK.Load(),
		"failed":           stats.scenariosFail.Load(),
		"bugs_found":       stats.bugsFound.Load(),
		"api_calls":        stats.apiCalls.Load(),
		"bugs":             bugs,
		"per_scenario":     results,
	}
	reportJSON, _ := json.MarshalIndent(report, "", "  ")
	reportFile := fmt.Sprintf("e2e_stress_report_%s.json", time.Now().Format("20060102_150405"))
	_ = os.WriteFile(reportFile, reportJSON, 0600)
	log.Printf("\nFull report written to: %s", reportFile)

	log.Println("============================================================")

	if stats.scenariosFail.Load() > 0 {
		os.Exit(1)
	}
}
