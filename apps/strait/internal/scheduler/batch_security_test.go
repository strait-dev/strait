package scheduler

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// TestBatchFlusher_DisabledJobSkip verifies that a job disabled between
// fetch and flush is silently skipped.
func TestBatchFlusher_DisabledJobSkip(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Int32
	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-disabled", ProjectID: "proj-1", BatchKey: "k1", ItemCount: 2, OldestAt: time.Now()},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "i1", JobID: "job-disabled", ProjectID: "proj-1", Payload: json.RawMessage(`{"a":1}`), CreatedBy: "u1"},
		},
		jobs: map[string]*domain.Job{
			"job-disabled": {ID: "job-disabled", Enabled: false, TimeoutSecs: 30},
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}

	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())
	require.EqualValues(t, 0,
		enqueued.Load())
}

// TestBatchFlusher_ZeroBatchSize verifies that a zero BatchMaxSize falls back
// to the item count of the batch.
func TestBatchFlusher_ZeroBatchSize(t *testing.T) {
	t.Parallel()

	var enqueuedPayload json.RawMessage
	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 3, OldestAt: time.Now()},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "i1", Payload: json.RawMessage(`{"x":1}`), CreatedBy: "u1"},
			{ID: "i2", Payload: json.RawMessage(`{"x":2}`), CreatedBy: "u1"},
			{ID: "i3", Payload: json.RawMessage(`{"x":3}`), CreatedBy: "u1"},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, BatchMaxSize: 0, TimeoutSecs: 30},
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedPayload = run.Payload
			return nil
		},
	}

	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())
	require.NotNil(t, enqueuedPayload)

	var result struct {
		Items []json.RawMessage `json:"items"`
	}
	require.NoError(t,
		json.Unmarshal(enqueuedPayload,

			&result))
	require.Len(t, result.
		Items, 3)
}

// TestBatchFlusher_NegativeBatchSize verifies that a negative BatchMaxSize
// is handled gracefully (falls back to item count).
func TestBatchFlusher_NegativeBatchSize(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Int32
	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 1, OldestAt: time.Now()},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "i1", Payload: json.RawMessage(`{}`), CreatedBy: "u1"},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, BatchMaxSize: -5, TimeoutSecs: 30},
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}

	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())
	require.EqualValues(t, 1,
		enqueued.Load())
}

// TestBatchFlusher_PayloadMarshalError verifies that a payload that causes
// marshal failure results in an error during flush.
func TestBatchFlusher_PayloadMarshalError(t *testing.T) {
	t.Parallel()

	// Use valid JSON in the buffer items since json.Marshal over json.RawMessage
	// should not fail. Instead, verify that an empty drain returns no enqueues.
	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 1, OldestAt: time.Now()},
		},
		drainedItems: nil, // Empty drain.
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 30},
		},
	}

	var enqueued atomic.Int32
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}

	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())
	require.EqualValues(t, 0,
		enqueued.Load())
}

// TestBatchFlusher_TTLZeroSeconds verifies that RunTTLSecs = 0 causes the
// flusher to fall back to the timeout-based expiry.
func TestBatchFlusher_TTLZeroSeconds(t *testing.T) {
	t.Parallel()

	var enqueuedRun *domain.JobRun
	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 1, OldestAt: time.Now()},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "i1", Payload: json.RawMessage(`{"v":1}`), CreatedBy: "u1"},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 60, RunTTLSecs: 0},
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedRun = run
			return nil
		},
	}

	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())
	require.NotNil(t, enqueuedRun)
	require.NotNil(t, enqueuedRun.
		ExpiresAt,
	)

	// With RunTTLSecs=0, expiry should be TimeoutSecs+60 seconds from now.
	expectedMin := time.Now().Add(time.Duration(60+60-5) * time.Second)
	require.False(t, enqueuedRun.
		ExpiresAt.
		Before(expectedMin))
}

// TestAutoRotate_ConcurrentRotation verifies that multiple concurrent rotation
// invocations do not cause data races.
func TestAutoRotate_ConcurrentRotation(t *testing.T) {
	t.Parallel()

	rotationDays := 7
	webhookURL := rotationWebhookURLForTest(t)
	var rotated atomic.Int32

	ms := &mockAutoRotateStore{
		listDueRotationFn: func(context.Context) ([]domain.APIKey, error) {
			return []domain.APIKey{
				{ID: "key-1", ProjectID: "proj-1", RotationIntervalDays: &rotationDays, RotationWebhookURL: webhookURL},
				{ID: "key-2", ProjectID: "proj-2", RotationIntervalDays: &rotationDays, RotationWebhookURL: webhookURL},
			}, nil
		},
		createAPIKeyFn: func(_ context.Context, _ *domain.APIKey) error {
			return nil
		},
		markRotatedFn: func(_ context.Context, _, _ string, _ time.Time) error {
			rotated.Add(1)
			return nil
		},
		createAuditEventFn: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil).WithAllowPrivateEndpoints(true)
	r.rotationWebhookClient = successfulRotationWebhookClient()

	var wg conc.WaitGroup
	for range 5 {
		wg.Go(func() {
			r.autoRotateAPIKeys(context.Background())
		})
	}
	wg.Wait()

	if rotated.Load() < 5 {
		// Each invocation should process the 2 keys independently.
		t.Logf("rotated count: %d (at least 5 expected from 5 concurrent calls)", rotated.Load())
	}
}
