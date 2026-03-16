package strait

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoRequest_GET_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/jobs" {
			t.Errorf("expected /v1/jobs, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected auth header, got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"job_1","name":"test"}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))

	var result map[string]string
	err := client.doRequest(context.Background(), RequestOptions{
		Method: "GET",
		Path:   "/v1/jobs",
	}, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "job_1" {
		t.Errorf("expected id 'job_1', got %q", result["id"])
	}
}

func TestDoRequest_POST_WithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if body["name"] != "test-job" {
			t.Errorf("expected name 'test-job', got %q", body["name"])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"job_2"}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))

	var result map[string]string
	err := client.doRequest(context.Background(), RequestOptions{
		Method: "POST",
		Path:   "/v1/jobs",
		Body:   map[string]string{"name": "test-job"},
	}, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "job_2" {
		t.Errorf("expected id 'job_2', got %q", result["id"])
	}
}

func TestDoRequest_QueryParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("expected limit=10, got %q", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("cursor") != "abc" {
			t.Errorf("expected cursor=abc, got %q", r.URL.Query().Get("cursor"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))
	err := client.doRequest(context.Background(), RequestOptions{
		Method: "GET",
		Path:   "/v1/runs",
		Query:  map[string]string{"limit": "10", "cursor": "abc"},
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoRequest_DefaultHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			t.Errorf("expected X-Custom header, got %q", r.Header.Get("X-Custom"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithBearerToken("test-token"),
		WithDefaultHeaders(map[string]string{"X-Custom": "value"}),
	)
	err := client.doRequest(context.Background(), RequestOptions{
		Method: "GET",
		Path:   "/v1/health",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoRequest_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))
	err := client.doRequest(context.Background(), RequestOptions{
		Method: "GET",
		Path:   "/v1/jobs/missing",
	}, nil)

	if err == nil {
		t.Fatal("expected error for 404")
	}
	var target *NotFoundError
	if !errors.As(err, &target) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestDoRequest_401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid token"}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("bad-token"))
	err := client.doRequest(context.Background(), RequestOptions{
		Method: "GET",
		Path:   "/v1/jobs",
	}, nil)

	var target *UnauthorizedError
	if !errors.As(err, &target) {
		t.Errorf("expected UnauthorizedError, got %T: %v", err, err)
	}
}

func TestDoRequest_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))
	var result map[string]any
	err := client.doRequest(context.Background(), RequestOptions{
		Method: "GET",
		Path:   "/v1/health",
	}, &result)

	var target *DecodeError
	if !errors.As(err, &target) {
		t.Errorf("expected DecodeError, got %T: %v", err, err)
	}
}

func TestDoRequest_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.doRequest(ctx, RequestOptions{
		Method: "GET",
		Path:   "/v1/health",
	}, nil)

	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	var target *TransportError
	if !errors.As(err, &target) {
		t.Errorf("expected TransportError, got %T: %v", err, err)
	}
}

func TestDoRequest_PerRequestHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "idem_123" {
			t.Errorf("expected idempotency key header, got %q", r.Header.Get("Idempotency-Key"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))
	err := client.doRequest(context.Background(), RequestOptions{
		Method:  "POST",
		Path:    "/v1/jobs",
		Headers: map[string]string{"Idempotency-Key": "idem_123"},
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoRequest_EmptyResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithBearerToken("test-token"))
	var result map[string]any
	err := client.doRequest(context.Background(), RequestOptions{
		Method: "DELETE",
		Path:   "/v1/jobs/job_1",
	}, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
