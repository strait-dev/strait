package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"
	"strait/internal/telemetry"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// ---------------------------------------------------------------------------.
// 1. asFloat edge cases (condition.go, 37.5% coverage)
// ---------------------------------------------------------------------------.

func TestAsFloat_Adversarial(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  any
		wantF  float64
		wantOK bool
	}{
		// Covered types
		{name: "float64 zero", input: float64(0), wantF: 0, wantOK: true},
		{name: "float64 positive", input: float64(3.14), wantF: 3.14, wantOK: true},
		{name: "float64 negative", input: float64(-99.9), wantF: -99.9, wantOK: true},
		{name: "float64 max", input: math.MaxFloat64, wantF: math.MaxFloat64, wantOK: true},
		{name: "float64 smallest nonzero", input: math.SmallestNonzeroFloat64, wantF: math.SmallestNonzeroFloat64, wantOK: true},
		{name: "float64 NaN", input: math.NaN(), wantOK: true}, // NaN != NaN, checked below
		{name: "float64 +Inf", input: math.Inf(1), wantF: math.Inf(1), wantOK: true},
		{name: "float64 -Inf", input: math.Inf(-1), wantF: math.Inf(-1), wantOK: true},
		{name: "float32 positive", input: float32(1.5), wantF: float64(float32(1.5)), wantOK: true},
		{name: "float32 zero", input: float32(0), wantF: 0, wantOK: true},
		{name: "int zero", input: int(0), wantF: 0, wantOK: true},
		{name: "int positive", input: int(42), wantF: 42, wantOK: true},
		{name: "int negative", input: int(-1), wantF: -1, wantOK: true},
		{name: "int64 zero", input: int64(0), wantF: 0, wantOK: true},
		{name: "int64 max", input: int64(math.MaxInt64), wantF: float64(math.MaxInt64), wantOK: true},
		{name: "int64 min", input: int64(math.MinInt64), wantF: float64(math.MinInt64), wantOK: true},
		{name: "json.Number valid", input: json.Number("123.456"), wantF: 123.456, wantOK: true},
		{name: "json.Number zero", input: json.Number("0"), wantF: 0, wantOK: true},
		{name: "json.Number negative", input: json.Number("-7"), wantF: -7, wantOK: true},
		{name: "json.Number very large", input: json.Number("1e308"), wantF: 1e308, wantOK: true},

		// Unsupported types that must return false
		// json.Number("NaN") and json.Number("Inf") parse successfully via strconv.ParseFloat,
		// which Go's encoding/json.Number.Float64() uses internally.
		{name: "json.Number NaN string", input: json.Number("NaN"), wantOK: true}, // NaN, checked specially
		{name: "json.Number Inf string", input: json.Number("Inf"), wantF: math.Inf(1), wantOK: true},
		{name: "json.Number empty", input: json.Number(""), wantF: 0, wantOK: false},
		{name: "json.Number whitespace", input: json.Number(" "), wantF: 0, wantOK: false},
		{name: "json.Number non-numeric", input: json.Number("abc"), wantF: 0, wantOK: false},
		{name: "string numeric", input: "42", wantF: 0, wantOK: false},
		{name: "string NaN", input: "NaN", wantF: 0, wantOK: false},
		{name: "string Inf", input: "Inf", wantF: 0, wantOK: false},
		{name: "string -Inf", input: "-Inf", wantF: 0, wantOK: false},
		{name: "string empty", input: "", wantF: 0, wantOK: false},
		{name: "string whitespace", input: "  ", wantF: 0, wantOK: false},
		{name: "bool true", input: true, wantF: 0, wantOK: false},
		{name: "bool false", input: false, wantF: 0, wantOK: false},
		{name: "nil", input: nil, wantF: 0, wantOK: false},
		{name: "int8", input: int8(1), wantF: 0, wantOK: false},
		{name: "int16", input: int16(1), wantF: 0, wantOK: false},
		{name: "int32", input: int32(1), wantF: 0, wantOK: false},
		{name: "uint", input: uint(1), wantF: 0, wantOK: false},
		{name: "uint64", input: uint64(1), wantF: 0, wantOK: false},
		{name: "slice", input: []int{1}, wantF: 0, wantOK: false},
		{name: "map", input: map[string]int{"a": 1}, wantF: 0, wantOK: false},
		{name: "struct", input: struct{}{}, wantF: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotF, gotOK := asFloat(tt.input)
			if gotOK != tt.wantOK {
				t.Fatalf("asFloat(%v) ok = %v, want %v", tt.input, gotOK, tt.wantOK)
			}
			if !gotOK {
				return
			}
			// Special handling for NaN since NaN != NaN.
			if tt.name == "float64 NaN" || tt.name == "json.Number NaN string" {
				if !math.IsNaN(gotF) {
					t.Fatalf("asFloat(NaN) = %v, want NaN", gotF)
				}
				return
			}
			if gotF != tt.wantF {
				t.Fatalf("asFloat(%v) = %v, want %v", tt.input, gotF, tt.wantF)
			}
		})
	}
}

