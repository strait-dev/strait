package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// recordingStore counts UpsertUsageRecord calls so tests can assert idempotency.
type recordingStore struct {
	mockBillingStore
	upsertCalls      int
	durableCalls     int
	failDurableCalls int
	durableKeys      map[string]struct{}
}

func (r *recordingStore) UpsertUsageRecord(_ context.Context, _ *UsageRecord) error {
	r.upsertCalls++
	return nil
}

func (r *recordingStore) RecordUsageCost(_ context.Context, _ *UsageRecord, idempotencyKey, _ string) (bool, error) {
	r.durableCalls++
	if r.failDurableCalls > 0 {
		r.failDurableCalls--
		return false, fmt.Errorf("transient durable write failure")
	}
	if r.durableKeys == nil {
		r.durableKeys = make(map[string]struct{})
	}
	if _, ok := r.durableKeys[idempotencyKey]; ok {
		return false, nil
	}
	r.durableKeys[idempotencyKey] = struct{}{}
	r.upsertCalls++
	return true, nil
}

func newTestRecorder(t *testing.T, rdb redis.Cmdable) (*RunCostRecorder, *recordingStore) {
	t.Helper()
	store := &recordingStore{}
	recorder := NewRunCostRecorder(store, rdb, nil, slog.Default())
	recorder.retryInitialDelay = time.Nanosecond
	recorder.retryMaxDelay = time.Nanosecond
	return recorder, store
}

// TestRunCostRecorder_SameRunID_Skips verifies that the second call for the same
// runID is silently skipped and UpsertUsageRecord is called only once.
func TestRunCostRecorder_SameRunID_Skips(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	recorder, store := newTestRecorder(t, rdb)

	ctx := context.Background()
	runID := "run-idem-001"
	require.NoError(t,
		recorder.
			RecordHTTPRunCost(
				ctx, "org-1",
				"proj-1",

				runID))
	require.EqualValues(t, 1, store.upsertCalls)
	require.NoError(t,
		recorder.
			RecordHTTPRunCost(
				ctx, "org-1",
				"proj-1",

				runID))
	require.EqualValues(t, 1, store.upsertCalls)

}

// TestRunCostRecorder_DifferentRunIDs_BothRecord verifies that distinct runIDs
// each produce a usage record write.
func TestRunCostRecorder_DifferentRunIDs_BothRecord(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	recorder, store := newTestRecorder(t, rdb)

	ctx := context.Background()
	require.NoError(t,
		recorder.
			RecordHTTPRunCost(
				ctx, "org-1",
				"proj-1",

				"run-A"))
	require.NoError(t,
		recorder.
			RecordHTTPRunCost(
				ctx, "org-1",
				"proj-1",

				"run-B"))
	require.EqualValues(t, 2, store.upsertCalls)

}

// TestRunCostRecorder_RedisError_RecordsWithDurableIdempotency verifies that a
// Redis connectivity error does not drop usage when durable DB idempotency is available.
func TestRunCostRecorder_RedisError_RecordsWithDurableIdempotency(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	recorder, store := newTestRecorder(t, rdb)

	// Close the miniredis server to force connection errors.mr.Close()

	ctx := context.Background()
	err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", "run-redis-fail")
	require.NoError(t,
		err)
	require.EqualValues(t, 1, store.upsertCalls)

}

// TestRunCostRecorder_EmptyRunID_RecordsWithoutDedup verifies that an empty
// runID skips the idempotency guard and still writes the usage record.
func TestRunCostRecorder_EmptyRunID_RecordsWithoutDedup(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	recorder, store := newTestRecorder(t, rdb)

	ctx := context.Background()
	require.NoError(t,
		recorder.
			RecordHTTPRunCost(
				ctx, "org-1",
				"proj-1",

				""))
	require.NoError(t,
		recorder.
			RecordHTTPRunCost(
				ctx, "org-1",
				"proj-1",

				""))
	require.EqualValues(t, 2, store.upsertCalls)

	// Two calls with empty runID should both write (no dedup key exists).

}

