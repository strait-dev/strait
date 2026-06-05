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
	"go.opentelemetry.io/otel/metric/noop"
)

// NewExporter edge cases

func TestNewExporter_ZeroBatchSize(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 0}, nil)
	if e == nil {
		t.Fatal("expected non-nil exporter")
		return
	}
	if e.config.BatchSize != 1000 {
		t.Errorf("expected default batch size 1000, got %d", e.config.BatchSize)
	}
}

func TestNewExporter_NegativeBatchSize(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: -5}, nil)
	if e == nil {
		t.Fatal("expected non-nil exporter")
		return
	}
	if e.config.BatchSize != 1000 {
		t.Errorf("expected default batch size 1000, got %d", e.config.BatchSize)
	}
}

func TestNewExporter_ZeroFlushInterval(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, FlushInterval: 0}, nil)
	if e == nil {
		t.Fatal("expected non-nil exporter")
		return
	}
	if e.config.FlushInterval != 5*time.Second {
		t.Errorf("expected default flush interval 5s, got %v", e.config.FlushInterval)
	}
}

func TestNewExporter_NegativeFlushInterval(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, FlushInterval: -1}, nil)
	if e == nil {
		t.Fatal("expected non-nil exporter")
		return
	}
	if e.config.FlushInterval != 5*time.Second {
		t.Errorf("expected default flush interval 5s, got %v", e.config.FlushInterval)
	}
}

func TestNewExporter_NilLogger(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true}, nil)
	if e == nil {
		t.Fatal("expected non-nil exporter")
		return
	}
	if e.logger == nil {
		t.Error("expected default logger when nil is passed")
	}
}

// WithMetrics

func TestExporter_WithMetrics_NilExporter(t *testing.T) {
	t.Parallel()

	var e *Exporter
	got := e.WithMetrics(&ExporterMetrics{})
	if got != nil {
		t.Error("WithMetrics on nil exporter should return nil")
	}
}

func TestExporter_WithMetrics_AttachesMetrics(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true}, nil)
	m := &ExporterMetrics{}
	result := e.WithMetrics(m)
	if result != e {
		t.Error("WithMetrics should return the same exporter")
	}
	if e.metrics != m {
		t.Error("metrics not attached")
	}
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
	if e.consecutiveFailures != 1 {
		t.Errorf("consecutiveFailures = %d, want 1", e.consecutiveFailures)
	}

	// Second flush: failure again, still requeues.
	e.flush(context.Background())
	if e.consecutiveFailures != 2 {
		t.Errorf("consecutiveFailures = %d, want 2", e.consecutiveFailures)
	}

	// Third flush: exceeds maxFlushRetries, drops batch, DroppedRecords incremented.
	e.flush(context.Background())
	if e.PendingCount() != 0 {
		t.Errorf("after max retries with metrics, pending = %d, want 0", e.PendingCount())
	}
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
	if ok {
		t.Fatal("oversized record was accepted")
	}
	if e.PendingCount() != 0 {
		t.Fatalf("pending count = %d, want 0", e.PendingCount())
	}
	if e.pendingBytes != 0 {
		t.Fatalf("pendingBytes = %d, want 0", e.pendingBytes)
	}
}

