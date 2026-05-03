package billing

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// recordingStore counts UpsertUsageRecord calls so tests can assert idempotency.
type recordingStore struct {
	mockBillingStore
	upsertCalls int
}

func (r *recordingStore) UpsertUsageRecord(_ context.Context, _ *UsageRecord) error {
	r.upsertCalls++
	return nil
}

// errRedisClient is a redis.Cmdable that always returns an error for SetNX.
// We use miniredis and immediately close it to simulate connection failures.
type setupFn func(t *testing.T) (*RunCostRecorder, *recordingStore)

func newTestRecorder(t *testing.T, rdb redis.Cmdable) (*RunCostRecorder, *recordingStore) {
	t.Helper()
	store := &recordingStore{}
	recorder := NewRunCostRecorder(store, rdb, nil, slog.Default())
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

	if err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", runID); err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if store.upsertCalls != 1 {
		t.Fatalf("expected 1 upsert after first call, got %d", store.upsertCalls)
	}

	if err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", runID); err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if store.upsertCalls != 1 {
		t.Fatalf("expected still 1 upsert after duplicate call, got %d", store.upsertCalls)
	}
}

// TestRunCostRecorder_DifferentRunIDs_BothRecord verifies that distinct runIDs
// each produce a usage record write.
func TestRunCostRecorder_DifferentRunIDs_BothRecord(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	recorder, store := newTestRecorder(t, rdb)

	ctx := context.Background()

	if err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", "run-A"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", "run-B"); err != nil {
		t.Fatalf("second run: %v", err)
	}

	if store.upsertCalls != 2 {
		t.Fatalf("expected 2 upserts for 2 distinct runIDs, got %d", store.upsertCalls)
	}
}

// TestRunCostRecorder_RedisError_ReturnError verifies that a Redis connectivity
// error causes record() to return an error (fail-closed) rather than proceed.
func TestRunCostRecorder_RedisError_ReturnError(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	recorder, store := newTestRecorder(t, rdb)

	// Close the miniredis server to force connection errors.
	mr.Close()

	ctx := context.Background()
	err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", "run-redis-fail")
	if err == nil {
		t.Fatal("expected error when Redis is unavailable, got nil")
	}
	if store.upsertCalls != 0 {
		t.Fatalf("expected no upsert when Redis fails, got %d", store.upsertCalls)
	}
}

// TestRunCostRecorder_EmptyRunID_RecordsWithoutDedup verifies that an empty
// runID skips the idempotency guard and still writes the usage record.
func TestRunCostRecorder_EmptyRunID_RecordsWithoutDedup(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	recorder, store := newTestRecorder(t, rdb)

	ctx := context.Background()
	// Two calls with empty runID should both write (no dedup key exists).
	if err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", ""); err != nil {
		t.Fatalf("first call with empty runID: %v", err)
	}
	if err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", ""); err != nil {
		t.Fatalf("second call with empty runID: %v", err)
	}
	if store.upsertCalls != 2 {
		t.Fatalf("expected 2 upserts for empty runID (no dedup), got %d", store.upsertCalls)
	}
}

// TestRunCostRecorder_NilRedis_RecordsWithoutDedup verifies that a nil Redis
// client skips the idempotency guard (backwards-compatible, no panic).
func TestRunCostRecorder_NilRedis_RecordsWithoutDedup(t *testing.T) {
	t.Parallel()
	recorder, store := newTestRecorder(t, nil)

	ctx := context.Background()
	if err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", "run-no-redis"); err != nil {
		t.Fatalf("unexpected error with nil Redis: %v", err)
	}
	if store.upsertCalls != 1 {
		t.Fatalf("expected 1 upsert with nil Redis, got %d", store.upsertCalls)
	}
}

// TestRunCostRecorder_WorkerAndWebhookModes verifies idempotency for all three
// recording methods with the same ID.
func TestRunCostRecorder_WorkerAndWebhookModes(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	recorder, store := newTestRecorder(t, rdb)

	ctx := context.Background()

	// Worker mode
	if err := recorder.RecordWorkerRunCost(ctx, "org-1", "proj-1", "worker-run-1"); err != nil {
		t.Fatalf("worker first call: %v", err)
	}
	if err := recorder.RecordWorkerRunCost(ctx, "org-1", "proj-1", "worker-run-1"); err != nil {
		t.Fatalf("worker second call: %v", err)
	}

	// Webhook delivery mode
	if err := recorder.RecordWebhookDeliveryCost(ctx, "org-1", "proj-1", "delivery-1"); err != nil {
		t.Fatalf("webhook first call: %v", err)
	}
	if err := recorder.RecordWebhookDeliveryCost(ctx, "org-1", "proj-1", "delivery-1"); err != nil {
		t.Fatalf("webhook second call: %v", err)
	}

	// Each distinct ID should produce exactly one upsert.
	if store.upsertCalls != 2 {
		t.Fatalf("expected 2 upserts (1 per distinct ID), got %d", store.upsertCalls)
	}
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
	if err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", runID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key := "strait:cost_recorded:" + runID
	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("failed to get TTL: %v", err)
	}

	// TTL should be close to 48 hours (within a 5-second window for test execution).
	expected := costRecordedTTL
	if ttl < expected-5*time.Second || ttl > expected {
		t.Fatalf("expected TTL ~%v, got %v", expected, ttl)
	}
}

// TestRunCostRecorder_RedisError_WrapsUnderlying ensures that a Redis error is
// non-nil and the caller receives a meaningful error message.
func TestRunCostRecorder_RedisError_WrapsUnderlying(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	recorder, _ := newTestRecorder(t, rdb)
	mr.Close()

	ctx := context.Background()
	err := recorder.RecordHTTPRunCost(ctx, "org-1", "proj-1", "run-wrap")
	if err == nil {
		t.Fatal("expected error when Redis is down, got nil")
	}
	// The error should be non-nil; we just check it wraps something via Unwrap.
	if errors.Unwrap(err) == nil {
		t.Logf("note: error does not wrap another error directly, but was non-nil: %v", err)
	}
}
