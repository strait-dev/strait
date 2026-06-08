package api

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestSDKPayloadMarshalers(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	stream, err := marshalSDKStreamChunkPayload("hello", "stderr", true, ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"stream_chunk","chunk":"hello","stream_id":"stderr","done":true,"timestamp":"2026-06-07T12:00:00Z"}`, string(stream))

	resources, err := marshalSDKResourceSampleData(128.5, 76.25, 42.75)
	require.NoError(t, err)
	require.JSONEq(t, `{"memory_mb":128.5,"memory_percent":76.25,"cpu_percent":42.75}`, string(resources))

	oomRisk, err := marshalSDKOOMRiskData(920, 1000)
	require.NoError(t, err)
	require.JSONEq(t, `{"memory_mb":920,"memory_limit_mb":1000,"usage_percent":92}`, string(oomRisk))

	status, err := marshalSDKStatusChangePayload("run-1", "executing", "completed", ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"status_change","run_id":"run-1","from":"executing","to":"completed","timestamp":"2026-06-07T12:00:00Z"}`, string(status))

	failed, err := marshalSDKFailedStatusChangePayload("run-1", "executing", "failed", "boom", ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"status_change","run_id":"run-1","from":"executing","to":"failed","error":"boom","timestamp":"2026-06-07T12:00:00Z"}`, string(failed))

	event, err := marshalSDKRunEventPayload(domain.EventLog, "run-1", "info", "hello", json.RawMessage(`{"ok":true}`), ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"event","event_type":"log","run_id":"run-1","level":"info","message":"hello","data":{"ok":true},"timestamp":"2026-06-07T12:00:00Z"}`, string(event))

	progressData, err := marshalSDKProgressData(42.5, "step-a", 30)
	require.NoError(t, err)
	require.JSONEq(t, `{"percent":42.5,"step":"step-a","eta_seconds":30}`, string(progressData))

	progressEvent, err := marshalSDKProgressEventPayload("run-1", "halfway", 42.5, "step-a", 30, ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"event","event_type":"progress","run_id":"run-1","level":"info","message":"halfway","data":{"percent":42.5,"step":"step-a","eta_seconds":30},"timestamp":"2026-06-07T12:00:00Z"}`, string(progressEvent))
}

func TestSDKProgressDataOmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	payload, err := marshalSDKProgressData(42.5, "", 0)
	require.NoError(t, err)
	require.JSONEq(t, `{"percent":42.5}`, string(payload))
}

func TestSDKStatusChangePayloadPreservesNanosecondTimestamp(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 7, 12, 0, 0, 123456789, time.UTC)
	payload, err := marshalSDKStatusChangePayload("run-1", "executing", "completed", ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"status_change","run_id":"run-1","from":"executing","to":"completed","timestamp":"2026-06-07T12:00:00.123456789Z"}`, string(payload))
}

func TestSDKPayloadMarshalersEscapeEventFields(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 7, 12, 0, 0, 123456789, time.FixedZone("offset", -3*60*60))

	stream, err := marshalSDKStreamChunkPayload("hello \"\\\n<&>", "std<err>", true, ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"stream_chunk","chunk":"hello \"\\\n<&>","stream_id":"std<err>","done":true,"timestamp":"2026-06-07T12:00:00.123456789-03:00"}`, string(stream))

	status, err := marshalSDKFailedStatusChangePayload("run-\"1", "executing\n", "failed\\", "boom <&>", ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"status_change","run_id":"run-\"1","from":"executing\n","to":"failed\\","error":"boom <&>","timestamp":"2026-06-07T12:00:00.123456789-03:00"}`, string(status))

	event, err := marshalSDKRunEventPayload(domain.EventLog, "run-\"1", "info\n", "hello <&>", json.RawMessage(`[1,2,3]`), ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"event","event_type":"log","run_id":"run-\"1","level":"info\n","message":"hello <&>","data":[1,2,3],"timestamp":"2026-06-07T12:00:00.123456789-03:00"}`, string(event))

	progress, err := marshalSDKProgressEventPayload("run-\"1", "halfway <&>", 42.5, "step\nA", 30, ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"event","event_type":"progress","run_id":"run-\"1","level":"info","message":"halfway <&>","data":{"percent":42.5,"step":"step\nA","eta_seconds":30},"timestamp":"2026-06-07T12:00:00.123456789-03:00"}`, string(progress))
}

