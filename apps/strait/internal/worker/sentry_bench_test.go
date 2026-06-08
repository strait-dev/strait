package worker

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/telemetry"
)

func benchmarkSentryBreadcrumbRun() *domain.JobRun {
	return &domain.JobRun{
		ID:            "run-1",
		JobID:         "job-1",
		ProjectID:     "proj-1",
		Attempt:       2,
		Status:        domain.StatusExecuting,
		ExecutionMode: domain.ExecutionModeHTTP,
	}
}

func BenchmarkAddWorkerRunBreadcrumb(b *testing.B) {
	run := benchmarkSentryBreadcrumbRun()
	job := &domain.Job{Version: 3, EnvironmentID: "env-1"}

	benchmarks := []struct {
		name string
		ctx  context.Context
		data func() map[string]any
	}{
		{
			name: "no_hub_nil_data",
			ctx:  context.Background(),
		},
		{
			name: "no_hub_existing_data",
			ctx:  context.Background(),
			data: func() map[string]any {
				return map[string]any{"phase": "dispatch"}
			},
		},
		{
			name: "hub_nil_data",
			ctx:  telemetry.EnsureSentryHub(context.Background()),
		},
		{
			name: "hub_existing_data",
			ctx:  telemetry.EnsureSentryHub(context.Background()),
			data: func() map[string]any {
				return map[string]any{"phase": "dispatch"}
			},
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var data map[string]any
				if bm.data != nil {
					data = bm.data()
				}
				addWorkerRunBreadcrumb(bm.ctx, "worker.dispatch", "run dispatch starting", run, job, data)
			}
		})
	}
}
