package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/telemetry"

	"github.com/sourcegraph/conc"
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

	var wg conc.WaitGroup
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

// ---------------------------------------------------------------------------.
// checkStepRetry boundary tests (callback_retry.go)
// ---------------------------------------------------------------------------.

func TestCheckStepRetry_MaxAttemptsZero(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "a", Attempt: 1}
	jobRun := &domain.JobRun{ID: "jr-1", Status: domain.StatusFailed}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"},
		[]domain.WorkflowStep{{StepRef: "a", RetryMaxAttempts: 0}},
	)
	shouldRetry, _, _, err := cb.checkStepRetry(context.Background(), stepRun, jobRun, wc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldRetry {
		t.Fatal("RetryMaxAttempts=0 should not retry")
	}
}

func TestCheckStepRetry_MaxAttemptsNegative(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "a", Attempt: 1}
	jobRun := &domain.JobRun{ID: "jr-1", Status: domain.StatusFailed}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"},
		[]domain.WorkflowStep{{StepRef: "a", RetryMaxAttempts: -1}},
	)
	shouldRetry, _, _, err := cb.checkStepRetry(context.Background(), stepRun, jobRun, wc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldRetry {
		t.Fatal("RetryMaxAttempts=-1 should not retry")
	}
}

func TestCheckStepRetry_AttemptEqualsMax(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "a", Attempt: 3}
	jobRun := &domain.JobRun{ID: "jr-1", Status: domain.StatusFailed}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"},
		[]domain.WorkflowStep{{StepRef: "a", RetryMaxAttempts: 3}},
	)
	shouldRetry, _, _, err := cb.checkStepRetry(context.Background(), stepRun, jobRun, wc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldRetry {
		t.Fatal("Attempt=3, MaxAttempts=3 should not retry (exhausted)")
	}
}

func TestCheckStepRetry_AttemptBelowMax(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "a", Attempt: 2}
	jobRun := &domain.JobRun{ID: "jr-1", Status: domain.StatusFailed}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"},
		[]domain.WorkflowStep{{StepRef: "a", RetryMaxAttempts: 3}},
	)
	shouldRetry, _, newAttempt, err := cb.checkStepRetry(context.Background(), stepRun, jobRun, wc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !shouldRetry {
		t.Fatal("Attempt=2, MaxAttempts=3 should retry")
	}
	if newAttempt != 3 {
		t.Errorf("newAttempt = %d, want 3", newAttempt)
	}
}

func TestCheckStepRetry_StepNotFound(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "missing", Attempt: 1}
	jobRun := &domain.JobRun{ID: "jr-1", Status: domain.StatusFailed}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"},
		[]domain.WorkflowStep{{StepRef: "a", RetryMaxAttempts: 3}},
	)
	_, _, _, err := cb.checkStepRetry(context.Background(), stepRun, jobRun, wc)
	if err == nil {
		t.Fatal("expected error for missing step definition")
	}
}

// ---------------------------------------------------------------------------.
// OnJobRunTerminal OutputTransform tests
// ---------------------------------------------------------------------------.

