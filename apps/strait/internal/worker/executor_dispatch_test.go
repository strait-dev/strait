package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"

	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"
)

type dispatchRoundTripFunc func(*http.Request) (*http.Response, error)

func (f dispatchRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type fakeEndpointSecretDecryptor struct{}

func (fakeEndpointSecretDecryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	const prefix = "encrypted:"
	if !strings.HasPrefix(string(ciphertext), prefix) {
		return nil, errors.New("unexpected ciphertext")
	}
	return ciphertext[len(prefix):], nil
}

func TestExecutorEndpointSigningSecretDecryptsEncryptedField(t *testing.T) {
	t.Parallel()

	encrypted := "enc:v1:" + base64.StdEncoding.EncodeToString([]byte("encrypted:plain-endpoint-secret"))
	exec := &Executor{secretDecryptor: fakeEndpointSecretDecryptor{}}

	got, err := exec.endpointSigningSecret(&domain.Job{EndpointSigningSecret: encrypted})
	if err != nil {
		t.Fatalf("endpointSigningSecret: %v", err)
	}
	if got != "plain-endpoint-secret" {
		t.Fatalf("endpointSigningSecret = %q, want plaintext", got)
	}
}

func TestExecutorEndpointSigningSecretPreservesLegacyPlaintext(t *testing.T) {
	t.Parallel()

	exec := &Executor{}
	got, err := exec.endpointSigningSecret(&domain.Job{EndpointSigningSecret: "legacy-plain-secret"})
	if err != nil {
		t.Fatalf("endpointSigningSecret: %v", err)
	}
	if got != "legacy-plain-secret" {
		t.Fatalf("endpointSigningSecret = %q, want legacy plaintext", got)
	}
	if straitcrypto.IsEncryptedField(got) {
		t.Fatal("legacy plaintext should not be treated as encrypted")
	}
}

func TestDispatchHeaderInputsFirstAttemptSkipsCheckpoint(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		listSecretsFn: func(_ context.Context, jobID, environment string) ([]domain.JobSecret, error) {
			if jobID != "job-1" || environment != "env-1" {
				t.Fatalf("ListJobSecretsByJob args = %q %q, want job-1 env-1", jobID, environment)
			}
			return []domain.JobSecret{{SecretKey: "API_KEY", EncryptedValue: "secret"}}, nil
		},
		getLatestCheckpointFn: func(context.Context, string) (*domain.RunCheckpoint, error) {
			t.Fatal("first-attempt dispatch headers must not load checkpoints")
			return nil, nil
		},
	}
	exec := &Executor{store: store}
	job := &domain.Job{ID: "job-1", EnvironmentID: "env-1"}
	run := &domain.JobRun{ID: "run-1", Attempt: 1}

	inputs, err := exec.dispatchHeaderInputs(context.Background(), job, run)
	if err != nil {
		t.Fatalf("dispatchHeaderInputs() error = %v", err)
	}
	if len(inputs.secrets) != 1 || inputs.secrets[0].SecretKey != "API_KEY" {
		t.Fatalf("secrets = %+v, want API_KEY secret", inputs.secrets)
	}
	if inputs.checkpoint != nil {
		t.Fatalf("checkpoint = %+v, want nil on first attempt", inputs.checkpoint)
	}
}

func TestDispatchHeaderInputsRetryUsesCache(t *testing.T) {
	t.Parallel()

	var secretCalls int
	var checkpointCalls int
	store := &mockExecutorStore{
		listSecretsFn: func(context.Context, string, string) ([]domain.JobSecret, error) {
			secretCalls++
			return []domain.JobSecret{{SecretKey: "API_KEY", EncryptedValue: "secret"}}, nil
		},
		getLatestCheckpointFn: func(context.Context, string) (*domain.RunCheckpoint, error) {
			checkpointCalls++
			return &domain.RunCheckpoint{ID: "cp-1", RunID: "run-1"}, nil
		},
	}
	exec := &Executor{store: store}
	job := &domain.Job{ID: "job-1", EnvironmentID: "env-1"}
	run := &domain.JobRun{ID: "run-1", Attempt: 2}
	ctx := withDispatchCache(context.Background())

	first, err := exec.dispatchHeaderInputs(ctx, job, run)
	if err != nil {
		t.Fatalf("first dispatchHeaderInputs() error = %v", err)
	}
	second, err := exec.dispatchHeaderInputs(ctx, job, run)
	if err != nil {
		t.Fatalf("second dispatchHeaderInputs() error = %v", err)
	}

	if secretCalls != 1 {
		t.Fatalf("secret calls = %d, want 1 cached call", secretCalls)
	}
	if checkpointCalls != 1 {
		t.Fatalf("checkpoint calls = %d, want 1 cached call", checkpointCalls)
	}
	if first.checkpoint == nil || first.checkpoint.ID != "cp-1" {
		t.Fatalf("first checkpoint = %+v, want cp-1", first.checkpoint)
	}
	if second.checkpoint == nil || second.checkpoint.ID != "cp-1" {
		t.Fatalf("second checkpoint = %+v, want cp-1", second.checkpoint)
	}
}

