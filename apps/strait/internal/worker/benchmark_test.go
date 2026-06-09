package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func BenchmarkCircuitBreakerAllow(b *testing.B) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, OpenDuration: time.Second})

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cb.Allow()
		}
	})
}

func BenchmarkCircuitBreakerRecordSuccess(b *testing.B) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Second})

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		cb.RecordSuccess()
	}
}

func BenchmarkCircuitBreakerRecordFailure(b *testing.B) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: int(^uint(0) >> 1), OpenDuration: time.Second})

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		cb.RecordFailure()
	}
}

func BenchmarkPoolSubmit(b *testing.B) {
	p := NewPool(4)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		p.Submit(ctx, func() {})
	}

	b.StopTimer()
	_ = p.Shutdown(context.Background())
}

func BenchmarkValidateEndpointURL(b *testing.B) {
	endpoint := "https://example.com/webhook"

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if err := validateEndpointURL(endpoint); err != nil {
			b.Fatalf("validateEndpointURL() error = %v", err)
		}
	}
}

func BenchmarkEndpointStateKey(b *testing.B) {
	projectID := "project-0123456789abcdef"
	endpoint := "https://example.com/webhook?tenant=project-0123456789abcdef"

	b.ReportAllocs()
	for b.Loop() {
		key := endpointStateKey(projectID, endpoint)
		if key == "" {
			b.Fatal("endpointStateKey returned empty key")
		}
	}
}

func TestWorkerJobHealthKeyString(t *testing.T) {
	t.Parallel()

	got := workerJobHealthKeyString(jobHealthKey{JobID: "job-1", Bucket: 42})
	if got != "job-1\x0042" {
		t.Fatalf("workerJobHealthKeyString() = %q, want %q", got, "job-1\x0042")
	}
}

func BenchmarkWorkerJobHealthKeyString(b *testing.B) {
	key := jobHealthKey{JobID: "job-health-cache-key", Bucket: 1_234_567}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		out := workerJobHealthKeyString(key)
		if out == "" {
			b.Fatal("workerJobHealthKeyString() returned empty key")
		}
	}
}

func TestRunStatusChangePayloadAndChannel(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	payload, err := marshalRunStatusChangePayload(
		"run-1",
		"job-1",
		"proj-1",
		map[string]any{"from": "queued", "to": "executing"},
		timestamp,
	)
	if err != nil {
		t.Fatalf("marshalRunStatusChangePayload() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got["type"] != "status_change" ||
		got["run_id"] != "run-1" ||
		got["job_id"] != "job-1" ||
		got["project_id"] != "proj-1" ||
		got["from"] != "queued" ||
		got["to"] != "executing" ||
		got["timestamp"] != timestamp.Format(time.RFC3339) {
		t.Fatalf("unexpected status payload: %v", got)
	}
	if got := runPubSubChannel("run-1"); got != "run:run-1" {
		t.Fatalf("runPubSubChannel() = %q, want %q", got, "run:run-1")
	}
}

func TestRunStatusChangePayloadEscapesTransitionStrings(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	payload, err := marshalRunStatusChangePayload(
		`run-"1"`,
		"job-1",
		"proj-1",
		map[string]any{"from": "queued\nphase", "to": `executing"phase`},
		timestamp,
	)
	if err != nil {
		t.Fatalf("marshalRunStatusChangePayload() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; payload=%s", err, payload)
	}
	if got["run_id"] != `run-"1"` ||
		got["from"] != "queued\nphase" ||
		got["to"] != `executing"phase` {
		t.Fatalf("unexpected escaped status payload: %v", got)
	}
}

func TestRunStatusChangePayloadPreservesNanosecondTimestamp(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2026, 6, 7, 12, 0, 0, 123456789, time.UTC)
	payload, err := marshalRunStatusChangePayload(
		"run-1",
		"job-1",
		"proj-1",
		map[string]any{"from": "queued", "to": "executing"},
		timestamp,
	)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(payload, &got))
	require.Equal(t, timestamp.Format(time.RFC3339Nano), got["timestamp"])
}

