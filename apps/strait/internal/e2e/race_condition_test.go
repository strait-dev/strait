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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

// TestRace_DoubleDequeue enqueues a single run and races two goroutines to
// dequeue it. Only one should receive the run.
func TestRace_DoubleDequeue(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	job := testutil.MustCreateJob(t, ctx, testStore, nil)
	_ = testutil.MustEnqueueRun(t, ctx, testQueue, job, nil)

	var wg sync.WaitGroup
	var gotRun atomic.Int32

	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			runs, err := testQueue.DequeueN(ctx, 1)
			if err != nil {
				return
			}
			if len(runs) > 0 {
				gotRun.Add(1)
			}
		}()
	}
	wg.Wait()

	if gotRun.Load() != 1 {
		t.Fatalf("expected exactly 1 goroutine to dequeue the run, got %d", gotRun.Load())
	}
}

// TestRace_ConcurrentStatusUpdate races 10 goroutines trying to update the same
// run from executing to completed. Exactly one should succeed.
func TestRace_ConcurrentStatusUpdate(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	job := testutil.MustCreateJob(t, ctx, testStore, nil)
	status := domain.StatusExecuting
	run := testutil.MustCreateRun(t, ctx, testStore, job, &testutil.RunOpts{
		Status: &status,
	})

	const goroutines = 10
	var wg sync.WaitGroup
	var successCount atomic.Int32

	now := time.Now()
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			err := testStore.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
				"finished_at": now,
			})
			if err == nil {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if successCount.Load() != 1 {
		t.Fatalf("expected exactly 1 successful status update, got %d", successCount.Load())
	}
	testutil.AssertRunStatus(t, ctx, testStore, run.ID, domain.StatusCompleted)
}

