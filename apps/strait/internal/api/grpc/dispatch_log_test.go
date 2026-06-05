package grpc

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

type recordingSlogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingSlogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *recordingSlogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingSlogHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *recordingSlogHandler) WithGroup(string) slog.Handler {
	return h
}

func (h *recordingSlogHandler) dispatchRecords() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, 0, len(h.records))
	for _, r := range h.records {
		if r.Message == "worker dispatch trace" {
			out = append(out, r.Clone())
		}
	}
	return out
}

type debugDiscardHandler struct{}

func (debugDiscardHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (debugDiscardHandler) Handle(context.Context, slog.Record) error {
	return nil
}

func (h debugDiscardHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h debugDiscardHandler) WithGroup(string) slog.Handler {
	return h
}

func TestWorkerDispatch_EmitsOneDebugTrace(t *testing.T) {
	handler := &recordingSlogHandler{}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	d := NewWorkerDispatcher(NewConnectionRegistry(), nil, "jwt-key", NewResultChannelRegistry())
	run := &domain.JobRun{ID: "run-log-1", ProjectID: "proj-a", JobID: "job-1"}
	job := &domain.Job{ID: "job-1", Queue: "default", Slug: "job"}

	_, err := d.WorkerDispatch(context.Background(), run, job)
	require.Error(
		t, err)

	records := handler.dispatchRecords()
	require.Len(t,
		records, 1)

	attrs := map[string]slog.Value{}
	records[0].Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value
		return true
	})
	for _, key := range []string{"run_id", "job_id", "queue", "project_id", "decision", "result", "duration_ms", "error"} {
		if _, ok := attrs[key]; !ok {
			require.Failf(t, "test failure",

				"dispatch debug record missing key %q; attrs=%v", key, attrs)
		}
	}
	require.Equal(
		t, "no_worker",
		attrs["decision"].String())
	require.Equal(
		t, "error",
		attrs["result"].String())
}

func BenchmarkDispatchHotPath(b *testing.B) {
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(debugDiscardHandler{}))
	b.Cleanup(func() { slog.SetDefault(oldLogger) })

	trace := dispatchTrace{
		Started:   time.Now(),
		RunID:     "run-bench",
		JobID:     "job-bench",
		Queue:     "default",
		ProjectID: "proj-bench",
		WorkerID:  "worker-bench",
		TaskID:    "task-bench",
		Decision:  "worker_reserved",
		Result:    "result_received",
	}
	b.ReportAllocs()
	for b.Loop() {
		trace.Started = time.Now()
		trace.finish(nil)
	}
}