func TestOnJobRunTerminal_OutputTransformApplied(t *testing.T) {
	t.Parallel()
	var storedFields map[string]any
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "transform-step", Status: domain.StepRunning,
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "transform-step", OutputTransform: "result"},
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, fields map[string]any) error {
			storedFields = fields
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		listDependentsByDependencyJobFn: func(_ context.Context, _ string) ([]domain.JobDependency, error) {
			return nil, nil
		},
		listStepRunStatusesByWorkflowRunFn: func(_ context.Context, _ string) (map[string]domain.StepRunStatus, error) {
			return nil, nil
		},
		listRunningStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listRunnableStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
	}

	cb := newTestCallback(ms)
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:                "jr-1",
		WorkflowStepRunID: "sr-1",
		Status:            domain.StatusCompleted,
		Result:            json.RawMessage(`{"result":"value","extra":"data"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if storedFields == nil {
		t.Fatal("expected fields to be stored")
	}
	output, ok := storedFields["output"].(json.RawMessage)
	if !ok {
		t.Fatal("expected output field in stored fields")
	}
	if string(output) == `{"result":"value","extra":"data"}` {
		t.Error("output should be transformed, not raw")
	}
	if len(output) == 0 {
		t.Error("output should not be empty after transform")
	}
}

func TestOnJobRunTerminal_NoOutputTransform(t *testing.T) {
	t.Parallel()
	var storedFields map[string]any
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "no-transform", Status: domain.StepRunning,
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "no-transform", OutputTransform: ""},
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, fields map[string]any) error {
			storedFields = fields
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		listDependentsByDependencyJobFn: func(_ context.Context, _ string) ([]domain.JobDependency, error) {
			return nil, nil
		},
		listStepRunStatusesByWorkflowRunFn: func(_ context.Context, _ string) (map[string]domain.StepRunStatus, error) {
			return nil, nil
		},
		listRunningStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listRunnableStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
	}

	cb := newTestCallback(ms)
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:                "jr-1",
		WorkflowStepRunID: "sr-1",
		Status:            domain.StatusCompleted,
		Result:            json.RawMessage(`{"result":"value"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output, ok := storedFields["output"].(json.RawMessage)
	if !ok {
		t.Fatal("expected output field")
	}
	if string(output) != `{"result":"value"}` {
		t.Errorf("output should be preserved as-is: got %s", string(output))
	}
}

func TestOnJobRunTerminal_EmptyResult_NoTransform(t *testing.T) {
	t.Parallel()
	var storedFields map[string]any
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "step-a", Status: domain.StepRunning,
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "step-a", OutputTransform: "result"},
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, fields map[string]any) error {
			storedFields = fields
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		listDependentsByDependencyJobFn: func(_ context.Context, _ string) ([]domain.JobDependency, error) {
			return nil, nil
		},
		listStepRunStatusesByWorkflowRunFn: func(_ context.Context, _ string) (map[string]domain.StepRunStatus, error) {
			return nil, nil
		},
		listRunningStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listRunnableStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
	}

	cb := newTestCallback(ms)
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:                "jr-1",
		WorkflowStepRunID: "sr-1",
		Status:            domain.StatusCompleted,
		Result:            nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, hasOutput := storedFields["output"]; hasOutput {
		t.Error("empty result should not produce output field")
	}
}

// ---------------------------------------------------------------------------.
// emitEventIfConfigured source type branches
// ---------------------------------------------------------------------------.

func TestEmitEventIfConfigured_JobRunSourceType(t *testing.T) {
	t.Parallel()
	var requeued bool
	ms := &mockCallbackStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:         "evt-1",
				EventKey:   "my-event",
				SourceType: domain.EventSourceJobRun,
				JobRunID:   "jr-waiting",
				Status:     domain.EventTriggerStatusWaiting,
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, _ map[string]any) error {
			if id == "jr-waiting" && from == domain.StatusWaiting && to == domain.StatusQueued {
				requeued = true
			}
			return nil
		},
	}
	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{
		ID: "sr-1", StepRef: "emitter", Output: json.RawMessage(`{"ok":true}`),
	}
	step := &domain.WorkflowStep{StepRef: "emitter", EventEmitKey: "my-event"}
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}
	cb.emitEventIfConfigured(context.Background(), stepRun, step, wfRun)
	if !requeued {
		t.Fatal("expected job run to be re-queued via UpdateRunStatus")
	}
}

func TestEmitEventIfConfigured_JobRunSourceType_EmptyJobRunID(t *testing.T) {
	t.Parallel()
	var updateRunCalled bool
	ms := &mockCallbackStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:         "evt-1",
				EventKey:   "my-event",
				SourceType: domain.EventSourceJobRun,
				JobRunID:   "",
				Status:     domain.EventTriggerStatusWaiting,
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			updateRunCalled = true
			return nil
		},
	}
	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "emitter"}
	step := &domain.WorkflowStep{StepRef: "emitter", EventEmitKey: "my-event"}
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}
	cb.emitEventIfConfigured(context.Background(), stepRun, step, wfRun)
	if updateRunCalled {
		t.Fatal("should not call UpdateRunStatus when JobRunID is empty")
	}
}

