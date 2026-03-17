package strait

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestMiddleware_OnRequest(t *testing.T) {
	var captured MiddlewareRequestContext
	mw := Middleware{
		OnRequest: func(ctx MiddlewareRequestContext) {
			captured = ctx
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

	_ = client.doRequest(t.Context(), RequestOptions{Method: "GET", Path: "/v1/health"}, nil)

	if captured.Method != "GET" {
		t.Errorf("expected method GET, got %q", captured.Method)
	}
	if captured.URL == "" {
		t.Error("expected URL to be captured")
	}
}

func TestMiddleware_OnResponse(t *testing.T) {
	var captured MiddlewareResponseContext
	mw := Middleware{
		OnResponse: func(ctx MiddlewareResponseContext) {
			captured = ctx
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

	_ = client.doRequest(t.Context(), RequestOptions{Method: "GET", Path: "/v1/health"}, nil)

	if captured.Status != 200 {
		t.Errorf("expected status 200, got %d", captured.Status)
	}
	if captured.DurationMs < 0 {
		t.Error("expected non-negative duration")
	}
}

func TestMiddleware_OnError(t *testing.T) {
	var captured MiddlewareErrorContext
	mw := Middleware{
		OnError: func(ctx MiddlewareErrorContext) {
			captured = ctx
		},
	}

	client := NewClient(
		WithBaseURL("http://localhost:1"),
		WithBearerToken("test-token"),
		WithMiddleware(mw),
	)

	_ = client.doRequest(t.Context(), RequestOptions{Method: "GET", Path: "/v1/health"}, nil)

	if captured.Err == nil {
		t.Error("expected error to be captured")
	}
}

func TestMiddleware_MultipleHooksExecuteInOrder(t *testing.T) {
	var mu sync.Mutex
	var order []string

	mw1 := Middleware{
		OnRequest: func(_ MiddlewareRequestContext) {
			mu.Lock()
			order = append(order, "mw1")
			mu.Unlock()
		},
	}
	mw2 := Middleware{
		OnRequest: func(_ MiddlewareRequestContext) {
			mu.Lock()
			order = append(order, "mw2")
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
		WithMiddleware(mw1, mw2),
	)

	_ = client.doRequest(t.Context(), RequestOptions{Method: "GET", Path: "/v1/health"}, nil)

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "mw1" || order[1] != "mw2" {
		t.Errorf("expected [mw1, mw2], got %v", order)
	}
}

func TestMiddleware_NoMiddleware(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))
	err := client.doRequest(t.Context(), RequestOptions{Method: "GET", Path: "/v1/health"}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrapTransportWithMiddleware_NoMiddleware(t *testing.T) {
	base := http.DefaultTransport
	result := wrapTransportWithMiddleware(base, nil)
	if result != base {
		t.Error("expected same transport when no middleware")
	}
}
