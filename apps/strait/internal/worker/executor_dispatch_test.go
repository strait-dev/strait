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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.NoError(
		t, err)
	require.Equal(t,
		"plain-endpoint-secret",
		got)
}

func TestExecutorEndpointSigningSecretPreservesLegacyPlaintext(t *testing.T) {
	t.Parallel()

	exec := &Executor{}
	got, err := exec.endpointSigningSecret(&domain.Job{EndpointSigningSecret: "legacy-plain-secret"})
	require.NoError(
		t, err)
	require.Equal(t,
		"legacy-plain-secret",
		got)
	require.False(t,
		straitcrypto.IsEncryptedField(got))
}

func TestDispatchHeaderInputsFirstAttemptSkipsCheckpoint(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		listSecretsFn: func(_ context.Context, jobID, environment string) ([]domain.JobSecret, error) {
			require.False(t,
				jobID != "job-1" ||
					environment !=
						"env-1")

			return []domain.JobSecret{{SecretKey: "API_KEY", EncryptedValue: "secret"}}, nil
		},
		getLatestCheckpointFn: func(context.Context, string) (*domain.RunCheckpoint, error) {
			require.Fail(t,

				"first-attempt dispatch headers must not load checkpoints")
			return nil, nil
		},
	}
	exec := &Executor{store: store}
	job := &domain.Job{ID: "job-1", EnvironmentID: "env-1"}
	run := &domain.JobRun{ID: "run-1", Attempt: 1}

	inputs, err := exec.dispatchHeaderInputs(context.Background(), job, run)
	require.NoError(
		t, err)
	require.False(t,
		len(inputs.secrets) != 1 || inputs.
			secrets[0].
			SecretKey !=
			"API_KEY",
	)
	require.Nil(t, inputs.checkpoint)
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
	require.NoError(
		t, err)

	second, err := exec.dispatchHeaderInputs(ctx, job, run)
	require.NoError(
		t, err)
	require.Equal(t, 1, secretCalls)
	require.Equal(t, 1, checkpointCalls)
	require.False(t,
		first.checkpoint ==
			nil || first.checkpoint.
			ID !=
			"cp-1",
	)
	require.False(t,
		second.checkpoint ==
			nil || second.
			checkpoint.
			ID !=
			"cp-1")
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
	require.Equal(t,
		`{"cursor":42}`, headers.
			Get("X-Last-Checkpoint"))
	require.Equal(t,
		cpTime.Format(time.
			RFC3339), headers.
			Get("X-Checkpoint-At"))
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
		require.Fail(t,

			"should not call GetLatestCheckpoint on first attempt")
		return nil, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)
	require.Empty(t,
		headers.Get("X-Last-Checkpoint"))
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
	require.Empty(t,
		headers.Get("X-Last-Checkpoint"))
	require.Equal(t,
		domain.StatusCompleted,
		run.Status)
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
		require.Failf(t, "test failure",

			"tracedDispatch: %v", err)
	}
	require.Equal(t,
		`{"cursor":42}`, headers.
			Get("X-Last-Checkpoint"))
	require.Equal(t,
		cpTime.Format(time.
			RFC3339), headers.
			Get("X-Checkpoint-At"))
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
	require.Equal(t,
		"connection timeout",
		headers.Get("X-Previous-Error"))
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
	require.Empty(t,
		headers.Get("X-Previous-Error"),
	)
}

func TestHTTPDispatchTraceRecorderExecutionTrace(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	recorder := newHTTPDispatchTraceRecorder(start)
	recorder.recordConnectStart(start.Add(10 * time.Millisecond))
	recorder.recordConnectDone(start.Add(20 * time.Millisecond))
	recorder.recordFirstByte(start.Add(35 * time.Millisecond))

	trace := recorder.executionTrace(start.Add(50 * time.Millisecond))
	require.EqualValues(t, 10, trace.ConnectMs)
	require.EqualValues(t, 15, trace.TtfbMs)
	require.EqualValues(t, 15, trace.TransferMs)
	require.EqualValues(t, 40, trace.DispatchMs)
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
				require.ErrorAs(t,
					err, &endpointErr)
				require.Equal(t,
					tt.statusCode, endpointErr.
						StatusCode,
				)
				require.Equal(t,
					tt.body, endpointErr.
						Body)

				return
			}
			require.NoError(
				t, err)

			if tt.want == "" {
				require.Nil(t, got)

				return
			}
			require.Equal(t,
				tt.want, string(got))
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
			require.Equal(t,
				tt.want, got)
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
	require.NoError(
		t, err)

	mu.Lock()
	defer mu.Unlock()

	got := capturedHeaders.Get("Traceparent")
	assert.Equal(t,
		"00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",

		got)
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
	require.NoError(
		t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t,
		"00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",

		capturedHeaders.
			Get("Traceparent"))
	assert.Equal(t,
		"congo=t61rcWkgMzE",
		capturedHeaders.
			Get("Tracestate"))
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
	require.NoError(
		t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t,
		"0123456789abcdef0123456789abcdef-0123456789abcdef-1",

		capturedHeaders.
			Get(sentry.SentryTraceHeader))
	require.Equal(t,
		"sentry-release=test-release,sentry-public_key=public",

		capturedHeaders.
			Get(sentry.SentryBaggageHeader))
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
	require.NoError(
		t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t,
		capturedHeaders.
			Get("Traceparent"))
	assert.Empty(t,
		capturedHeaders.
			Get("Tracestate"))
	assert.Empty(t,
		capturedHeaders.
			Get(sentry.SentryTraceHeader))
	assert.Empty(t,
		capturedHeaders.
			Get(sentry.SentryBaggageHeader))
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
	require.NoError(
		t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t,
		capturedHeaders.
			Get("Traceparent"))
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
	require.NoError(
		t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t,
		capturedHeaders.
			Get("Traceparent"))
	assert.Empty(t,
		capturedHeaders.
			Get("Tracestate"))
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
	require.NoError(
		t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t,
		"00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",

		capturedHeaders.
			Get("Traceparent"))
	assert.Equal(t,
		"custom-value", capturedHeaders.
			Get(
				"X-Custom-Header",
			))
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
	require.NoError(
		t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t,
		capturedHeaders.
			Get("Secret"))

	if _, ok := capturedHeaders["Secret"]; ok {
		assert.Fail(t,

			"non-trace metadata 'secret' should not appear as a request header")
	}
	assert.Equal(t,
		"00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",

		capturedHeaders.
			Get("Traceparent"))
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
	require.Error(t,
		err)
	require.ErrorIs(t,
		err, rootErr)

	got := err.Error()
	for _, leaked := range []string{"hooks.example.com", "user:pass", "/private/path", "token=secret", "#frag"} {
		require.NotContains(t,
			got, leaked)
	}
	require.False(t,
		!strings.Contains(
			got, "http dispatch:",
		) || !strings.Contains(got,
			"context deadline exceeded",
		),
	)
}