func BenchmarkRunStatusChangePayloadAndChannel(b *testing.B) {
	timestamp := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	data := map[string]any{"from": "queued", "to": "executing"}

	b.Run("payload", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			payload, err := marshalRunStatusChangePayload("run-1", "job-1", "proj-1", data, timestamp)
			if err != nil {
				b.Fatal(err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalRunStatusChangePayload() returned empty payload")
			}
		}
	})

	b.Run("channel", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			channel := runPubSubChannel("run-1")
			if channel == "" {
				b.Fatal("runPubSubChannel() returned empty channel")
			}
		}
	})
}

func BenchmarkBuildDispatchHeadersWithSecrets(b *testing.B) {
	exec := &Executor{}
	job := &domain.Job{
		ID:          "job-1",
		ProjectID:   "project-1",
		TimeoutSecs: 30,
	}
	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   job.ID,
		Attempt: 1,
		Payload: []byte(`{"ok":true}`),
	}
	secrets := make([]domain.JobSecret, 16)
	for i := range secrets {
		secrets[i] = domain.JobSecret{
			SecretKey:      "SECRET_" + string(rune('A'+i)),
			EncryptedValue: "value",
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		headers, err := exec.buildDispatchHeaders(job, run, secrets, nil)
		if err != nil {
			b.Fatalf("buildDispatchHeaders() error = %v", err)
		}
		if len(headers) != len(secrets) {
			b.Fatalf("buildDispatchHeaders() returned %d headers, want %d", len(headers), len(secrets))
		}
	}
}

func BenchmarkSignHTTPDispatch(b *testing.B) {
	body := []byte(`{"event":"run.completed","run_id":"run-1","status":"completed"}`)
	secret := "endpoint-signing-secret"
	timestamp := "1780839000"

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		signature := SignHTTPDispatch(secret, timestamp, body)
		if len(signature) == 0 {
			b.Fatal("SignHTTPDispatch() returned empty signature")
		}
	}
}

func TestSignHTTPDispatchDeterministic(t *testing.T) {
	t.Parallel()

	body := []byte(`{"event":"run.completed","run_id":"run-1","status":"completed"}`)
	got := SignHTTPDispatch("endpoint-signing-secret", "1780839000", body)
	require.Equal(t, got, SignHTTPDispatch("endpoint-signing-secret", "1780839000", body))
	require.NotEqual(t, got, SignHTTPDispatch("different-secret", "1780839000", body))
	require.NotEqual(t, got, SignHTTPDispatch("endpoint-signing-secret", "1780839001", body))
	require.NotEqual(t, got, SignHTTPDispatch("endpoint-signing-secret", "1780839000", []byte(`{"event":"other"}`)))
}

func TestApplyWebhookSignatureReplacesExistingHeaderValues(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodPost, "https://example.com/webhook", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header["X-Strait-Timestamp"] = []string{"old", "duplicate"}
	req.Header["X-Strait-Signature"] = []string{"old", "duplicate"}
	req.Header["X-Webhook-Signature"] = []string{"old", "duplicate"}

	applyWebhookSignature(req, "webhook-secret", []byte(`{"ok":true}`))

	if len(req.Header.Values("X-Strait-Timestamp")) != 1 ||
		len(req.Header.Values("X-Strait-Signature")) != 1 ||
		len(req.Header.Values("X-Webhook-Signature")) != 1 {
		t.Fatalf("signature headers should replace existing values: %v", req.Header)
	}
}

func BenchmarkApplyWebhookSignature(b *testing.B) {
	body := []byte(`{"event":"run.completed","run_id":"run-1","status":"completed"}`)
	req, err := http.NewRequest(http.MethodPost, "https://example.com/webhook", nil)
	if err != nil {
		b.Fatalf("NewRequest() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		applyWebhookSignature(req, "webhook-secret", body)
		if req.Header.Get("X-Webhook-Signature") == "" {
			b.Fatal("applyWebhookSignature() did not set signature")
		}
	}
}
