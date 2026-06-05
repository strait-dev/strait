//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadVolume() int {
	switch os.Getenv("LOADTEST_VOLUME_TIER") {
	case "large":
		return 5000
	case "extreme":
		return 10000
	default:
		return 500
	}
}

func makeRun(jobID, projectID string, payload string) *domain.JobRun {
	return &domain.JobRun{
		ID:            newID(),
		JobID:         jobID,
		ProjectID:     projectID,
		Payload:       json.RawMessage(payload),
		Priority:      1,
		ExecutionMode: domain.ExecutionModeHTTP,
	}
}

func TestLoadQueue_EnqueueThroughput(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	volume := loadVolume()
	projectID := "proj-lq-enq-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"queue-enq","slug":"queue-enq-%d","endpoint_url":"https://example.com/enq","max_attempts":3,"timeout_secs":60}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	start := time.Now()
	for i := range volume {
		run := makeRun(jobID, projectID, fmt.Sprintf(`{"i":%d}`, i))
		require.NoError(t, testQueue.
			Enqueue(ctx,
				run))

	}
	elapsed := time.Since(start)
	t.Logf("Enqueued %d items in %v (%.0f/sec)", volume, elapsed, float64(volume)/elapsed.Seconds())
}

func TestLoadQueue_DequeueThroughput(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	volume := loadVolume()
	projectID := "proj-lq-deq-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"queue-deq","slug":"queue-deq-%d","endpoint_url":"https://example.com/deq","max_attempts":3,"timeout_secs":60}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	for i := range volume {
		run := makeRun(jobID, projectID, fmt.Sprintf(`{"i":%d}`, i))
		require.NoError(t, testQueue.
			Enqueue(ctx,
				run))

	}

	start := time.Now()
	dequeued := len(dequeueRunsEventually(t, testQueue, volume))
	elapsed := time.Since(start)
	t.Logf("Dequeued %d items in %v (%.0f/sec)", dequeued, elapsed, float64(dequeued)/elapsed.Seconds())
	assert.GreaterOrEqual(
		t, dequeued,
		volume/
			2)

}

func TestLoadQueue_ConcurrentEnqueue(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	projectID := "proj-lq-cenq-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"queue-cenq","slug":"queue-cenq-%d","endpoint_url":"https://example.com/cenq","max_attempts":3,"timeout_secs":60}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	const workers = 20
	perWorker := loadVolume() / workers

	var wg conc.WaitGroup
	var failures atomic.Int64
	start := time.Now()

	for w := range workers {
		workerID := w
		wg.Go(func() {
			for i := range perWorker {
				run := makeRun(jobID, projectID, fmt.Sprintf(`{"w":%d,"i":%d}`, workerID, i))
				if err := testQueue.Enqueue(ctx, run); err != nil {
					failures.Add(1)
				}
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	total := workers * perWorker
	failCount := failures.Load()

	t.Logf("Concurrent enqueue: %d workers x %d items = %d total in %v (%.0f/sec, %d failures)",
		workers, perWorker, total, elapsed, float64(total)/elapsed.Seconds(), failCount)
	assert.LessOrEqual(t,

		failCount,
		int64(total)/10)

}

func TestLoadQueue_ConcurrentDequeue(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	volume := loadVolume()
	projectID := "proj-lq-cdeq-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"queue-cdeq","slug":"queue-cdeq-%d","endpoint_url":"https://example.com/cdeq","max_attempts":3,"timeout_secs":60}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	for i := range volume {
		run := makeRun(jobID, projectID, fmt.Sprintf(`{"i":%d}`, i))
		require.NoError(t, testQueue.
			Enqueue(ctx,
				run))

	}

	const workers = 10
	var wg conc.WaitGroup
	var totalDequeued atomic.Int64
	start := time.Now()

	for range workers {
		wg.Go(func() {
			for {
				run, err := testQueue.Dequeue(ctx)
				if err != nil || run == nil {
					return
				}
				totalDequeued.Add(1)
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	dequeued := totalDequeued.Load()

	t.Logf("Concurrent dequeue: %d workers dequeued %d items in %v (%.0f/sec)",
		workers, dequeued, elapsed, float64(dequeued)/elapsed.Seconds())
}

func TestLoadQueue_EnqueueDequeueInterleaved(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	projectID := "proj-lq-inter-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"queue-inter","slug":"queue-inter-%d","endpoint_url":"https://example.com/inter","max_attempts":3,"timeout_secs":60}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	const duration = 5 * time.Second
	var enqueued, dequeued atomic.Int64
	var wg conc.WaitGroup

	ctx2, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	for range 5 {
		wg.Go(func() {
			for {
				select {
				case <-ctx2.Done():
					return
				default:
					run := makeRun(jobID, projectID, `{"interleaved":true}`)
					if err := testQueue.Enqueue(ctx, run); err == nil {
						enqueued.Add(1)
					}
				}
			}
		})
	}

	for range 5 {
		wg.Go(func() {
			for {
				select {
				case <-ctx2.Done():
					return
				default:
					run, err := testQueue.Dequeue(ctx)
					if err == nil && run != nil {
						dequeued.Add(1)
					}
				}
			}
		})
	}

	wg.Wait()
	t.Logf("Interleaved: enqueued=%d dequeued=%d in %v", enqueued.Load(), dequeued.Load(), duration)
}

func TestLoadQueue_FSMTransitionThroughput(t *testing.T) {
	mustClean(t)
	ctx := context.Background()
	volume := loadVolume() / 5
	projectID := "proj-lq-fsm-" + fmt.Sprintf("%d", time.Now().UnixNano())

	w := doRequest(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"queue-fsm","slug":"queue-fsm-%d","endpoint_url":"https://example.com/fsm","max_attempts":3,"timeout_secs":60}`,
		projectID, time.Now().UnixNano(),
	))
	require.EqualValues(t, 201,

		w.Code)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	runIDs := make([]string, volume)
	for i := range volume {
		run := makeRun(jobID, projectID, fmt.Sprintf(`{"i":%d}`, i))
		require.NoError(t, testQueue.
			Enqueue(ctx,
				run))

	}

	dequeuedRuns := dequeueRunsEventually(t, testQueue, volume)
	for i, run := range dequeuedRuns {
		runIDs[i] = run.ID
	}

	start := time.Now()
	transitioned := 0
	for _, id := range runIDs {
		if id == "" {
			continue
		}
		err := testStore.UpdateRunStatus(ctx, id, domain.StatusDequeued, domain.StatusExecuting, map[string]any{})
		if err == nil {
			transitioned++
		}
	}
	elapsed := time.Since(start)
	t.Logf("FSM transitions (dequeued->executing): %d/%d in %v (%.0f/sec)",
		transitioned, len(runIDs), elapsed, float64(transitioned)/elapsed.Seconds())
}
