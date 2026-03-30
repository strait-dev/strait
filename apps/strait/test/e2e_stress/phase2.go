package main

// Phase 2 scenarios: managed execution, webhook delivery, SSE streaming,
// cron, job dependencies, workflow approvals, event triggers, log drains,
// circuit breaker, checkpoint/resume, long-running jobs, error scenarios.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	mathrand "math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

// --------------------------------------------------------------------------.
// Phase 2 scenario list.
// --------------------------------------------------------------------------.

var phase2Scenarios = []scenario{
	// Managed execution with Docker containers.
	{"managed_fast_10s", scenarioManagedFast10s},
	{"managed_slow_30s", scenarioManagedSlow30s},
	{"managed_slow_60s", scenarioManagedSlow60s},
	{"managed_error_exit1", scenarioManagedErrorExit1},
	{"managed_error_timeout", scenarioManagedErrorTimeout},
	{"managed_checkpoint_recovery", scenarioManagedCheckpointRecovery},

	// Webhook delivery verification.
	{"webhook_delivery_on_complete", scenarioWebhookDeliveryOnComplete},
	{"webhook_delivery_on_failure", scenarioWebhookDeliveryOnFailure},

	// SSE streaming.
	{"sse_run_stream", scenarioSSERunStream},

	// Cron scheduling.
	{"cron_job_trigger", scenarioCronJobTrigger},

	// Job dependencies.
	{"job_dependency_chain", scenarioJobDependencyChain},

	// Workflow approvals.
	{"workflow_approval_gate", scenarioWorkflowApprovalGate},

	// Event triggers.
	{"event_trigger_wait_and_send", scenarioEventTriggerWaitAndSend},

	// Log drains.
	{"log_drain_crud", scenarioLogDrainCRUD},

	// Circuit breaker behavior.
	{"circuit_breaker_flaky", scenarioCircuitBreakerFlaky},

	// Concurrent managed execution.
	{"concurrent_managed_jobs", scenarioConcurrentManagedJobs},

	// HTTP + managed mix.
	{"mixed_execution_modes", scenarioMixedExecutionModes},
}

// --------------------------------------------------------------------------.
// Managed execution scenarios.
// --------------------------------------------------------------------------.

func scenarioManagedFast10s(ctx *testCtx, iter int) error {
	return runManagedJob(ctx, iter, "managed-fast-10s", "registry.fly.io/strait:loadtest-python", map[string]string{
		"SCRIPT_NAME":   "fast_processor.py",
		"WORK_DURATION": "10",
	}, 60, "managed_fast_10s")
}

func scenarioManagedSlow30s(ctx *testCtx, iter int) error {
	return runManagedJob(ctx, iter, "managed-slow-30s", "registry.fly.io/strait:loadtest-python", map[string]string{
		"SCRIPT_NAME":   "slow_cpu_work.py",
		"WORK_DURATION": "30",
	}, 120, "managed_slow_30s")
}

func scenarioManagedSlow60s(ctx *testCtx, iter int) error {
	return runManagedJob(ctx, iter, "managed-slow-60s", "registry.fly.io/strait:loadtest-python", map[string]string{
		"SCRIPT_NAME":   "slow_cpu_work.py",
		"WORK_DURATION": "60",
	}, 180, "managed_slow_60s")
}

func scenarioManagedErrorExit1(ctx *testCtx, iter int) error {
	jobID, err := createManagedJob(ctx, iter, "managed-err1", "registry.fly.io/strait:loadtest-errors", 1, 60)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	// Trigger with error scenario.
	code, result, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{
		"payload": map[string]any{"error_scenario": "exit_code_1"},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "managed_error_exit1", iter, "trigger failed", "POST trigger", code, raw)
	}
	runID := str(result["id"])

	// Wait for the run to finish (should fail).
	return waitForRunTerminal(ctx, runID, iter, "managed_error_exit1", 90*time.Second, []string{"failed", "crashed"})
}

