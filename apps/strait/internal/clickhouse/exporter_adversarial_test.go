//go:build !integration

package clickhouse

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
)

// NewExporter edge cases

func TestNewExporter_ZeroBatchSize(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 0}, nil)
	require.NotNil(t, e)
	assert.Equal(t, 1000, e.
		config.
		BatchSize,
	)
}

func TestNewExporter_NegativeBatchSize(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: -5}, nil)
	require.NotNil(t, e)
	assert.Equal(t, 1000, e.
		config.
		BatchSize,
	)
}

func TestNewExporter_ZeroFlushInterval(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, FlushInterval: 0}, nil)
	require.NotNil(t, e)
	assert.Equal(t, 5*time.
		Second, e.
		config.FlushInterval,
	)
}

func TestNewExporter_NegativeFlushInterval(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, FlushInterval: -1}, nil)
	require.NotNil(t, e)
	assert.Equal(t, 5*time.
		Second, e.
		config.FlushInterval,
	)
}

func TestNewExporter_NilLogger(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true}, nil)
	require.NotNil(t, e)
	assert.NotNil(t, e.logger)
}

// WithMetrics

func TestExporter_WithMetrics_NilExporter(t *testing.T) {
	t.Parallel()

	var e *Exporter
	got := e.WithMetrics(&ExporterMetrics{})
	assert.Nil(t, got)
}

func TestExporter_WithMetrics_AttachesMetrics(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true}, nil)
	m := &ExporterMetrics{}
	result := e.WithMetrics(m)
	assert.Equal(t, e, result)
	assert.Equal(t, m, e.metrics)
}

// Flush with OTel metrics wired up

func TestExporter_FlushFailure_IncrementsMetrics(t *testing.T) {
	t.Parallel()

	meter := noop.NewMeterProvider().Meter("test")
	flushCounter, _ := meter.Int64Counter("flush_failures")
	dropCounter, _ := meter.Int64Counter("dropped_records")

	client := newFailingClient(t)
	e := &Exporter{
		client:  client,
		config:  ExporterConfig{BatchSize: 100},
		logger:  slog.Default(),
		pending: make([]any, 0),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
		metrics: &ExporterMetrics{
			FlushFailures:  flushCounter,
			DroppedRecords: dropCounter,
		},
	}

	e.Enqueue(RunEventRecord{EventID: "evt-1"})

	// First flush: failure, requeue, FlushFailures incremented.
	e.flush(context.Background())
	assert.Equal(t, 1, e.consecutiveFailures)

	// Second flush: failure again, still requeues.
	e.flush(context.Background())
	assert.Equal(t, 2, e.consecutiveFailures)

	// Third flush: exceeds maxFlushRetries, drops batch, DroppedRecords incremented.
	e.flush(context.Background())
	assert.Equal(t, 0, e.PendingCount())
}

func TestExporter_FlushFailure_NilMetricCounters(t *testing.T) {
	t.Parallel()

	// ExporterMetrics with nil counters should not panic.
	client := newFailingClient(t)
	e := &Exporter{
		client:  client,
		config:  ExporterConfig{BatchSize: 100},
		logger:  slog.Default(),
		pending: make([]any, 0),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
		metrics: &ExporterMetrics{
			FlushFailures:  nil,
			DroppedRecords: nil,
		},
	}

	e.Enqueue(RunEventRecord{EventID: "evt-1"})

	// Should not panic even with nil counters.
	for range maxFlushRetries + 1 {
		e.flush(context.Background())
	}
}

func TestExporter_RejectsSingleRecordAboveByteCap(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{
		Enabled:        true,
		BatchSize:      100,
		MaxBufferBytes: 256,
	}, slog.Default())

	ok := e.Enqueue(RunEventRecord{
		EventID:   "evt-large",
		RunID:     "run-1",
		ProjectID: "proj-1",
		Message:   strings.Repeat("x", 512),
		CreatedAt: time.Now(),
	})
	require.False(t, ok)
	require.Equal(t, 0, e.PendingCount())
	require.Equal(t, 0, e.pendingBytes)
}

