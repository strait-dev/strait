package integration

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTestMassiveParallel(t *testing.T) {
	if os.Getenv("STRAIT_E2E") == "" {
		t.Skip("set STRAIT_E2E=1 to run")
	}

	client := newStraitClient(t)
	ep := startJobEndpoint(t)
	endpointURL := fmt.Sprintf("http://%s%s", ep.addr, ep.runPath)

	const (
		numJobs     = 10 // create 10 different jobs
		runsPerJob  = 30 // 30 runs per job = 300 total
		totalRuns   = numJobs * runsPerJob
		pollTimeout = 300 * time.Second
		batchSize   = 20 // trigger 20 at a time
	)

	// Track memory before.
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	startTime := time.Now()

	// Create jobs.
	jobIDs := make([]string, 0, numJobs)
	for i := range numJobs {
		slug := fmt.Sprintf("load-job-%d-%d", time.Now().UnixNano(), i)
		resp, body, err := client.do("POST", "/v1/jobs", map[string]any{
			"project_id":   projectID,
			"slug":         slug,
			"name":         fmt.Sprintf("Load Test Job %d", i),
			"endpoint_url": endpointURL,
			"timeout_secs": 60,
			"max_attempts": 1,
		})
		require.False(t, err != nil ||
			resp.
				StatusCode !=
				201)

		var job struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body, &job)
		jobIDs = append(jobIDs, job.ID)
	}
	t.Logf("Created %d jobs", numJobs)

	// Fire all runs in batches.
	type runInfo struct {
		ID    string
		JobID string
		Index int
	}
	allRuns := make([]runInfo, 0, totalRuns)
	var triggerMu sync.Mutex
	var triggerFailed atomic.Int32

	for batch := 0; batch < totalRuns; batch += batchSize {
		end := min(batch+batchSize, totalRuns)
		var wg conc.WaitGroup
		for i := batch; i < end; i++ {
			idx := i
			wg.Go(func() {
				jobIdx := idx % numJobs
				jobID := jobIDs[jobIdx]

				// Vary payload sizes to stress serialization.
				payloadSize := 100 + rand.IntN(4000)
				payload := map[string]any{
					"index":  idx,
					"job":    jobIdx,
					"data":   strings.Repeat("x", payloadSize),
					"nested": map[string]any{"a": 1, "b": []int{1, 2, 3}},
				}

				resp, body, err := client.do("POST", fmt.Sprintf("/v1/jobs/%s/trigger", jobID), map[string]any{
					"payload": payload,
				})
				if err != nil || resp.StatusCode != 201 {
					triggerFailed.Add(1)
					return
				}
				var run struct {
					ID string `json:"id"`
				}
				_ = json.Unmarshal(body, &run)

				triggerMu.Lock()
				allRuns = append(allRuns, runInfo{ID: run.ID, JobID: jobID, Index: idx})
				triggerMu.Unlock()
			})
		}
		wg.Wait()
	}

	triggered := len(allRuns)
	t.Logf("Triggered %d/%d runs (%d trigger failures) in %v",
		triggered, totalRuns, triggerFailed.Load(), time.Since(startTime).Round(time.Millisecond))
	require.NotEqual(t, 0, triggered)

	// Poll all runs for completion.
	statusCounts := make(map[string]int)
	var statusMu sync.Mutex
	deadline := time.Now().Add(pollTimeout)

	var pollWg conc.WaitGroup
	// Poll in batches of 50 to avoid overwhelming the API.
	sem := make(chan struct{}, 50)
	for _, r := range allRuns {
		ri := r
		sem <- struct{}{}
		pollWg.Go(func() {
			defer func() { <-sem }()

			for time.Now().Before(deadline) {
				_, body, err := client.do("GET", "/v1/runs/"+ri.ID, nil)
				if err != nil {
					time.Sleep(500 * time.Millisecond)
					continue
				}
				var run struct {
					Status string `json:"status"`
					Error  string `json:"error"`
				}
				_ = json.Unmarshal(body, &run)

				if run.Status == "completed" || run.Status == "failed" || run.Status == "dead_letter" {
					statusMu.Lock()
					statusCounts[run.Status]++
					statusMu.Unlock()
					return
				}
				time.Sleep(300 * time.Millisecond)
			}
			statusMu.Lock()
			statusCounts["timeout"]++
			statusMu.Unlock()
		})
	}
	pollWg.Wait()

	elapsed := time.Since(startTime)

	// Memory after.
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Service health + memory.
	_, healthBody, _ := client.do("GET", "/health/ready", nil)

	dispatched := int(ep.received.Load())
	epErrors := ep.getErrors()

	t.Logf("")
	t.Logf("=== Load Test Results ===")
	t.Logf("Jobs created:     %d", numJobs)
	t.Logf("Runs triggered:   %d/%d", triggered, totalRuns)
	t.Logf("Trigger failures: %d", triggerFailed.Load())
	t.Logf("Dispatches recv:  %d", dispatched)
	t.Logf("Endpoint errors:  %d", len(epErrors))
	t.Logf("Duration:         %v", elapsed.Round(time.Millisecond))
	t.Logf("Throughput:       %.1f runs/sec", float64(triggered)/elapsed.Seconds())
	t.Logf("")
	t.Logf("Status breakdown:")
	for status, count := range statusCounts {
		t.Logf("  %-15s %d", status, count)
	}
	t.Logf("")
	t.Logf("Test process memory:")
	t.Logf("  Alloc before:   %d MB", memBefore.Alloc/1024/1024)
	t.Logf("  Alloc after:    %d MB", memAfter.Alloc/1024/1024)
	t.Logf("  Total alloc:    %d MB", memAfter.TotalAlloc/1024/1024)
	t.Logf("  Num GC:         %d", memAfter.NumGC-memBefore.NumGC)
	t.Logf("")
	t.Logf("Service health:   %s", string(healthBody))

	// Print first 10 endpoint errors if any.
	if len(epErrors) > 0 {
		t.Logf("")
		t.Logf("First endpoint errors (max 10):")
		for i, e := range epErrors {
			if i >= 10 {
				t.Logf("  ... and %d more", len(epErrors)-10)
				break
			}
			t.Logf("  %s", e)
		}
	}

	// Assertions.
	completed := statusCounts["completed"]
	assert.GreaterOrEqual(t, completed,

		triggered*90/
			100)
	assert.LessOrEqual(t, statusCounts["timeout"], 0)
}

