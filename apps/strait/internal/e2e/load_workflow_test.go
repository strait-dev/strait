//go:build integration

package e2e_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadWorkflow_CreateThroughput(t *testing.T) {
	mustClean(t)
	projectID := "proj-lwf-create-" + fmt.Sprintf("%d", time.Now().UnixNano())
	volume := loadVolume()

	start := time.Now()
	for i := range volume {
		w := doRequest(t, "POST", "/v1/workflows/", fmt.Sprintf(
			`{"project_id":"%s","name":"load-wf-%d","slug":"load-wf-%d-%d","enabled":true}`,
			projectID, i, time.Now().UnixNano(), i,
		))
		if w.Code != 201 {
			t.Fatalf("create workflow %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	elapsed := time.Since(start)
	t.Logf("Created %d workflows in %v (%.0f/sec)", volume, elapsed, float64(volume)/elapsed.Seconds())
}

func TestLoadWorkflow_TriggerThroughput(t *testing.T) {
	mustClean(t)
	projectID := "proj-lwf-trig-" + fmt.Sprintf("%d", time.Now().UnixNano())
	volume := loadVolume() / 2

	w := doRequest(t, "POST", "/v1/workflows/", fmt.Sprintf(
		`{"project_id":"%s","name":"trig-wf","slug":"trig-wf-%d","enabled":true}`,
		projectID, time.Now().UnixNano(),
	))
	if w.Code != 201 {
		t.Fatalf("create workflow: %d", w.Code)
	}
	wfID := asString(t, mustDecodeObject(t, w), "id")

	start := time.Now()
	triggered := 0
	for i := range volume {
		resp := doRequest(t, "POST", "/v1/workflows/"+wfID+"/trigger",
			fmt.Sprintf(`{"payload":{"i":%d}}`, i))
		if resp.Code == 201 {
			triggered++
		}
	}
	elapsed := time.Since(start)
	t.Logf("Triggered %d/%d workflow runs in %v (%.0f/sec)",
		triggered, volume, elapsed, float64(triggered)/elapsed.Seconds())
}

func TestLoadWorkflow_ConcurrentTrigger(t *testing.T) {
	mustClean(t)
	projectID := "proj-lwf-ctrig-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/workflows/", fmt.Sprintf(
		`{"project_id":"%s","name":"ctrig-wf","slug":"ctrig-wf-%d","enabled":true}`,
		projectID, time.Now().UnixNano(),
	))
	if w.Code != 201 {
		t.Fatalf("create workflow: %d", w.Code)
	}
	wfID := asString(t, mustDecodeObject(t, w), "id")

	const workers = 10
	perWorker := loadVolume() / (workers * 2)
	var wg sync.WaitGroup
	var successes, failures atomic.Int64
	start := time.Now()

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perWorker {
				resp := doRequest(t, "POST", "/v1/workflows/"+wfID+"/trigger",
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

	t.Logf("Concurrent workflow triggers: %d successes, %d failures in %v (%.0f/sec)",
		successes.Load(), failures.Load(), elapsed, float64(total)/elapsed.Seconds())
}

func TestLoadWorkflow_ListRunsPaginated(t *testing.T) {
	mustClean(t)
	projectID := "proj-lwf-lruns-" + fmt.Sprintf("%d", time.Now().UnixNano())
	volume := loadVolume() / 5

	w := doRequest(t, "POST", "/v1/workflows/", fmt.Sprintf(
		`{"project_id":"%s","name":"lruns-wf","slug":"lruns-wf-%d","enabled":true}`,
		projectID, time.Now().UnixNano(),
	))
	if w.Code != 201 {
		t.Fatalf("create workflow: %d", w.Code)
	}
	wfID := asString(t, mustDecodeObject(t, w), "id")

	for i := range volume {
		doRequest(t, "POST", "/v1/workflows/"+wfID+"/trigger",
			fmt.Sprintf(`{"payload":{"i":%d}}`, i))
	}

	start := time.Now()
	const iterations = 50
	for range iterations {
		resp := doRequest(t, "GET", "/v1/workflows/"+wfID+"/runs?limit=50", "")
		if resp.Code != 200 {
			t.Fatalf("list workflow runs: %d", resp.Code)
		}
	}
	elapsed := time.Since(start)
	t.Logf("Listed workflow runs %d times in %v (%.0f/sec)", iterations, elapsed, float64(iterations)/elapsed.Seconds())
}

func TestLoadWorkflow_MultiWorkflowCreation(t *testing.T) {
	mustClean(t)
	const projectCount = 5
	const wfPerProject = 20

	var wg sync.WaitGroup
	var total atomic.Int64
	start := time.Now()

	for p := range projectCount {
		wg.Add(1)
		go func(projIdx int) {
			defer wg.Done()
			projectID := fmt.Sprintf("proj-lwf-multi-%d-%d", time.Now().UnixNano(), projIdx)
			for i := range wfPerProject {
				resp := doRequest(t, "POST", "/v1/workflows/", fmt.Sprintf(
					`{"project_id":"%s","name":"mwf-%d","slug":"mwf-%d-%d-%d","enabled":true}`,
					projectID, i, time.Now().UnixNano(), projIdx, i,
				))
				if resp.Code == 201 {
					total.Add(1)
				}
			}
		}(p)
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Multi-project workflow creation: %d workflows across %d projects in %v (%.0f/sec)",
		total.Load(), projectCount, elapsed, float64(total.Load())/elapsed.Seconds())
}