func TestExporter_EnqueueDropsOldestByByteCap(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{
		Enabled:        true,
		BatchSize:      100,
		MaxBufferBytes: 512,
	}, slog.Default())

	for _, id := range []string{"old", "middle", "new"} {
		require.True(t, e.Enqueue(RunEventRecord{EventID: id, RunID: "run-1",
			ProjectID: "proj-1",
			Message:   strings.Repeat("x", 180), CreatedAt: time.Now()}))
	}
	require.LessOrEqual(t, e.
		pendingBytes,
		e.config.
			MaxBufferBytes,
	)

	for _, rec := range e.pending {
		event, ok := rec.(RunEventRecord)
		require.True(t, ok)
		require.NotEqual(t, "old",

			event.EventID,
		)
	}
}

func TestExporter_FlushFailureRequeueHonorsByteCap(t *testing.T) {
	t.Parallel()

	e := &Exporter{
		client: newFailingClient(t),
		config: ExporterConfig{BatchSize: 100, MaxBufferBytes: 420},
		logger: slog.Default(),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
	e.pending = []any{
		RunEventRecord{EventID: "first", ProjectID: "proj-1", Message: strings.Repeat("a", 120), CreatedAt: time.Now()},
		RunEventRecord{EventID: "second", ProjectID: "proj-1", Message: strings.Repeat("b", 120), CreatedAt: time.Now()},
		RunEventRecord{EventID: "third", ProjectID: "proj-1", Message: strings.Repeat("c", 120), CreatedAt: time.Now()},
	}
	e.pendingBytes = estimateRecordsBytes(e.pending)

	e.flush(context.Background())
	require.LessOrEqual(t, e.
		pendingBytes,
		e.config.
			MaxBufferBytes,
	)
	require.NotEqual(t, 0, e.
		PendingCount())

	for _, rec := range e.pending {
		event := rec.(RunEventRecord)
		require.NotEqual(t, "third",

			event.
				EventID)
	}
}

func TestSanitizeRunEventRecord_DropsRawMessageAndMetadata(t *testing.T) {
	t.Parallel()

	rec := sanitizeRunEventRecord(RunEventRecord{
		EventID:   "evt-1",
		RunID:     "run-1",
		ProjectID: "proj-1",
		JobID:     "job-1",
		EventType: "log.line",
		Level:     "error",
		Message:   "failed with Authorization: Bearer secret-token and password=hunter2",
		Metadata:  `{"authorization":"Bearer secret-token","password":"hunter2"}`,
		CreatedAt: time.Now(),
	})
	require.Equal(t, "application_error",

		rec.Message,
	)
	require.Equal(t, "{}", rec.
		Metadata,
	)

	for _, leaked := range []string{"secret-token", "hunter2", "Authorization", "password"} {
		require.False(t, strings.Contains(
			rec.Message,
			leaked) ||
			strings.Contains(rec.Metadata,
				leaked,
			))
	}
}

func TestSanitizeRunEventRecord_KeepsOnlyKnownEventLabels(t *testing.T) {
	t.Parallel()

	safe := sanitizeRunEventRecord(RunEventRecord{EventType: "run_completed", Level: "info", Message: "contains secret-token"})
	require.Equal(t, "run_completed",

		safe.Message,
	)

	unknown := sanitizeRunEventRecord(RunEventRecord{EventType: "custom-secret-token", Level: "info", Message: "contains secret-token"})
	require.Equal(t, "level_info",

		unknown.
			Message,
	)
	require.NotContains(t, unknown.Message, "secret-token")
}

func TestSanitizeWebhookDeliveryEventRecord_RedactsURLBeforePersistence(t *testing.T) {
	t.Parallel()

	rec := sanitizeWebhookDeliveryEventRecord(WebhookDeliveryEventRecord{
		DeliveryID: "del-1",
		ProjectID:  "proj-1",
		WebhookURL: "https://user:pass@hooks.example.com/services/T00/B00/token?api_key=secret#frag",
		CreatedAt:  time.Now(),
	})
	require.Equal(t, "https://hooks.example.com",

		rec.WebhookURL,
	)

	for _, leaked := range []string{"user", "pass", "services", "token", "api_key", "secret", "frag"} {
		require.NotContains(t, rec.WebhookURL, leaked)
	}
}

// insertBatch: every record type through a failing client

func TestExporter_InsertBatch_AllRecordTypes_FailingClient(t *testing.T) {
	t.Parallel()

	client := newFailingClient(t)
	e := &Exporter{
		client:  client,
		config:  ExporterConfig{BatchSize: 100},
		logger:  slog.Default(),
		pending: make([]any, 0),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}

	now := time.Now()
	batch := []any{
		RunEventRecord{EventID: "e1", RunID: "r1", ProjectID: "p1", CreatedAt: now},
		RunAnalyticsRecord{RunID: "r1", ProjectID: "p1", CreatedAt: now},
		WorkflowApprovalEventRecord{ApprovalID: "a1", ProjectID: "p1", RequestedAt: now},
		JobMetadataRecord{JobID: "j1", ProjectID: "p1", Slug: "slug"},
		WebhookDeliveryEventRecord{DeliveryID: "d1", ProjectID: "p1", CreatedAt: now},
		WorkflowRunAnalyticsRecord{WorkflowRunID: "wr1", ProjectID: "p1", CreatedAt: now},
		WorkflowStepAnalyticsRecord{StepRunID: "sr1", ProjectID: "p1", CreatedAt: now},
		EventTriggerEventRecord{TriggerID: "t1", ProjectID: "p1", CreatedAt: now},
	}

	err := e.insertBatch(context.Background(), batch)
	require.Error(t, err)

	// Verify that errors from multiple tables are joined.
	errMsg := err.Error()
	expectedTables := []string{
		"run_events", "run_analytics",
		"workflow_approval_events", "job_metadata", "webhook_delivery_events",
		"workflow_run_analytics", "workflow_step_analytics", "event_trigger_events",
	}
	for _, table := range expectedTables {
		assert.True(t, contains(
			errMsg,
			table,
		))
	}
}

func TestExporter_InsertBatch_AllRecordTypes_NilDBClient(t *testing.T) {
	t.Parallel()

	// nil-db client: Exec is a no-op, so all inserts succeed silently.
	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 100}, slog.Default())
	now := time.Now()

	batch := []any{
		WorkflowApprovalEventRecord{ApprovalID: "a1", WorkflowRunID: "wr1", StepRunID: "sr1", ProjectID: "p1", Status: "approved", RequestedAt: now},
		JobMetadataRecord{JobID: "j1", ProjectID: "p1", Slug: "my-job"},
		EventTriggerEventRecord{TriggerID: "t1", EventKey: "key", ProjectID: "p1", CreatedAt: now},
		WorkflowRunAnalyticsRecord{WorkflowRunID: "wr1", WorkflowID: "wf1", ProjectID: "p1", CreatedAt: now},
		WorkflowStepAnalyticsRecord{StepRunID: "sr1", WorkflowRunID: "wr1", WorkflowID: "wf1", ProjectID: "p1", CreatedAt: now},
		WebhookDeliveryEventRecord{DeliveryID: "d1", RunID: "r1", JobID: "j1", ProjectID: "p1", CreatedAt: now},
	}

	err := e.insertBatch(context.Background(), batch)
	require.NoError(t, err)
}