// ---------------------------------------------------------------------------.
// scheduleRunnableSteps concurrency key tests
// ---------------------------------------------------------------------------.

func TestScheduleRunnableSteps_ConcurrencyKeyBlocking(t *testing.T) {
	t.Parallel()
	var enqueuedSteps []string
	ms := &mockCallbackStore{}
	eng := NewWorkflowEngine(&mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}, &mockEngineQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedSteps = append(enqueuedSteps, run.JobID)
			return nil
		},
	}, slog.Default())
	cb := NewStepCallback(ms, eng, slog.Default())

	steps := []domain.WorkflowStep{
		{StepRef: "a", ConcurrencyKey: "ck-1", JobID: "j-a"},
		{StepRef: "b", ConcurrencyKey: "ck-1", JobID: "j-b"},
	}
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}
	runningSteps := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepRunning},
	}
	runnableSteps := []domain.WorkflowStepRun{
		{ID: "sr-b", StepRef: "b", Status: domain.StepPending, DepsCompleted: 1, DepsRequired: 1},
	}
	statuses := map[string]domain.StepRunStatus{"a": domain.StepRunning}

	err := cb.scheduleRunnableSteps(context.Background(), wfRun, steps, statuses, runningSteps, runnableSteps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, jobID := range enqueuedSteps {
		if jobID == "j-b" {
			t.Fatal("step b should be blocked by concurrency key ck-1")
		}
	}
}

func TestScheduleRunnableSteps_DifferentConcurrencyKeys(t *testing.T) {
	t.Parallel()
	var enqueuedSteps []string
	ms := &mockCallbackStore{}
	eng := NewWorkflowEngine(&mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}, &mockEngineQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedSteps = append(enqueuedSteps, run.JobID)
			return nil
		},
	}, slog.Default())
	cb := NewStepCallback(ms, eng, slog.Default())

	steps := []domain.WorkflowStep{
		{StepRef: "a", ConcurrencyKey: "ck-1", JobID: "j-a"},
		{StepRef: "b", ConcurrencyKey: "ck-2", JobID: "j-b"},
	}
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}
	runningSteps := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepRunning},
	}
	runnableSteps := []domain.WorkflowStepRun{
		{ID: "sr-b", StepRef: "b", Status: domain.StepPending, DepsCompleted: 1, DepsRequired: 1},
	}
	statuses := map[string]domain.StepRunStatus{"a": domain.StepRunning}

	err := cb.scheduleRunnableSteps(context.Background(), wfRun, steps, statuses, runningSteps, runnableSteps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, jobID := range enqueuedSteps {
		if jobID == "j-b" {
			found = true
		}
	}
	if !found {
		t.Fatal("step b should NOT be blocked (different concurrency key)")
	}
}

// ---------------------------------------------------------------------------.
// scheduleRunnableSteps resource class capacity
// ---------------------------------------------------------------------------.

func TestScheduleRunnableSteps_ResourceClassCapacityExhausted(t *testing.T) {
	t.Parallel()
	var enqueuedSteps []string
	ms := &mockCallbackStore{}
	eng := NewWorkflowEngine(&mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}, &mockEngineQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedSteps = append(enqueuedSteps, run.JobID)
			return nil
		},
	}, slog.Default())
	cb := NewStepCallback(ms, eng, slog.Default())

	steps := []domain.WorkflowStep{
		{StepRef: "existing", ResourceClass: "large", JobID: "j-existing"},
		{StepRef: "new-step", ResourceClass: "large", JobID: "j-new"},
	}
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}

	runningSteps := make([]domain.WorkflowStepRun, 5)
	for i := range runningSteps {
		runningSteps[i] = domain.WorkflowStepRun{
			ID: "sr-existing-" + string(rune('0'+i)), StepRef: "existing", Status: domain.StepRunning,
		}
	}
	runnableSteps := []domain.WorkflowStepRun{
		{ID: "sr-new", StepRef: "new-step", Status: domain.StepPending, DepsCompleted: 1, DepsRequired: 1},
	}
	statuses := map[string]domain.StepRunStatus{}

	err := cb.scheduleRunnableSteps(context.Background(), wfRun, steps, statuses, runningSteps, runnableSteps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, jobID := range enqueuedSteps {
		if jobID == "j-new" {
			t.Fatal("new-step should be blocked by large resource class capacity (5/5)")
		}
	}
}