func TestLoadTestRapidFireSameJob(t *testing.T) {
	if os.Getenv("STRAIT_E2E") == "" {
		t.Skip("set STRAIT_E2E=1 to run")
	}

	client := newStraitClient(t)
	ep := startJobEndpoint(t)
	endpointURL := fmt.Sprintf("http://%s%s", ep.addr, ep.runPath)

	slug := fmt.Sprintf("rapid-fire-%d", time.Now().UnixNano())
	resp, body, _ := client.do("POST", "/v1/jobs", map[string]any{
		"project_id":   projectID,
		"slug":         slug,
		"name":         "Rapid Fire Job",
		"endpoint_url": endpointURL,
		"timeout_secs": 30,
		"max_attempts": 3,
	})
	require.Equal(t, 201, resp.
		StatusCode,
	)

	var job struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &job)

	// Fire 200 runs as fast as possible against the same job.
	const numRuns = 200
	runIDs := make([]string, 0, numRuns)
	var mu sync.Mutex
	var failed atomic.Int32

	start := time.Now()
	var wg conc.WaitGroup
	for i := range numRuns {
		idx := i
		wg.Go(func() {
			resp, body, err := client.do("POST", fmt.Sprintf("/v1/jobs/%s/trigger", job.ID), map[string]any{
				"payload": map[string]any{"i": idx},
			})
			if err != nil || resp.StatusCode != 201 {
				failed.Add(1)
				return
			}
			var run struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(body, &run)
			mu.Lock()
			runIDs = append(runIDs, run.ID)
			mu.Unlock()
		})
	}
	wg.Wait()
	triggerDuration := time.Since(start)

	t.Logf("Triggered %d/%d in %v (%.0f triggers/sec, %d failed)",
		len(runIDs), numRuns, triggerDuration.Round(time.Millisecond),
		float64(len(runIDs))/triggerDuration.Seconds(), failed.Load())

	// Wait for all to reach terminal state.
	statusCounts := make(map[string]int)
	deadline := time.Now().Add(180 * time.Second)

	for time.Now().Before(deadline) {
		allDone := true
		statusCounts = make(map[string]int) // reset each poll
		for _, id := range runIDs {
			_, body, _ := client.do("GET", "/v1/runs/"+id, nil)
			var run struct {
				Status string `json:"status"`
			}
			_ = json.Unmarshal(body, &run)
			statusCounts[run.Status]++
			if run.Status != "completed" && run.Status != "failed" && run.Status != "dead_letter" {
				allDone = false
			}
		}
		if allDone {
			break
		}
		time.Sleep(2 * time.Second)
	}

	elapsed := time.Since(start)
	dispatched := int(ep.received.Load())

	t.Logf("")
	t.Logf("=== Rapid-Fire Results ===")
	t.Logf("Total runs:       %d", len(runIDs))
	t.Logf("Dispatches:       %d", dispatched)
	t.Logf("Duration:         %v", elapsed.Round(time.Millisecond))
	t.Logf("Throughput:       %.1f runs/sec", float64(len(runIDs))/elapsed.Seconds())
	t.Logf("Status breakdown:")
	for status, count := range statusCounts {
		t.Logf("  %-15s %d", status, count)
	}
	t.Logf("Endpoint errors:  %d", len(ep.getErrors()))

	completed := statusCounts["completed"]
	assert.GreaterOrEqual(t, completed,

		len(runIDs)*
			85/100,
	)
}

