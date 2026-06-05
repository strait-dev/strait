package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/clickhouse"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
)

// adaptive.go:Run -- context cancellation, error handling, probe updates

func TestAdaptiveRun_NilProbe_ReturnsImmediately(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 10, 5)

	done := make(chan struct{})
	concWG.Go(func() {
		a.Run(t.Context(), time.Millisecond, nil, slog.Default())
		close(done)
	})

	select {
	case <-done:
		// expected: Run returns immediately for nil probe
	case <-time.After(time.Second):
		t.Fatal("Run did not return for nil probe")
	}
}

func TestAdaptiveRun_ContextCancellation(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 100, 10)
	ctx, cancel := context.WithCancel(context.Background())

	var probeCalls atomic.Int32
	probe := func(_ context.Context) (int, float64, error) {
		probeCalls.Add(1)
		return 0, 0.0, nil
	}

	done := make(chan struct{})
	concWG.Go(func() {
		a.Run(ctx, time.Millisecond, probe, slog.Default())
		close(done)
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if probeCalls.Load() >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()

	select {
	case <-done:
		if probeCalls.Load() == 0 {
			t.Fatal("probe was never called before cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after context cancellation")
	}
}

func TestAdaptiveRun_ProbeError_ContinuesPolling(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 100, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var callCount atomic.Int32
	probe := func(_ context.Context) (int, float64, error) {
		n := callCount.Add(1)
		if n <= 3 {
			return 0, 0, fmt.Errorf("transient error %d", n)
		}
		// After errors, return a signal that should scale up.
		return 5000, 0.95, nil
	}

	done := make(chan struct{})
	concWG.Go(func() {
		a.Run(ctx, time.Millisecond, probe, slog.Default())
		close(done)
	})

	// Wait until the probe has been called enough times for the scale-up.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if callCount.Load() > 4 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	<-done

	if callCount.Load() <= 3 {
		t.Fatalf("expected probe to be called after errors, got %d calls", callCount.Load())
	}
	// After the successful probe with deep queue, limit should have increased.
	if a.CurrentLimit() <= 10 {
		t.Fatalf("expected limit to increase from 10, got %d", a.CurrentLimit())
	}
}

func TestAdaptiveRun_ZeroInterval_DefaultsTo10s(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 100, 10)
	ctx, cancel := context.WithCancel(context.Background())

	var probeCalls atomic.Int32
	probe := func(_ context.Context) (int, float64, error) {
		probeCalls.Add(1)
		return 0, 0.0, nil
	}
	concWG.Go(func() {
		a.Run(ctx, 0, probe, nil) // zero interval, nil logger
	})

	// With a 10s default interval, no probe should fire in 50ms.
	time.Sleep(50 * time.Millisecond)
	cancel()

	if probeCalls.Load() > 0 {
		t.Fatalf("expected 0 probe calls with 10s default interval in 50ms window, got %d", probeCalls.Load())
	}
}

func TestAdaptiveRun_NilLogger_DoesNotPanic(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 100, 10)
	ctx, cancel := context.WithCancel(context.Background())

	var probeCalls atomic.Int32
	probe := func(_ context.Context) (int, float64, error) {
		probeCalls.Add(1)
		return 0, 0, errors.New("probe error with nil logger")
	}

	done := make(chan struct{})
	concWG.Go(func() {
		a.Run(ctx, time.Millisecond, probe, nil)
		close(done)
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if probeCalls.Load() >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()

	select {
	case <-done:
		// no panic
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit")
	}
}

func TestAdaptiveRun_UpdatesLimit_WhenProbeSignalsLoad(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 200, 10)
	ctx, cancel := context.WithCancel(context.Background())

	probe := func(_ context.Context) (int, float64, error) {
		return 2000, 0.95, nil // deep queue, high utilization
	}

	done := make(chan struct{})
	concWG.Go(func() {
		a.Run(ctx, time.Millisecond, probe, slog.Default())
		close(done)
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if a.CurrentLimit() > 10 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	<-done

	if a.CurrentLimit() <= 10 {
		t.Fatalf("Run did not update limit: got %d, wanted > 10", a.CurrentLimit())
	}
}

func TestAdaptiveConcurrency_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 1000, 50)
	var wg conc.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			for j := range 100 {
				_ = a.Observe(i*100+j, float64(i)/10.0)
				_ = a.CurrentLimit()
			}
		})
	}

	wg.Wait()

	lim := a.CurrentLimit()
	if lim < 1 || lim > 1000 {
		t.Fatalf("limit out of bounds after concurrent access: %d", lim)
	}
}