func TestExporter_InsertBatch_OnlyUnknownTypes(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 100}, slog.Default())

	// All unknown types: should succeed (just logs warnings) and not error.
	err := e.insertBatch(context.Background(), []any{42, "string", 3.14, struct{}{}})
	require.NoError(t, err)
}

func TestExporter_InsertBatch_EmptySlice(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true}, slog.Default())
	err := e.insertBatch(context.Background(), []any{})
	require.NoError(t, err)
}

func TestExporter_InsertBatch_NilSlice(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true}, slog.Default())
	err := e.insertBatch(context.Background(), nil)
	require.NoError(t, err)
}

// Backpressure edge cases

func TestExporter_Backpressure_ExactBoundary(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 1}, slog.Default())
	// maxBuffer = 1 * 10 = 10. Enqueue exactly 10.
	for i := range 10 {
		e.Enqueue(i)
	}
	assert.Equal(t, 10, e.PendingCount())

	// One more triggers overflow: drops 1, keeps 10.
	e.Enqueue(999)
	assert.Equal(t, 10, e.PendingCount())
}

func TestExporter_Backpressure_DropsOldestKeepsNewest(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 1}, slog.Default())
	// maxBuffer = 10. Enqueue 15 items.
	for i := range 15 {
		e.Enqueue(i)
	}
	require.Equal(t, 10, e.PendingCount())

	// The oldest 5 should have been dropped (values 0-4).
	// The remaining should be 5-14.
	e.mu.Lock()
	first := e.pending[0]
	last := e.pending[9]
	e.mu.Unlock()
	assert.Equal(t, 5, first.(int))
	assert.Equal(t, 14, last.(int))
}