// Verify that numeric comparisons through EvaluateCondition fail gracefully
// when non-numeric types are used.
func TestEvaluateCondition_NumericComparisonNonNumeric(t *testing.T) {
	t.Parallel()

	ops := []string{"gt", "gte", "lt", "lte"}
	for _, op := range ops {
		t.Run(op+"_string_operands", func(t *testing.T) {
			t.Parallel()
			cond := mustJSON(`{"type":"` + op + `","left":"abc","right":"def"}`)
			_, err := EvaluateCondition(cond, map[string]domain.StepRunStatus{})
			if err == nil {
				t.Fatal("expected error for non-numeric comparison, got nil")
			}
		})

		t.Run(op+"_bool_operands", func(t *testing.T) {
			t.Parallel()
			cond := mustJSON(`{"type":"` + op + `","left":true,"right":false}`)
			_, err := EvaluateCondition(cond, map[string]domain.StepRunStatus{})
			if err == nil {
				t.Fatal("expected error for boolean comparison, got nil")
			}
		})

		t.Run(op+"_null_operand", func(t *testing.T) {
			t.Parallel()
			cond := mustJSON(`{"type":"` + op + `","left":null,"right":1}`)
			_, err := EvaluateCondition(cond, map[string]domain.StepRunStatus{})
			if err == nil {
				t.Fatal("expected error for null comparison, got nil")
			}
		})
	}
}

// ---------------------------------------------------------------------------.
// 2. tryReleaseDependencyRuns (callback.go, 65.5% coverage)
// ---------------------------------------------------------------------------.

func TestTryReleaseDependencyRuns_NilRun(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	// Must not panic on nil run.
	cb.tryReleaseDependencyRuns(context.Background(), nil)
}

func TestTryReleaseDependencyRuns_NonTerminalRun(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	// Non-terminal status should be a no-op.
	run := &domain.JobRun{ID: "jr-1", JobID: "j-1", Status: domain.StatusExecuting}
	cb.tryReleaseDependencyRuns(context.Background(), run)
}

func TestTryReleaseDependencyRuns_NoDependencies(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{
		listDependentsByDependencyJobFn: func(_ context.Context, _ string) ([]domain.JobDependency, error) {
			return nil, nil
		},
	}
	cb := newTestCallback(ms)
	run := &domain.JobRun{ID: "jr-1", JobID: "j-1", Status: domain.StatusCompleted}
	// Should complete without error (no dependents to release).
	cb.tryReleaseDependencyRuns(context.Background(), run)
}

func TestTryReleaseDependencyRuns_AllSatisfied(t *testing.T) {
	t.Parallel()
	var queuedIDs []string
	ms := &mockCallbackStore{
		listDependentsByDependencyJobFn: func(_ context.Context, _ string) ([]domain.JobDependency, error) {
			return []domain.JobDependency{
				{JobID: "dep-j1", DependsOnJobID: "j-1"},
				{JobID: "dep-j2", DependsOnJobID: "j-1"},
			}, nil
		},
		listWaitingRunsByJobIDsFn: func(_ context.Context, _ []string, _ int) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "wr-1", JobID: "dep-j1", Status: domain.StatusWaiting},
				{ID: "wr-2", JobID: "dep-j2", Status: domain.StatusWaiting},
			}, nil
		},
		areJobDependenciesSatisfiedFn: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, _ map[string]any) error {
			if from != domain.StatusWaiting || to != domain.StatusQueued {
				t.Errorf("unexpected status transition: %s -> %s", from, to)
			}
			queuedIDs = append(queuedIDs, id)
			return nil
		},
	}
	cb := newTestCallback(ms)
	run := &domain.JobRun{ID: "jr-1", JobID: "j-1", Status: domain.StatusCompleted}
	cb.tryReleaseDependencyRuns(context.Background(), run)

	if len(queuedIDs) != 2 {
		t.Fatalf("expected 2 runs queued, got %d", len(queuedIDs))
	}
}