// ---------------------------------------------------------------------------.
// scheduleRunnableSteps DependsOn parent outputs
// ---------------------------------------------------------------------------.

func TestScheduleRunnableSteps_NoDependsOn_NoParentOutputs(t *testing.T) {
	t.Parallel()
	var getStepOutputsCalled bool
	ms := &mockCallbackStore{
		getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
			getStepOutputsCalled = true
			return nil, nil
		},
	}
	eng := NewWorkflowEngine(&mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}, &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil },
	}, slog.Default())
	cb := NewStepCallback(ms, eng, slog.Default())

	steps := []domain.WorkflowStep{{StepRef: "a", JobID: "j-a"}}
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}
	runnableSteps := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", WorkflowRunID: "wr-1", Status: domain.StepPending, DepsCompleted: 0, DepsRequired: 0},
	}

	err := cb.scheduleRunnableSteps(context.Background(), wfRun, steps, map[string]domain.StepRunStatus{}, nil, runnableSteps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if getStepOutputsCalled {
		t.Error("GetStepOutputs should not be called for step with no DependsOn")
	}
}

func TestScheduleRunnableSteps_WithDependsOn_GetsParentOutputs(t *testing.T) {
	t.Parallel()
	var getStepOutputsCalled bool
	ms := &mockCallbackStore{
		getStepOutputsFn: func(_ context.Context, _ string, refs []string) (map[string]json.RawMessage, error) {
			getStepOutputsCalled = true
			if len(refs) != 1 || refs[0] != "parent" {
				t.Fatalf("unexpected step refs: %v", refs)
			}
			return map[string]json.RawMessage{"parent": json.RawMessage(`{"data":"out"}`)}, nil
		},
	}
	eng := NewWorkflowEngine(&mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}, &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil },
	}, slog.Default())
	cb := NewStepCallback(ms, eng, slog.Default())

	steps := []domain.WorkflowStep{
		{StepRef: "parent", JobID: "j-parent"},
		{StepRef: "child", DependsOn: []string{"parent"}, JobID: "j-child"},
	}
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}
	runnableSteps := []domain.WorkflowStepRun{
		{ID: "sr-child", StepRef: "child", WorkflowRunID: "wr-1", Status: domain.StepPending, DepsCompleted: 1, DepsRequired: 1},
	}
	statuses := map[string]domain.StepRunStatus{"parent": domain.StepCompleted}

	err := cb.scheduleRunnableSteps(context.Background(), wfRun, steps, statuses, nil, runnableSteps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !getStepOutputsCalled {
		t.Fatal("expected GetStepOutputs to be called for step with DependsOn")
	}
}

// ---------------------------------------------------------------------------.
// propagateToParent -- direct lookup vs fallback
// ---------------------------------------------------------------------------.

func TestPropagateToParent_WithParentStepRunID(t *testing.T) {
	t.Parallel()
	var directLookupUsed bool
	ms := &mockCallbackStore{
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			if id == "parent-wr" {
				return &domain.WorkflowRun{
					ID: "parent-wr", WorkflowID: "parent-wf", Status: domain.WfStatusRunning,
				}, nil
			}
			return nil, nil
		},
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			if id == "parent-sr" {
				directLookupUsed = true
				return &domain.WorkflowStepRun{
					ID: "parent-sr", StepRef: "sub-step", WorkflowRunID: "parent-wr", Status: domain.StepRunning,
				}, nil
			}
			return nil, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "sub-step"}}, nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		listStepRunStatusesByWorkflowRunFn: func(_ context.Context, _ string) (map[string]domain.StepRunStatus, error) {
			return nil, nil
		},
		listRunningStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listRunnableStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := newTestCallback(ms)
	childRun := &domain.WorkflowRun{
		ID:                  "child-wr",
		WorkflowID:          "child-wf",
		ParentWorkflowRunID: "parent-wr",
		ParentStepRunID:     "parent-sr",
		Status:              domain.WfStatusCompleted,
	}
	err := cb.propagateToParent(context.Background(), childRun, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !directLookupUsed {
		t.Fatal("expected direct lookup via ParentStepRunID, not fallback scan")
	}
}