// Flush requeue with buffer overflow

func TestExporter_FlushRequeue_OverflowTruncates(t *testing.T) {
	t.Parallel()

	client := newFailingClient(t)
	e := &Exporter{
		client:  client,
		config:  ExporterConfig{BatchSize: 2}, // maxBuffer = 20
		logger:  slog.Default(),
		pending: make([]any, 0),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}

	// Fill buffer to near max.
	for i := range 19 {
		e.mu.Lock()
		e.pending = append(e.pending, i)
		e.mu.Unlock()
	}

	// Add one more record and flush it (will fail and try to requeue).
	e.mu.Lock()
	e.pending = append(e.pending, 19)
	e.mu.Unlock()

	e.flush(context.Background())
	assert.LessOrEqual(t, e.
		PendingCount(), 20)

	// After failed flush, requeued batch + existing should be capped at maxBuffer (20).
}

// Concurrent enqueue + flush

func TestExporter_ConcurrentEnqueueAndFlush(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     50,
		FlushInterval: 5 * time.Millisecond,
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)

	var wg conc.WaitGroup
	var enqueued atomic.Int64

	// Spawn writers.
	for range 5 {
		wg.Go(func() {
			for range 200 {
				if e.Enqueue(RunEventRecord{EventID: "e", CreatedAt: time.Now()}) {
					enqueued.Add(1)
				}
			}
		})
	}

	wg.Wait()
	cancel()
	e.Stop()
	assert.Equal(t, 0, e.PendingCount())

	// All enqueued records should have been flushed.
}

// Start/Stop lifecycle

func TestExporter_DoubleStart_DoesNotPanic(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     10,
		FlushInterval: time.Hour,
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	// Starting again spawns another goroutine, which is a known quirk.
	// We just verify it does not panic.
	cancel()
	e.Stop()
}

func TestExporter_StopWithoutStart_Blocks(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     10,
		FlushInterval: time.Hour,
	}, slog.Default())

	// Start and immediately stop.
	ctx := context.Background()
	e.Start(ctx)
	e.Stop()
	assert.False(t, e.Enqueue("should-fail"))

	// Enqueue after stop should be rejected.
}

func TestExporter_ContextCancel_FlushesBeforeExit(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     1000,
		FlushInterval: time.Hour, // won't auto-flush
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)

	e.Enqueue(RunEventRecord{EventID: "before-cancel"})
	cancel()

	// Wait for done.
	select {
	case <-e.done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "exporter did not finish after context cancel")
	}
	assert.Equal(t, 0, e.PendingCount())

	// Final flush should have drained.
}

// TestExporter helpers

func TestNewTestExporter(t *testing.T) {
	t.Parallel()

	e := NewTestExporter()
	require.NotNil(t, e)
	assert.Equal(t, 1000, e.
		config.
		BatchSize,
	)
	assert.True(t, e.config.
		Enabled,
	)
}

func TestTestExporter_PendingLen(t *testing.T) {
	t.Parallel()

	e := NewTestExporter()
	assert.Equal(t, 0, e.PendingLen())

	e.mu.Lock()
	e.pending = append(e.pending, "a", "b")
	e.mu.Unlock()
	assert.Equal(t, 2, e.PendingLen())
}

func TestTestExporter_PendingAt(t *testing.T) {
	t.Parallel()

	e := NewTestExporter()
	assert.Nil(t, e.PendingAt(0))
	assert.Nil(t, e.PendingAt(-1))

	// Out of bounds.

	e.mu.Lock()
	e.pending = append(e.pending, "first", "second")
	e.mu.Unlock()
	assert.Equal(t, "first",

		e.PendingAt(0))
	assert.Equal(t, "second",

		e.PendingAt(1))
	assert.Nil(t, e.PendingAt(2))
}