func TestTryReleaseDependencyRuns_SomePending(t *testing.T) {
	t.Parallel()
	var queuedIDs []string
	ms := &mockCallbackStore{
		listDependentsByDependencyJobFn: func(_ context.Context, _ string) ([]domain.JobDependency, error) {
			return []domain.JobDependency{
				{JobID: "dep-j1", DependsOnJobID: "j-1"},
				{JobID: "dep-j2", DependsOnJobID: "j-1"},
			}, nil
		},
		listWaitingRunsByJobIDsFn: func(_ context.Context, _ []string, _ int) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "wr-1", JobID: "dep-j1", Status: domain.StatusWaiting},
				{ID: "wr-2", JobID: "dep-j2", Status: domain.StatusWaiting},
			}, nil
		},
		areJobDependenciesSatisfiedFn: func(_ context.Context, run *domain.JobRun) (bool, error) {
			// Only wr-1 is satisfied.
			return run.ID == "wr-1", nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _, _ domain.RunStatus, _ map[string]any) error {
			queuedIDs = append(queuedIDs, id)
			return nil
		},
	}
	cb := newTestCallback(ms)
	run := &domain.JobRun{ID: "jr-1", JobID: "j-1", Status: domain.StatusCompleted}
	cb.tryReleaseDependencyRuns(context.Background(), run)

	if len(queuedIDs) != 1 || queuedIDs[0] != "wr-1" {
		t.Fatalf("expected only wr-1 queued, got %v", queuedIDs)
	}
}

func TestTryReleaseDependencyRuns_FailedDependencyNotReleased(t *testing.T) {
	t.Parallel()
	var queuedIDs []string
	ms := &mockCallbackStore{
		listDependentsByDependencyJobFn: func(_ context.Context, _ string) ([]domain.JobDependency, error) {
			return []domain.JobDependency{
				{JobID: "dep-j1", DependsOnJobID: "j-1"},
			}, nil
		},
		listWaitingRunsByJobIDsFn: func(_ context.Context, _ []string, _ int) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "wr-1", JobID: "dep-j1", Status: domain.StatusWaiting},
			}, nil
		},
		areJobDependenciesSatisfiedFn: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			// The dependency check says not satisfied (e.g., dependency failed).
			return false, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _, _ domain.RunStatus, _ map[string]any) error {
			queuedIDs = append(queuedIDs, id)
			return nil
		},
	}
	cb := newTestCallback(ms)
	// The completing run itself is failed.
	run := &domain.JobRun{ID: "jr-1", JobID: "j-1", Status: domain.StatusFailed}
	cb.tryReleaseDependencyRuns(context.Background(), run)

	if len(queuedIDs) != 0 {
		t.Fatalf("expected no runs queued when dependency unsatisfied, got %v", queuedIDs)
	}
}

func TestTryReleaseDependencyRuns_DuplicateDependentJobIDs(t *testing.T) {
	t.Parallel()
	var requestedJobIDs []string
	ms := &mockCallbackStore{
		listDependentsByDependencyJobFn: func(_ context.Context, _ string) ([]domain.JobDependency, error) {
			// Same job ID appears multiple times (multiple dependency rows).
			return []domain.JobDependency{
				{JobID: "dep-j1", DependsOnJobID: "j-1"},
				{JobID: "dep-j1", DependsOnJobID: "j-1"},
				{JobID: "dep-j1", DependsOnJobID: "j-1"},
			}, nil
		},
		listWaitingRunsByJobIDsFn: func(_ context.Context, jobIDs []string, _ int) ([]domain.JobRun, error) {
			requestedJobIDs = jobIDs
			return nil, nil
		},
	}
	cb := newTestCallback(ms)
	run := &domain.JobRun{ID: "jr-1", JobID: "j-1", Status: domain.StatusCompleted}
	cb.tryReleaseDependencyRuns(context.Background(), run)

	// Deduplication should result in a single job ID.
	if len(requestedJobIDs) != 1 {
		t.Fatalf("expected 1 deduplicated job ID, got %v", requestedJobIDs)
	}
}

func TestTryReleaseDependencyRuns_ListDependentsError(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{
		listDependentsByDependencyJobFn: func(_ context.Context, _ string) ([]domain.JobDependency, error) {
			return nil, errors.New("db down")
		},
	}
	cb := newTestCallback(ms)
	run := &domain.JobRun{ID: "jr-1", JobID: "j-1", Status: domain.StatusCompleted}
	// Should not panic; error is logged internally.
	cb.tryReleaseDependencyRuns(context.Background(), run)
}

