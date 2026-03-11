//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestLoadStore_CreateJobThroughput(t *testing.T) {
	mustClean(t)
	volume := loadVolume()
	projectID := "proj-ls-cjob-" + fmt.Sprintf("%d", time.Now().UnixNano())

	start := time.Now()
	for i := range volume {
		w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
			`{"project_id":"%s","name":"store-job-%d","slug":"store-job-%d-%d","endpoint_url":"https://example.com/store","max_attempts":1,"timeout_secs":30}`,
			projectID, i, time.Now().UnixNano(), i,
		))
		if w.Code != 201 {
			t.Fatalf("create job %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	elapsed := time.Since(start)
	t.Logf("Created %d jobs in %v (%.0f/sec)", volume, elapsed, float64(volume)/elapsed.Seconds())
}

func TestLoadStore_ConcurrentJobCreation(t *testing.T) {
	mustClean(t)
	projectID := "proj-ls-ccjob-" + fmt.Sprintf("%d", time.Now().UnixNano())

	const workers = 20
	perWorker := loadVolume() / workers
	var wg sync.WaitGroup
	var successes, failures atomic.Int64
	start := time.Now()

	for w := range workers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := range perWorker {
				resp := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
					`{"project_id":"%s","name":"cc-job-w%d-%d","slug":"cc-job-%d-w%d-%d","endpoint_url":"https://example.com/cc","max_attempts":1,"timeout_secs":30}`,
					projectID, workerID, i, time.Now().UnixNano(), workerID, i,
				))
				if resp.Code == 201 {
					successes.Add(1)
				} else {
					failures.Add(1)
				}
			}
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(start)
	total := int64(workers * perWorker)

	t.Logf("Concurrent job creation: %d/%d succeeded in %v (%.0f/sec)",
		successes.Load(), total, elapsed, float64(total)/elapsed.Seconds())

	if failures.Load() > total/10 {
		t.Errorf("too many failures: %d/%d", failures.Load(), total)
	}
}

func TestLoadStore_EventTriggerThroughput(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	volume := loadVolume()
	projectID := "proj-ls-evt-" + fmt.Sprintf("%d", time.Now().UnixNano())

	start := time.Now()
	for i := range volume {
		trigger := &domain.EventTrigger{
			ID:          fmt.Sprintf("ls-evt-%d-%d", time.Now().UnixNano(), i),
			EventKey:    fmt.Sprintf("ls:evt:%d:%d", time.Now().UnixNano(), i),
			ProjectID:   projectID,
			SourceType:  domain.EventSourceJobRun,
			TriggerType: "event",
			Status:      domain.EventTriggerStatusWaiting,
			TimeoutSecs: 3600,
			RequestedAt: time.Now(),
			ExpiresAt:   time.Now().Add(time.Hour),
		}
		if err := testStore.CreateEventTrigger(ctx, trigger); err != nil {
			t.Fatalf("create trigger %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	t.Logf("Created %d event triggers in %v (%.0f/sec)", volume, elapsed, float64(volume)/elapsed.Seconds())
}

func TestLoadStore_RunStatusTransitions(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	volume := loadVolume() / 5
	projectID := "proj-ls-trans-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"store-trans","slug":"store-trans-%d","endpoint_url":"https://example.com/trans","max_attempts":1,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	if w.Code != 201 {
		t.Fatalf("create job: %d", w.Code)
	}
	jobID := asString(t, mustDecodeObject(t, w), "id")

	runIDs := make([]string, volume)
	for i := range volume {
		resp := doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger",
			fmt.Sprintf(`{"payload":{"i":%d}}`, i))
		if resp.Code == 201 {
			runIDs[i] = asString(t, mustDecodeObject(t, resp), "id")
		}
	}

	start := time.Now()
	transitioned := 0
	for _, id := range runIDs {
		if id == "" {
			continue
		}
		err := testStore.UpdateRunStatus(ctx, id, domain.StatusQueued, domain.StatusDequeued, map[string]any{
			"started_at": time.Now().UTC(),
		})
		if err != nil {
			continue
		}
		err = testStore.UpdateRunStatus(ctx, id, domain.StatusDequeued, domain.StatusExecuting, map[string]any{})
		if err != nil {
			continue
		}
		err = testStore.UpdateRunStatus(ctx, id, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
			"finished_at": time.Now().UTC(),
		})
		if err == nil {
			transitioned++
		}
	}
	elapsed := time.Since(start)
	t.Logf("Full FSM transitions (queued->completed): %d/%d in %v (%.0f/sec)",
		transitioned, volume, elapsed, float64(transitioned)/elapsed.Seconds())
}

func TestLoadStore_ConcurrentStatusTransitions(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	volume := loadVolume() / 5
	projectID := "proj-ls-ctrans-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"store-ctrans","slug":"store-ctrans-%d","endpoint_url":"https://example.com/ctrans","max_attempts":1,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	if w.Code != 201 {
		t.Fatalf("create job: %d", w.Code)
	}
	jobID := asString(t, mustDecodeObject(t, w), "id")

	runIDs := make([]string, volume)
	for i := range volume {
		resp := doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger",
			fmt.Sprintf(`{"payload":{"i":%d}}`, i))
		if resp.Code == 201 {
			runIDs[i] = asString(t, mustDecodeObject(t, resp), "id")
		}
	}

	const workers = 10
	var wg sync.WaitGroup
	var successes, failures atomic.Int64
	start := time.Now()

	chunkSize := len(runIDs) / workers
	for w := range workers {
		wg.Add(1)
		startIdx := w * chunkSize
		endIdx := startIdx + chunkSize
		if w == workers-1 {
			endIdx = len(runIDs)
		}
		go func(ids []string) {
			defer wg.Done()
			for _, id := range ids {
				if id == "" {
					continue
				}
				err := testStore.UpdateRunStatus(ctx, id, domain.StatusQueued, domain.StatusDequeued, map[string]any{
					"started_at": time.Now().UTC(),
				})
				if err == nil {
					err = testStore.UpdateRunStatus(ctx, id, domain.StatusDequeued, domain.StatusExecuting, map[string]any{})
				}
				if err == nil {
					successes.Add(1)
				} else {
					failures.Add(1)
				}
			}
		}(runIDs[startIdx:endIdx])
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Concurrent transitions: %d successes, %d failures in %v (%.0f/sec)",
		successes.Load(), failures.Load(), elapsed, float64(volume)/elapsed.Seconds())
}

func TestLoadStore_ListingWithPagination(t *testing.T) {
	mustClean(t)
	projectID := "proj-ls-page-" + fmt.Sprintf("%d", time.Now().UnixNano())
	volume := loadVolume()

	for i := range volume {
		doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
			`{"project_id":"%s","name":"page-job-%d","slug":"page-job-%d-%d","endpoint_url":"https://example.com/page","max_attempts":1,"timeout_secs":30}`,
			projectID, i, time.Now().UnixNano(), i,
		))
	}

	pageSizes := []int{10, 50, 100}
	for _, limit := range pageSizes {
		start := time.Now()
		pages := 0
		total := 0
		for {
			resp := doRequest(t, "GET", fmt.Sprintf("/v1/jobs/?project_id=%s&limit=%d&offset=%d", projectID, limit, total), "")
			if resp.Code != 200 {
				break
			}
			items := mustDecodeList(t, resp)
			total += len(items)
			pages++
			if len(items) < limit {
				break
			}
		}
		elapsed := time.Since(start)
		t.Logf("Pagination (limit=%d): %d items in %d pages in %v (%.0f items/sec)",
			limit, total, pages, elapsed, float64(total)/elapsed.Seconds())
	}
}