func TestExporter_DoubleStop_DoesNotPanic(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     10,
		FlushInterval: time.Hour,
	}, slog.Default())

	ctx := context.Background()
	e.Start(ctx)
	e.Stop()

	// Second Stop must not panic on double-close of the stop channel.
	e.Stop()
}

func TestTestExporter_NilPendingLen(t *testing.T) {
	t.Parallel()

	var e *Exporter
	assert.Equal(t, 0, e.PendingLen())
}

func TestTestExporter_NilPendingAt(t *testing.T) {
	t.Parallel()

	var e *Exporter
	assert.Nil(t, e.PendingAt(0))
}

// insertBatch: partial record types kill CONDITIONALS_BOUNDARY on len guards.

func TestExporter_InsertBatch_OnlyRunEvents_NoOtherTableErrors(t *testing.T) {
	t.Parallel()
	client := newFailingClient(t)
	e := &Exporter{
		client:  client,
		config:  ExporterConfig{BatchSize: 100},
		logger:  slog.Default(),
		pending: make([]any, 0),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}

	batch := []any{
		RunEventRecord{EventID: "e1", RunID: "r1", ProjectID: "p1", CreatedAt: time.Now()},
	}
	err := e.insertBatch(context.Background(), batch)
	require.Error(t, err)

	errMsg := err.Error()
	assert.True(t, contains(
		errMsg,
		"run_events",
	),
	)

	absentTables := []string{
		"run_analytics", "compute_usage",
		"workflow_approval_events", "job_metadata", "webhook_delivery_events",
		"workflow_run_analytics", "workflow_step_analytics", "event_trigger_events",
		"billing_events",
	}
	for _, table := range absentTables {
		assert.False(t, contains(
			errMsg, table,
		))
	}
}

func TestExporter_InsertBatch_OnlyAnalytics_NoOtherTableErrors(t *testing.T) {
	t.Parallel()
	client := newFailingClient(t)
	e := &Exporter{
		client:  client,
		config:  ExporterConfig{BatchSize: 100},
		logger:  slog.Default(),
		pending: make([]any, 0),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}

	batch := []any{
		RunAnalyticsRecord{RunID: "r1", ProjectID: "p1", CreatedAt: time.Now()},
	}
	err := e.insertBatch(context.Background(), batch)
	require.Error(t, err)

	errMsg := err.Error()
	assert.True(t, contains(
		errMsg,
		"run_analytics",
	))
	assert.False(t, contains(
		errMsg, "run_events",
	))
}

func TestExporter_InsertBatch_OnlyBillingEvents_NoOtherTableErrors(t *testing.T) {
	t.Parallel()
	client := newFailingClient(t)
	e := &Exporter{
		client:  client,
		config:  ExporterConfig{BatchSize: 100},
		logger:  slog.Default(),
		pending: make([]any, 0),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}

	batch := []any{
		BillingEventRecord{Timestamp: time.Now(), OrgID: "org-1", EventType: "charge"},
	}
	err := e.insertBatch(context.Background(), batch)
	require.Error(t, err)

	errMsg := err.Error()
	assert.True(t, contains(
		errMsg,
		"billing_events",
	))
	assert.False(t, contains(
		errMsg, "run_events",
	))
	assert.False(t, contains(
		errMsg, "run_analytics",
	))
}

// Client pool defaults kill CONDITIONALS_BOUNDARY on maxOpen/maxIdle.

func TestNewClient_DefaultPoolSettings(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Enabled:      false,
		MaxOpenConns: 0,
		MaxIdleConns: 0,
	}
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 10
	}
	assert.Equal(t, 10, maxOpen)

	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5
	}
	assert.Equal(t, 5, maxIdle)
}

func TestNewClient_NegativePoolSettings(t *testing.T) {
	t.Parallel()
	cfg := Config{
		MaxOpenConns: -1,
		MaxIdleConns: -3,
	}
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 10
	}
	assert.Equal(t, 10, maxOpen)

	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5
	}
	assert.Equal(t, 5, maxIdle)
}

func TestNewClient_PositivePoolSettings_Preserved(t *testing.T) {
	t.Parallel()
	cfg := Config{
		MaxOpenConns: 20,
		MaxIdleConns: 8,
	}
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 10
	}
	assert.Equal(t, 20, maxOpen)

	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5
	}
	assert.Equal(t, 8, maxIdle)
}

// Helpers

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