func TestLoadTestWorkflowFanOut(t *testing.T) {
	if os.Getenv("STRAIT_E2E") == "" {
		t.Skip("set STRAIT_E2E=1 to run")
	}

	client := newStraitClient(t)
	ep := startJobEndpoint(t)
	endpointURL := fmt.Sprintf("http://%s%s", ep.addr, ep.runPath)

	// Create 3 jobs for the workflow.
	jobIDs := make([]string, 3)
	for i := range 3 {
		slug := fmt.Sprintf("wf-fan-%d-%d", time.Now().UnixNano(), i)
		resp, body, _ := client.do("POST", "/v1/jobs", map[string]any{
			"project_id":   projectID,
			"slug":         slug,
			"name":         fmt.Sprintf("WF Fan Job %d", i),
			"endpoint_url": endpointURL,
			"timeout_secs": 30,
			"max_attempts": 1,
		})
		require.Equal(t, 201, resp.
			StatusCode,
		)

		var job struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body, &job)
		jobIDs[i] = job.ID
	}

	// Create workflow: step1 → (step2a, step2b, step2c) → step3 (fan-out/fan-in).
	wfSlug := fmt.Sprintf("wf-fan-%d", time.Now().UnixNano())
	resp, body, _ := client.do("POST", "/v1/workflows", map[string]any{
		"project_id": projectID,
		"slug":       wfSlug,
		"name":       "Fan-Out Workflow",
		"steps": []map[string]any{
			{"step_ref": "init", "step_type": "job", "job_id": jobIDs[0], "depends_on": []string{}},
			{"step_ref": "fan-a", "step_type": "job", "job_id": jobIDs[1], "depends_on": []string{"init"}},
			{"step_ref": "fan-b", "step_type": "job", "job_id": jobIDs[1], "depends_on": []string{"init"}},
			{"step_ref": "fan-c", "step_type": "job", "job_id": jobIDs[2], "depends_on": []string{"init"}},
			{"step_ref": "merge", "step_type": "job", "job_id": jobIDs[0], "depends_on": []string{"fan-a", "fan-b", "fan-c"}},
		},
	})
	require.Equal(t, 201, resp.
		StatusCode,
	)

	var wf struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &wf)

	// Trigger 20 workflow runs concurrently.
	const numWfRuns = 20
	wfRunIDs := make([]string, 0, numWfRuns)
	var mu sync.Mutex
	var wg conc.WaitGroup

	start := time.Now()
	for i := range numWfRuns {
		idx := i
		wg.Go(func() {
			resp, body, _ := client.do("POST", fmt.Sprintf("/v1/workflows/%s/trigger", wf.ID), map[string]any{
				"payload": map[string]any{"wf_idx": idx},
			})
			if resp.StatusCode != 201 {
				return
			}
			var run struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(body, &run)
			mu.Lock()
			wfRunIDs = append(wfRunIDs, run.ID)
			mu.Unlock()
		})
	}
	wg.Wait()
	t.Logf("Triggered %d workflow runs (5 steps each = %d total job dispatches expected)", len(wfRunIDs), len(wfRunIDs)*5)

	// Poll workflow runs.
	statusCounts := make(map[string]int)
	deadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		allDone := true
		statusCounts = make(map[string]int)
		for _, id := range wfRunIDs {
			_, body, _ := client.do("GET", "/v1/workflow-runs/"+id, nil)
			var run struct {
				Status string `json:"status"`
			}
			_ = json.Unmarshal(body, &run)
			statusCounts[run.Status]++
			if run.Status != "completed" && run.Status != "failed" && run.Status != "cancelled" {
				allDone = false
			}
		}
		if allDone {
			break
		}
		time.Sleep(2 * time.Second)
	}

	elapsed := time.Since(start)
	dispatched := int(ep.received.Load())

	t.Logf("")
	t.Logf("=== Workflow Fan-Out Results ===")
	t.Logf("Workflow runs:    %d", len(wfRunIDs))
	t.Logf("Dispatches:       %d (expected %d)", dispatched, len(wfRunIDs)*5)
	t.Logf("Duration:         %v", elapsed.Round(time.Millisecond))
	t.Logf("Status breakdown:")
	for status, count := range statusCounts {
		t.Logf("  %-15s %d", status, count)
	}

	completed := statusCounts["completed"]
	assert.GreaterOrEqual(t, completed,

		len(wfRunIDs)*
			80/
			100)
}