func scenarioManagedErrorTimeout(ctx *testCtx, iter int) error {
	// Create a job with very short timeout (15s) and run infinite_loop.
	jobID, err := createManagedJob(ctx, iter, "managed-timeout", "registry.fly.io/strait:loadtest-errors", 1, 15)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	code, result, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{
		"payload": map[string]any{"error_scenario": "infinite_loop"},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "managed_error_timeout", iter, "trigger failed", "POST trigger", code, raw)
	}
	runID := str(result["id"])

	// Should timeout.
	return waitForRunTerminal(ctx, runID, iter, "managed_error_timeout", 120*time.Second, []string{"timed_out", "failed", "crashed"})
}

func scenarioManagedCheckpointRecovery(ctx *testCtx, iter int) error {
	// Use the error scenario that checkpoints then crashes.
	jobID, err := createManagedJob(ctx, iter, "managed-cp", "registry.fly.io/strait:loadtest-errors", 3, 60)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	code, result, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{
		"payload": map[string]any{"error_scenario": "panic_after_checkpoint"},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "managed_checkpoint_recovery", iter, "trigger failed", "POST trigger", code, raw)
	}
	runID := str(result["id"])

	// Wait for terminal state (may retry multiple times).
	return waitForRunTerminal(ctx, runID, iter, "managed_checkpoint_recovery", 180*time.Second, []string{"completed", "failed", "crashed"})
}

// --------------------------------------------------------------------------.
// Webhook delivery scenarios.
// --------------------------------------------------------------------------.

func scenarioWebhookDeliveryOnComplete(ctx *testCtx, iter int) error {
	// Create webhook subscription for run.completed.
	code, result, raw, err := apiCall("POST", "/v1/webhooks/subscriptions", map[string]any{"project_id": ctx.projectID,
		"webhook_url": ctx.echoURL + "/webhook-receiver",
		"event_types": []string{"run.completed"},
		"secret":      randomHex(16),
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "webhook_delivery_on_complete", iter, "create subscription failed", "POST subscriptions", code, raw)
	}
	subID := str(result["id"])
	defer func() { _, _, _, _ = apiCall("DELETE", "/v1/webhooks/subscriptions/"+subID, nil, ctx.apiKey) }()

	// Create and trigger a fast HTTP job.
	jobID, err := createTestJob(ctx, iter, "wh-complete", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	err = triggerAndWait(ctx, jobID, nil, iter, "webhook_delivery_on_complete", 30*time.Second)
	if err != nil {
		return err
	}

	// Give webhook worker time to dispatch.
	time.Sleep(3 * time.Second)

	// Check webhook deliveries.
	code, _, raw, err = apiCall("GET", "/v1/webhooks/deliveries?limit=5", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "webhook_delivery_on_complete", iter, "list deliveries failed", "GET deliveries", code, raw)
	}
	return nil
}

func scenarioWebhookDeliveryOnFailure(ctx *testCtx, iter int) error {
	// Create webhook subscription for run.failed.
	code, result, raw, err := apiCall("POST", "/v1/webhooks/subscriptions", map[string]any{"project_id": ctx.projectID,
		"webhook_url": ctx.echoURL + "/webhook-receiver",
		"event_types": []string{"run.failed"},
		"secret":      randomHex(16),
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "webhook_delivery_on_failure", iter, "create subscription failed", "POST subscriptions", code, raw)
	}
	subID := str(result["id"])
	defer func() { _, _, _, _ = apiCall("DELETE", "/v1/webhooks/subscriptions/"+subID, nil, ctx.apiKey) }()

	// Trigger a job that always fails.
	jobID, err := createTestJob(ctx, iter, "wh-fail", "/always-fail", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	// Trigger and wait for it to fail.
	code, _, raw, err = apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "webhook_delivery_on_failure", iter, "trigger failed", "POST trigger", code, raw)
	}

	// Wait for webhook delivery.
	time.Sleep(8 * time.Second)

	code, _, raw, err = apiCall("GET", "/v1/webhooks/deliveries?limit=5", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "webhook_delivery_on_failure", iter, "list deliveries failed", "GET deliveries", code, raw)
	}
	return nil
}

// --------------------------------------------------------------------------.
// SSE streaming scenarios.
// --------------------------------------------------------------------------.