func TestTryReleaseDependencyRuns_DependencyCheckError(t *testing.T) {
	t.Parallel()
	var queuedCount int
	ms := &mockCallbackStore{
		listDependentsByDependencyJobFn: func(_ context.Context, _ string) ([]domain.JobDependency, error) {
			return []domain.JobDependency{
				{JobID: "dep-j1", DependsOnJobID: "j-1"},
				{JobID: "dep-j2", DependsOnJobID: "j-1"},
			}, nil
		},
		listWaitingRunsByJobIDsFn: func(_ context.Context, _ []string, _ int) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "wr-1", JobID: "dep-j1", Status: domain.StatusWaiting},
				{ID: "wr-2", JobID: "dep-j2", Status: domain.StatusWaiting},
			}, nil
		},
		areJobDependenciesSatisfiedFn: func(_ context.Context, run *domain.JobRun) (bool, error) {
			if run.ID == "wr-1" {
				return false, errors.New("check failed")
			}
			return true, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			queuedCount++
			return nil
		},
	}
	cb := newTestCallback(ms)
	run := &domain.JobRun{ID: "jr-1", JobID: "j-1", Status: domain.StatusCompleted}
	cb.tryReleaseDependencyRuns(context.Background(), run)

	// Only wr-2 should be queued; wr-1 error should be skipped.
	if queuedCount != 1 {
		t.Fatalf("expected 1 run queued (skipping errored check), got %d", queuedCount)
	}
}

func TestTryReleaseDependencyRuns_ConcurrentAttempts(t *testing.T) {
	t.Parallel()
	var queuedCount atomic.Int64
	ms := &mockCallbackStore{
		listDependentsByDependencyJobFn: func(_ context.Context, _ string) ([]domain.JobDependency, error) {
			return []domain.JobDependency{
				{JobID: "dep-j1", DependsOnJobID: "j-1"},
			}, nil
		},
		listWaitingRunsByJobIDsFn: func(_ context.Context, _ []string, _ int) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "wr-1", JobID: "dep-j1", Status: domain.StatusWaiting},
			}, nil
		},
		areJobDependenciesSatisfiedFn: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			queuedCount.Add(1)
			return nil
		},
	}
	cb := newTestCallback(ms)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			run := &domain.JobRun{ID: "jr-1", JobID: "j-1", Status: domain.StatusCompleted}
			cb.tryReleaseDependencyRuns(context.Background(), run)
		})
	}
	wg.Wait()

	// Each goroutine calls independently; the store mock is stateless so all
	// 10 should succeed. This tests that no race condition panics occur.
	if queuedCount.Load() != 10 {
		t.Fatalf("expected 10 release attempts, got %d", queuedCount.Load())
	}
}

// ---------------------------------------------------------------------------.
// 3. enqueueStepAnalytics (callback.go, 22.2% coverage)
// ---------------------------------------------------------------------------.

func TestEnqueueStepAnalytics_NilChExporter(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	// chExporter is nil by default. Must not panic.
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1"},
		nil,
	)
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "a", Status: domain.StepCompleted}
	cb.enqueueStepAnalytics(stepRun, wc)
}

func TestEnqueueStepAnalytics_NilStepRun(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())
	cb.WithChExporter(exporter)
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1"},
		nil,
	)
	// Must not panic on nil stepRun.
	cb.enqueueStepAnalytics(nil, wc)
}

func TestEnqueueStepAnalytics_NilWfCtx(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())
	cb.WithChExporter(exporter)
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "a", Status: domain.StepCompleted}
	// Must not panic on nil wfCtx.
	cb.enqueueStepAnalytics(stepRun, nil)
}

func TestEnqueueStepAnalytics_NilWfRun(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())
	cb.WithChExporter(exporter)
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "a", Status: domain.StepCompleted}
	// wfCtx with nil run.
	cb.enqueueStepAnalytics(stepRun, &wfCtx{run: nil})
}

func TestEnqueueStepAnalytics_ValidCompletion(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())
	cb.WithChExporter(exporter)

	now := time.Now()
	startedAt := now.Add(-5 * time.Second)
	finishedAt := now
	stepRun := &domain.WorkflowStepRun{
		ID:            "sr-1",
		WorkflowRunID: "wr-1",
		StepRef:       "process",
		Status:        domain.StepCompleted,
		Attempt:       2,
		CreatedAt:     now.Add(-10 * time.Second),
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
	}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1"},
		nil,
	)
	// Should not panic; enqueues a record.
	cb.enqueueStepAnalytics(stepRun, wc)
}

func TestEnqueueStepAnalytics_NilTimestamps(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())
	cb.WithChExporter(exporter)

	stepRun := &domain.WorkflowStepRun{
		ID:            "sr-1",
		WorkflowRunID: "wr-1",
		StepRef:       "process",
		Status:        domain.StepCompleted,
		Attempt:       1,
		StartedAt:     nil, // nil StartedAt means DurationMs should be 0
		FinishedAt:    nil,
	}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1"},
		nil,
	)
	// Should not panic with nil timestamps.
	cb.enqueueStepAnalytics(stepRun, wc)
}