func TestLoadTestAPIEndpointStress(t *testing.T) {
	if os.Getenv("STRAIT_E2E") == "" {
		t.Skip("set STRAIT_E2E=1 to run")
	}

	client := newStraitClient(t)

	// Hammer read-heavy endpoints concurrently.
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/v1/jobs"},
		{"GET", "/v1/runs"},
		{"GET", "/v1/runs?status=queued"},
		{"GET", "/v1/runs?error_class=timeout"},
		{"GET", "/v1/workflows"},
		{"GET", "/v1/workflow-runs"},
		{"GET", "/v1/events"},
		{"GET", "/v1/events/stats"},
		{"GET", "/v1/stats"},
		{"GET", "/v1/analytics/performance?period_hours=1"},
		{"GET", "/v1/regions"},
		{"GET", "/health"},
		{"GET", "/health/ready"},
	}

	const (
		concurrency = 50
		duration    = 10 * time.Second
	)

	type result struct {
		endpoint string
		count    int64
		errors   int64
		latP50   time.Duration
		latP99   time.Duration
	}

	results := make([]result, len(endpoints))
	var wg conc.WaitGroup

	start := time.Now()
	for epIdx, ep := range endpoints {
		idx, method, path := epIdx, ep.method, ep.path
		wg.Go(func() {
			var count, errors atomic.Int64
			var latencies []time.Duration
			var latMu sync.Mutex

			var innerWg conc.WaitGroup
			for range concurrency {
				innerWg.Go(func() {
					for time.Since(start) < duration {
						reqStart := time.Now()
						resp, _, err := client.do(method, path, nil)
						lat := time.Since(reqStart)

						count.Add(1)
						latMu.Lock()
						latencies = append(latencies, lat)
						latMu.Unlock()

						if err != nil || resp.StatusCode >= 500 {
							errors.Add(1)
						}
					}
				})
			}
			innerWg.Wait()

			slices.SortFunc(latencies, func(a, b time.Duration) int { return int(a - b) })
			var p50, p99 time.Duration
			if len(latencies) > 0 {
				p50 = latencies[len(latencies)*50/100]
				p99 = latencies[len(latencies)*99/100]
			}

			results[idx] = result{
				endpoint: fmt.Sprintf("%s %s", method, path),
				count:    count.Load(),
				errors:   errors.Load(),
				latP50:   p50,
				latP99:   p99,
			}
		})
	}
	wg.Wait()

	t.Logf("")
	t.Logf("=== API Endpoint Stress Results (%v, %d concurrent per endpoint) ===", duration, concurrency)
	t.Logf("%-50s %8s %8s %10s %10s", "ENDPOINT", "REQS", "ERRORS", "P50", "P99")
	t.Logf("%s", strings.Repeat("-", 90))

	var totalReqs, totalErrors int64
	for _, r := range results {
		t.Logf("%-50s %8d %8d %10v %10v", r.endpoint, r.count, r.errors, r.latP50.Round(time.Microsecond), r.latP99.Round(time.Microsecond))
		totalReqs += r.count
		totalErrors += r.errors
	}
	t.Logf("%s", strings.Repeat("-", 90))
	t.Logf("%-50s %8d %8d", "TOTAL", totalReqs, totalErrors)
	t.Logf("Overall RPS: %.0f", float64(totalReqs)/duration.Seconds())

	// Check service is still healthy after the stress.
	_, healthBody, _ := client.do("GET", "/health/ready", nil)
	t.Logf("")
	t.Logf("Post-stress health: %s", string(healthBody))

	errorRate := float64(totalErrors) / float64(totalReqs) * 100
	assert.LessOrEqual(t, errorRate,
		1.0,
	)

	// Check no endpoint has p99 > 10s (generous for dev with 25 DB conns under 650 concurrent queries).
	for _, r := range results {
		assert.LessOrEqual(t, r.latP99,
			10*
				time.Second)
	}
}

// startJobEndpoint for load tests needs to handle concurrent requests.
// The base startJobEndpoint from e2e_test.go is reused.
// We just need the net import to not conflict.
var _ = net.Listen
