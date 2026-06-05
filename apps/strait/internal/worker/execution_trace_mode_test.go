package worker

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	orcstore "strait/internal/store"

	"github.com/stretchr/testify/require"
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
			require.Equal(t,
				tt.want, normalizeExecutionTraceMode(tt.in))

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
			require.Failf(t, "test failure", "normalizeExecutionTraceMode(%q) returned invalid mode %q", in, got)
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
			require.Equal(t,
				tt.want, got,
			)

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
	require.EqualValues(t, 12, trace.
		DispatchMs,
	)
	require.EqualValues(t, 425, trace.
		QueueWaitMs,
	)
	require.EqualValues(t, 275, trace.
		DequeueMs,
	)
	require.EqualValues(t, 1, trace.TotalMs)

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
	require.EqualValues(t, 0, trace.QueueWaitMs)
	require.EqualValues(t, 0, trace.DequeueMs)
	require.EqualValues(t, 0, trace.TotalMs)

}

func TestPopulateExecutionTraceRunTimingsHandlesNilInputs(t *testing.T) {
	t.Parallel()

	populateExecutionTraceRunTimings(nil, &domain.JobRun{}, time.Now(), time.Now())

	trace := &domain.ExecutionTrace{DispatchMs: 5}
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	populateExecutionTraceRunTimings(trace, nil, start, start.Add(10*time.Millisecond))
	require.EqualValues(t, 5, trace.DispatchMs)
	require.EqualValues(t, 10, trace.
		TotalMs)
	require.False(t,
		trace.QueueWaitMs !=
			0 || trace.DequeueMs !=
			0)

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
			require.Fail(t,

				"GetJobHealthStats should not be called when prefetched stats are provided")
			return nil, nil
		},
	}
	exec := NewExecutor(ExecutorConfig{Store: store})
	stats := &orcstore.JobHealthStats{P95DurationSecs: 10}
	require.True(t,
		exec.handleSuccessWithStats(context.Background(), run, job, nil, nil, stats))
	require.Len(t, store.
		statusUpdates(),
		1)

}
