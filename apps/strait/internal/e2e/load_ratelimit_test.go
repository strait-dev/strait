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

func TestLoadRateLimit_BurstTrigger(t *testing.T) {
	mustClean(t)
	projectID := "proj-lrl-burst-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"rl-burst","slug":"rl-burst-%d","endpoint_url":"https://example.com/rl","max_attempts":1,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	const burst = 200
	var successes, failures atomic.Int64
	var wg conc.WaitGroup
	start := time.Now()

	for range burst {
		wg.Go(func() {
			resp := doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger", `{"payload":{"burst":true}}`)
			if resp.Code == 201 {
				successes.Add(1)
			} else {
				failures.Add(1)
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Burst trigger: %d successes, %d failures in %v (%.0f/sec)",
		successes.Load(), failures.Load(), elapsed, float64(burst)/elapsed.Seconds())
}

func TestLoadRateLimit_SustainedRead(t *testing.T) {
	mustClean(t)
	projectID := "proj-lrl-read-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"rl-read","slug":"rl-read-%d","endpoint_url":"https://example.com/rl","max_attempts":1,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	const duration = 5 * time.Second
	var total, successes atomic.Int64
	var wg conc.WaitGroup

	deadline := time.Now().Add(duration)

	const workers = 10
	for range workers {
		wg.Go(func() {
			for time.Now().Before(deadline) {
				resp := doRequest(t, "GET", "/v1/jobs/", "", projectID)
				total.Add(1)
				if resp.Code == 200 {
					successes.Add(1)
				}
			}
		})
	}
	wg.Wait()

	t.Logf("Sustained read: %d/%d succeeded in %v (%.0f/sec)",
		successes.Load(), total.Load(), duration, float64(total.Load())/duration.Seconds())
	assert.GreaterOrEqual(
		t, successes.
			Load(), total.Load()/2)

}

func TestLoadRateLimit_MixedBurstReadWrite(t *testing.T) {
	mustClean(t)
	projectID := "proj-lrl-mix-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"rl-mix","slug":"rl-mix-%d","endpoint_url":"https://example.com/rl","max_attempts":1,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	const workers = 10
	const requestsPerWorker = 50
	var reads, writes, readSuccess, writeSuccess atomic.Int64
	var wg conc.WaitGroup

	for range workers {
		wg.Go(func() {
			for i := range requestsPerWorker {
				if i%3 == 0 {
					writes.Add(1)
					resp := doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger",
						fmt.Sprintf(`{"payload":{"i":%d}}`, i))
					if resp.Code == 201 {
						writeSuccess.Add(1)
					}
				} else {
					reads.Add(1)
					resp := doRequest(t, "GET", "/v1/jobs/", "", projectID)
					if resp.Code == 200 {
						readSuccess.Add(1)
					}
				}
			}
		})
	}
	wg.Wait()

	t.Logf("Mixed burst: reads=%d/%d writes=%d/%d",
		readSuccess.Load(), reads.Load(), writeSuccess.Load(), writes.Load())
}
