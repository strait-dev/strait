package worker

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	orcstore "strait/internal/store"
)

func TestNormalizeExecutionTraceMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want executionTraceMode
	}{
		{name: "empty defaults off", in: "", want: executionTraceOff},
		{name: "off", in: "off", want: executionTraceOff},
		{name: "errors", in: "errors", want: executionTraceErrors},
		{name: "full", in: "full", want: executionTraceFull},
		{name: "case and spaces", in: " FULL ", want: executionTraceFull},
		{name: "unknown defaults off", in: "always", want: executionTraceOff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeExecutionTraceMode(tt.in); got != tt.want {
				t.Fatalf("normalizeExecutionTraceMode(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func FuzzNormalizeExecutionTraceMode(f *testing.F) {
	for _, seed := range []string{"", "off", "errors", "full", " FULL ", "invalid"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		got := normalizeExecutionTraceMode(in)
		switch got {
		case executionTraceOff, executionTraceErrors, executionTraceFull:
		default:
			t.Fatalf("normalizeExecutionTraceMode(%q) returned invalid mode %q", in, got)
		}
	})
}

func TestExecutorExecutionTraceModeFieldSelection(t *testing.T) {
	t.Parallel()

	trace := &domain.ExecutionTrace{DispatchMs: 10}
	tests := []struct {
		name   string
		mode   executionTraceMode
		status domain.RunStatus
		want   bool
	}{
		{name: "off success", mode: executionTraceOff, status: domain.StatusCompleted, want: false},
		{name: "off failure", mode: executionTraceOff, status: domain.StatusDeadLetter, want: false},
		{name: "errors success", mode: executionTraceErrors, status: domain.StatusCompleted, want: false},
		{name: "errors failure", mode: executionTraceErrors, status: domain.StatusDeadLetter, want: true},
		{name: "full success", mode: executionTraceFull, status: domain.StatusCompleted, want: true},
		{name: "full failure", mode: executionTraceFull, status: domain.StatusTimedOut, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			exec := &Executor{executionTraceMode: tt.mode}
			fields := map[string]any{}
			exec.addExecutionTraceField(fields, tt.status, trace)
			_, got := fields["execution_trace"]
			if got != tt.want {
				t.Fatalf("execution_trace present = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPopulateExecutionTraceRunTimings(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startedAt := createdAt.Add(150 * time.Millisecond)
	executeStart := createdAt.Add(425 * time.Millisecond)
	traceEnd := executeStart.Add(1250 * time.Microsecond)
	trace := &domain.ExecutionTrace{DispatchMs: 12}
	run := &domain.JobRun{
		CreatedAt: createdAt,
		StartedAt: &startedAt,
	}

	populateExecutionTraceRunTimings(trace, run, executeStart, traceEnd)

	if trace.DispatchMs != 12 {
		t.Fatalf("DispatchMs = %d, want preserved 12", trace.DispatchMs)
	}
	if trace.QueueWaitMs != 425 {
		t.Fatalf("QueueWaitMs = %d, want 425", trace.QueueWaitMs)
	}
	if trace.DequeueMs != 275 {
		t.Fatalf("DequeueMs = %d, want 275", trace.DequeueMs)
	}
	if trace.TotalMs != 1 {
		t.Fatalf("TotalMs = %d, want sub-millisecond floor 1", trace.TotalMs)
	}
}

func TestPopulateExecutionTraceRunTimingsClampsNegativeDurations(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startedAt := createdAt.Add(2 * time.Second)
	executeStart := createdAt.Add(time.Second)
	trace := &domain.ExecutionTrace{}
	run := &domain.JobRun{
		CreatedAt: createdAt.Add(3 * time.Second),
		StartedAt: &startedAt,
	}

	populateExecutionTraceRunTimings(trace, run, executeStart, executeStart.Add(-time.Millisecond))

	if trace.QueueWaitMs != 0 {
		t.Fatalf("QueueWaitMs = %d, want 0", trace.QueueWaitMs)
	}
	if trace.DequeueMs != 0 {
		t.Fatalf("DequeueMs = %d, want 0", trace.DequeueMs)
	}
	if trace.TotalMs != 0 {
		t.Fatalf("TotalMs = %d, want 0", trace.TotalMs)
	}
}

func TestPopulateExecutionTraceRunTimingsHandlesNilInputs(t *testing.T) {
	t.Parallel()

	populateExecutionTraceRunTimings(nil, &domain.JobRun{}, time.Now(), time.Now())

	trace := &domain.ExecutionTrace{DispatchMs: 5}
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	populateExecutionTraceRunTimings(trace, nil, start, start.Add(10*time.Millisecond))

	if trace.DispatchMs != 5 {
		t.Fatalf("DispatchMs = %d, want preserved 5", trace.DispatchMs)
	}
	if trace.TotalMs != 10 {
		t.Fatalf("TotalMs = %d, want 10", trace.TotalMs)
	}
	if trace.QueueWaitMs != 0 || trace.DequeueMs != 0 {
		t.Fatalf("run timings = queue %d dequeue %d, want zeroes", trace.QueueWaitMs, trace.DequeueMs)
	}
}

func BenchmarkExecutionTraceModeFieldSelection(b *testing.B) {
	trace := &domain.ExecutionTrace{DispatchMs: 10}
	benches := []struct {
		name   string
		mode   executionTraceMode
		status domain.RunStatus
	}{
		{name: "off_success", mode: executionTraceOff, status: domain.StatusCompleted},
		{name: "errors_failure", mode: executionTraceErrors, status: domain.StatusDeadLetter},
		{name: "full_success", mode: executionTraceFull, status: domain.StatusCompleted},
	}

	for _, bb := range benches {
		b.Run(bb.name, func(b *testing.B) {
			b.ReportAllocs()
			exec := &Executor{executionTraceMode: bb.mode}
			fieldsWritten := 0
			for b.Loop() {
				fields := make(map[string]any, 1)
				exec.addExecutionTraceField(fields, bb.status, trace)
				if _, ok := fields["execution_trace"]; ok {
					fieldsWritten++
				}
			}
			b.ReportMetric(float64(fieldsWritten)/float64(b.N), "fields/op")
		})
	}
}

func TestExecutorHandleSuccessWithStatsDoesNotReloadHealthStats(t *testing.T) {
	t.Parallel()

	run := testRun(1)
	started := time.Now().Add(-100 * time.Millisecond)
	run.StartedAt = &started
	job := testJob("https://example.com", 1, 5)
	store := &mockExecutorStore{
		getJobHealthStatsFn: func(context.Context, string, time.Time) (*orcstore.JobHealthStats, error) {
			t.Fatal("GetJobHealthStats should not be called when prefetched stats are provided")
			return nil, nil
		},
	}
	exec := NewExecutor(ExecutorConfig{Store: store})
	stats := &orcstore.JobHealthStats{P95DurationSecs: 10}

	if !exec.handleSuccessWithStats(context.Background(), run, job, nil, nil, stats) {
		t.Fatal("handleSuccessWithStats() = false, want true")
	}
	if len(store.statusUpdates()) != 1 {
		t.Fatalf("status updates = %d, want 1", len(store.statusUpdates()))
	}
}