func TestDispatch_RetryIncludesCheckpointHeaders(t *testing.T) {
	t.Parallel()

	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	cpTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}
	store.getLatestCheckpointFn = func(_ context.Context, _ string) (*domain.RunCheckpoint, error) {
		return &domain.RunCheckpoint{
			ID:        "cp-1",
			RunID:     "run-1",
			Sequence:  1,
			State:     json.RawMessage(`{"cursor":42}`),
			CreatedAt: cpTime,
		}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(2)

	exec.execute(context.Background(), run)

	if headers.Get("X-Last-Checkpoint") != `{"cursor":42}` {
		t.Fatalf("X-Last-Checkpoint = %q, want %q", headers.Get("X-Last-Checkpoint"), `{"cursor":42}`)
	}
	if headers.Get("X-Checkpoint-At") != cpTime.Format(time.RFC3339) {
		t.Fatalf("X-Checkpoint-At = %q, want %q", headers.Get("X-Checkpoint-At"), cpTime.Format(time.RFC3339))
	}
}

func TestDispatch_FirstAttemptNoCheckpointHeaders(t *testing.T) {
	t.Parallel()

	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}
	store.getLatestCheckpointFn = func(_ context.Context, _ string) (*domain.RunCheckpoint, error) {
		t.Fatal("should not call GetLatestCheckpoint on first attempt")
		return nil, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	if headers.Get("X-Last-Checkpoint") != "" {
		t.Fatalf("expected no X-Last-Checkpoint on first attempt, got %q", headers.Get("X-Last-Checkpoint"))
	}
}

func TestDispatch_NoCheckpointGraceful(t *testing.T) {
	t.Parallel()

	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}
	store.getLatestCheckpointFn = func(_ context.Context, _ string) (*domain.RunCheckpoint, error) {
		return nil, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(2)

	exec.execute(context.Background(), run)

	if headers.Get("X-Last-Checkpoint") != "" {
		t.Fatalf("expected no X-Last-Checkpoint when none exists, got %q", headers.Get("X-Last-Checkpoint"))
	}
	if run.Status != domain.StatusCompleted {
		t.Fatalf("run status = %s, want completed", run.Status)
	}
}

// TestTracedDispatch_RetryEmitsCheckpointHeadersWhenSecretsCacheWarm pins the
// durable-resume contract: a warm dispatch secrets cache must not suppress the
// checkpoint load on a retry.
func TestTracedDispatch_RetryEmitsCheckpointHeadersWhenSecretsCacheWarm(t *testing.T) {
	t.Parallel()

	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	cpTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	store := &mockExecutorStore{}
	store.getLatestCheckpointFn = func(_ context.Context, _ string) (*domain.RunCheckpoint, error) {
		return &domain.RunCheckpoint{
			ID:        "cp-1",
			RunID:     "run-1",
			Sequence:  1,
			State:     json.RawMessage(`{"cursor":42}`),
			CreatedAt: cpTime,
		}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())

	ctx := withDispatchCache(context.Background())
	job := testJob(server.URL, 3, 5)
	dispatchCacheSet(ctx, dispatchSecretsCacheKey(job), []domain.JobSecret{})

	if _, _, err := exec.tracedDispatch(ctx, job, testRun(2)); err != nil {
		t.Fatalf("tracedDispatch: %v", err)
	}

	if got := headers.Get("X-Last-Checkpoint"); got != `{"cursor":42}` {
		t.Fatalf("X-Last-Checkpoint = %q, want %q (the warm secrets cache must not suppress the checkpoint load)", got, `{"cursor":42}`)
	}
	if got := headers.Get("X-Checkpoint-At"); got != cpTime.Format(time.RFC3339) {
		t.Fatalf("X-Checkpoint-At = %q, want %q", got, cpTime.Format(time.RFC3339))
	}
}

func TestDispatch_RetryIncludesPreviousError(t *testing.T) {
	t.Parallel()

	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(2)
	run.Error = "connection timeout"

	exec.execute(context.Background(), run)

	if headers.Get("X-Previous-Error") != "connection timeout" {
		t.Fatalf("X-Previous-Error = %q, want %q", headers.Get("X-Previous-Error"), "connection timeout")
	}
}

func TestDispatch_RetryNoPreviousError(t *testing.T) {
	t.Parallel()

	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(2)
	run.Error = ""

	exec.execute(context.Background(), run)

	if headers.Get("X-Previous-Error") != "" {
		t.Fatalf("expected no X-Previous-Error when empty, got %q", headers.Get("X-Previous-Error"))
	}
}