func TestAdaptiveConcurrency_ZeroQueueDepth_ScaleDown(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 100, 1)
	// At minimum: two idle checks should not go below min.
	_ = a.Observe(0, 0.0)
	next := a.Observe(0, 0.0)
	if next != 1 {
		t.Fatalf("expected min=1, got %d", next)
	}
}

func TestAdaptiveConcurrency_MaxQueueDepth_ScaleUp(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 50, 25)
	// Max int queue depth should not overflow.
	next := a.Observe(1<<30, 1.0)
	if next != 50 {
		t.Fatalf("expected max=50, got %d", next)
	}
}

func TestAdaptiveConcurrency_RapidFluctuations(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(5, 100, 20)

	// Rapidly alternate between deep queue and idle.
	for range 50 {
		a.Observe(5000, 0.99)
		a.Observe(0, 0.01)
	}

	lim := a.CurrentLimit()
	if lim < 5 || lim > 100 {
		t.Fatalf("limit out of bounds after fluctuations: %d", lim)
	}
}

func TestAdaptiveConcurrency_NegativeInterval_DefaultsGracefully(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 10, 5)
	ctx, cancel := context.WithCancel(context.Background())

	probe := func(_ context.Context) (int, float64, error) {
		return 0, 0.0, nil
	}
	concWG.Go(func() {
		a.Run(ctx, -1*time.Second, probe, slog.Default())
	})

	// Negative interval should be treated as 10s default.
	time.Sleep(50 * time.Millisecond)
	cancel()
}

// executor_dispatch.go:ingestStripeUsageEvent -- Stripe usage event ingestion

// mockBillingEnforcerForStripeUsage wraps billing.Enforcer methods needed by ingestStripeUsageEvent.
// Since billing.Enforcer is a concrete type, we test ingestStripeUsageEvent via the
// Executor struct directly, relying on nil checks for early returns.

func TestIngestStripeUsageEvent_NilStripeUsageReporter(t *testing.T) {
	t.Parallel()

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:         pool,
		Queue:        &mockExecQueue{},
		Store:        &mockExecutorStore{},
		PollInterval: time.Millisecond,
		// StripeUsageReporter is nil, BillingEnforcer is nil
	})

	// Should return immediately without panic.
	exec.ingestStripeUsageEvent(context.Background(), "proj-1", "run-1", billing.HTTPCostPerRunMicrousd)
}

func TestIngestStripeUsageEvent_NilBillingEnforcer(t *testing.T) {
	t.Parallel()

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:                pool,
		Queue:               &mockExecQueue{},
		Store:               &mockExecutorStore{},
		PollInterval:        time.Millisecond,
		StripeUsageReporter: billing.NewStripeUsageReporter("sk_test_key", nil),
		// BillingEnforcer is nil
	})

	// Should return immediately without panic.
	exec.ingestStripeUsageEvent(context.Background(), "proj-1", "run-1", billing.HTTPCostPerRunMicrousd)
}

func TestIngestStripeUsageEvent_BothNil(t *testing.T) {
	t.Parallel()

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:         pool,
		Queue:        &mockExecQueue{},
		Store:        &mockExecutorStore{},
		PollInterval: time.Millisecond,
	})

	// Both nil: should silently return.
	exec.ingestStripeUsageEvent(context.Background(), "proj-1", "run-1", billing.HTTPCostPerRunMicrousd)
}

// subscriber_clickhouse.go -- event transformation adversarial cases

func TestRunEventsFromDomain_EmptySlice(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-1", ProjectID: "proj-1", JobID: "job-1"}
	records := runEventsFromDomain(run, nil)
	if len(records) != 0 {
		t.Fatalf("expected 0 records for nil events, got %d", len(records))
	}

	records = runEventsFromDomain(run, []domain.RunEvent{})
	if len(records) != 0 {
		t.Fatalf("expected 0 records for empty events, got %d", len(records))
	}
}

func TestRunEventsFromDomain_EmptyStrings(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "", ProjectID: "", JobID: ""}
	events := []domain.RunEvent{
		{ID: "", RunID: "", Type: "", Level: "", Message: "", Data: nil},
	}

	records := runEventsFromDomain(run, events)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.EventID != "" || r.RunID != "" || r.ProjectID != "" || r.JobID != "" {
		t.Errorf("expected all empty IDs, got %+v", r)
	}
	if r.Metadata != "" {
		t.Errorf("expected empty metadata for nil Data, got %q", r.Metadata)
	}
}

