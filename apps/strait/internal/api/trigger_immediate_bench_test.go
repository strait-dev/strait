package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

var benchmarkImmediateRunSink *domain.JobRun

func BenchmarkNewImmediateTriggerRun_Minimal(b *testing.B) {
	srv := &Server{}
	job := benchmarkJob("job-123")
	state := &triggerRequestState{
		job:     job,
		req:     TriggerRequest{},
		payload: json.RawMessage(`{}`),
	}
	input := &TriggerJobInput{}
	expiresAt := time.Date(2026, 6, 9, 12, 5, 0, 0, time.UTC)
	cfg := immediateTriggerRunConfig{
		expiresAt: expiresAt,
		status:    domain.StatusQueued,
	}

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchmarkImmediateRunSink = srv.newImmediateTriggerRun(ctx, input, state, cfg)
	}
}

func BenchmarkNewImmediateTriggerRun_WithRequestMetadata(b *testing.B) {
	srv := &Server{}
	job := benchmarkJob("job-123")
	job.Tags = map[string]string{"team": "platform", "env": "prod"}
	job.DefaultRunMetadata = map[string]string{"retention": "short"}
	state := &triggerRequestState{
		job: job,
		req: TriggerRequest{
			Tags:           map[string]string{"env": "staging"},
			Priority:       5,
			ConcurrencyKey: "tenant-1",
		},
		payload: json.RawMessage(`{"dependency_key":"dep-1"}`),
	}
	input := &TriggerJobInput{
		Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00",
		Tracestate:  "vendor=value",
	}
	expiresAt := time.Date(2026, 6, 9, 12, 5, 0, 0, time.UTC)
	cfg := immediateTriggerRunConfig{
		expiresAt: expiresAt,
		status:    domain.StatusQueued,
	}

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchmarkImmediateRunSink = srv.newImmediateTriggerRun(ctx, input, state, cfg)
	}
}