func TestHTTPDispatchTraceRecorderExecutionTrace(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	recorder := newHTTPDispatchTraceRecorder(start)
	recorder.recordConnectStart(start.Add(10 * time.Millisecond))
	recorder.recordConnectDone(start.Add(20 * time.Millisecond))
	recorder.recordFirstByte(start.Add(35 * time.Millisecond))

	trace := recorder.executionTrace(start.Add(50 * time.Millisecond))
	if trace.ConnectMs != 10 {
		t.Fatalf("ConnectMs = %d, want 10", trace.ConnectMs)
	}
	if trace.TtfbMs != 15 {
		t.Fatalf("TtfbMs = %d, want 15", trace.TtfbMs)
	}
	if trace.TransferMs != 15 {
		t.Fatalf("TransferMs = %d, want 15", trace.TransferMs)
	}
	if trace.DispatchMs != 40 {
		t.Fatalf("DispatchMs = %d, want 40", trace.DispatchMs)
	}
}

func TestReadDispatchResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
		wantErr    bool
	}{
		{
			name:       "JSON success",
			statusCode: http.StatusOK,
			body:       `{"ok":true}`,
			want:       `{"ok":true}`,
		},
		{
			name:       "text success normalizes to JSON string",
			statusCode: http.StatusOK,
			body:       "ok",
			want:       `"ok"`,
		},
		{
			name:       "empty success returns nil result",
			statusCode: http.StatusNoContent,
		},
		{
			name:       "endpoint error preserves response body",
			statusCode: http.StatusServiceUnavailable,
			body:       "service down",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := &http.Response{
				StatusCode: tt.statusCode,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			got, err := readDispatchResponse(context.Background(), resp)
			if tt.wantErr {
				var endpointErr *domain.EndpointError
				if !errors.As(err, &endpointErr) {
					t.Fatalf("error = %T %v, want EndpointError", err, err)
				}
				if endpointErr.StatusCode != tt.statusCode {
					t.Fatalf("status = %d, want %d", endpointErr.StatusCode, tt.statusCode)
				}
				if endpointErr.Body != tt.body {
					t.Fatalf("body = %q, want %q", endpointErr.Body, tt.body)
				}
				return
			}
			if err != nil {
				t.Fatalf("readDispatchResponse() error = %v", err)
			}
			if tt.want == "" {
				if got != nil {
					t.Fatalf("result = %s, want nil", got)
				}
				return
			}
			if string(got) != tt.want {
				t.Fatalf("result = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestHTTPDispatchConcurrencyLimit(t *testing.T) {
	t.Parallel()

	degradedScore := &domain.EndpointHealthScore{HealthScore: 45}

	tests := []struct {
		name     string
		job      *domain.Job
		prefetch dispatchPrefetch
		want     int
	}{
		{
			name: "job limit without health score",
			job:  &domain.Job{MaxConcurrency: 8},
			want: 8,
		},
		{
			name: "healthy score preserves job limit",
			job:  &domain.Job{MaxConcurrency: 8},
			prefetch: dispatchPrefetch{
				healthScore: &domain.EndpointHealthScore{HealthScore: 90},
			},
			want: 8,
		},
		{
			name: "degraded score throttles job limit",
			job:  &domain.Job{MaxConcurrency: 8},
			prefetch: dispatchPrefetch{
				healthScore: degradedScore,
			},
			want: ThrottledConcurrency(degradedScore, 8),
		},
		{
			name: "unlimited job remains unlimited",
			job:  &domain.Job{MaxConcurrency: 0},
			prefetch: dispatchPrefetch{
				healthScore: degradedScore,
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := httpDispatchConcurrencyLimit(tt.job, tt.prefetch)
			if got != tt.want {
				t.Fatalf("httpDispatchConcurrencyLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHTTPDispatch_InjectsTraceparentHeader(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			domain.RunMetadataTraceParent: "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
		},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	got := capturedHeaders.Get("Traceparent")
	if got != "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01" {
		t.Errorf("Traceparent header = %q, want %q", got, "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01")
	}
}

func TestHTTPDispatch_InjectsTracestateHeader(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			domain.RunMetadataTraceParent: "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
			domain.RunMetadataTraceState:  "congo=t61rcWkgMzE",
		},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if tp := capturedHeaders.Get("Traceparent"); tp != "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01" {
		t.Errorf("Traceparent header = %q, want traceparent value", tp)
	}
	if ts := capturedHeaders.Get("Tracestate"); ts != "congo=t61rcWkgMzE" {
		t.Errorf("Tracestate header = %q, want %q", ts, "congo=t61rcWkgMzE")
	}
}

func TestHTTPDispatch_InjectsSentryTraceHeaders(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			domain.RunMetadataSentryTrace:   "0123456789abcdef0123456789abcdef-0123456789abcdef-1",
			domain.RunMetadataSentryBaggage: "sentry-release=test-release,sentry-public_key=public",
		},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if got := capturedHeaders.Get(sentry.SentryTraceHeader); got != "0123456789abcdef0123456789abcdef-0123456789abcdef-1" {
		t.Fatalf("sentry-trace header = %q, want Sentry trace metadata", got)
	}
	if got := capturedHeaders.Get(sentry.SentryBaggageHeader); got != "sentry-release=test-release,sentry-public_key=public" {
		t.Fatalf("baggage header = %q, want Sentry baggage metadata", got)
	}
}

func TestHTTPDispatch_NoTraceMetadata(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:       "run-1",
		JobID:    "job-1",
		Attempt:  1,
		Metadata: map[string]string{"some_key": "some_value"},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if tp := capturedHeaders.Get("Traceparent"); tp != "" {
		t.Errorf("expected no Traceparent header, got %q", tp)
	}
	if ts := capturedHeaders.Get("Tracestate"); ts != "" {
		t.Errorf("expected no Tracestate header, got %q", ts)
	}
	if st := capturedHeaders.Get(sentry.SentryTraceHeader); st != "" {
		t.Errorf("expected no Sentry trace header, got %q", st)
	}
	if baggage := capturedHeaders.Get(sentry.SentryBaggageHeader); baggage != "" {
		t.Errorf("expected no Sentry baggage header, got %q", baggage)
	}
}

func TestHTTPDispatch_EmptyTraceParent(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			domain.RunMetadataTraceParent: "",
		},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if tp := capturedHeaders.Get("Traceparent"); tp != "" {
		t.Errorf("expected no Traceparent header when _trace_parent is empty, got %q", tp)
	}
}

func TestHTTPDispatch_NilMetadata(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:       "run-1",
		JobID:    "job-1",
		Attempt:  1,
		Metadata: nil,
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if tp := capturedHeaders.Get("Traceparent"); tp != "" {
		t.Errorf("expected no Traceparent header when metadata is nil, got %q", tp)
	}
	if ts := capturedHeaders.Get("Tracestate"); ts != "" {
		t.Errorf("expected no Tracestate header when metadata is nil, got %q", ts)
	}
}

func TestHTTPDispatch_TraceHeadersCoexistWithExtraHeaders(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			domain.RunMetadataTraceParent: "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
		},
	}

	extraHeaders := map[string]string{
		"X-Custom-Header": "custom-value",
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, extraHeaders)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if tp := capturedHeaders.Get("Traceparent"); tp != "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01" {
		t.Errorf("Traceparent header = %q, want traceparent value", tp)
	}
	if ch := capturedHeaders.Get("X-Custom-Header"); ch != "custom-value" {
		t.Errorf("X-Custom-Header = %q, want %q", ch, "custom-value")
	}
}

func TestHTTPDispatch_NonTraceMetadataNotLeaked(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			"secret":                      "super-secret-value",
			domain.RunMetadataTraceParent: "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
		},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if v := capturedHeaders.Get("Secret"); v != "" {
		t.Errorf("non-trace metadata 'secret' leaked as header: %q", v)
	}
	if _, ok := capturedHeaders["Secret"]; ok {
		t.Error("non-trace metadata 'secret' should not appear as a request header")
	}
	if tp := capturedHeaders.Get("Traceparent"); tp != "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01" {
		t.Errorf("Traceparent header = %q, want traceparent value", tp)
	}
}

func TestHTTPDispatch_RedactsEndpointURLFromClientErrors(t *testing.T) {
	t.Parallel()

	rawURL := "https://user:pass@hooks.example.com/private/path?token=secret#frag"
	rootErr := context.DeadlineExceeded
	client := &http.Client{
		Transport: dispatchRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: req.Method, URL: rawURL, Err: rootErr}
		}),
	}
	e := &Executor{httpClient: client}

	_, err := e.dispatchToEndpoint(t.Context(), rawURL, &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}, nil)
	if err == nil {
		t.Fatal("dispatchToEndpoint returned nil error")
	}
	if !errors.Is(err, rootErr) {
		t.Fatalf("dispatchToEndpoint error does not unwrap deadline: %v", err)
	}
	got := err.Error()
	for _, leaked := range []string{"hooks.example.com", "user:pass", "/private/path", "token=secret", "#frag"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("dispatchToEndpoint leaked endpoint data %q in error %q", leaked, got)
		}
	}
	if !strings.Contains(got, "http dispatch:") || !strings.Contains(got, "context deadline exceeded") {
		t.Fatalf("dispatchToEndpoint error = %q, want sanitized dispatch context and root error", got)
	}
}
