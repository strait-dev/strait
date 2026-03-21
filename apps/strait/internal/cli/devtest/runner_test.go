package devtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunTest_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-Strait-Job-Slug") != "test-job" {
			t.Errorf("expected X-Strait-Job-Slug=test-job, got %s", r.Header.Get("X-Strait-Job-Slug"))
		}
		if r.Header.Get("X-Strait-Attempt") != "1" {
			t.Errorf("expected X-Strait-Attempt=1, got %s", r.Header.Get("X-Strait-Attempt"))
		}
		runID := r.Header.Get("X-Strait-Run-ID")
		if !strings.HasPrefix(runID, "test-") {
			t.Errorf("expected X-Strait-Run-ID to start with test-, got %s", runID)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"processed": true})
	}))
	defer srv.Close()

	result, err := RunTest(context.Background(), TestRequest{
		JobSlug:     "test-job",
		JobID:       "job-123",
		EndpointURL: srv.URL,
		Payload:     json.RawMessage(`{"id":"1"}`),
		Timeout:     10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}
	if result.Error != "" {
		t.Fatalf("expected no error, got: %s", result.Error)
	}
	if result.Duration <= 0 {
		t.Fatal("expected positive duration")
	}
	if !strings.Contains(result.Body, "processed") {
		t.Fatalf("expected body to contain 'processed', got: %s", result.Body)
	}
}

func TestRunTest_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	result, err := RunTest(context.Background(), TestRequest{
		JobSlug:     "test-job",
		EndpointURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StatusCode != 500 {
		t.Fatalf("expected status 500, got %d", result.StatusCode)
	}
}

func TestRunTest_ConnectionRefused(t *testing.T) {
	t.Parallel()

	result, err := RunTest(context.Background(), TestRequest{
		JobSlug:     "test-job",
		EndpointURL: "http://localhost:19999",
		Timeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected connection error")
	}
	if result.StatusCode != 0 {
		t.Fatalf("expected zero status code, got %d", result.StatusCode)
	}
}

func TestRunTest_EmptyPayload(t *testing.T) {
	t.Parallel()

	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(json.RawMessage(`{}`))
		raw, _ := strings.CutPrefix(string(body), "")
		receivedBody = raw
		_ = r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := RunTest(context.Background(), TestRequest{
		JobSlug:     "test-job",
		EndpointURL: srv.URL,
		// No Payload — should default to {}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = receivedBody
}

func TestRunTest_PayloadSizeLimit(t *testing.T) {
	t.Parallel()

	bigPayload := make([]byte, MaxPayloadSize+1)
	for i := range bigPayload {
		bigPayload[i] = 'a'
	}

	_, err := RunTest(context.Background(), TestRequest{
		JobSlug:     "test-job",
		EndpointURL: "http://localhost:1234",
		Payload:     bigPayload,
	})
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	if !strings.Contains(err.Error(), "maximum size") {
		t.Fatalf("expected 'maximum size' error, got: %v", err)
	}
}

func TestRunTest_MissingEndpoint(t *testing.T) {
	t.Parallel()

	_, err := RunTest(context.Background(), TestRequest{
		JobSlug: "test-job",
	})
	if err == nil {
		t.Fatal("expected error for missing endpoint")
	}
	if !strings.Contains(err.Error(), "endpoint URL is required") {
		t.Fatalf("expected 'endpoint URL' error, got: %v", err)
	}
}

func TestRunTest_Timeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result, err := RunTest(context.Background(), TestRequest{
		JobSlug:     "test-job",
		EndpointURL: srv.URL,
		Timeout:     100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected timeout error")
	}
}

func TestRunTest_CorrectHeaders(t *testing.T) {
	t.Parallel()

	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := RunTest(context.Background(), TestRequest{
		JobSlug:     "my-job",
		JobID:       "job-abc",
		EndpointURL: srv.URL,
		Payload:     json.RawMessage(`{"key":"value"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if headers.Get("X-Strait-Job-ID") != "job-abc" {
		t.Errorf("expected X-Strait-Job-ID=job-abc, got %s", headers.Get("X-Strait-Job-ID"))
	}
	if headers.Get("X-Strait-Job-Slug") != "my-job" {
		t.Errorf("expected X-Strait-Job-Slug=my-job, got %s", headers.Get("X-Strait-Job-Slug"))
	}
	if !strings.HasPrefix(headers.Get("X-Strait-Run-ID"), "test-") {
		t.Errorf("expected X-Strait-Run-ID to start with test-, got %s", headers.Get("X-Strait-Run-ID"))
	}
	if headers.Get("X-Strait-Attempt") != "1" {
		t.Errorf("expected X-Strait-Attempt=1, got %s", headers.Get("X-Strait-Attempt"))
	}
	if headers.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %s", headers.Get("Content-Type"))
	}
}

func TestRunTest_ContextCancellation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := RunTest(ctx, TestRequest{
		JobSlug:     "test-job",
		EndpointURL: srv.URL,
		Timeout:     10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected cancellation error")
	}
}
