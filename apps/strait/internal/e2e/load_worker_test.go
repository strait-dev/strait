//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestLoadWorker_BulkTriggerThroughput(t *testing.T) {
	mustClean(t)
	projectID := "proj-lw-bulk-" + fmt.Sprintf("%d", time.Now().UnixNano())
	volume := loadVolume()

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"worker-bulk","slug":"worker-bulk-%d","endpoint_url":"https://example.com/bulk","max_attempts":1,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	if w.Code != 201 {
		t.Fatalf("create job: %d %s", w.Code, w.Body.String())
	}
	jobID := asString(t, mustDecodeObject(t, w), "id")

	start := time.Now()
	for i := range volume {
		resp := doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger",
			fmt.Sprintf(`{"payload":{"i":%d}}`, i))
		if resp.Code != 201 {
			t.Fatalf("trigger %d: %d %s", i, resp.Code, resp.Body.String())
		}
	}
	elapsed := time.Since(start)
	t.Logf("Triggered %d runs in %v (%.0f/sec)", volume, elapsed, float64(volume)/elapsed.Seconds())
}

func TestLoadWorker_ConcurrentTriggers(t *testing.T) {
	mustClean(t)
	projectID := "proj-lw-conc-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"worker-conc","slug":"worker-conc-%d","endpoint_url":"https://example.com/conc","max_attempts":1,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	if w.Code != 201 {
		t.Fatalf("create job: %d %s", w.Code, w.Body.String())
	}
	jobID := asString(t, mustDecodeObject(t, w), "id")

	const workers = 20
	perWorker := loadVolume() / workers
	var wg sync.WaitGroup
	var successes, failures atomic.Int64
	start := time.Now()

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perWorker {
				resp := doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger",
					fmt.Sprintf(`{"payload":{"i":%d}}`, i))
				if resp.Code == 201 {
					successes.Add(1)
				} else {
					failures.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	total := int64(workers * perWorker)

	t.Logf("Concurrent triggers: %d successes, %d failures in %v (%.0f/sec)",
		successes.Load(), failures.Load(), elapsed, float64(total)/elapsed.Seconds())

	if failures.Load() > total/10 {
		t.Errorf("too many failures: %d/%d", failures.Load(), total)
	}
}

func TestLoadWorker_SDKHeartbeatFlood(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	projectID := "proj-lw-hb-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"worker-hb","slug":"worker-hb-%d","endpoint_url":"https://example.com/hb","max_attempts":1,"timeout_secs":60}`,
		projectID, time.Now().UnixNano(),
	))
	if w.Code != 201 {
		t.Fatalf("create job: %d %s", w.Code, w.Body.String())
	}
	jobID := asString(t, mustDecodeObject(t, w), "id")

	trigResp := doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger", `{"payload":{}}`)
	if trigResp.Code != 201 {
		t.Fatalf("trigger: %d", trigResp.Code)
	}
	runData := mustDecodeObject(t, trigResp)
	runID := asString(t, runData, "id")
	runToken := asString(t, runData, "run_token")

	err := testStore.UpdateRunStatus(ctx, runID, domain.StatusQueued, domain.StatusDequeued, map[string]any{
		"started_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("transition dequeued: %v", err)
	}
	err = testStore.UpdateRunStatus(ctx, runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{})
	if err != nil {
		t.Fatalf("transition executing: %v", err)
	}

	const heartbeats = 1000
	const workers = 10
	var wg sync.WaitGroup
	var successes atomic.Int64
	start := time.Now()

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range heartbeats / workers {
				resp := doSDKRequest(t, "POST", "/sdk/v1/runs/"+runID+"/heartbeat", runToken, "")
				if resp.Code == 200 {
					successes.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Heartbeat flood: %d/%d succeeded in %v (%.0f/sec)",
		successes.Load(), heartbeats, elapsed, float64(heartbeats)/elapsed.Seconds())
}

func TestLoadWorker_RunCreationAndListing(t *testing.T) {
	mustClean(t)
	projectID := "proj-lw-list-" + fmt.Sprintf("%d", time.Now().UnixNano())
	volume := loadVolume()

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"worker-list","slug":"worker-list-%d","endpoint_url":"https://example.com/list","max_attempts":1,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	if w.Code != 201 {
		t.Fatalf("create job: %d %s", w.Code, w.Body.String())
	}
	jobID := asString(t, mustDecodeObject(t, w), "id")

	for i := range volume {
		doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger", fmt.Sprintf(`{"payload":{"i":%d}}`, i))
	}

	start := time.Now()
	var totalListed int
	var requests int
	cursor := ""
	for {
		path := "/v1/runs/?limit=100"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		resp := doRequest(t, "GET", path, "", projectID)
		if resp.Code != 200 {
			t.Fatalf("list runs: %d", resp.Code)
		}

		var listResp struct {
			Data []map[string]any `json:"data"`
			Meta struct {
				NextCursor string `json:"next_cursor"`
			} `json:"meta"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			t.Fatalf("decode list: %v", err)
		}

		totalListed += len(listResp.Data)
		requests++

		if listResp.Meta.NextCursor == "" || len(listResp.Data) == 0 {
			break
		}
		cursor = listResp.Meta.NextCursor
	}
	elapsed := time.Since(start)
	t.Logf("Listed %d runs in %d pages in %v (%.0f runs/sec)",
		totalListed, requests, elapsed, float64(totalListed)/elapsed.Seconds())
}
