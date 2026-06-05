// Package integration runs end-to-end tests against a live Strait service.
// It spins up a local HTTP server that acts as a job endpoint (simulating
// what a real user's HTTP-mode job endpoint does), receives dispatches from
// the Strait worker, calls back SDK endpoints, and completes.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	straitBase  = "http://localhost:8080"
	projectID   = "test-project"
	testTimeout = 120 * time.Second
)

// straitClient is an authenticated HTTP client for the Strait API.
type straitClient struct {
	apiKey string
	http   *http.Client
}

func newStraitClient(t *testing.T) *straitClient {
	t.Helper()
	key, err := os.ReadFile("/tmp/strait-test-api-key")
	if err != nil {
		t.Skipf("no api key at /tmp/strait-test-api-key — run smoke_test.sh first: %v", err)
	}
	return &straitClient{
		apiKey: string(bytes.TrimSpace(key)),
		http:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *straitClient) do(method, path string, body any) (*http.Response, []byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, straitBase+path, reqBody)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data, nil
}

// sdkClient calls SDK endpoints using the run token (like an SDK inside a job).
type sdkClient struct {
	runToken string
	runID    string
	http     *http.Client
}

func (s *sdkClient) post(path string, body any) (int, []byte, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", straitBase+"/sdk/v1/runs/"+s.runID+path, bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.runToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data, nil
}

func (s *sdkClient) get(path string) (int, []byte, error) {
	req, err := http.NewRequest("GET", straitBase+"/sdk/v1/runs/"+s.runID+path, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.runToken)
	resp, err := s.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data, nil
}

// jobEndpoint is a local HTTP server that simulates a customer's HTTP-mode job
// endpoint. When Strait dispatches to it, it exercises every SDK callback.
type jobEndpoint struct {
	server   *http.Server
	addr     string
	runPath  string
	mu       sync.Mutex
	received atomic.Int32
	errors   []string
}

func startJobEndpoint(t *testing.T) *jobEndpoint {
	t.Helper()

	var concWG conc.WaitGroup
	ep := &jobEndpoint{}
	mux := http.NewServeMux()
	// Use a unique path to avoid dispatch interference between tests.
	runPath := fmt.Sprintf("/run-%d", time.Now().UnixNano())
	ep.runPath = runPath
	mux.HandleFunc(runPath, ep.handleRun)
	ep.server = &http.Server{Handler: mux}

	// Listen on an allowed port (Strait validates endpoint URLs).
	// Use the machine's hostname so the URL passes the "no localhost" check.
	// Try allowed ports in order until one is free.
	allowedPorts := []string{"9000", "4000", "5000", "3000"}
	var listener net.Listener
	for _, p := range allowedPorts {
		l, listenErr := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "0.0.0.0:"+p)
		if listenErr == nil {
			listener = l
			break
		}
	}
	require.NotNil(t, listener)

	hostname, _ := os.Hostname()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	ep.addr = hostname + ":" + port
	concWG.Go(func() { _ = ep.server.Serve(listener) })
	t.Cleanup(func() {
		_ = ep.server.Shutdown(context.Background())
		concWG.Wait()
	})
	return ep
}