func scenarioSSERunStream(ctx *testCtx, iter int) error {
	// Create and trigger a job.
	jobID, err := createTestJob(ctx, iter, "sse-stream", "/slow-process", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	code, result, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "sse_run_stream", iter, "trigger failed", "POST trigger", code, raw)
	}
	runID := str(result["id"])

	// Connect to SSE stream.
	sseURL := straitURL + "/v1/runs/" + runID + "/stream"
	req, _ := http.NewRequest("GET", sseURL, nil)
	req.Header.Set("Authorization", "Bearer "+ctx.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	sseCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req = req.WithContext(sseCtx)

	stats.apiCalls.Add(1)
	resp, err := httpClient.Do(req)
	if err != nil {
		// Timeouts are OK for SSE.
		if sseCtx.Err() != nil {
			return nil
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return reportBug(ctx, "sse_run_stream", iter, "SSE stream failed", "GET stream", resp.StatusCode, string(body))
	}

	// Read a few SSE events.
	scanner := bufio.NewScanner(resp.Body)
	eventCount := 0
	for scanner.Scan() && eventCount < 5 {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			eventCount++
		}
	}
	return nil
}

// --------------------------------------------------------------------------.
// Cron scheduling scenarios.
// --------------------------------------------------------------------------.

func scenarioCronJobTrigger(ctx *testCtx, iter int) error {
	slug := fmt.Sprintf("e2e-cron-%d-%d", iter, time.Now().UnixMilli())
	// Create a job with a cron schedule (every minute).
	code, result, raw, err := apiCall("POST", "/v1/jobs", map[string]any{
		"project_id":   ctx.projectID,
		"name":         "E2E Cron Job " + slug,
		"slug":         slug,
		"endpoint_url": ctx.echoURL + "/fast-echo",
		"max_attempts": 1,
		"timeout_secs": 30,
		"cron":         "* * * * *",
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "cron_job_trigger", iter, "create cron job failed", "POST jobs", code, raw)
	}
	jobID := str(result["id"])
	defer deleteJob(ctx, jobID)

	// Verify the job was created with cron.
	code, jobResult, raw, err := apiCall("GET", "/v1/jobs/"+jobID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "cron_job_trigger", iter, "get job failed", "GET job", code, raw)
	}
	if str(jobResult["cron"]) != "* * * * *" {
		return reportBug(ctx, "cron_job_trigger", iter, "cron not saved", "GET job", code, raw)
	}

	// Disable cron so we don't leave it running.
	_, _, _, _ = apiCall("PATCH", "/v1/jobs/"+jobID, map[string]any{"cron": ""}, ctx.apiKey)
	return nil
}

// --------------------------------------------------------------------------.
// Job dependency scenarios.
// --------------------------------------------------------------------------.

func scenarioJobDependencyChain(ctx *testCtx, iter int) error {
	// Create two jobs: A and B where B depends on A.
	jobAID, err := createTestJob(ctx, iter, "dep-a", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobAID)

	jobBID, err := createTestJob(ctx, iter, "dep-b", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobBID)

	// Create dependency: B depends on A.
	code, _, raw, err := apiCall("POST", "/v1/jobs/"+jobBID+"/dependencies", map[string]any{
		"depends_on_job_id": jobAID,
		"condition":         "completed",
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "job_dependency_chain", iter, "create dependency failed", "POST dependencies", code, raw)
	}

	// List dependencies to verify.
	code, _, raw, err = apiCall("GET", "/v1/jobs/"+jobBID+"/dependencies", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "job_dependency_chain", iter, "list dependencies failed", "GET dependencies", code, raw)
	}

	// Trigger job A.
	err = triggerAndWait(ctx, jobAID, nil, iter, "job_dependency_chain", 30*time.Second)
	if err != nil {
		return err
	}

	// Now trigger job B (should work since A completed).
	err = triggerAndWait(ctx, jobBID, nil, iter, "job_dependency_chain", 30*time.Second)
	if err != nil {
		return err
	}

	return nil
}

// --------------------------------------------------------------------------.
// Workflow approval scenarios.
// --------------------------------------------------------------------------.

