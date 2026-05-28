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