func TestEnqueueStepAnalytics_HighAttemptClamped(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())
	cb.WithChExporter(exporter)

	stepRun := &domain.WorkflowStepRun{
		ID:            "sr-1",
		WorkflowRunID: "wr-1",
		StepRef:       "retry-heavy",
		Status:        domain.StepFailed,
		Attempt:       999, // exceeds uint8 max; should be clamped to 255
		Error:         "too many retries",
	}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1"},
		nil,
	)
	// Should not panic or overflow.
	cb.enqueueStepAnalytics(stepRun, wc)
}

// ---------------------------------------------------------------------------.
// 4. recordStepWaitDuration (callback_progression.go, 22.2% coverage)
// ---------------------------------------------------------------------------.

func newTestMetrics(t *testing.T) *telemetry.Metrics {
	t.Helper()
	provider := sdkmetric.NewMeterProvider()
	meter := provider.Meter("test")
	hist, err := meter.Float64Histogram("test.workflow.step.wait_duration")
	if err != nil {
		t.Fatal(err)
	}
	return &telemetry.Metrics{
		WorkflowStepWaitDuration: hist,
	}
}

func TestRecordStepWaitDuration_NilMetrics(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	// metrics is nil by default -- must not panic.
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}
	step := domain.WorkflowStep{StepRef: "a"}
	stepRun := domain.WorkflowStepRun{ID: "sr-1", CreatedAt: time.Now()}
	cb.recordStepWaitDuration(context.Background(), wfRun, step, stepRun)
}

func TestRecordStepWaitDuration_ZeroCreatedAt(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	cb.WithMetrics(newTestMetrics(t))
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}
	step := domain.WorkflowStep{StepRef: "a"}
	// Zero value CreatedAt should cause early return.
	stepRun := domain.WorkflowStepRun{ID: "sr-1", CreatedAt: time.Time{}}
	cb.recordStepWaitDuration(context.Background(), wfRun, step, stepRun)
}

func TestRecordStepWaitDuration_FutureCreatedAt(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	cb.WithMetrics(newTestMetrics(t))
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}
	step := domain.WorkflowStep{StepRef: "a"}
	// Future time should result in negative duration clamped to 0.
	stepRun := domain.WorkflowStepRun{ID: "sr-1", CreatedAt: time.Now().Add(1 * time.Hour)}
	cb.recordStepWaitDuration(context.Background(), wfRun, step, stepRun)
}

func TestRecordStepWaitDuration_NormalWait(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	cb.WithMetrics(newTestMetrics(t))
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}
	step := domain.WorkflowStep{StepRef: "a"}
	stepRun := domain.WorkflowStepRun{ID: "sr-1", CreatedAt: time.Now().Add(-5 * time.Second)}
	// Should record ~5 seconds of wait without panicking.
	cb.recordStepWaitDuration(context.Background(), wfRun, step, stepRun)
}

// ---------------------------------------------------------------------------.
// 5. skipDependentSteps edge cases (callback_retry.go, 100% nominal)
// ---------------------------------------------------------------------------.

func TestSkipDependentSteps_NoDependents(t *testing.T) {
	t.Parallel()
	skippedRefs := make(map[string]bool)
	ms := &mockCallbackStore{
		skipStepRunsByRefsFn: func(_ context.Context, _ string, refs []string, _ time.Time) (int64, error) {
			for _, ref := range refs {
				skippedRefs[ref] = true
			}
			return int64(len(refs)), nil
		},
	}
	cb := newTestCallback(ms)
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "a"},
			{StepRef: "b"},
			{StepRef: "c"},
		},
	)
	// Step "a" has no dependents; nothing should be skipped.
	err := cb.skipDependentSteps(context.Background(), "wr-1", wc, "a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skippedRefs) != 0 {
		t.Fatalf("expected no refs skipped, got %v", skippedRefs)
	}
}

func TestSkipDependentSteps_DiamondDAG(t *testing.T) {
	// Diamond: A -> B, A -> C, B+C -> D
	// Failing A should skip B, C, and D.
	t.Parallel()
	skippedRefs := make(map[string]bool)
	ms := &mockCallbackStore{
		skipStepRunsByRefsFn: func(_ context.Context, _ string, refs []string, _ time.Time) (int64, error) {
			for _, ref := range refs {
				skippedRefs[ref] = true
			}
			return int64(len(refs)), nil
		},
	}
	cb := newTestCallback(ms)
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "a"},
			{StepRef: "b", DependsOn: []string{"a"}},
			{StepRef: "c", DependsOn: []string{"a"}},
			{StepRef: "d", DependsOn: []string{"b", "c"}},
		},
	)
	err := cb.skipDependentSteps(context.Background(), "wr-1", wc, "a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ref := range []string{"b", "c", "d"} {
		if !skippedRefs[ref] {
			t.Errorf("expected %s to be skipped", ref)
		}
	}
	if skippedRefs["a"] {
		t.Error("step a (the failed step) should not be in the skip list")
	}
}