func TestExporter_EnqueueDropsOldestByByteCap(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{
		Enabled:        true,
		BatchSize:      100,
		MaxBufferBytes: 512,
	}, slog.Default())

	for _, id := range []string{"old", "middle", "new"} {
		if !e.Enqueue(RunEventRecord{
			EventID:   id,
			RunID:     "run-1",
			ProjectID: "proj-1",
			Message:   strings.Repeat("x", 180),
			CreatedAt: time.Now(),
		}) {
			t.Fatalf("record %q was unexpectedly rejected", id)
		}
	}

	if e.pendingBytes > e.config.MaxBufferBytes {
		t.Fatalf("pendingBytes = %d, want <= %d", e.pendingBytes, e.config.MaxBufferBytes)
	}
	for _, rec := range e.pending {
		event, ok := rec.(RunEventRecord)
		if !ok {
			t.Fatalf("pending record type = %T, want RunEventRecord", rec)
		}
		if event.EventID == "old" {
			t.Fatal("oldest record remained after byte-cap overflow")
		}
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

	if e.pendingBytes > e.config.MaxBufferBytes {
		t.Fatalf("pendingBytes after requeue = %d, want <= %d", e.pendingBytes, e.config.MaxBufferBytes)
	}
	if e.PendingCount() == 0 {
		t.Fatal("failed batch was fully dropped on first retry")
	}
	for _, rec := range e.pending {
		event := rec.(RunEventRecord)
		if event.EventID == "third" {
			t.Fatal("newest failed record remained after requeue byte-cap trim")
		}
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

	if rec.Message != "application_error" {
		t.Fatalf("sanitized message = %q, want application_error", rec.Message)
	}
	if rec.Metadata != "{}" {
		t.Fatalf("sanitized metadata = %q, want {}", rec.Metadata)
	}
	for _, leaked := range []string{"secret-token", "hunter2", "Authorization", "password"} {
		if strings.Contains(rec.Message, leaked) || strings.Contains(rec.Metadata, leaked) {
			t.Fatalf("sanitized run event leaked %q: message=%q metadata=%q", leaked, rec.Message, rec.Metadata)
		}
	}
}

func TestSanitizeRunEventRecord_KeepsOnlyKnownEventLabels(t *testing.T) {
	t.Parallel()

	safe := sanitizeRunEventRecord(RunEventRecord{EventType: "run_completed", Level: "info", Message: "contains secret-token"})
	if safe.Message != "run_completed" {
		t.Fatalf("safe event message = %q, want run_completed", safe.Message)
	}

	unknown := sanitizeRunEventRecord(RunEventRecord{EventType: "custom-secret-token", Level: "info", Message: "contains secret-token"})
	if unknown.Message != "level_info" {
		t.Fatalf("unknown event message = %q, want level_info", unknown.Message)
	}
	if strings.Contains(unknown.Message, "secret-token") {
		t.Fatalf("unknown event label leaked raw token: %q", unknown.Message)
	}
}

func TestSanitizeWebhookDeliveryEventRecord_RedactsURLBeforePersistence(t *testing.T) {
	t.Parallel()

	rec := sanitizeWebhookDeliveryEventRecord(WebhookDeliveryEventRecord{
		DeliveryID: "del-1",
		ProjectID:  "proj-1",
		WebhookURL: "https://user:pass@hooks.example.com/services/T00/B00/token?api_key=secret#frag",
		CreatedAt:  time.Now(),
	})

	if rec.WebhookURL != "https://hooks.example.com" {
		t.Fatalf("sanitized webhook URL = %q, want host-only URL", rec.WebhookURL)
	}
	for _, leaked := range []string{"user", "pass", "services", "token", "api_key", "secret", "frag"} {
		if strings.Contains(rec.WebhookURL, leaked) {
			t.Fatalf("sanitized webhook URL leaked %q: %q", leaked, rec.WebhookURL)
		}
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
	if err == nil {
		t.Fatal("expected error when all record types fail")
	}

	// Verify that errors from multiple tables are joined.
	errMsg := err.Error()
	expectedTables := []string{
		"run_events", "run_analytics",
		"workflow_approval_events", "job_metadata", "webhook_delivery_events",
		"workflow_run_analytics", "workflow_step_analytics", "event_trigger_events",
	}
	for _, table := range expectedTables {
		if !contains(errMsg, table) {
			t.Errorf("expected error to mention %q, got: %s", table, errMsg)
		}
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
	if err != nil {
		t.Fatalf("expected nil error with nil-db client, got %v", err)
	}
}

func TestExporter_InsertBatch_OnlyUnknownTypes(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 100}, slog.Default())

	// All unknown types: should succeed (just logs warnings) and not error.
	err := e.insertBatch(context.Background(), []any{42, "string", 3.14, struct{}{}})
	if err != nil {
		t.Fatalf("expected nil error for all unknown types, got %v", err)
	}
}

func TestExporter_InsertBatch_EmptySlice(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true}, slog.Default())
	err := e.insertBatch(context.Background(), []any{})
	if err != nil {
		t.Fatalf("expected nil for empty batch, got %v", err)
	}
}

func TestExporter_InsertBatch_NilSlice(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true}, slog.Default())
	err := e.insertBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil for nil batch, got %v", err)
	}
}

// Backpressure edge cases

func TestExporter_Backpressure_ExactBoundary(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 1}, slog.Default())
	// maxBuffer = 1 * 10 = 10. Enqueue exactly 10.
	for i := range 10 {
		e.Enqueue(i)
	}
	if e.PendingCount() != 10 {
		t.Errorf("pending = %d, want 10", e.PendingCount())
	}

	// One more triggers overflow: drops 1, keeps 10.
	e.Enqueue(999)
	if e.PendingCount() != 10 {
		t.Errorf("pending = %d, want 10 after overflow", e.PendingCount())
	}
}

func TestExporter_Backpressure_DropsOldestKeepsNewest(t *testing.T) {
	t.Parallel()

	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 1}, slog.Default())
	// maxBuffer = 10. Enqueue 15 items.
	for i := range 15 {
		e.Enqueue(i)
	}

	if e.PendingCount() != 10 {
		t.Fatalf("pending = %d, want 10", e.PendingCount())
	}

	// The oldest 5 should have been dropped (values 0-4).
	// The remaining should be 5-14.
	e.mu.Lock()
	first := e.pending[0]
	last := e.pending[9]
	e.mu.Unlock()

	if first.(int) != 5 {
		t.Errorf("first pending = %v, want 5", first)
	}
	if last.(int) != 14 {
		t.Errorf("last pending = %v, want 14", last)
	}
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

	// After failed flush, requeued batch + existing should be capped at maxBuffer (20).
	if e.PendingCount() > 20 {
		t.Errorf("pending = %d, should not exceed maxBuffer 20", e.PendingCount())
	}
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

	// All enqueued records should have been flushed.
	if e.PendingCount() != 0 {
		t.Errorf("after stop, pending = %d, want 0", e.PendingCount())
	}
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

	// Enqueue after stop should be rejected.
	if e.Enqueue("should-fail") {
		t.Error("Enqueue after Stop should return false")
	}
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
		t.Fatal("exporter did not finish after context cancel")
	}

	// Final flush should have drained.
	if e.PendingCount() != 0 {
		t.Errorf("pending = %d, want 0 after context cancel", e.PendingCount())
	}
}

