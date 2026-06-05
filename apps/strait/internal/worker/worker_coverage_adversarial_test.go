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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		require.Fail(t, "Run did not return for nil probe")
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
			require.Fail(t,

				"probe was never called before cancellation")
		}
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not exit after context cancellation")
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
	require.Greater(t,
		callCount.Load(), int32(3))
	require.Greater(t,
		a.CurrentLimit(), 10)

	// After the successful probe with deep queue, limit should have increased.
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
	require.LessOrEqual(t, probeCalls.
		Load(), int32(0),
	)
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
		require.Fail(t, "Run did not exit")
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
	require.Greater(t,
		a.CurrentLimit(), 10)
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
	require.False(t,
		lim < 1 || lim >
			1000)
}

func TestAdaptiveConcurrency_ZeroQueueDepth_ScaleDown(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 100, 1)
	// At minimum: two idle checks should not go below min.
	_ = a.Observe(0, 0.0)
	next := a.Observe(0, 0.0)
	require.Equal(t, 1, next)
}

func TestAdaptiveConcurrency_MaxQueueDepth_ScaleUp(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(1, 50, 25)
	// Max int queue depth should not overflow.
	next := a.Observe(1<<30, 1.0)
	require.Equal(t, 50, next)
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
	require.False(t,
		lim < 5 || lim >
			100)
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
	require.Empty(t, records)

	records = runEventsFromDomain(run, []domain.RunEvent{})
	require.Empty(t, records)
}

func TestRunEventsFromDomain_EmptyStrings(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{ID: "", ProjectID: "", JobID: ""}
	events := []domain.RunEvent{
		{ID: "", RunID: "", Type: "", Level: "", Message: "", Data: nil},
	}

	records := runEventsFromDomain(run, events)
	require.Len(t, records,
		1)

	r := records[0]
	assert.False(t,
		r.EventID != "" ||
			r.RunID !=
				"" ||
			r.ProjectID !=
				"" || r.JobID !=
			"")
	assert.Empty(t,
		r.Metadata)
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
	require.Len(t, records,
		1)
	assert.Equal(t,
		`{broken json`, records[0].
			Metadata,
	)

	// Data is treated as raw bytes, not validated for JSON correctness.
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
	require.Len(t, records,
		1)
	assert.Len(t, records[0].Metadata,
		len(bigData))
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
	require.Len(t, records,
		3)
	assert.False(t,
		records[0].EventType !=
			"log" ||
			records[0].Level !=
				"info")
	assert.False(t,
		records[1].EventType !=
			"error" ||
			records[1].
				Metadata != `{"code":500}`,
	)
	assert.False(t,
		records[2].Message !=
			"" ||
			records[2].Metadata !=
				"")

	// Verify each record preserves the correct fields.
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
	assert.Equal(t, 1, exporter.PendingCount())
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
	assert.Equal(t, 1, exporter.PendingCount())
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
	assert.Equal(t, 1, exporter.PendingCount())
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
	assert.Equal(t, 1, exporter.PendingCount())
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
	assert.Equal(t, 1, exporter.PendingCount())
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
	assert.Equal(t, 2, exporter.PendingCount())
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
			assert.Equal(t, 0, exporter.PendingCount())
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
	assert.Equal(t, 1, exporter.PendingCount())
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
	require.Len(t, records,
		1)

	r := records[0]
	assert.Equal(t,
		"evt-fields", r.EventID,
	)
	assert.Equal(t,
		"run-fields", r.RunID,
	)
	assert.Equal(t,
		"proj-fields", r.
			ProjectID)
	assert.Equal(t,
		"job-fields", r.JobID,
	)
	assert.Equal(t,
		"custom_type", r.
			EventType)
	assert.Equal(t,
		"warn", r.Level)
	assert.Equal(t,
		"something happened",
		r.Message,
	)
	assert.Equal(t,
		`[1,2,3]`, r.Metadata,
	)
	assert.True(t, r.
		CreatedAt.Equal(
		now))
}