func (ep *jobEndpoint) handleRun(w http.ResponseWriter, r *http.Request) {
	ep.received.Add(1)

	runID := r.Header.Get("X-Run-ID")
	runToken := r.Header.Get("X-Run-Token")
	if runID == "" {
		ep.addError("missing X-Run-ID header")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If no run token, the worker may not have JWT signing configured.
	// Still return 200 so the run completes via HTTP response, but skip SDK calls.
	if runToken == "" {
		ep.addError(fmt.Sprintf("[%s] missing X-Run-Token — skipping SDK callbacks", runID))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"skipped_sdk":true}`))
		return
	}

	sdk := &sdkClient{runToken: runToken, runID: runID, http: &http.Client{Timeout: 10 * time.Second}}

	// 1. Report heartbeat
	if code, _, err := sdk.post("/heartbeat", map[string]any{}); err != nil || code >= 300 {
		ep.addError(fmt.Sprintf("[%s] heartbeat failed: code=%d err=%v", runID, code, err))
	}

	// 2. Report resource snapshot
	if code, _, err := sdk.post("/resource-snapshot", map[string]any{
		"cpu_percent":      35.5 + rand.Float64()*30,
		"memory_mb":        256 + rand.Float64()*512,
		"memory_limit_mb":  1024,
		"network_rx_bytes": rand.IntN(100000),
		"network_tx_bytes": rand.IntN(50000),
	}); err != nil || code >= 300 {
		ep.addError(fmt.Sprintf("[%s] resource-snapshot failed: code=%d err=%v", runID, code, err))
	}

	// 3. Set run state (existing KV per-run)
	if code, _, err := sdk.post("/state", map[string]any{
		"key":   "progress",
		"value": map[string]any{"step": 1, "total": 3},
	}); err != nil || code >= 300 {
		ep.addError(fmt.Sprintf("[%s] set-state failed: code=%d err=%v", runID, code, err))
	}

	// 4. Read state back
	if code, _, err := sdk.get("/state/progress"); err != nil || code >= 300 {
		ep.addError(fmt.Sprintf("[%s] get-state failed: code=%d err=%v", runID, code, err))
	}

	// 5. Write persistent memory
	if code, _, err := sdk.post("/memory/last-query", map[string]any{
		"value":    map[string]any{"query": "test", "ts": time.Now().Unix()},
		"ttl_secs": 3600,
	}); err != nil || code >= 300 {
		ep.addError(fmt.Sprintf("[%s] set-memory failed: code=%d err=%v", runID, code, err))
	}

	// 6. Read persistent memory back
	if code, _, err := sdk.get("/memory/last-query"); err != nil || code >= 300 {
		ep.addError(fmt.Sprintf("[%s] get-memory failed: code=%d err=%v", runID, code, err))
	}

	// 7. Write output
	if code, _, err := sdk.post("/output", map[string]any{
		"output_key": "result",
		"value":      map[string]any{"answer": "42"},
	}); err != nil || code >= 300 {
		ep.addError(fmt.Sprintf("[%s] output failed: code=%d err=%v", runID, code, err))
	}

	// 10. Log
	if code, _, err := sdk.post("/log", map[string]any{
		"level":   "info",
		"message": "job completed successfully",
	}); err != nil || code >= 300 {
		ep.addError(fmt.Sprintf("[%s] log failed: code=%d err=%v", runID, code, err))
	}

	// 11. Complete the run
	if code, body, err := sdk.post("/complete", map[string]any{
		"result": map[string]any{"status": "done", "items_processed": 42},
	}); err != nil || code >= 300 {
		ep.addError(fmt.Sprintf("[%s] complete failed: code=%d body=%s err=%v", runID, code, string(body), err))
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (ep *jobEndpoint) addError(msg string) {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	ep.errors = append(ep.errors, msg)
}

func (ep *jobEndpoint) getErrors() []string {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	return append([]string(nil), ep.errors...)
}

// TestEndToEndJobExecution creates a real job, triggers it, and verifies the
// full lifecycle: dispatch → SDK callbacks → completion → verify run state.
func TestEndToEndJobExecution(t *testing.T) {
	if os.Getenv("STRAIT_E2E") == "" {
		t.Skip("set STRAIT_E2E=1 to run end-to-end tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	client := newStraitClient(t)

	// Start local job endpoint.
	ep := startJobEndpoint(t)
	endpointURL := fmt.Sprintf("http://%s%s", ep.addr, ep.runPath)
	t.Logf("Job endpoint listening at %s", endpointURL)

	// Create job pointing to our local endpoint.
	slug := fmt.Sprintf("e2e-job-%d", time.Now().UnixNano())
	resp, body, err := client.do("POST", "/v1/jobs", map[string]any{
		"project_id":   projectID,
		"slug":         slug,
		"name":         "E2E Test Job",
		"endpoint_url": endpointURL,
		"timeout_secs": 30,
		"max_attempts": 1,
	})
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode)

	var job struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &job)
	t.Logf("Created job %s (%s)", job.ID, slug)

	// Trigger 5 runs to test concurrent execution.
	const numRuns = 5
	runIDs := make([]string, 0, numRuns)
	for i := range numRuns {
		resp, body, err := client.do("POST", fmt.Sprintf("/v1/jobs/%s/trigger", job.ID), map[string]any{
			"payload": map[string]any{"iteration": i},
		})
		require.NoError(t, err)
		require.Equal(t, 201, resp.StatusCode)

		var run struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body, &run)
		runIDs = append(runIDs, run.ID)
	}
	t.Logf("Triggered %d runs: %v", numRuns, runIDs)

	// Poll until all runs reach terminal state.
	deadline := time.Now().Add(60 * time.Second)
	for {
		require.False(t, time.Now().After(deadline))

		allDone := true
		for _, id := range runIDs {
			resp, body, err := client.do("GET", "/v1/runs/"+id, nil)
			require.NoError(t, err)

			var run struct {
				Status     string `json:"status"`
				Error      string `json:"error"`
				ErrorClass string `json:"error_class"`
			}
			_ = json.Unmarshal(body, &run)
			_ = resp

			if run.Status != "completed" && run.Status != "failed" && run.Status != "dead_letter" {
				allDone = false
				break
			}
		}
		if allDone {
			break
		}
		select {
		case <-ctx.Done():
			require.Fail(t, "context cancelled waiting for runs")
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Verify results.
	completed := 0
	failed := 0
	for _, id := range runIDs {
		_, body, _ := client.do("GET", "/v1/runs/"+id, nil)
		var run struct {
			Status string          `json:"status"`
			Result json.RawMessage `json:"result"`
		}
		_ = json.Unmarshal(body, &run)

		if run.Status == "completed" {
			completed++

			// Verify SDK data was persisted. Responses may be paginated
			// ({"data":[...]}) or bare arrays — try both.
			countItems := func(body []byte) int {
				var paginated struct {
					Data []json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(body, &paginated); err == nil && len(paginated.Data) > 0 {
					return len(paginated.Data)
				}
				var arr []json.RawMessage
				if err := json.Unmarshal(body, &arr); err == nil {
					return len(arr)
				}
				return 0
			}

			_, resBody, _ := client.do("GET", "/v1/runs/"+id+"/resources", nil)
			resourcesN := countItems(resBody)
			assert.NotEqual(t, 0, resourcesN)

			_, stateBody, _ := client.do("GET", "/v1/runs/"+id+"/state", nil)
			stateN := countItems(stateBody)
			assert.NotEqual(t, 0, stateN)

			_, outBody, _ := client.do("GET", "/v1/runs/"+id+"/outputs", nil)
			outputsN := countItems(outBody)
			assert.NotEqual(t, 0, outputsN)

			t.Logf("run %s: COMPLETED (resources=%d state=%d outputs=%d)",
				id, resourcesN, stateN, outputsN)
		} else {
			failed++
			t.Logf("run %s: %s", id, run.Status)
		}
	}

	// Check endpoint errors.
	epErrs := ep.getErrors()
	for _, e := range epErrs {
		assert.Failf(t, "test failure",

			"endpoint error: %s", e)
	}

	dispatched := int(ep.received.Load())
	t.Logf("Endpoint received %d dispatches", dispatched)
	t.Logf("Results: %d completed, %d failed, %d endpoint errors", completed, failed, len(epErrs))
	assert.Equal(t, numRuns, completed)
}

// TestEndToEndWorkflowExecution tests a 2-step workflow with real execution.
func TestEndToEndWorkflowExecution(t *testing.T) {
	if os.Getenv("STRAIT_E2E") == "" {
		t.Skip("set STRAIT_E2E=1 to run end-to-end tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	client := newStraitClient(t)
	ep := startJobEndpoint(t)
	endpointURL := fmt.Sprintf("http://%s%s", ep.addr, ep.runPath)

	slug := fmt.Sprintf("e2e-wf-job-%d", time.Now().UnixNano())
	resp, body, err := client.do("POST", "/v1/jobs", map[string]any{
		"project_id":   projectID,
		"slug":         slug,
		"name":         "E2E Workflow Job",
		"endpoint_url": endpointURL,
		"timeout_secs": 30,
		"max_attempts": 1,
	})
	require.False(t, err != nil || resp.StatusCode != 201)

	var job struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &job)

	wfSlug := fmt.Sprintf("e2e-wf-%d", time.Now().UnixNano())
	resp, body, err = client.do("POST", "/v1/workflows", map[string]any{
		"project_id": projectID,
		"slug":       wfSlug,
		"name":       "E2E Workflow",
		"steps": []map[string]any{
			{"step_ref": "step1", "step_type": "job", "job_id": job.ID, "depends_on": []string{}},
			{"step_ref": "step2", "step_type": "job", "job_id": job.ID, "depends_on": []string{"step1"}},
		},
	})
	require.False(t, err != nil || resp.StatusCode != 201)

	var wf struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &wf)
	t.Logf("Created workflow %s with 2 steps", wf.ID)

	resp, body, err = client.do("POST", fmt.Sprintf("/v1/workflows/%s/trigger", wf.ID), map[string]any{
		"payload": map[string]any{"workflow_test": true},
	})
	require.False(t, err != nil || resp.StatusCode != 201)

	var wfRun struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &wfRun)
	t.Logf("Triggered workflow run %s", wfRun.ID)

	// Poll workflow run until terminal.
	deadline := time.Now().Add(60 * time.Second)
	var finalStatus string
	for {
		require.False(t, time.Now().After(deadline))

		_, body, _ := client.do("GET", "/v1/workflow-runs/"+wfRun.ID, nil)
		var run struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(body, &run)
		finalStatus = run.Status

		if run.Status == "completed" || run.Status == "failed" || run.Status == "cancelled" {
			break
		}
		select {
		case <-ctx.Done():
			require.Fail(t, "context cancelled")
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Check step statuses.
	_, stepsBody, _ := client.do("GET", "/v1/workflow-runs/"+wfRun.ID+"/steps", nil)
	var steps struct {
		Data []struct {
			StepRef string `json:"step_ref"`
			Status  string `json:"status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(stepsBody, &steps)

	for _, s := range steps.Data {
		t.Logf("  step %s: %s", s.StepRef, s.Status)
	}

	dispatched := int(ep.received.Load())
	t.Logf("Workflow run %s: %s (endpoint dispatches: %d)", wfRun.ID, finalStatus, dispatched)
	assert.Equal(t, "completed", finalStatus)
	assert.GreaterOrEqual(t, dispatched, 2)

	epErrs := ep.getErrors()
	for _, e := range epErrs {
		assert.Failf(t, "test failure",

			"endpoint error: %s", e)
	}
}

// TestEndToEndStressLoop runs the job execution test repeatedly.
func TestEndToEndStressLoop(t *testing.T) {
	if os.Getenv("STRAIT_E2E") == "" {
		t.Skip("set STRAIT_E2E=1 to run end-to-end tests")
	}

	iterations := 20
	if v := os.Getenv("STRAIT_E2E_ITERATIONS"); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &iterations)
	}

	client := newStraitClient(t)
	ep := startJobEndpoint(t)
	endpointURL := fmt.Sprintf("http://%s%s", ep.addr, ep.runPath)

	slug := fmt.Sprintf("e2e-stress-%d", time.Now().UnixNano())
	resp, body, _ := client.do("POST", "/v1/jobs", map[string]any{
		"project_id":   projectID,
		"slug":         slug,
		"name":         "E2E Stress Job",
		"endpoint_url": endpointURL,
		"timeout_secs": 30,
		"max_attempts": 1,
	})
	require.Equal(t, 201, resp.StatusCode)

	var job struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &job)

	var totalCompleted, totalFailed atomic.Int32

	// Send runs in batches of 10 to avoid overwhelming the worker pool.
	batchSize := 10
	for batchStart := 0; batchStart < iterations; batchStart += batchSize {
		batchEnd := min(batchStart+batchSize, iterations)
		var wg conc.WaitGroup
		for i := batchStart; i < batchEnd; i++ {
			iter := i
			wg.Go(func() {
				resp, body, err := client.do("POST", fmt.Sprintf("/v1/jobs/%s/trigger", job.ID), map[string]any{
					"payload": map[string]any{"iter": iter},
				})
				if err != nil || resp.StatusCode != 201 {
					totalFailed.Add(1)
					t.Logf("trigger %d failed: %s", iter, string(body))
					return
				}
				var run struct {
					ID string `json:"id"`
				}
				_ = json.Unmarshal(body, &run)

				// Poll for completion with generous timeout.
				deadline := time.Now().Add(60 * time.Second)
				for time.Now().Before(deadline) {
					_, b, _ := client.do("GET", "/v1/runs/"+run.ID, nil)
					var r struct {
						Status string `json:"status"`
					}
					_ = json.Unmarshal(b, &r)
					if r.Status == "completed" {
						totalCompleted.Add(1)
						return
					}
					if r.Status == "failed" || r.Status == "dead_letter" {
						totalFailed.Add(1)
						t.Logf("run %s (iter %d): %s", run.ID, iter, r.Status)
						return
					}
					time.Sleep(300 * time.Millisecond)
				}
				totalFailed.Add(1)
				t.Logf("run %s (iter %d): timeout", run.ID, iter)
			})
		}
		wg.Wait()
	}

	completed := int(totalCompleted.Load())
	failed := int(totalFailed.Load())
	dispatched := int(ep.received.Load())
	t.Logf("Stress test: %d/%d completed, %d failed, %d dispatches, %d endpoint errors",
		completed, iterations, failed, dispatched, len(ep.getErrors()))

	for _, e := range ep.getErrors() {
		assert.Failf(t, "test failure",

			"endpoint error: %s", e)
	}
	assert.Equal(t, iterations, completed)
}