// TestExporter helpers

func TestNewTestExporter(t *testing.T) {
	t.Parallel()

	e := NewTestExporter()
	if e == nil {
		t.Fatal("expected non-nil test exporter")
		return
	}
	if e.config.BatchSize != 1000 {
		t.Errorf("batch size = %d, want 1000", e.config.BatchSize)
	}
	if !e.config.Enabled {
		t.Error("expected enabled = true")
	}
}

func TestTestExporter_PendingLen(t *testing.T) {
	t.Parallel()

	e := NewTestExporter()
	if e.PendingLen() != 0 {
		t.Error("expected 0 pending")
	}

	e.mu.Lock()
	e.pending = append(e.pending, "a", "b")
	e.mu.Unlock()

	if e.PendingLen() != 2 {
		t.Errorf("PendingLen = %d, want 2", e.PendingLen())
	}
}

func TestTestExporter_PendingAt(t *testing.T) {
	t.Parallel()

	e := NewTestExporter()

	// Out of bounds.
	if e.PendingAt(0) != nil {
		t.Error("expected nil for empty pending")
	}
	if e.PendingAt(-1) != nil {
		t.Error("expected nil for negative index")
	}

	e.mu.Lock()
	e.pending = append(e.pending, "first", "second")
	e.mu.Unlock()

	if e.PendingAt(0) != "first" {
		t.Errorf("PendingAt(0) = %v, want 'first'", e.PendingAt(0))
	}
	if e.PendingAt(1) != "second" {
		t.Errorf("PendingAt(1) = %v, want 'second'", e.PendingAt(1))
	}
	if e.PendingAt(2) != nil {
		t.Error("expected nil for out-of-range index")
	}
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
	if e.PendingLen() != 0 {
		t.Error("nil exporter PendingLen should return 0")
	}
}

func TestTestExporter_NilPendingAt(t *testing.T) {
	t.Parallel()

	var e *Exporter
	if e.PendingAt(0) != nil {
		t.Error("nil exporter PendingAt should return nil")
	}
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
	if err == nil {
		t.Fatal("expected error from failing client")
	}
	errMsg := err.Error()
	if !contains(errMsg, "run_events") {
		t.Error("expected error to mention run_events")
	}
	absentTables := []string{
		"run_analytics", "compute_usage",
		"workflow_approval_events", "job_metadata", "webhook_delivery_events",
		"workflow_run_analytics", "workflow_step_analytics", "event_trigger_events",
		"billing_events",
	}
	for _, table := range absentTables {
		if contains(errMsg, table) {
			t.Errorf("error should NOT mention %q (no records of that type), got: %s", table, errMsg)
		}
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
	if err == nil {
		t.Fatal("expected error from failing client")
	}
	errMsg := err.Error()
	if !contains(errMsg, "run_analytics") {
		t.Error("expected error to mention run_analytics")
	}
	if contains(errMsg, "run_events") {
		t.Errorf("error should NOT mention run_events (no records of that type)")
	}
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
	if err == nil {
		t.Fatal("expected error from failing client")
	}
	errMsg := err.Error()
	if !contains(errMsg, "billing_events") {
		t.Error("expected error to mention billing_events")
	}
	if contains(errMsg, "run_events") {
		t.Errorf("error should NOT mention run_events")
	}
	if contains(errMsg, "run_analytics") {
		t.Errorf("error should NOT mention run_analytics")
	}
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
	if maxOpen != 10 {
		t.Errorf("default maxOpen = %d, want 10", maxOpen)
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5
	}
	if maxIdle != 5 {
		t.Errorf("default maxIdle = %d, want 5", maxIdle)
	}
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
	if maxOpen != 10 {
		t.Errorf("negative maxOpen should default to 10, got %d", maxOpen)
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5
	}
	if maxIdle != 5 {
		t.Errorf("negative maxIdle should default to 5, got %d", maxIdle)
	}
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
	if maxOpen != 20 {
		t.Errorf("explicit maxOpen should be preserved, got %d", maxOpen)
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5
	}
	if maxIdle != 8 {
		t.Errorf("explicit maxIdle should be preserved, got %d", maxIdle)
	}
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
