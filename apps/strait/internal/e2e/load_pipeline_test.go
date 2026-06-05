//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestLoadPipeline_TriggerDequeueComplete(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	projectID := "proj-lp-full-" + fmt.Sprintf("%d", time.Now().UnixNano())
	volume := loadVolume() / 5

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"pipe-full","slug":"pipe-full-%d","endpoint_url":"https://example.com/pipe","max_attempts":1,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	start := time.Now()

	for i := range volume {
		resp := doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger",
			fmt.Sprintf(`{"payload":{"i":%d}}`, i))
		require.EqualValues(t, 201,

			resp.Code,
		)

	}
	triggerElapsed := time.Since(start)

	dequeueStart := time.Now()
	dequeuedRuns := dequeueRunsEventually(t, testQueue, volume)
	runIDs := make([]string, 0, len(dequeuedRuns))
	for _, run := range dequeuedRuns {
		runIDs = append(runIDs, run.ID)
	}
	dequeueElapsed := time.Since(dequeueStart)

	completeStart := time.Now()
	completed := 0
	for _, id := range runIDs {
		err := testStore.UpdateRunStatus(ctx, id, domain.StatusDequeued, domain.StatusExecuting, map[string]any{})
		if err != nil {
			continue
		}
		err = testStore.UpdateRunStatus(ctx, id, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
			"finished_at": time.Now().UTC(),
		})
		if err == nil {
			completed++
		}
	}
	completeElapsed := time.Since(completeStart)
	totalElapsed := time.Since(start)

	t.Logf("Full pipeline (%d items):", volume)
	t.Logf("  trigger:  %v (%.0f/sec)", triggerElapsed, float64(volume)/triggerElapsed.Seconds())
	t.Logf("  dequeue:  %v (%.0f/sec), got %d", dequeueElapsed, float64(len(runIDs))/dequeueElapsed.Seconds(), len(runIDs))
	t.Logf("  complete: %v (%.0f/sec), completed %d", completeElapsed, float64(completed)/completeElapsed.Seconds(), completed)
	t.Logf("  total:    %v", totalElapsed)
}

func TestLoadPipeline_ConcurrentFullLifecycle(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	projectID := "proj-lp-conc-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"pipe-conc","slug":"pipe-conc-%d","endpoint_url":"https://example.com/pipe","max_attempts":1,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	const workers = 10
	perWorker := loadVolume() / (workers * 5)
	var wg conc.WaitGroup
	var triggered, completed atomic.Int64
	start := time.Now()

	for range workers {
		wg.Go(func() {
			for i := range perWorker {
				resp := doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger",
					fmt.Sprintf(`{"payload":{"i":%d}}`, i))
				if resp.Code != 201 {
					continue
				}
				triggered.Add(1)
				runID := asString(t, mustDecodeObject(t, resp), "id")

				err := testStore.UpdateRunStatus(ctx, runID, domain.StatusQueued, domain.StatusDequeued, map[string]any{
					"started_at": time.Now().UTC(),
				})
				if err != nil {
					continue
				}
				err = testStore.UpdateRunStatus(ctx, runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{})
				if err != nil {
					continue
				}
				err = testStore.UpdateRunStatus(ctx, runID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
					"finished_at": time.Now().UTC(),
				})
				if err == nil {
					completed.Add(1)
				}
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	total := int64(workers * perWorker)

	t.Logf("Concurrent pipeline: triggered=%d completed=%d/%d in %v (%.0f/sec)",
		triggered.Load(), completed.Load(), total, elapsed, float64(total)/elapsed.Seconds())
}

func TestLoadPipeline_MultiJobParallel(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	projectID := "proj-lp-multi-" + fmt.Sprintf("%d", time.Now().UnixNano())

	const jobCount = 5
	const runsPerJob = 50
	jobIDs := make([]string, jobCount)

	for j := range jobCount {
		w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
			`{"project_id":"%s","name":"multi-%d","slug":"multi-%d-%d","endpoint_url":"https://example.com/multi","max_attempts":1,"timeout_secs":30}`,
			projectID, j, time.Now().UnixNano(), j,
		))
		require.EqualValues(t, 201,

			w.Code)

		jobIDs[j] = asString(t, mustDecodeObject(t, w), "id")
	}

	var wg conc.WaitGroup
	var totalTriggered, totalCompleted atomic.Int64
	start := time.Now()

	for _, jid := range jobIDs {
		jobID := jid
		wg.Go(func() {
			for i := range runsPerJob {
				resp := doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger",
					fmt.Sprintf(`{"payload":{"i":%d}}`, i))
				if resp.Code != 201 {
					continue
				}
				totalTriggered.Add(1)
				runID := asString(t, mustDecodeObject(t, resp), "id")

				err := testStore.UpdateRunStatus(ctx, runID, domain.StatusQueued, domain.StatusDequeued, map[string]any{
					"started_at": time.Now().UTC(),
				})
				if err != nil {
					continue
				}
				err = testStore.UpdateRunStatus(ctx, runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{})
				if err != nil {
					continue
				}
				err = testStore.UpdateRunStatus(ctx, runID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
					"finished_at": time.Now().UTC(),
				})
				if err == nil {
					totalCompleted.Add(1)
				}
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	total := jobCount * runsPerJob

	t.Logf("Multi-job pipeline: %d jobs x %d runs = %d total, triggered=%d completed=%d in %v (%.0f/sec)",
		jobCount, runsPerJob, total, totalTriggered.Load(), totalCompleted.Load(),
		elapsed, float64(total)/elapsed.Seconds())
}

func TestLoadPipeline_FailureRecovery(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	projectID := "proj-lp-fail-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"pipe-fail","slug":"pipe-fail-%d","endpoint_url":"https://example.com/fail","max_attempts":3,"timeout_secs":30}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	volume := loadVolume() / 10
	start := time.Now()
	var failed, completed int

	for i := range volume {
		resp := doRequest(t, "POST", "/v1/jobs/"+jobID+"/trigger",
			fmt.Sprintf(`{"payload":{"i":%d}}`, i))
		if resp.Code != 201 {
			continue
		}
		runID := asString(t, mustDecodeObject(t, resp), "id")

		_ = testStore.UpdateRunStatus(ctx, runID, domain.StatusQueued, domain.StatusDequeued, map[string]any{
			"started_at": time.Now().UTC(),
		})
		_ = testStore.UpdateRunStatus(ctx, runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{})

		if i%3 == 0 {
			err := testStore.UpdateRunStatus(ctx, runID, domain.StatusExecuting, domain.StatusFailed, map[string]any{
				"finished_at": time.Now().UTC(),
				"error":       "simulated failure",
			})
			if err == nil {
				failed++
			}
		} else {
			err := testStore.UpdateRunStatus(ctx, runID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
				"finished_at": time.Now().UTC(),
			})
			if err == nil {
				completed++
			}
		}
	}
	elapsed := time.Since(start)

	t.Logf("Failure recovery pipeline: %d completed, %d failed out of %d in %v (%.0f/sec)",
		completed, failed, volume, elapsed, float64(volume)/elapsed.Seconds())
}