func scenarioWorkflowApprovalGate(ctx *testCtx, iter int) error {
	// Create a job for the workflow.
	jobID, err := createTestJob(ctx, iter, "wf-approval", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	slug := fmt.Sprintf("e2e-wf-approval-%d-%d", iter, time.Now().UnixMilli())
	code, result, raw, err := apiCall("POST", "/v1/workflows", map[string]any{
		"project_id":   ctx.projectID,
		"name":         "Approval WF " + slug,
		"slug":         slug,
		"timeout_secs": 120,
		"steps": []map[string]any{
			{
				"step_ref":              "approve-gate",
				"step_type":             "approval",
				"approval_timeout_secs": 60,
				"approval_approvers":    []string{"test-user@example.com"},
			},
			{
				"step_ref":   "run-after-approval",
				"job_id":     jobID,
				"depends_on": []string{"approve-gate"},
			},
		},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "workflow_approval_gate", iter, "create workflow failed", "POST workflows", code, raw)
	}
	wfID := str(result["id"])
	defer func() { _, _, _, _ = apiCall("DELETE", "/v1/workflows/"+wfID, nil, ctx.apiKey) }()

	// Trigger workflow.
	code, wfRunResult, raw, err := apiCall("POST", "/v1/workflows/"+wfID+"/trigger", map[string]any{
		"triggered_by": "manual",
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "workflow_approval_gate", iter, "trigger workflow failed", "POST trigger", code, raw)
	}
	wfRunID := str(wfRunResult["id"])

	// Wait briefly for approval to be created.
	time.Sleep(2 * time.Second)

	// List workflow runs to verify it's running/waiting.
	code, _, raw, err = apiCall("GET", "/v1/workflow-runs/"+wfRunID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "workflow_approval_gate", iter, "get workflow run failed", "GET wf-run", code, raw)
	}

	return nil
}

// --------------------------------------------------------------------------.
// Event trigger scenarios.
// --------------------------------------------------------------------------.