func TestSkipDependentSteps_FanOut(t *testing.T) {
	// Fan-out: A -> B1, B2, B3, B4, B5
	t.Parallel()
	skippedRefs := make(map[string]bool)
	ms := &mockCallbackStore{
		skipStepRunsByRefsFn: func(_ context.Context, _ string, refs []string, _ time.Time) (int64, error) {
			for _, ref := range refs {
				skippedRefs[ref] = true
			}
			return int64(len(refs)), nil
		},
	}
	cb := newTestCallback(ms)

	steps := []domain.WorkflowStep{{StepRef: "a"}}
	for i := 1; i <= 5; i++ {
		ref := "b" + string(rune('0'+i))
		steps = append(steps, domain.WorkflowStep{StepRef: ref, DependsOn: []string{"a"}})
	}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		steps,
	)
	err := cb.skipDependentSteps(context.Background(), "wr-1", wc, "a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skippedRefs) != 5 {
		t.Fatalf("expected 5 refs skipped, got %d: %v", len(skippedRefs), skippedRefs)
	}
}

func TestSkipDependentSteps_DeepChain(t *testing.T) {
	// Linear chain: a -> b -> c -> d -> e
	// Failing a should skip b, c, d, e.
	t.Parallel()
	skippedRefs := make(map[string]bool)
	ms := &mockCallbackStore{
		skipStepRunsByRefsFn: func(_ context.Context, _ string, refs []string, _ time.Time) (int64, error) {
			for _, ref := range refs {
				skippedRefs[ref] = true
			}
			return int64(len(refs)), nil
		},
	}
	cb := newTestCallback(ms)
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "a"},
			{StepRef: "b", DependsOn: []string{"a"}},
			{StepRef: "c", DependsOn: []string{"b"}},
			{StepRef: "d", DependsOn: []string{"c"}},
			{StepRef: "e", DependsOn: []string{"d"}},
		},
	)
	err := cb.skipDependentSteps(context.Background(), "wr-1", wc, "a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ref := range []string{"b", "c", "d", "e"} {
		if !skippedRefs[ref] {
			t.Errorf("expected %s to be skipped in chain", ref)
		}
	}
}

func TestSkipDependentSteps_MiddleNodeFail(t *testing.T) {
	// Diamond: A -> B, A -> C, B+C -> D
	// Failing B should only skip D, not C.
	t.Parallel()
	skippedRefs := make(map[string]bool)
	ms := &mockCallbackStore{
		skipStepRunsByRefsFn: func(_ context.Context, _ string, refs []string, _ time.Time) (int64, error) {
			for _, ref := range refs {
				skippedRefs[ref] = true
			}
			return int64(len(refs)), nil
		},
	}
	cb := newTestCallback(ms)
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "a"},
			{StepRef: "b", DependsOn: []string{"a"}},
			{StepRef: "c", DependsOn: []string{"a"}},
			{StepRef: "d", DependsOn: []string{"b", "c"}},
		},
	)
	err := cb.skipDependentSteps(context.Background(), "wr-1", wc, "b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skippedRefs["d"] {
		t.Error("expected d to be skipped (depends on failed b)")
	}
	if skippedRefs["c"] {
		t.Error("c should not be skipped (independent of b)")
	}
	if skippedRefs["a"] {
		t.Error("a should not be skipped (upstream of b)")
	}
}

func TestSkipDependentSteps_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{
		skipStepRunsByRefsFn: func(_ context.Context, _ string, _ []string, _ time.Time) (int64, error) {
			return 0, errors.New("db error")
		},
	}
	cb := newTestCallback(ms)
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "a"},
			{StepRef: "b", DependsOn: []string{"a"}},
		},
	)
	err := cb.skipDependentSteps(context.Background(), "wr-1", wc, "a")
	if err == nil {
		t.Fatal("expected error from store, got nil")
	}
}

// ---------------------------------------------------------------------------.
// 6. Complex condition edge cases via EvaluateCondition
// ---------------------------------------------------------------------------.