func TestPropagateToParent_WithoutParentStepRunID_FallbackScan(t *testing.T) {
	t.Parallel()
	var fallbackUsed bool
	ms := &mockCallbackStore{
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			if id == "parent-wr" {
				return &domain.WorkflowRun{
					ID: "parent-wr", WorkflowID: "parent-wf", Status: domain.WfStatusRunning,
				}, nil
			}
			return nil, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "sub-step", StepType: domain.WorkflowStepTypeSubWorkflow, SubWorkflowID: "child-wf"},
			}, nil
		},
		getStepRunByRunAndRefFn: func(_ context.Context, _ string, ref string) (*domain.WorkflowStepRun, error) {
			if ref == "sub-step" {
				fallbackUsed = true
				return &domain.WorkflowStepRun{
					ID: "parent-sr", StepRef: "sub-step", WorkflowRunID: "parent-wr", Status: domain.StepRunning,
				}, nil
			}
			return nil, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		listStepRunStatusesByWorkflowRunFn: func(_ context.Context, _ string) (map[string]domain.StepRunStatus, error) {
			return nil, nil
		},
		listRunningStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listRunnableStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := newTestCallback(ms)
	childRun := &domain.WorkflowRun{
		ID:                  "child-wr",
		WorkflowID:          "child-wf",
		ParentWorkflowRunID: "parent-wr",
		ParentStepRunID:     "",
		Status:              domain.WfStatusCompleted,
	}
	err := cb.propagateToParent(context.Background(), childRun, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fallbackUsed {
		t.Fatal("expected fallback scan via step matching when ParentStepRunID is empty")
	}
}

// ---------------------------------------------------------------------------.
// propagateToParent output aggregation
// ---------------------------------------------------------------------------.

func TestPropagateToParent_CompletedWithOutputs(t *testing.T) {
	t.Parallel()
	var storedOutput json.RawMessage
	ms := &mockCallbackStore{
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			if id == "parent-wr" {
				return &domain.WorkflowRun{
					ID: "parent-wr", WorkflowID: "parent-wf", Status: domain.WfStatusRunning,
				}, nil
			}
			return nil, nil
		},
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			if id == "parent-sr" {
				return &domain.WorkflowStepRun{
					ID: "parent-sr", StepRef: "sub-step", WorkflowRunID: "parent-wr", Status: domain.StepRunning,
				}, nil
			}
			return nil, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, fields map[string]any) error {
			if out, ok := fields["output"]; ok {
				storedOutput = out.(json.RawMessage)
			}
			return nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "sub-step"}}, nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		listStepRunStatusesByWorkflowRunFn: func(_ context.Context, _ string) (map[string]domain.StepRunStatus, error) {
			return nil, nil
		},
		listRunningStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listRunnableStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := newTestCallback(ms)
	childRun := &domain.WorkflowRun{
		ID:                  "child-wr",
		WorkflowID:          "child-wf",
		ParentWorkflowRunID: "parent-wr",
		ParentStepRunID:     "parent-sr",
		Status:              domain.WfStatusCompleted,
	}
	childStepRuns := []domain.WorkflowStepRun{
		{ID: "csr-1", StepRef: "step-a", Output: json.RawMessage(`{"a":"out"}`)},
		{ID: "csr-2", StepRef: "step-b", Output: json.RawMessage(`{"b":"out"}`)},
	}
	err := cb.propagateToParent(context.Background(), childRun, childStepRuns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if storedOutput == nil {
		t.Fatal("expected aggregated output on parent step")
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(storedOutput, &parsed); err != nil {
		t.Fatalf("failed to parse aggregated output: %v", err)
	}
	if _, ok := parsed["step-a"]; !ok {
		t.Error("expected step-a output in aggregated result")
	}
	if _, ok := parsed["step-b"]; !ok {
		t.Error("expected step-b output in aggregated result")
	}
}

func TestPropagateToParent_CompletedNoOutputs(t *testing.T) {
	t.Parallel()
	var storedOutput json.RawMessage
	var outputFieldSet bool
	ms := &mockCallbackStore{
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			if id == "parent-wr" {
				return &domain.WorkflowRun{
					ID: "parent-wr", WorkflowID: "parent-wf", Status: domain.WfStatusRunning,
				}, nil
			}
			return nil, nil
		},
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			if id == "parent-sr" {
				return &domain.WorkflowStepRun{
					ID: "parent-sr", StepRef: "sub-step", WorkflowRunID: "parent-wr", Status: domain.StepRunning,
				}, nil
			}
			return nil, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, fields map[string]any) error {
			if out, ok := fields["output"]; ok {
				outputFieldSet = true
				storedOutput = out.(json.RawMessage)
			}
			return nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "sub-step"}}, nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		listStepRunStatusesByWorkflowRunFn: func(_ context.Context, _ string) (map[string]domain.StepRunStatus, error) {
			return nil, nil
		},
		listRunningStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listRunnableStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := newTestCallback(ms)
	childRun := &domain.WorkflowRun{
		ID:                  "child-wr",
		WorkflowID:          "child-wf",
		ParentWorkflowRunID: "parent-wr",
		ParentStepRunID:     "parent-sr",
		Status:              domain.WfStatusCompleted,
	}
	childStepRuns := []domain.WorkflowStepRun{
		{ID: "csr-1", StepRef: "step-a", Output: nil},
	}
	err := cb.propagateToParent(context.Background(), childRun, childStepRuns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputFieldSet {
		t.Errorf("should not set output field when no child step has output, got %s", string(storedOutput))
	}
}