func TestSDKRunEventPayloadHandlesRawDataEdges(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	payload, err := marshalSDKRunEventPayload(domain.EventLog, "run-1", "info", "hello", nil, ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"event","event_type":"log","run_id":"run-1","level":"info","message":"hello","data":null,"timestamp":"2026-06-07T12:00:00Z"}`, string(payload))

	payload, err = marshalSDKRunEventPayload(domain.EventLog, "run-1", "info", "hello", json.RawMessage(`{"broken"`), ts)
	require.Error(t, err)
	require.Nil(t, payload)
}

func TestSDKNumericPayloadMarshalersRejectNonFiniteFloats(t *testing.T) {
	t.Parallel()

	_, err := marshalSDKResourceSampleData(math.NaN(), 76.25, 42.75)
	require.Error(t, err)

	_, err = marshalSDKOOMRiskData(920, 0)
	require.Error(t, err)

	_, err = marshalSDKProgressData(math.Inf(1), "step-a", 30)
	require.Error(t, err)

	_, err = marshalSDKProgressEventPayload("run-1", "halfway", math.NaN(), "step-a", 30, time.Now().UTC())
	require.Error(t, err)
}

func TestAPIRunPubSubChannel(t *testing.T) {
	t.Parallel()

	require.Equal(t, "run:run-1", apiRunPubSubChannel("run-1"))
}

func BenchmarkSDKPayloadMarshalers(b *testing.B) {
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	b.Run("stream_chunk", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			payload, err := marshalSDKStreamChunkPayload("hello world", "stdout", false, ts)
			if err != nil {
				b.Fatalf("marshalSDKStreamChunkPayload() error = %v", err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalSDKStreamChunkPayload() returned empty payload")
			}
		}
	})

	b.Run("resource_sample", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			payload, err := marshalSDKResourceSampleData(128.5, 76.25, 42.75)
			if err != nil {
				b.Fatalf("marshalSDKResourceSampleData() error = %v", err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalSDKResourceSampleData() returned empty payload")
			}
		}
	})

	b.Run("oom_risk", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			payload, err := marshalSDKOOMRiskData(920, 1000)
			if err != nil {
				b.Fatalf("marshalSDKOOMRiskData() error = %v", err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalSDKOOMRiskData() returned empty payload")
			}
		}
	})

	b.Run("status_completed", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			payload, err := marshalSDKStatusChangePayload("run-1", "executing", "completed", ts)
			if err != nil {
				b.Fatalf("marshalSDKStatusChangePayload() error = %v", err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalSDKStatusChangePayload() returned empty payload")
			}
		}
	})

	b.Run("status_failed", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			payload, err := marshalSDKFailedStatusChangePayload("run-1", "executing", "failed", "boom", ts)
			if err != nil {
				b.Fatalf("marshalSDKFailedStatusChangePayload() error = %v", err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalSDKFailedStatusChangePayload() returned empty payload")
			}
		}
	})

	b.Run("run_event", func(b *testing.B) {
		data := json.RawMessage(`{"ok":true}`)
		b.ReportAllocs()
		for range b.N {
			payload, err := marshalSDKRunEventPayload(domain.EventLog, "run-1", "info", "hello", data, ts)
			if err != nil {
				b.Fatalf("marshalSDKRunEventPayload() error = %v", err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalSDKRunEventPayload() returned empty payload")
			}
		}
	})

	b.Run("progress_data", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			payload, err := marshalSDKProgressData(42.5, "step-a", 30)
			if err != nil {
				b.Fatalf("marshalSDKProgressData() error = %v", err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalSDKProgressData() returned empty payload")
			}
		}
	})

	b.Run("progress_event", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			payload, err := marshalSDKProgressEventPayload("run-1", "halfway", 42.5, "step-a", 30, ts)
			if err != nil {
				b.Fatalf("marshalSDKProgressEventPayload() error = %v", err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalSDKProgressEventPayload() returned empty payload")
			}
		}
	})
}

func BenchmarkAPIRunPubSubChannel(b *testing.B) {
	runID := "run_0123456789abcdef"

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		channel := apiRunPubSubChannel(runID)
		if channel == "" {
			b.Fatal("apiRunPubSubChannel() returned empty channel")
		}
	}
}
