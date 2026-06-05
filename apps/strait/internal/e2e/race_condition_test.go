//go:build integration

package e2e_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRace_DoubleDequeue enqueues a single run and races two goroutines to
// dequeue it. Only one should receive the run.
func TestRace_DoubleDequeue(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	q := newIsolatedQueue(t)
	job := testutil.MustCreateJob(t, ctx, testStore, nil)
	_ = testutil.MustEnqueueRun(t, ctx, q, job, nil)

	var wg conc.WaitGroup
	var gotRun atomic.Int32

	for range 2 {
		wg.Go(func() {
			runs, err := q.DequeueN(ctx, 1)
			if err != nil {
				return
			}
			if len(runs) > 0 {
				gotRun.Add(1)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, gotRun.
		Load())

}

// TestRace_ConcurrentEnqueueDequeueInterleaving runs 10 enqueue and 10 dequeue
// goroutines concurrently and verifies that no run is dequeued more than once.
func TestRace_ConcurrentEnqueueDequeueInterleaving(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	q := newIsolatedQueue(t)
	job := testutil.MustCreateJob(t, ctx, testStore, nil)

	const half = 10
	var wg conc.WaitGroup
	var dequeuedTotal atomic.Int32

	// Start enqueue goroutines.
	for range half {
		wg.Go(func() {
			_ = testutil.MustEnqueueRun(t, ctx, q, job, nil)
		})
	}

	// Start dequeue goroutines.
	for range half {
		wg.Go(func() {
			runs, err := q.DequeueN(ctx, 1)
			if err == nil && len(runs) > 0 {
				dequeuedTotal.Add(int32(len(runs)))
			}
		})
	}
	wg.Wait()
	require.LessOrEqual(t,

		dequeuedTotal.
			Load(), int32(half))

	// Dequeued should not exceed enqueued.

}

// TestRace_ConcurrentJobPauseAndTrigger pauses a job while a trigger is
// in flight. The trigger should either succeed (if it won the race) or
// be rejected because the job is paused.
func TestRace_ConcurrentJobPauseAndTrigger(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	projectID := "proj-pause-" + newID()
	job := createJob(t, projectID, "pause-race-job", "pause-race-slug-"+newID())
	jobID := asString(t, job, "id")

	var wg conc.WaitGroup

	var triggerStatus atomic.Int32
	wg.Go(func() {
		req := authedRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{}`)
		w := httptest.NewRecorder()
		testServer.ServeHTTP(w, req)
		triggerStatus.Store(int32(w.Code))
	})

	wg.Go(func() {
		_ = testStore.PauseJob(ctx, jobID, "race test")
	})

	wg.Wait()

	code := triggerStatus.Load()
	require.False(t, code !=
		http.StatusCreated &&
		code != http.
			StatusBadRequest &&
		code != http.
			StatusConflict,
	)

	// Trigger either succeeded (201), was blocked by pause (400 "job is
	// disabled/paused"), or hit a conflict (409).

}

// TestRace_AdvisoryLockContention puts heavy concurrent pressure on
// UpsertJobMemoryWithQuota which uses advisory locks, verifying no deadlock.
func TestRace_AdvisoryLockContention(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	job := testutil.MustCreateJob(t, ctx, testStore, nil)

	const goroutines = 10
	const maxPerKey = 2048
	const maxPerJob = 10240

	var wg conc.WaitGroup
	var errCount atomic.Int32

	for i := range goroutines {
		idx := i
		wg.Go(func() {
			mem := &domain.JobMemory{
				ID:        uuid.Must(uuid.NewV7()).String(),
				JobID:     job.ID,
				ProjectID: job.ProjectID,
				MemoryKey: fmt.Sprintf("lock-key-%d", idx%3),
				Value:     json.RawMessage(fmt.Sprintf(`{"v":%d}`, idx)),
				SizeBytes: 50,
			}
			if err := testStore.UpsertJobMemoryWithQuota(ctx, mem, maxPerKey, maxPerJob); err != nil {
				errCount.Add(1)
			}
		})
	}
	wg.Wait()

	// No deadlock or panic means success. Some quota errors are expected.
	totalSize, err := testStore.SumJobMemorySizeBytes(ctx, job.ID)
	require.NoError(t, err)
	require.LessOrEqual(t,

		totalSize,
		maxPerJob,
	)

}

// TestRace_PubSubSubscribeUnsubscribe concurrently subscribes, publishes, and
// unsubscribes on the Redis pubsub to verify no panics or data races.
func TestRace_PubSubSubscribeUnsubscribe(t *testing.T) {
	mustClean(t)

	if testEnv.Redis == nil || testEnv.Redis.Client == nil {
		t.Skip("Redis not available in test environment")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	channel := "test-race-pubsub-" + newID()
	client := testEnv.Redis.Client

	const goroutines = 10
	var wg conc.WaitGroup

	// Subscribers.
	for range goroutines / 2 {
		wg.Go(func() {
			sub := client.Subscribe(ctx, channel)
			defer sub.Close()
			// Receive one message or timeout.
			ch := sub.Channel()
			select {
			case <-ch:
			case <-ctx.Done():
			}
		})
	}

	// Publishers.
	for i := range goroutines / 2 {
		idx := i
		wg.Go(func() {
			client.Publish(ctx, channel, fmt.Sprintf("msg-%d", idx))
		})
	}

	wg.Wait()
}

// TestRace_CacheInvalidationDuringRead concurrently updates a job via store
// while reading it via the API to verify no stale-cache panics.
func TestRace_CacheInvalidationDuringRead(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	projectID := "proj-cache-" + newID()
	jobResp := createJob(t, projectID, "cache-job", "cache-slug-"+newID())
	jobID := asString(t, jobResp, "id")

	var wg conc.WaitGroup

	// Writer goroutine: update the job description.
	wg.Go(func() {
		for i := range 5 {
			job, err := testStore.GetJob(ctx, jobID)
			if err != nil {
				continue
			}
			job.Description = fmt.Sprintf("updated-%d", i)
			_ = testStore.UpdateJob(ctx, job)
		}
	})

	// Reader goroutines: read the job via API.
	const readers = 5
	for range readers {
		wg.Go(func() {
			w := doRequest(t, http.MethodGet, "/v1/jobs/"+jobID, "")
			assert.False(t, w.Code >=
				500)

		})
	}

	wg.Wait()
}

// TestRace_ConcurrentAPIKeyRotation revokes an API key while requests are
// actively using it. Requests should get either 200 (key still valid) or
// 401 (key revoked), never a 5xx.
func TestRace_ConcurrentAPIKeyRotation(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	projectID := "proj-rotate-" + newID()

	rawKey := "sk_test_" + newID()
	h := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(h[:])

	apiKey := &domain.APIKey{
		ID:        uuid.Must(uuid.NewV7()).String(),
		ProjectID: projectID,
		Name:      "rotate-key",
		KeyHash:   keyHash,
		KeyPrefix: rawKey[:12],
		Scopes:    []string{"*"},
		CreatedAt: time.Now(),
	}
	require.NoError(t, testStore.
		CreateAPIKey(
			ctx, apiKey))

	var wg conc.WaitGroup

	// Revoke the key in a goroutine.
	wg.Go(func() {
		_ = testStore.RevokeAPIKey(ctx, apiKey.ID)
	})

	// Concurrently make requests using the key.
	const requestors = 5
	for range requestors {
		wg.Go(func() {
			w := doSDKRequest(t, http.MethodGet, "/v1/jobs/", rawKey, "")
			assert.False(t, w.Code >=
				500)

		})
	}

	wg.Wait()
}

// TestRace_StatsAggregationDuringCompletions completes runs while
// simultaneously reading job health stats to verify no panics.
func TestRace_StatsAggregationDuringCompletions(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	job := testutil.MustCreateJob(t, ctx, testStore, nil)

	// Create several executing runs.
	const runCount = 5
	runs := make([]*domain.JobRun, runCount)
	for i := range runCount {
		status := domain.StatusExecuting
		runs[i] = testutil.MustCreateRun(t, ctx, testStore, job, &testutil.RunOpts{
			Status: &status,
		})
	}

	var wg conc.WaitGroup

	// Complete runs concurrently.
	for i := range runCount {
		idx := i
		wg.Go(func() {
			_ = testStore.UpdateRunStatus(ctx, runs[idx].ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
				"finished_at": time.Now(),
			})
		})
	}

	// Read stats concurrently.
	const statReaders = 5
	for range statReaders {
		wg.Go(func() {
			_, err := testStore.GetJobHealthStats(ctx, job.ID, time.Now().Add(-1*time.Hour))
			if err != nil {
				// Stats queries can fail if the table is being heavily modified,
				// but they should not panic.
				return
			}
		})
	}

	wg.Wait()
}

// TestRace_WebhookDeliveryRetryAndNew concurrently creates and retries
// webhook deliveries to verify no data races in the delivery pipeline.
func TestRace_WebhookDeliveryRetryAndNew(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	job := testutil.MustCreateJob(t, ctx, testStore, nil)
	status := domain.StatusCompleted
	run := testutil.MustCreateRun(t, ctx, testStore, job, &testutil.RunOpts{
		Status: &status,
	})

	// Create an initial webhook delivery.
	delivery := &domain.WebhookDelivery{
		ID:          uuid.Must(uuid.NewV7()).String(),
		JobID:       job.ID,
		RunID:       run.ID,
		WebhookURL:  "https://example.com/hook",
		Status:      "failed",
		Attempts:    1,
		MaxAttempts: 3,
	}
	require.NoError(t, testStore.
		CreateWebhookDelivery(ctx, delivery))

	var wg conc.WaitGroup

	// Retry the existing delivery.
	wg.Go(func() {
		_, _ = testStore.RetryWebhookDelivery(ctx, delivery.ID)
	})

	// Create new deliveries concurrently.
	const newDeliveries = 3
	for range newDeliveries {
		wg.Go(func() {
			d := &domain.WebhookDelivery{
				ID:          uuid.Must(uuid.NewV7()).String(),
				JobID:       job.ID,
				RunID:       run.ID,
				WebhookURL:  "https://example.com/hook",
				Status:      "pending",
				Attempts:    0,
				MaxAttempts: 3,
			}
			_ = testStore.CreateWebhookDelivery(ctx, d)
		})
	}

	wg.Wait()
}

// TestRace_CircuitBreakerStateFlip concurrently records successes and failures
// on the endpoint circuit breaker to verify no data races.
func TestRace_CircuitBreakerStateFlip(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	endpointURL := "https://example.com/circuit-" + newID()

	const goroutines = 10
	var wg conc.WaitGroup

	for i := range goroutines {
		idx := i
		wg.Go(func() {
			if idx%2 == 0 {
				_ = testStore.RecordEndpointCircuitFailure(ctx, endpointURL, time.Now(), 3, 30*time.Second)
			} else {
				_ = testStore.RecordEndpointCircuitSuccess(ctx, endpointURL)
			}
		})
	}
	wg.Wait()

	// Verify we can still query the circuit state without error.
	state, err := testStore.GetEndpointCircuitState(ctx, endpointURL)
	require.NoError(t, err)
	require.NotNil(t, state)

}
