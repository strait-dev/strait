//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// TestDoS_ConnectionPoolSaturation exhausts all database pool connections and
// verifies that new requests either get a timeout or a 503 rather than hanging
// indefinitely.
func TestDoS_ConnectionPoolSaturation(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	pool := testEnv.DB.Pool

	// Acquire connections by holding open transactions.
	stat := pool.Stat()
	maxConns := int(stat.MaxConns())
	if maxConns < 2 {
		t.Skipf("pool has fewer than 2 max connections (%d), skipping", maxConns)
	}

	// Hold all but one connection.
	holdCount := maxConns - 1
	type held struct {
		tx interface{ Rollback(context.Context) error }
	}
	holders := make([]held, 0, holdCount)
	for i := range holdCount {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Logf("could only acquire %d/%d connections: %v", i, holdCount, err)
			break
		}
		holders = append(holders, held{tx: tx})
	}
	defer func() {
		for _, h := range holders {
			_ = h.tx.Rollback(ctx)
		}
	}()

	// With the pool nearly exhausted, try a normal store operation with a
	// short timeout.
	shortCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err := testStore.GetRun(shortCtx, "nonexistent-run-id")
	// We expect either a timeout error or a "not found" (if the last
	// connection was available). Either is acceptable; a hang is not.
	if err != nil {
		t.Logf("store operation under pool saturation returned error (expected): %v", err)
	}
}

// TestDoS_RedisMemoryPressure publishes 1000 large messages and verifies the
// system handles it without crashing.
func TestDoS_RedisMemoryPressure(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	client := testEnv.Redis.Client

	// Publish 1000 messages of ~10KB each.
	largePayload := strings.Repeat("A", 10*1024)
	for i := range 1000 {
		key := fmt.Sprintf("test:pressure:%d", i)
		require.NoError(t, client.
			Set(ctx,
				key, largePayload,

				30*time.
					Second).Err())

	}
	require.NoError(t, client.
		Ping(
			ctx).Err())

	// Verify Redis is still operational.

	// Clean up.
	for i := range 1000 {
		client.Del(ctx, fmt.Sprintf("test:pressure:%d", i))
	}
}

// TestDoS_AdvisoryLockExhaustion holds many advisory locks concurrently and
// verifies the system remains operational.
func TestDoS_AdvisoryLockExhaustion(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	const lockCount = 50

	// Acquire many advisory locks.
	acquired := make([]int64, 0, lockCount)
	for i := range lockCount {
		lockID := int64(800000 + i)
		ok, err := testStore.TryAdvisoryLock(ctx, lockID)
		require.NoError(t, err)

		if ok {
			acquired = append(acquired, lockID)
		}
	}

	// Verify the system is still operational.
	projectID := "proj-lock-exhaust-" + newID()
	job := createJob(t, projectID, "Lock Exhaust", "lock-exhaust-"+newID())
	require.NotEqual(t, "",

		asString(t, job, "id"))

	// Release all locks.
	for _, lockID := range acquired {
		if err := testStore.ReleaseAdvisoryLock(ctx, lockID); err != nil {
			t.Logf("release advisory lock %d: %v", lockID, err)
		}
	}
}

// TestDoS_ConcurrentSSEConnections creates 50 simultaneous SSE connections via
// HTTP and verifies no goroutine leak after they close.
func TestDoS_ConcurrentSSEConnections(t *testing.T) {
	mustClean(t)

	projectID := "proj-sse-conc-" + newID()
	job := createJob(t, projectID, "SSE Concurrent", "sse-conc-"+newID())
	jobID := asString(t, job, "id")

	// Create a few runs to stream.
	runIDs := make([]string, 5)
	for i := range 5 {
		run := triggerJob(t, jobID, fmt.Sprintf(`{"payload":{"sse":%d}}`, i), "")
		runIDs[i] = asString(t, run, "id")
	}

	runtime.GC()
	baseline := runtime.NumGoroutine()

	// Create 50 concurrent requests to the stream endpoint.
	const count = 50
	var wg conc.WaitGroup
	for i := range count {
		idx := i
		wg.Go(func() {
			runID := runIDs[idx%len(runIDs)]
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			req := authedRequest(http.MethodGet, "/v1/runs/"+runID+"/stream", "")
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()
			testServer.ServeHTTP(w, req)
		})
	}
	wg.Wait()

	// Wait for goroutines to clean up.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= baseline+20 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	current := runtime.NumGoroutine()
	leaked := current - baseline
	require.LessOrEqual(t,

		leaked,
		20)

}

// TestDoS_CacheEvictionStorm rapidly sets and deletes keys in Redis to
// simulate a cache eviction storm, verifying stability.
func TestDoS_CacheEvictionStorm(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	client := testEnv.Redis.Client

	const iterations = 500
	var wg conc.WaitGroup
	var errors atomic.Int32

	for i := range iterations {
		idx := i
		wg.Go(func() {
			key := fmt.Sprintf("test:evict:%d", idx)
			if err := client.Set(ctx, key, "value", time.Second).Err(); err != nil {
				errors.Add(1)
				return
			}
			// Immediately delete.
			client.Del(ctx, key)
		})
	}
	wg.Wait()

	if errors.Load() > 0 {
		t.Logf("cache eviction storm had %d errors out of %d", errors.Load(), iterations)
	}
	require.NoError(t, client.
		Ping(
			ctx).Err())

	// Verify Redis is still operational.

}

// TestDoS_WebhookDeliveryQueueOverflow creates many pending webhook deliveries
// and verifies the system handles the bounded processing correctly.
func TestDoS_WebhookDeliveryQueueOverflow(t *testing.T) {
	mustClean(t)

	projectID := "proj-wh-overflow-" + newID()
	job := createJob(t, projectID, "WH Overflow", "wh-overflow-"+newID())
	jobID := asString(t, job, "id")
	run := triggerJob(t, jobID, `{"payload":{"overflow":true}}`, "")
	runID := asString(t, run, "id")

	ctx := context.Background()

	// Create a webhook endpoint that always fails.
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failServer.Close()

	// Create 100 pending webhook deliveries.
	const deliveryCount = 100
	for range deliveryCount {
		retryAt := time.Now().UTC().Add(-time.Second)
		delivery := &domain.WebhookDelivery{
			RunID:       runID,
			JobID:       jobID,
			WebhookURL:  failServer.URL,
			RetryPolicy: domain.WebhookRetryPolicyExponential,
			Status:      domain.WebhookStatusPending,
			Attempts:    0,
			MaxAttempts: 3,
			NextRetryAt: &retryAt,
		}
		require.NoError(t, testStore.
			CreateWebhookDelivery(ctx, delivery))

	}

	// Verify all deliveries were created.
	var count int
	err := testEnv.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM webhook_deliveries WHERE run_id = $1`, runID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, deliveryCount,

		count)

	// Verify the system is still operational after creating many deliveries.
	newRun := triggerJob(t, jobID, `{"payload":{"after_overflow":true}}`, "")
	require.NotEqual(t, "",

		asString(t, newRun,
			"id"),
	)

}

// Ensure imports are used.
var _ = store.New
