//go:build integration

package e2e_test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadWebhook_SubscriptionCRUD(t *testing.T) {
	mustClean(t)
	projectID := "proj-lwh-crud-" + fmt.Sprintf("%d", time.Now().UnixNano())
	volume := loadVolume() / 5

	start := time.Now()
	subIDs := make([]string, volume)
	for i := range volume {
		w := doRequest(t, "POST", "/v1/webhooks/subscriptions/", fmt.Sprintf(
			`{"project_id":"%s","webhook_url":"https://example.com/wh-%d","event_types":["run.completed"]}`,
			projectID, i,
		))
		require.EqualValues(t, 201,

			w.Code)

		resp := mustDecodeObject(t, w)
		sub, ok := resp["subscription"].(map[string]any)
		require.True(t, ok)

		subIDs[i] = asString(t, sub, "id")
	}
	createElapsed := time.Since(start)
	t.Logf("Created %d subscriptions in %v (%.0f/sec)", volume, createElapsed, float64(volume)/createElapsed.Seconds())

	start = time.Now()
	listResp := doRequest(t, "GET", "/v1/webhooks/subscriptions/", "", projectID)
	require.EqualValues(t, 200,

		listResp.
			Code)

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
	require.EqualValues(t, 201,

		w.Code)

	const iterations = 100
	start := time.Now()
	for range iterations {
		resp := doRequest(t, "GET", "/v1/webhooks/deliveries/", "", projectID)
		require.EqualValues(t, 200,

			resp.Code,
		)

	}
	elapsed := time.Since(start)
	t.Logf("Listed deliveries %d times in %v (%.0f/sec)", iterations, elapsed, float64(iterations)/elapsed.Seconds())
}

func TestLoadWebhook_ConcurrentSubscriptionCreation(t *testing.T) {
	mustClean(t)
	projectID := "proj-lwh-conc-" + fmt.Sprintf("%d", time.Now().UnixNano())

	const workers = 10
	perWorker := loadVolume() / (workers * 5)
	var wg conc.WaitGroup
	var successes, failures atomic.Int64
	start := time.Now()

	for w := range workers {
		workerID := w
		wg.Go(func() {
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
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	total := int64(workers * perWorker)

	t.Logf("Concurrent subscription creation: %d/%d succeeded in %v (%.0f/sec)",
		successes.Load(), total, elapsed, float64(total)/elapsed.Seconds())
	assert.LessOrEqual(t,

		failures.
			Load(),
		total/10)

}