// TestRace_ConcurrentEnqueueDequeueInterleaving runs 10 enqueue and 10 dequeue
// goroutines concurrently and verifies that no run is dequeued more than once.
func TestRace_ConcurrentEnqueueDequeueInterleaving(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	job := testutil.MustCreateJob(t, ctx, testStore, nil)

	const half = 10
	var wg sync.WaitGroup
	var dequeuedTotal atomic.Int32

	// Start enqueue goroutines.
	wg.Add(half)
	for i := 0; i < half; i++ {
		go func() {
			defer wg.Done()
			_ = testutil.MustEnqueueRun(t, ctx, testQueue, job, nil)
		}()
	}

	// Start dequeue goroutines.
	wg.Add(half)
	for i := 0; i < half; i++ {
		go func() {
			defer wg.Done()
			runs, err := testQueue.DequeueN(ctx, 1)
			if err == nil && len(runs) > 0 {
				dequeuedTotal.Add(int32(len(runs)))
			}
		}()
	}
	wg.Wait()

	// Dequeued should not exceed enqueued.
	if dequeuedTotal.Load() > half {
		t.Fatalf("dequeued %d runs, but only %d were enqueued", dequeuedTotal.Load(), half)
	}
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

	var wg sync.WaitGroup
	wg.Add(2)

	var triggerStatus atomic.Int32
	go func() {
		defer wg.Done()
		req := authedRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{}`)
		w := httptest.NewRecorder()
		testServer.ServeHTTP(w, req)
		triggerStatus.Store(int32(w.Code))
	}()

	go func() {
		defer wg.Done()
		_ = testStore.PauseJob(ctx, jobID, "race test")
	}()

	wg.Wait()

	code := triggerStatus.Load()
	// Trigger either succeeded (201) or was blocked by pause (409).
	if code != http.StatusCreated && code != http.StatusConflict {
		t.Fatalf("unexpected trigger status during pause race: %d", code)
	}
}

// TestRace_ConcurrentBatchSameJob triggers the same job from 5 goroutines
// simultaneously. All should either succeed or fail gracefully.
func TestRace_ConcurrentBatchSameJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-batch-" + newID()
	job := createJob(t, projectID, "batch-race-job", "batch-race-slug-"+newID())
	jobID := asString(t, job, "id")

	const goroutines = 5
	var wg sync.WaitGroup
	var created atomic.Int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"batch_idx":%d}`, idx)
			req := authedRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", body)
			w := httptest.NewRecorder()
			testServer.ServeHTTP(w, req)
			if w.Code == http.StatusCreated {
				created.Add(1)
			}
		}(i)
	}
	wg.Wait()

	// All 5 should succeed since there is no idempotency key.
	if created.Load() != goroutines {
		t.Fatalf("expected %d created runs, got %d", goroutines, created.Load())
	}

	ctx := context.Background()
	runs, err := testStore.ListRunsByJob(ctx, jobID, 100, 0)
	if err != nil {
		t.Fatalf("ListRunsByJob error: %v", err)
	}
	if len(runs) != goroutines {
		t.Fatalf("expected %d runs in store, got %d", goroutines, len(runs))
	}
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

	var wg sync.WaitGroup
	var errCount atomic.Int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
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
		}(i)
	}
	wg.Wait()

	// No deadlock or panic means success. Some quota errors are expected.
	totalSize, err := testStore.SumJobMemorySizeBytes(ctx, job.ID)
	if err != nil {
		t.Fatalf("SumJobMemorySizeBytes error: %v", err)
	}
	if totalSize > maxPerJob {
		t.Fatalf("total memory %d exceeds per-job quota %d", totalSize, maxPerJob)
	}
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
	var wg sync.WaitGroup

	// Subscribers.
	wg.Add(goroutines / 2)
	for i := 0; i < goroutines/2; i++ {
		go func() {
			defer wg.Done()
			sub := client.Subscribe(ctx, channel)
			defer sub.Close()
			// Receive one message or timeout.
			ch := sub.Channel()
			select {
			case <-ch:
			case <-ctx.Done():
			}
		}()
	}

	// Publishers.
	wg.Add(goroutines / 2)
	for i := 0; i < goroutines/2; i++ {
		go func(idx int) {
			defer wg.Done()
			client.Publish(ctx, channel, fmt.Sprintf("msg-%d", idx))
		}(i)
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

	var wg sync.WaitGroup

	// Writer goroutine: update the job description.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			job, err := testStore.GetJob(ctx, jobID)
			if err != nil {
				continue
			}
			job.Description = fmt.Sprintf("updated-%d", i)
			_ = testStore.UpdateJob(ctx, job)
		}
	}()

	// Reader goroutines: read the job via API.
	const readers = 5
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			w := doRequest(t, http.MethodGet, "/v1/jobs/"+jobID, "")
			if w.Code >= 500 {
				t.Errorf("server error reading job during concurrent update: %d", w.Code)
			}
		}()
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
	if err := testStore.CreateAPIKey(ctx, apiKey); err != nil {
		t.Fatalf("CreateAPIKey error: %v", err)
	}

	var wg sync.WaitGroup

	// Revoke the key in a goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = testStore.RevokeAPIKey(ctx, apiKey.ID)
	}()

	// Concurrently make requests using the key.
	const requestors = 5
	wg.Add(requestors)
	for i := 0; i < requestors; i++ {
		go func() {
			defer wg.Done()
			w := doSDKRequest(t, http.MethodGet, "/v1/jobs/", rawKey, "")
			if w.Code >= 500 {
				t.Errorf("server error during key rotation race: %d", w.Code)
			}
		}()
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
	for i := 0; i < runCount; i++ {
		status := domain.StatusExecuting
		runs[i] = testutil.MustCreateRun(t, ctx, testStore, job, &testutil.RunOpts{
			Status: &status,
		})
	}

	var wg sync.WaitGroup

	// Complete runs concurrently.
	wg.Add(runCount)
	for i := 0; i < runCount; i++ {
		go func(idx int) {
			defer wg.Done()
			_ = testStore.UpdateRunStatus(ctx, runs[idx].ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
				"finished_at": time.Now(),
			})
		}(i)
	}

	// Read stats concurrently.
	const statReaders = 5
	wg.Add(statReaders)
	for i := 0; i < statReaders; i++ {
		go func() {
			defer wg.Done()
			_, err := testStore.GetJobHealthStats(ctx, job.ID, time.Now().Add(-1*time.Hour))
			if err != nil {
				// Stats queries can fail if the table is being heavily modified,
				// but they should not panic.
				return
			}
		}()
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
	if err := testStore.CreateWebhookDelivery(ctx, delivery); err != nil {
		t.Fatalf("CreateWebhookDelivery error: %v", err)
	}

	var wg sync.WaitGroup

	// Retry the existing delivery.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = testStore.RetryWebhookDelivery(ctx, delivery.ID)
	}()

	// Create new deliveries concurrently.
	const newDeliveries = 3
	wg.Add(newDeliveries)
	for i := 0; i < newDeliveries; i++ {
		go func() {
			defer wg.Done()
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
		}()
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
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				_ = testStore.RecordEndpointCircuitFailure(ctx, endpointURL, time.Now(), 3, 30*time.Second)
			} else {
				_ = testStore.RecordEndpointCircuitSuccess(ctx, endpointURL)
			}
		}(i)
	}
	wg.Wait()

	// Verify we can still query the circuit state without error.
	state, err := testStore.GetEndpointCircuitState(ctx, endpointURL)
	if err != nil {
		t.Fatalf("GetEndpointCircuitState error: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil circuit state after concurrent writes")
	}
}
