//go:build integration

package e2e_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadWebhook_SubscriptionCRUD(t *testing.T) {
	mustClean(t)
	projectID := "proj-lwh-crud-" + fmt.Sprintf("%d", time.Now().UnixNano())
	volume := loadVolume() / 5

	start := time.Now()
	subIDs := make([]string, volume)
	for i := range volume {
		w := doRequest(t, "POST", "/v1/webhooks/subscriptions/", fmt.Sprintf(
			`{"project_id":"%s","webhook_url":"https://example.com/wh-%d","event_types":["run.completed"],"secret":"whsec-%d"}`,
			projectID, i, i,
		))
		if w.Code != 201 {
			t.Fatalf("create subscription %d: %d %s", i, w.Code, w.Body.String())
		}
		subIDs[i] = asString(t, mustDecodeObject(t, w), "id")
	}
	createElapsed := time.Since(start)
	t.Logf("Created %d subscriptions in %v (%.0f/sec)", volume, createElapsed, float64(volume)/createElapsed.Seconds())

	start = time.Now()
	listResp := doRequest(t, "GET", "/v1/webhooks/subscriptions/?project_id="+projectID, "")
	if listResp.Code != 200 {
		t.Fatalf("list subscriptions: %d", listResp.Code)
	}
	t.Logf("Listed subscriptions in %v", time.Since(start))

	start = time.Now()
	deleted := 0
	for _, id := range subIDs {
		w := doRequest(t, "DELETE", "/v1/webhooks/subscriptions/"+id, "")
		if w.Code == 200 || w.Code == 204 {
			deleted++
		}
	}
	deleteElapsed := time.Since(start)
	t.Logf("Deleted %d/%d subscriptions in %v (%.0f/sec)", deleted, volume, deleteElapsed, float64(deleted)/deleteElapsed.Seconds())
}

func TestLoadWebhook_DeliveryListing(t *testing.T) {
	mustClean(t)
	projectID := "proj-lwh-del-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"wh-deliv","slug":"wh-deliv-%d","endpoint_url":"https://example.com/wh","max_attempts":1,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	if w.Code != 201 {
		t.Fatalf("create job: %d", w.Code)
	}

	const iterations = 100
	start := time.Now()
	for range iterations {
		resp := doRequest(t, "GET", "/v1/webhooks/deliveries/?project_id="+projectID, "")
		if resp.Code != 200 {
			t.Fatalf("list deliveries: %d", resp.Code)
		}
	}
	elapsed := time.Since(start)
	t.Logf("Listed deliveries %d times in %v (%.0f/sec)", iterations, elapsed, float64(iterations)/elapsed.Seconds())
}

func TestLoadWebhook_ConcurrentSubscriptionCreation(t *testing.T) {
	mustClean(t)
	projectID := "proj-lwh-conc-" + fmt.Sprintf("%d", time.Now().UnixNano())

	const workers = 10
	perWorker := loadVolume() / (workers * 5)
	var wg sync.WaitGroup
	var successes, failures atomic.Int64
	start := time.Now()

	for w := range workers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := range perWorker {
				resp := doRequest(t, "POST", "/v1/webhooks/subscriptions/", fmt.Sprintf(
					`{"project_id":"%s","webhook_url":"https://example.com/wh-w%d-%d","event_types":["run.completed"],"secret":"whsec-w%d-%d"}`,
					projectID, workerID, i, workerID, i,
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

	t.Logf("Concurrent subscription creation: %d/%d succeeded in %v (%.0f/sec)",
		successes.Load(), total, elapsed, float64(total)/elapsed.Seconds())

	if failures.Load() > total/10 {
		t.Errorf("too many failures: %d/%d", failures.Load(), total)
	}
}