func TestRunEventsFromDomain_MalformedJSON(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-1", ProjectID: "proj-1", JobID: "job-1"}
	events := []domain.RunEvent{
		{
			ID:      "evt-1",
			RunID:   "run-1",
			Type:    "log",
			Level:   "info",
			Message: "test",
			Data:    json.RawMessage(`{broken json`),
		},
	}

	records := runEventsFromDomain(run, events)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	// Data is treated as raw bytes, not validated for JSON correctness.
	if records[0].Metadata != `{broken json` {
		t.Errorf("expected raw bytes as metadata, got %q", records[0].Metadata)
	}
}

func TestRunEventsFromDomain_LargeDataField(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-1", ProjectID: "proj-1", JobID: "job-1"}
	bigData := make([]byte, 0, 100_002)
	bigData = append(bigData, '"')
	for range 100_000 {
		bigData = append(bigData, 'x')
	}
	bigData = append(bigData, '"')

	events := []domain.RunEvent{
		{ID: "evt-1", RunID: "run-1", Data: json.RawMessage(bigData)},
	}

	records := runEventsFromDomain(run, events)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if len(records[0].Metadata) != len(bigData) {
		t.Errorf("metadata length = %d, want %d", len(records[0].Metadata), len(bigData))
	}
}

func TestRunEventsFromDomain_MixedEventTypes(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "run-1", ProjectID: "proj-1", JobID: "job-1"}
	now := time.Now()
	events := []domain.RunEvent{
		{ID: "evt-1", RunID: "run-1", Type: "log", Level: "info", Message: "step 1", CreatedAt: now},
		{ID: "evt-2", RunID: "run-1", Type: "error", Level: "error", Message: "failed", Data: json.RawMessage(`{"code":500}`), CreatedAt: now.Add(time.Second)},
		{ID: "evt-3", RunID: "run-1", Type: "checkpoint", Level: "debug", Message: "", Data: nil, CreatedAt: now.Add(2 * time.Second)},
	}

	records := runEventsFromDomain(run, events)
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Verify each record preserves the correct fields.
	if records[0].EventType != "log" || records[0].Level != "info" {
		t.Errorf("record 0 mismatch: type=%s level=%s", records[0].EventType, records[0].Level)
	}
	if records[1].EventType != "error" || records[1].Metadata != `{"code":500}` {
		t.Errorf("record 1 mismatch: type=%s metadata=%s", records[1].EventType, records[1].Metadata)
	}
	if records[2].Message != "" || records[2].Metadata != "" {
		t.Errorf("record 2 mismatch: message=%q metadata=%q", records[2].Message, records[2].Metadata)
	}
}

func TestClickHouseSubscriber_DurationCalculation_NoStartTime(t *testing.T) {
	t.Parallel()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	now := time.Now()

	sub := ClickHouseSubscriber(exporter, nil)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run: &domain.JobRun{
			ID:         "run-no-start",
			ProjectID:  "proj-1",
			Status:     domain.StatusCompleted,
			StartedAt:  nil,
			FinishedAt: &now,
		},
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", exporter.PendingCount())
	}
}

func TestClickHouseSubscriber_DurationCalculation_NoFinishTime(t *testing.T) {
	t.Parallel()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	now := time.Now()

	sub := ClickHouseSubscriber(exporter, nil)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run: &domain.JobRun{
			ID:        "run-no-finish",
			ProjectID: "proj-1",
			Status:    domain.StatusCompleted,
			StartedAt: &now,
		},
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", exporter.PendingCount())
	}
}

func TestClickHouseSubscriber_NegativeDuration(t *testing.T) {
	t.Parallel()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	now := time.Now()
	future := now.Add(5 * time.Second)

	// FinishedAt before StartedAt: negative duration should be clamped to 0.
	sub := ClickHouseSubscriber(exporter, nil)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run: &domain.JobRun{
			ID:         "run-neg-dur",
			ProjectID:  "proj-1",
			Status:     domain.StatusCompleted,
			StartedAt:  &future,
			FinishedAt: &now,
		},
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", exporter.PendingCount())
	}
}