func scenarioEventTriggerWaitAndSend(ctx *testCtx, iter int) error {
	eventKey := fmt.Sprintf("e2e-event-%d-%d", iter, time.Now().UnixMilli())

	// Create an event trigger.
	code, result, raw, err := apiCall("POST", "/v1/event-triggers", map[string]any{
		"event_key":    eventKey,
		"timeout_secs": 30,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "event_trigger_wait_and_send", iter, "create event trigger failed", "POST event-triggers", code, raw)
	}
	triggerID := str(result["id"])
	_ = triggerID

	// Send the event.
	time.Sleep(1 * time.Second)
	code, _, raw, err = apiCall("POST", "/v1/events/"+eventKey+"/send", map[string]any{
		"payload": map[string]any{"status": "received", "data": "test"},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	// Event send can return 200 or 201.
	if code != 200 && code != 201 {
		return reportBug(ctx, "event_trigger_wait_and_send", iter, fmt.Sprintf("send event failed (status=%d)", code), "POST send", code, raw)
	}

	return nil
}

// --------------------------------------------------------------------------.
// Log drain scenarios.
// --------------------------------------------------------------------------.

func scenarioLogDrainCRUD(ctx *testCtx, iter int) error {
	// Create log drain.
	code, result, raw, err := apiCall("POST", "/v1/log-drains", map[string]any{
		"project_id":   ctx.projectID,
		"name":         fmt.Sprintf("e2e-drain-%d", iter),
		"drain_type":   "http",
		"endpoint_url": ctx.echoURL + "/webhook-receiver",
		"auth_type":    "header",
		"auth_config":  map[string]any{"X-Custom-Key": "test-value"},
		"level_filter": []string{"error", "warn"},
		"enabled":      true,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, "log_drain_crud", iter, "create log drain failed", "POST log-drains", code, raw)
	}
	drainID := str(result["id"])

	// List.
	code, _, raw, err = apiCall("GET", "/v1/log-drains", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "log_drain_crud", iter, "list log drains failed", "GET log-drains", code, raw)
	}

	// Get.
	code, _, raw, err = apiCall("GET", "/v1/log-drains/"+drainID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "log_drain_crud", iter, "get log drain failed", "GET log-drain", code, raw)
	}

	// Delete.
	code, _, raw, err = apiCall("DELETE", "/v1/log-drains/"+drainID, nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 204 {
		return reportBug(ctx, "log_drain_crud", iter, "delete log drain failed", "DELETE log-drain", code, raw)
	}
	return nil
}

// --------------------------------------------------------------------------.
// Circuit breaker scenarios.
// --------------------------------------------------------------------------.

func scenarioCircuitBreakerFlaky(ctx *testCtx, iter int) error {
	// Create a job pointing to the flaky endpoint with retries.
	jobID, err := createTestJob(ctx, iter, "cb-flaky", "/flaky", 5, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	// Trigger multiple runs to exercise the circuit breaker.
	for range 5 {
		_, _, _, _ = apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{
			"payload": map[string]any{"test": "circuit_breaker"},
		}, ctx.apiKey)
	}

	// Check job health endpoint.
	time.Sleep(2 * time.Second)
	code, _, raw, err := apiCall("GET", "/v1/jobs/"+jobID+"/health", nil, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 200 {
		return reportBug(ctx, "circuit_breaker_flaky", iter, "job health failed", "GET health", code, raw)
	}
	return nil
}

// --------------------------------------------------------------------------.
// Concurrent managed execution.
// --------------------------------------------------------------------------.

func scenarioConcurrentManagedJobs(ctx *testCtx, iter int) error {
	// Create a single managed job and trigger it multiple times concurrently.
	jobID, err := createManagedJob(ctx, iter, "conc-managed", "registry.fly.io/strait:loadtest-errors", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	var wg sync.WaitGroup
	errCh := make(chan error, 3)
	for range 3 {
		wg.Go(func() {
			code, _, raw, callErr := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{
				"payload": map[string]any{"error_scenario": "clean_exit"},
			}, ctx.apiKey)
			if callErr != nil {
				errCh <- callErr
				return
			}
			if code != 201 {
				errCh <- fmt.Errorf("concurrent managed trigger returned %d: %s", code, truncate(raw, 200))
			}
		})
	}
	wg.Wait()
	close(errCh)

	for e := range errCh {
		return reportBug(ctx, "concurrent_managed_jobs", iter, e.Error(), "POST trigger (concurrent managed)", 0, "")
	}
	return nil
}

// --------------------------------------------------------------------------.
// Mixed execution mode scenarios.
// --------------------------------------------------------------------------.

func scenarioMixedExecutionModes(ctx *testCtx, iter int) error {
	// Create both an HTTP job and a managed job, trigger both.
	httpJobID, err := createTestJob(ctx, iter, "mix-http", "/fast-echo", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, httpJobID)

	managedJobID, err := createManagedJob(ctx, iter, "mix-managed", "registry.fly.io/strait:loadtest-errors", 1, 30)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, managedJobID)

	// Trigger both.
	code1, _, raw1, err := apiCall("POST", "/v1/jobs/"+httpJobID+"/trigger", map[string]any{}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code1 != 201 {
		return reportBug(ctx, "mixed_execution_modes", iter, "HTTP job trigger failed", "POST trigger", code1, raw1)
	}

	code2, _, raw2, err := apiCall("POST", "/v1/jobs/"+managedJobID+"/trigger", map[string]any{
		"payload": map[string]any{"error_scenario": "clean_exit"},
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code2 != 201 {
		return reportBug(ctx, "mixed_execution_modes", iter, "managed job trigger failed", "POST trigger", code2, raw2)
	}
	return nil
}

// --------------------------------------------------------------------------.
// Helpers.
// --------------------------------------------------------------------------.

func createManagedJob(ctx *testCtx, iter int, kind, imageURI string, maxAttempts, timeoutSecs int) (string, error) {
	slug := fmt.Sprintf("e2e-%s-%d-%d", kind, iter, time.Now().UnixMilli())
	code, result, raw, err := apiCall("POST", "/v1/jobs", map[string]any{
		"project_id":     ctx.projectID,
		"name":           fmt.Sprintf("E2E %s %d", kind, iter),
		"slug":           slug,
		"execution_mode": "managed",
		"image_uri":      imageURI,
		"machine_preset": "micro",
		"max_attempts":   maxAttempts,
		"timeout_secs":   timeoutSecs,
	}, ctx.apiKey)
	if err != nil {
		return "", err
	}
	if code != 201 {
		return "", fmt.Errorf("create managed job failed (%d): %s", code, raw)
	}
	return str(result["id"]), nil
}

func runManagedJob(ctx *testCtx, iter int, kind, imageURI string, envVars map[string]string, timeoutSecs int, scenarioName string) error {
	jobID, err := createManagedJob(ctx, iter, kind, imageURI, 1, timeoutSecs)
	if err != nil {
		return err
	}
	defer deleteJob(ctx, jobID)

	payload := map[string]any{}
	for k, v := range envVars {
		payload[k] = v
	}

	code, result, raw, err := apiCall("POST", "/v1/jobs/"+jobID+"/trigger", map[string]any{
		"payload": payload,
	}, ctx.apiKey)
	if err != nil {
		return err
	}
	if code != 201 {
		return reportBug(ctx, scenarioName, iter, "trigger managed job failed", "POST trigger", code, raw)
	}
	runID := str(result["id"])

	// Wait for completion with longer timeout for managed jobs.
	return waitForRunTerminal(ctx, runID, iter, scenarioName, time.Duration(timeoutSecs+30)*time.Second, []string{"completed"})
}

func waitForRunTerminal(ctx *testCtx, runID string, iter int, scenarioName string, timeout time.Duration, acceptStatuses []string) error {
	deadline := time.Now().Add(timeout)
	accepted := make(map[string]bool)
	for _, s := range acceptStatuses {
		accepted[s] = true
	}

	for time.Now().Before(deadline) {
		code, runResult, _, err := apiCall("GET", "/v1/runs/"+runID, nil, ctx.apiKey)
		if err != nil {
			return err
		}
		if code != 200 {
			time.Sleep(2 * time.Second)
			continue
		}
		status := str(runResult["status"])

		// Terminal statuses.
		switch status {
		case "completed", "failed", "timed_out", "crashed", "system_failed", "canceled", "expired", "dead_letter":
			if accepted[status] {
				return nil
			}
			return reportBug(ctx, scenarioName, iter,
				fmt.Sprintf("run finished with unexpected status %s (wanted %v)", status, acceptStatuses),
				"GET run", code, truncate(fmt.Sprintf("%v", runResult), 300))
		}

		time.Sleep(2 * time.Second)
	}
	return reportBug(ctx, scenarioName, iter, "run did not reach terminal state within timeout", "GET run", 0, "")
}

// --------------------------------------------------------------------------.
// Phase 2 entry point (called from main if PHASE=2).
// --------------------------------------------------------------------------.

func runPhase2(ctx *testCtx, iters, conc int) {
	log.Printf("=== Phase 2: Managed Execution & Deep Integration ===")
	log.Printf("Scenarios: %d", len(phase2Scenarios))
	log.Printf("Iterations: %d", iters)
	log.Printf("Concurrency: %d", conc)

	startTime := time.Now()
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup

	for i := 1; i <= iters; i++ {
		s := phase2Scenarios[mathrand.Intn(len(phase2Scenarios))] //nolint:gosec // G404: test code.

		wg.Add(1)
		sem <- struct{}{}
		go func(s scenario, iter int) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[PANIC] scenario=%s iter=%d: %v", s.Name, iter, r)
					stats.scenariosFail.Add(1)
					_ = reportBug(ctx, s.Name, iter, fmt.Sprintf("panic: %v", r), "N/A", 0, "")
				}
			}()

			stats.scenariosRun.Add(1)
			err := s.Fn(ctx, iter)
			if err != nil {
				stats.scenariosFail.Add(1)
				trackResult(s.Name, false)
			} else {
				stats.scenariosOK.Add(1)
				trackResult(s.Name, true)
			}

			run := stats.scenariosRun.Load()
			if run%50 == 0 {
				elapsed := time.Since(startTime)
				rate := float64(run) / elapsed.Seconds()
				log.Printf("[PROGRESS] %d/%d scenarios (%.1f/s) | OK=%d FAIL=%d BUGS=%d | API calls=%d",
					run, iters, rate,
					stats.scenariosOK.Load(), stats.scenariosFail.Load(), stats.bugsFound.Load(),
					stats.apiCalls.Load())
			}
		}(s, i)
	}
	wg.Wait()
}
