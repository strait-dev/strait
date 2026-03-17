package strait

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestIntegration_RegisterTriggerWait(t *testing.T) {
	callCount := 0
	mu := sync.Mutex{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()

		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/jobs":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["slug"] != "test-job" {
				t.Errorf("expected slug 'test-job', got %v", body["slug"])
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"job_abc","slug":"test-job"}`))

		case r.Method == "POST" && r.URL.Path == "/v1/jobs/job_abc/trigger":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"run_123","status":"queued"}`))

		case r.Method == "GET" && r.URL.Path == "/v1/runs/run_123":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"run_123","status":"completed"}`))

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		}
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithBearerToken("test-token"),
	)

	// Register
	var createResult map[string]any
	err := client.DoRequest(context.Background(), "POST", "/v1/jobs",
		nil, nil, map[string]any{"slug": "test-job", "name": "Test Job", "project_id": "proj_1", "endpoint_url": "https://worker.dev"},
		&createResult)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if createResult["id"] != "job_abc" {
		t.Errorf("expected job_abc, got %v", createResult["id"])
	}

	// Trigger
	var triggerResult map[string]any
	err = client.DoRequest(context.Background(), "POST", "/v1/jobs/job_abc/trigger",
		nil, nil, map[string]any{"payload": map[string]any{"key": "val"}}, &triggerResult)
	if err != nil {
		t.Fatalf("trigger failed: %v", err)
	}
	if triggerResult["id"] != "run_123" {
		t.Errorf("expected run_123, got %v", triggerResult["id"])
	}

	// Get run
	var runResult map[string]any
	err = client.DoRequest(context.Background(), "GET", "/v1/runs/run_123",
		nil, nil, nil, &runResult)
	if err != nil {
		t.Fatalf("get run failed: %v", err)
	}
	if runResult["status"] != "completed" {
		t.Errorf("expected completed, got %v", runResult["status"])
	}

	mu.Lock()
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
	mu.Unlock()
}

func TestIntegration_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.DoRequest(ctx, "GET", "/v1/health", nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	var te *TransportError
	if !errors.As(err, &te) {
		t.Errorf("expected TransportError, got %T", err)
	}
}

func TestIntegration_MiddlewareThroughClient(t *testing.T) {
	var capturedPaths []string
	var mu sync.Mutex

	mw := Middleware{
		OnRequest: func(ctx MiddlewareRequestContext) {
			mu.Lock()
			capturedPaths = append(capturedPaths, ctx.URL)
			mu.Unlock()
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithBearerToken("test-token"),
		WithMiddleware(mw),
	)

	_ = client.DoRequest(context.Background(), "GET", "/v1/health", nil, nil, nil, nil)
	_ = client.DoRequest(context.Background(), "GET", "/v1/jobs", nil, nil, nil, nil)

	mu.Lock()
	defer mu.Unlock()
	if len(capturedPaths) != 2 {
		t.Errorf("expected 2 captured paths, got %d", len(capturedPaths))
	}
}

func TestIntegration_ErrorResponseParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"job already exists","code":"CONFLICT"}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))

	err := client.DoRequest(context.Background(), "POST", "/v1/jobs", nil, nil, map[string]any{"slug": "test"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}

	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Errorf("expected ConflictError, got %T: %v", err, err)
	}
	if ce.Message != "job already exists" {
		t.Errorf("expected parsed error message, got %q", ce.Message)
	}
}

func TestIntegration_429RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))
	err := client.DoRequest(context.Background(), "POST", "/v1/jobs/j1/trigger", nil, nil, nil, nil)

	var rle *RateLimitedError
	if !errors.As(err, &rle) {
		t.Errorf("expected RateLimitedError, got %T", err)
	}
}

func TestIntegration_CustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Project-ID") != "proj_123" {
			t.Errorf("expected X-Project-ID header")
		}
		if r.Header.Get("Idempotency-Key") != "idem_456" {
			t.Errorf("expected Idempotency-Key header")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"run_1"}`))
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithBearerToken("test-token"),
		WithDefaultHeaders(map[string]string{"X-Project-ID": "proj_123"}),
	)

	err := client.DoRequest(context.Background(), "POST", "/v1/jobs/j1/trigger",
		nil, map[string]string{"Idempotency-Key": "idem_456"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIntegration_AllAuthTypes(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
	}{
		{"Bearer", WithBearerToken("tok_123")},
		{"APIKey", WithAPIKey("sk_live_abc")},
		{"RunToken", WithRunToken("rt_xyz")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth := r.Header.Get("Authorization")
				if auth == "" {
					t.Error("expected Authorization header")
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			client := NewClient(WithBaseURL(server.URL), tt.opt)
			err := client.DoRequest(context.Background(), "GET", "/v1/health", nil, nil, nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestIntegration_MiddlewareOnError(t *testing.T) {
	var errorCaptured bool
	mw := Middleware{
		OnError: func(_ MiddlewareErrorContext) {
			errorCaptured = true
		},
	}

	client := NewClient(
		WithBaseURL("http://localhost:1"),
		WithBearerToken("test-token"),
		WithMiddleware(mw),
	)

	_ = client.DoRequest(context.Background(), "GET", "/v1/health", nil, nil, nil, nil)
	if !errorCaptured {
		t.Error("expected error to be captured by middleware")
	}
}

func TestIntegration_LargeResponsePayload(t *testing.T) {
	items := make([]map[string]string, 100)
	for i := range items {
		items[i] = map[string]string{"id": "job_" + string(rune('0'+i%10))}
	}
	respBytes, _ := json.Marshal(map[string]any{"data": items})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBytes)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))
	var result map[string]any
	err := client.DoRequest(context.Background(), "GET", "/v1/jobs", nil, nil, nil, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := result["data"].([]any)
	if !ok || len(data) != 100 {
		t.Errorf("expected 100 items, got %d", len(data))
	}
}

func TestIntegration_EmptyBody204(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))
	err := client.DoRequest(context.Background(), "DELETE", "/v1/jobs/j1", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
