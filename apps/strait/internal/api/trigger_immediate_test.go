package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/telemetry"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestNewImmediateTriggerRunBuildsRunEnvelope(t *testing.T) {
	t.Parallel()

	scheduledAt := time.Date(2026, 6, 4, 13, 0, 0, 0, time.UTC)
	expiresAt := scheduledAt.Add(5 * time.Minute)
	ctx := context.WithValue(context.Background(), ctxActorIDKey, "apikey:trigger")
	srv := &Server{}
	state := &triggerRequestState{
		job: &domain.Job{
			ID:                 "job-1",
			ProjectID:          "project-1",
			Tags:               map[string]string{"team": "platform", "region": "default"},
			DefaultRunMetadata: map[string]string{"dependency_key": "default-dep", "retention": "short"},
			Version:            7,
			VersionID:          "version-7",
			ExecutionMode:      domain.ExecutionModeWorker,
			Queue:              "critical",
		},
		req: TriggerRequest{
			Tags:           map[string]string{"region": "eu"},
			Priority:       8,
			ConcurrencyKey: "customer-1",
		},
		payload:        json.RawMessage(`{"dependency_key":"payload-dep","ok":true}`),
		idempotencyKey: "idem-1",
	}
	input := &TriggerJobInput{
		Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00",
		Tracestate:  "vendor=value",
		SentryTrace: "trace",
		Baggage:     "tenant=project-1",
	}

	run := srv.newImmediateTriggerRun(ctx, input, state, immediateTriggerRunConfig{
		scheduledAt: &scheduledAt,
		expiresAt:   expiresAt,
		status:      domain.StatusDelayed,
	})
	require.NotEmpty(t, run.ID)
	require.False(t, run.JobID !=
		"job-1" ||
		run.ProjectID !=
			"project-1")
	require.False(t, run.Status !=
		domain.StatusDelayed ||
		run.Attempt != 1)
	require.Equal(t, domain.TriggerManual,
		run.
			TriggeredBy,
	)
	require.Equal(t, "apikey:trigger",
		run.CreatedBy,
	)
	require.False(t, run.Priority !=
		8 || run.
		ConcurrencyKey !=
		"customer-1" ||
		run.
			IdempotencyKey != "idem-1")
	require.False(t, run.JobVersion !=
		7 || run.
		JobVersionID !=
		"version-7")
	require.False(t, run.ExecutionMode !=
		domain.
			ExecutionModeWorker ||
		run.QueueName !=
			"critical")
	require.False(t, run.ScheduledAt ==
		nil ||
		!run.ScheduledAt.
			Equal(scheduledAt),
	)
	require.False(t, run.ExpiresAt ==
		nil ||
		!run.ExpiresAt.
			Equal(expiresAt))
	require.JSONEq(t, `{"dependency_key":"payload-dep","ok":true}`,

		string(run.Payload))
	require.False(t, run.Tags["team"] != "platform" ||

		run.Tags["region"] != "eu",
	)
	require.Equal(t, "payload-dep",
		run.Metadata["dependency_key"])
	require.Equal(t, "short", run.
		Metadata["retention"])
	require.Equal(t, triggerJobRoute,
		run.Metadata[domain.
			RunMetadataSentryRoute])
	require.Equal(t, input.Traceparent,
		run.Metadata[domain.
			RunMetadataTraceParent],
	)
	require.Equal(t, input.Baggage,
		run.Metadata[domain.
			RunMetadataSentryBaggage])
}

func TestExtractDependencyKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload json.RawMessage
		want    string
	}{
		{name: "empty", payload: nil, want: ""},
		{name: "empty object", payload: json.RawMessage(`{}`), want: ""},
		{name: "invalid", payload: json.RawMessage(`{`), want: ""},
		{name: "missing", payload: json.RawMessage(`{"ok":true}`), want: ""},
		{name: "token in value", payload: json.RawMessage(`{"message":"\"dependency_key\""}`), want: ""},
		{name: "nested key ignored", payload: json.RawMessage(`{"nested":{"dependency_key":"dep-nested"}}`), want: ""},
		{name: "non-string", payload: json.RawMessage(`{"dependency_key":42}`), want: ""},
		{name: "string", payload: json.RawMessage(`{"dependency_key":"dep-1"}`), want: "dep-1"},
		{name: "string with whitespace", payload: json.RawMessage(`{"dependency_key" : "dep-spaced"}`), want: "dep-spaced"},
		{name: "escaped string value", payload: json.RawMessage(`{"dependency_key":"dep-\u0031"}`), want: "dep-1"},
		{name: "escaped key", payload: json.RawMessage(`{"dependency\u005fkey":"dep-2"}`), want: "dep-2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, extractDependencyKey(tt.
				payload))
		})
	}
}

func TestImmediateTriggerAuditDetailsOmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	details := immediateTriggerAuditDetails(&domain.JobRun{
		ID:          "run-1",
		Priority:    0,
		TriggeredBy: domain.TriggerManual,
	}, nil, "", false)

	require.Equal(t, "run-1", details["run_id"])
	require.Equal(t, domain.TriggerManual, details["triggered_by"])
	require.NotContains(t, details, "scheduled_at")
	require.NotContains(t, details, "idempotency_key_hash")
	require.NotContains(t, details, "tag_keys")
	require.NotContains(t, details, "waiting")
}

func TestImmediateTriggerAuditDetailsIncludesOperationalFields(t *testing.T) {
	t.Parallel()

	scheduledAt := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	details := immediateTriggerAuditDetails(&domain.JobRun{
		ID:          "run-1",
		Priority:    5,
		Tags:        map[string]string{"team": "platform", "env": "prod"},
		TriggeredBy: domain.TriggerManual,
	}, &scheduledAt, "idem-123", true)

	require.Equal(t, "run-1", details["run_id"])
	require.Equal(t, &scheduledAt, details["scheduled_at"])
	require.Equal(t, 5, details["priority"])
	require.Equal(t, "f6fdb32bfd0ba473", details["idempotency_key_hash"])
	require.Equal(t, []string{"env", "team"}, details["tag_keys"])
	require.Equal(t, true, details["waiting"])
}

func TestImmediateTriggerAuditDetailsJSONOmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	detailsJSON := immediateTriggerAuditDetailsJSON(&domain.JobRun{
		ID:          "run-1",
		Priority:    0,
		TriggeredBy: domain.TriggerManual,
	}, nil, "", false)

	require.JSONEq(t, `{"run_id":"run-1","priority":0,"triggered_by":"manual"}`, string(detailsJSON))
}

func TestImmediateTriggerAuditDetailsJSONIncludesOperationalFields(t *testing.T) {
	t.Parallel()

	scheduledAt := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	detailsJSON := immediateTriggerAuditDetailsJSON(&domain.JobRun{
		ID:          `run-"quoted"`,
		Priority:    5,
		TriggeredBy: domain.TriggerManual,
	}, &scheduledAt, "idem-123", true)

	require.JSONEq(t, `{
		"run_id":"run-\"quoted\"",
		"priority":5,
		"triggered_by":"manual",
		"scheduled_at":"2026-06-09T12:00:00Z",
		"idempotency_key_hash":"f6fdb32bfd0ba473",
		"waiting":true
	}`, string(detailsJSON))
}

func TestTriggerDependenciesSatisfiedSkipsSatisfactionCheckWhenNoDependencies(t *testing.T) {
	t.Parallel()

	listCalls := 0
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	counter, err := provider.Meter("trigger-immediate-test").Int64Counter("strait_trigger_dependency_gate_total")
	require.NoError(t, err)

	ms := &APIStoreMock{
		ListJobDependenciesFunc: func(context.Context, string, int, *time.Time) ([]domain.JobDependency, error) {
			listCalls++
			return nil, nil
		},
		AreJobDependenciesSatisfiedFunc: func(context.Context, *domain.JobRun) (bool, error) {
			require.Fail(t, "dependency satisfaction query must be skipped for jobs with no dependency edges")
			return false, nil
		},
	}
	cache := newJobDependencyCache(time.Minute)
	t.Cleanup(cache.Stop)
	srv := &Server{
		store:              ms,
		jobDependencyCache: cache,
		metrics:            &telemetry.Metrics{TriggerDependencyGate: counter},
	}

	satisfied, err := srv.triggerDependenciesSatisfied(context.Background(), &domain.JobRun{JobID: "job-1"})

	require.NoError(t, err)
	require.True(t, satisfied)
	require.Equal(t, 1, listCalls)
	assertTriggerDependencyGateMetric(t, reader, "skipped")
}

func BenchmarkExtractDependencyKey(b *testing.B) {
	cases := []struct {
		name    string
		payload json.RawMessage
	}{
		{name: "missing", payload: json.RawMessage(`{"ok":true,"value":123}`)},
		{name: "empty_object", payload: json.RawMessage(`{}`)},
		{name: "present", payload: json.RawMessage(`{"dependency_key":"dep-1","ok":true}`)},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = extractDependencyKey(tc.payload)
			}
		})
	}
}

func assertTriggerDependencyGateMetric(t *testing.T, reader *sdkmetric.ManualReader, result string) {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "strait_trigger_dependency_gate_total" {
				continue
			}
			sum, ok := metric.Data.(metricdata.Sum[int64])
			require.Truef(t, ok, "unexpected metric type %T", metric.Data)
			for _, point := range sum.DataPoints {
				if point.Value != 1 {
					continue
				}
				got, ok := point.Attributes.Value("result")
				if ok && got.AsString() == result {
					return
				}
			}
		}
	}
	require.Failf(t, "test failure", "metric result=%q not found in %#v", result, rm.ScopeMetrics)
}