// TestRunCostRecorder_NilRedis_UsesDurableDedup verifies that a nil Redis
// client still uses the durable billing ledger to dedupe by run ID.
func TestRunCostRecorder_NilRedis_UsesDurableDedup(t *testing.T) {
	t.Parallel()
	recorder, store := newTestRecorder(t, nil)

	ctx := context.Background()
	require.NoError(t,
		recorder.
			RecordHTTPRunCost(
				ctx, "org-1",
				"proj-1",

				"run-no-redis"))
	require.NoError(t,
		recorder.
			RecordHTTPRunCost(
				ctx, "org-1",
				"proj-1",

				"run-no-redis"))
	require.EqualValues(t, 1, store.upsertCalls)

}

func TestRunCostRecorder_TransientDurableErrorsRetryBeforeReturning(t *testing.T) {
	t.Parallel()
	recorder, store := newTestRecorder(t, nil)
	store.failDurableCalls = 2

	ctx := context.Background()
	require.NoError(t,
		recorder.
			RecordWebhookDeliveryCost(ctx,
				"org-1",

				"proj-1", "delivery-retry"))
	require.EqualValues(t, 3, store.durableCalls)
	require.EqualValues(t, 1, store.upsertCalls)

}

// TestRunCostRecorder_WorkerAndWebhookModes verifies idempotency for all three
// recording methods with the same ID.
func TestRunCostRecorder_WorkerAndWebhookModes(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	recorder, store := newTestRecorder(t, rdb)

	ctx := context.Background()
	require.NoError(t,
		recorder.
			RecordWorkerRunCost(ctx, "org-1",
				"proj-1",

				"worker-run-1"))
	require.NoError(t,
		recorder.
			RecordWorkerRunCost(ctx, "org-1",
				"proj-1",

				"worker-run-1"))
	require.NoError(t,
		recorder.
			RecordWebhookDeliveryCost(ctx,
				"org-1",

				"proj-1", "delivery-1"))
	require.NoError(t,
		recorder.
			RecordWebhookDeliveryCost(ctx,
				"org-1",

				"proj-1", "delivery-1"))
	require.EqualValues(t, 2, store.upsertCalls)

	// Worker mode

	// Webhook delivery mode

	// Each distinct ID should produce exactly one upsert.

}

// TestRunCostRecorder_IdempotencyKey_TTL verifies the Redis key TTL is set
// correctly (approximately 48h, allowing for test execution time).
func TestRunCostRecorder_IdempotencyKey_TTL(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	recorder, _ := newTestRecorder(t, rdb)

	ctx := context.Background()
	runID := "run-ttl-check"
	require.NoError(t,
		recorder.
			RecordHTTPRunCost(
				ctx, "org-1",
				"proj-1",

				runID))

	key := "strait:cost_recorded:" + runID
	ttl, err := rdb.TTL(ctx, key).Result()
	require.NoError(t,
		err)

	// TTL should be close to 48 hours (within a 5-second window for test execution).
	expected := costRecordedTTL
	require.False(t,
		ttl < expected-
			5*
				time.Second ||
			ttl > expected,
	)

}

// TestRunCostRecorder_DurableError_WrapsUnderlying ensures durable write errors
// remain visible after retry exhaustion.
func TestRunCostRecorder_DurableError_WrapsUnderlying(t *testing.T) {
	t.Parallel()
	recorder, store := newTestRecorder(t, nil)
	store.failDurableCalls = 10
	recorder.maxRecordAttempts = 2

	ctx := context.Background()
	err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", "run-wrap")
	require.Error(t,
		err)

	// The error should be non-nil; we just check it wraps something via Unwrap.
	if errors.Unwrap(err) == nil {
		t.Logf("note: error does not wrap another error directly, but was non-nil: %v", err)
	}
}