func TestClickHouseSubscriber_ZeroQueueWait(t *testing.T) {
	t.Parallel()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	sub := ClickHouseSubscriber(exporter, nil)
	sub(context.Background(), RunLifecycleEvent{
		Type:      EventCompleted,
		Run:       &domain.JobRun{ID: "run-zqw", ProjectID: "proj-1"},
		QueueWait: 0,
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", exporter.PendingCount())
	}
}

func TestClickHouseSubscriber_NilJob_EmptyExecutionMode(t *testing.T) {
	t.Parallel()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	sub := ClickHouseSubscriber(exporter, nil)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  &domain.JobRun{ID: "run-nilj", ProjectID: "proj-1"},
		Job:  nil, // nil job
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", exporter.PendingCount())
	}
}

func TestClickHouseSubscriber_AnalyticsAndEventsEnqueued(t *testing.T) {
	t.Parallel()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 1000,
	}, slog.Default())

	now := time.Now()
	started := now.Add(-3 * time.Second)

	eventLister := &mockEventLister{
		events: []domain.RunEvent{
			{ID: "evt-1", RunID: "run-all", Type: "log", Level: "info", Message: "hello", CreatedAt: now},
		},
	}

	sub := ClickHouseSubscriber(exporter, eventLister)

	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run: &domain.JobRun{
			ID:         "run-all",
			ProjectID:  "proj-1",
			JobID:      "job-1",
			Status:     domain.StatusCompleted,
			StartedAt:  &started,
			FinishedAt: &now,
		},
		Job: &domain.Job{
			ExecutionMode: "http",
		},
		QueueWait: 100 * time.Millisecond,
	})

	// 1 analytics + 1 event = 2.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if exporter.PendingCount() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if exporter.PendingCount() != 2 {
		t.Errorf("expected 2 pending (analytics+event), got %d", exporter.PendingCount())
	}
}

func TestClickHouseSubscriber_NonTerminalTypes_NoEnqueue(t *testing.T) {
	t.Parallel()

	nonTerminal := []RunEventType{EventSnoozed, EventRetried}

	for _, et := range nonTerminal {
		t.Run(string(et), func(t *testing.T) {
			t.Parallel()

			exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
				Enabled:   true,
				BatchSize: 100,
			}, slog.Default())

			sub := ClickHouseSubscriber(exporter, nil)
			sub(context.Background(), RunLifecycleEvent{
				Type: et,
				Run:  &domain.JobRun{ID: "run-nt", ProjectID: "proj-1"},
			})

			if exporter.PendingCount() != 0 {
				t.Errorf("expected 0 pending for non-terminal %s, got %d", et, exporter.PendingCount())
			}
		})
	}
}

func TestClickHouseSubscriber_TagsMarshalSpecialChars(t *testing.T) {
	t.Parallel()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	sub := ClickHouseSubscriber(exporter, nil)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run: &domain.JobRun{
			ID:        "run-special",
			ProjectID: "proj-1",
			Status:    domain.StatusCompleted,
			Tags:      map[string]string{"key\"with\"quotes": "val\nwith\nnewlines", "emoji": "hello"},
		},
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", exporter.PendingCount())
	}
}

func TestRunEventsFromDomain_PreservesAllFields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	run := &domain.JobRun{ID: "run-fields", ProjectID: "proj-fields", JobID: "job-fields"}
	events := []domain.RunEvent{
		{
			ID:        "evt-fields",
			RunID:     "run-fields",
			Type:      "custom_type",
			Level:     "warn",
			Message:   "something happened",
			Data:      json.RawMessage(`[1,2,3]`),
			CreatedAt: now,
		},
	}

	records := runEventsFromDomain(run, events)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	r := records[0]
	if r.EventID != "evt-fields" {
		t.Errorf("EventID = %q, want %q", r.EventID, "evt-fields")
	}
	if r.RunID != "run-fields" {
		t.Errorf("RunID = %q, want %q", r.RunID, "run-fields")
	}
	if r.ProjectID != "proj-fields" {
		t.Errorf("ProjectID = %q, want %q", r.ProjectID, "proj-fields")
	}
	if r.JobID != "job-fields" {
		t.Errorf("JobID = %q, want %q", r.JobID, "job-fields")
	}
	if r.EventType != "custom_type" {
		t.Errorf("EventType = %q, want %q", r.EventType, "custom_type")
	}
	if r.Level != "warn" {
		t.Errorf("Level = %q, want %q", r.Level, "warn")
	}
	if r.Message != "something happened" {
		t.Errorf("Message = %q, want %q", r.Message, "something happened")
	}
	if r.Metadata != `[1,2,3]` {
		t.Errorf("Metadata = %q, want %q", r.Metadata, `[1,2,3]`)
	}
	if !r.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", r.CreatedAt, now)
	}
}