// ---------------------------------------------------------------------------.
// propagateToParent canceled child
// ---------------------------------------------------------------------------.

func TestPropagateToParent_CanceledChild(t *testing.T) {
	t.Parallel()
	var parentStepFailed bool
	var errorMsg string
	ms := &mockCallbackStore{
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			if id == "parent-wr" {
				return &domain.WorkflowRun{
					ID: "parent-wr", WorkflowID: "parent-wf", Status: domain.WfStatusRunning,
				}, nil
			}
			return nil, nil
		},
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			if id == "parent-sr" {
				return &domain.WorkflowStepRun{
					ID: "parent-sr", StepRef: "sub-step", WorkflowRunID: "parent-wr", Status: domain.StepRunning,
				}, nil
			}
			return nil, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, fields map[string]any) error {
			if status == domain.StepFailed {
				parentStepFailed = true
				if e, ok := fields["error"].(string); ok {
					errorMsg = e
				}
			}
			return nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "sub-step"}}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		cancelNonTerminalStepRunsFn: func(_ context.Context, _ string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
	}

	cb := newTestCallback(ms)
	childRun := &domain.WorkflowRun{
		ID:                  "child-wr",
		WorkflowID:          "child-wf",
		ParentWorkflowRunID: "parent-wr",
		ParentStepRunID:     "parent-sr",
		Status:              domain.WfStatusCanceled,
	}
	err := cb.propagateToParent(context.Background(), childRun, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !parentStepFailed {
		t.Fatal("expected parent step to be failed when child is canceled")
	}
	if errorMsg == "" {
		t.Error("expected error message describing canceled sub-workflow")
	}
}

// ---------------------------------------------------------------------------.
// OnEventReceived tests
// ---------------------------------------------------------------------------.