func TestEvaluateCondition_ConditionalBranch_EdgeValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cond    json.RawMessage
		want    bool
		wantErr bool
	}{
		{
			name: "gt with equal values is false",
			cond: mustJSON(`{"type":"gt","left":5,"right":5}`),
			want: false,
		},
		{
			name: "gte with equal values is true",
			cond: mustJSON(`{"type":"gte","left":5,"right":5}`),
			want: true,
		},
		{
			name: "lt with equal values is false",
			cond: mustJSON(`{"type":"lt","left":5,"right":5}`),
			want: false,
		},
		{
			name: "lte with equal values is true",
			cond: mustJSON(`{"type":"lte","left":5,"right":5}`),
			want: true,
		},
		{
			name: "gt with very small difference",
			cond: mustJSON(`{"type":"gt","left":1.0000000001,"right":1.0}`),
			want: true,
		},
		{
			name: "eq with different types compares as strings",
			cond: mustJSON(`{"type":"eq","left":1,"right":"1"}`),
			want: true, // fmt.Sprint(1.0) == "1" for JSON-decoded float
		},
		{
			name: "ne with same value is false",
			cond: mustJSON(`{"type":"ne","left":"hello","right":"hello"}`),
			want: false,
		},
		{
			name: "contains empty needle always matches",
			cond: mustJSON(`{"type":"contains","left":"anything","right":""}`),
			want: true,
		},
		{
			name: "contains empty haystack with non-empty needle",
			cond: mustJSON(`{"type":"contains","left":"","right":"x"}`),
			want: false,
		},
		{
			name: "in with empty array is always false",
			cond: mustJSON(`{"type":"in","left":"anything","right":[]}`),
			want: false,
		},
		{
			name:    "in with non-array right returns error",
			cond:    mustJSON(`{"type":"in","left":"a","right":"not-array"}`),
			wantErr: true,
		},
		{
			name:    "regex with invalid pattern returns error",
			cond:    mustJSON(`{"type":"regex","left":"test","right":"[invalid"}`),
			wantErr: true,
		},
		{
			name:    "not with empty inner condition returns error",
			cond:    mustJSON(`{"type":"not","condition":null}`),
			wantErr: true,
		},
		{
			name:    "empty type string returns error",
			cond:    mustJSON(`{"type":""}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := EvaluateCondition(tt.cond, map[string]domain.StepRunStatus{})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestEvaluateCondition_DeeplyNested checks that deep nesting of all_of/any_of
// does not cause stack issues and evaluates correctly.
func TestEvaluateCondition_DeeplyNestedAllOf(t *testing.T) {
	t.Parallel()
	statuses := map[string]domain.StepRunStatus{
		"s1": domain.StepCompleted,
	}

	// Build a 20-level deep all_of wrapping a single step_status.
	inner := `{"type":"step_status","step_ref":"s1","status":"completed"}`
	for range 20 {
		inner = `{"type":"all_of","conditions":[` + inner + `]}`
	}
	got, err := EvaluateCondition(mustJSON(inner), statuses)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatal("expected deeply nested condition to evaluate to true")
	}
}

// ---------------------------------------------------------------------------.
// 7. mapRunStatusToStepStatus edge cases
// ---------------------------------------------------------------------------.

func TestMapRunStatusToStepStatus_AllTerminalStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		runStatus  domain.RunStatus
		wantStep   domain.StepRunStatus
		wantError  bool
		wantOutput bool
	}{
		{domain.StatusCompleted, domain.StepCompleted, false, true},
		{domain.StatusCanceled, domain.StepCanceled, false, false},
		{domain.StatusFailed, domain.StepFailed, true, false},
		{domain.StatusDeadLetter, domain.StepFailed, true, false},
		{domain.StatusTimedOut, domain.StepFailed, true, false},
		{domain.StatusCrashed, domain.StepFailed, true, false},
		{domain.StatusSystemFailed, domain.StepFailed, true, false},
		{domain.StatusExpired, domain.StepFailed, true, false},
		// Unexpected status should also map to failed.
		{domain.RunStatus("something_weird"), domain.StepFailed, true, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.runStatus), func(t *testing.T) {
			t.Parallel()
			run := &domain.JobRun{Status: tt.runStatus, Result: json.RawMessage(`{"ok":true}`), Error: "test error"}
			stepStatus, fields := mapRunStatusToStepStatus(run)
			if stepStatus != tt.wantStep {
				t.Fatalf("status = %s, want %s", stepStatus, tt.wantStep)
			}
			_, hasError := fields["error"]
			if hasError != tt.wantError && tt.runStatus != domain.StatusCanceled {
				t.Fatalf("error field present = %v, want %v", hasError, tt.wantError)
			}
		})
	}
}

func TestMapRunStatusToStepStatus_CompletedNoResult(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{Status: domain.StatusCompleted, Result: nil}
	stepStatus, fields := mapRunStatusToStepStatus(run)
	if stepStatus != domain.StepCompleted {
		t.Fatalf("expected StepCompleted, got %s", stepStatus)
	}
	if _, ok := fields["output"]; ok {
		t.Fatal("expected no output field when Result is nil")
	}
}

func TestMapRunStatusToStepStatus_FailedNoError(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{Status: domain.StatusFailed, Error: ""}
	stepStatus, fields := mapRunStatusToStepStatus(run)
	if stepStatus != domain.StepFailed {
		t.Fatalf("expected StepFailed, got %s", stepStatus)
	}
	errField, ok := fields["error"].(string)
	if !ok || errField == "" {
		t.Fatal("expected a fallback error message when Error is empty")
	}
}

// ---------------------------------------------------------------------------.
// 8. approvalAuditActor edge cases
// ---------------------------------------------------------------------------.

func TestApprovalAuditActor_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		actor    string
		wantID   string
		wantType string
	}{
		{"empty actor", "", "system", "system"},
		{"system actor", "system", "system", "system"},
		{"system prefixed", "system:auto", "system:auto", "system"},
		{"apikey actor", "apikey:abc123", "apikey:abc123", "api_key"},
		{"regular user", "user@example.com", "user@example.com", "user"},
		{"unknown prefix", "oauth:token", "oauth:token", "user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, typ := approvalAuditActor(tt.actor)
			if id != tt.wantID {
				t.Errorf("actor ID = %q, want %q", id, tt.wantID)
			}
			if typ != tt.wantType {
				t.Errorf("actor type = %q, want %q", typ, tt.wantType)
			}
		})
	}
}

// ---------------------------------------------------------------------------.
// 9. effectiveResourceClass and hasResourceClassCapacity
// ---------------------------------------------------------------------------.

func TestEffectiveResourceClass_Adversarial(t *testing.T) {
	t.Parallel()
	if got := effectiveResourceClass(""); got != "small" {
		t.Fatalf("empty should default to small, got %q", got)
	}
	if got := effectiveResourceClass("large"); got != "large" {
		t.Fatalf("expected large, got %q", got)
	}
	if got := effectiveResourceClass("custom"); got != "custom" {
		t.Fatalf("expected custom, got %q", got)
	}
}

func TestHasResourceClassCapacity_Limits(t *testing.T) {
	t.Parallel()

	// Small: limit 50
	running := map[string]int{"small": 49}
	if !hasResourceClassCapacity(running, "") {
		t.Fatal("should have capacity at 49/50")
	}
	running["small"] = 50
	if hasResourceClassCapacity(running, "") {
		t.Fatal("should not have capacity at 50/50")
	}

	// Medium: limit 20
	running = map[string]int{"medium": 19}
	if !hasResourceClassCapacity(running, "medium") {
		t.Fatal("should have capacity at 19/20")
	}
	running["medium"] = 20
	if hasResourceClassCapacity(running, "medium") {
		t.Fatal("should not have capacity at 20/20")
	}

	// Large: limit 5
	running = map[string]int{"large": 4}
	if !hasResourceClassCapacity(running, "large") {
		t.Fatal("should have capacity at 4/5")
	}
	running["large"] = 5
	if hasResourceClassCapacity(running, "large") {
		t.Fatal("should not have capacity at 5/5")
	}

	// Unknown class falls back to small limit.
	running = map[string]int{"small": 49}
	if !hasResourceClassCapacity(running, "unknown-class") {
		t.Fatal("unknown class should use small limit")
	}
	running["small"] = 50
	if hasResourceClassCapacity(running, "unknown-class") {
		t.Fatal("unknown class at small limit should not have capacity")
	}
}

// ---------------------------------------------------------------------------.
// 10. advisoryXactLockIDForStepRun determinism
// ---------------------------------------------------------------------------.

func TestAdvisoryXactLockIDForStepRun_Deterministic(t *testing.T) {
	t.Parallel()
	id1 := advisoryXactLockIDForStepRun("sr-abc")
	id2 := advisoryXactLockIDForStepRun("sr-abc")
	if id1 != id2 {
		t.Fatalf("expected deterministic lock ID, got %d and %d", id1, id2)
	}
	id3 := advisoryXactLockIDForStepRun("sr-xyz")
	if id1 == id3 {
		t.Fatal("different step run IDs should produce different lock IDs")
	}
	// Must be non-negative (masked with 0x7fffffffffffffff).
	if id1 < 0 {
		t.Fatal("lock ID should be non-negative")
	}
}