func TestOnEventReceived_NilTrigger(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	err := cb.OnEventReceived(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error for nil trigger, got: %v", err)
	}
}

func TestOnEventReceived_NonWorkflowStepSource(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	trigger := &domain.EventTrigger{
		ID:                "evt-1",
		SourceType:        domain.EventSourceJobRun,
		WorkflowStepRunID: "sr-1",
	}
	err := cb.OnEventReceived(context.Background(), trigger)
	if err != nil {
		t.Fatalf("expected nil error for non-workflow-step source, got: %v", err)
	}
}

func TestOnEventReceived_EmptyStepRunID(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)
	trigger := &domain.EventTrigger{
		ID:                "evt-1",
		SourceType:        domain.EventSourceWorkflowStep,
		WorkflowStepRunID: "",
	}
	err := cb.OnEventReceived(context.Background(), trigger)
	if err != nil {
		t.Fatalf("expected nil error for empty WorkflowStepRunID, got: %v", err)
	}
}

func TestOnEventReceived_TerminalNonCompleted(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID: id, StepRef: "failed-step", WorkflowRunID: "wr-1", Status: domain.StepFailed,
			}, nil
		},
	}
	cb := newTestCallback(ms)
	trigger := &domain.EventTrigger{
		ID:                "evt-1",
		SourceType:        domain.EventSourceWorkflowStep,
		WorkflowStepRunID: "sr-1",
	}
	err := cb.OnEventReceived(context.Background(), trigger)
	if err != nil {
		t.Fatalf("expected nil error for terminal non-completed step, got: %v", err)
	}
}

func TestOnEventReceived_NonTerminalStep_WithPayload(t *testing.T) {
	t.Parallel()
	var storedFields map[string]any
	ms := &mockCallbackStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID: "sr-1", StepRef: "wait-step", WorkflowRunID: "wr-1", Status: domain.StepWaiting,
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, fields map[string]any) error {
			storedFields = fields
			return nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "wait-step"}}, nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		listStepRunStatusesByWorkflowRunFn: func(_ context.Context, _ string) (map[string]domain.StepRunStatus, error) {
			return nil, nil
		},
		listRunningStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listRunnableStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := newTestCallback(ms)
	trigger := &domain.EventTrigger{
		ID:                "evt-1",
		SourceType:        domain.EventSourceWorkflowStep,
		WorkflowStepRunID: "sr-1",
		ResponsePayload:   json.RawMessage(`{"event":"data"}`),
	}
	err := cb.OnEventReceived(context.Background(), trigger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if storedFields == nil {
		t.Fatal("expected step run to be updated")
	}
	output, ok := storedFields["output"]
	if !ok {
		t.Fatal("expected output field when trigger has ResponsePayload")
	}
	if string(output.(json.RawMessage)) != `{"event":"data"}` {
		t.Errorf("unexpected output: %s", string(output.(json.RawMessage)))
	}
}

func TestOnEventReceived_NonTerminalStep_EmptyPayload(t *testing.T) {
	t.Parallel()
	var storedFields map[string]any
	ms := &mockCallbackStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID: "sr-1", StepRef: "wait-step", WorkflowRunID: "wr-1", Status: domain.StepWaiting,
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, fields map[string]any) error {
			storedFields = fields
			return nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "wait-step"}}, nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		listStepRunStatusesByWorkflowRunFn: func(_ context.Context, _ string) (map[string]domain.StepRunStatus, error) {
			return nil, nil
		},
		listRunningStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listRunnableStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := newTestCallback(ms)
	trigger := &domain.EventTrigger{
		ID:                "evt-1",
		SourceType:        domain.EventSourceWorkflowStep,
		WorkflowStepRunID: "sr-1",
		ResponsePayload:   nil,
	}
	err := cb.OnEventReceived(context.Background(), trigger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if storedFields == nil {
		t.Fatal("expected step run to be updated")
	}
	if _, hasOutput := storedFields["output"]; hasOutput {
		t.Error("should not set output field when ResponsePayload is empty")
	}
}
