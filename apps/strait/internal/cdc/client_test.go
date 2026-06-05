package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestClientReceiveSuccess(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http_pull_consumers/consumer-1/receive" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/http_pull_consumers/consumer-1/receive")
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}
		if r.Header.Get("Authorization") != "Bearer token-1" {
			t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer token-1")
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if reqBody["batch_size"] != float64(2) {
			t.Fatalf("batch_size = %v, want 2", reqBody["batch_size"])
		}
		if reqBody["wait_for"] != float64(1000) {
			t.Fatalf("wait_for = %v, want 1000", reqBody["wait_for"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"insert","metadata":{"table_schema":"public","table_name":"job_runs","commit_timestamp":"2025-01-01T00:00:00Z","idempotency_key":"key-1"}},{"ack_id":"a2","record":{"id":2},"action":"update","metadata":{"table_schema":"public","table_name":"jobs","commit_timestamp":"2025-01-01T00:00:01Z","idempotency_key":"key-2"}}]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-1", "token-1")
	messages, err := client.Receive(context.Background(), 2, 1000)
	if err != nil {
		t.Fatalf("Receive returned error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
	if messages[0].AckID != "a1" {
		t.Fatalf("messages[0].AckID = %q, want %q", messages[0].AckID, "a1")
	}
	if messages[1].Metadata.ConsumerName != "consumer-1" {
		t.Fatalf("ConsumerName = %q, want %q", messages[1].Metadata.ConsumerName, "consumer-1")
	}
}

func TestClientReceiveEmptyBatch(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http_pull_consumers/consumer-2/receive" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/http_pull_consumers/consumer-2/receive")
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}
		if r.Header.Get("Authorization") != "Bearer token-2" {
			t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer token-2")
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if reqBody["batch_size"] != float64(10) {
			t.Fatalf("batch_size = %v, want 10", reqBody["batch_size"])
		}
		if _, ok := reqBody["wait_for"]; ok {
			t.Fatalf("wait_for should be omitted, got %v", reqBody["wait_for"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-2", "token-2")
	messages, err := client.Receive(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("Receive returned error: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("len(messages) = %d, want 0", len(messages))
	}
}

func TestClientReceiveServerError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http_pull_consumers/consumer-err/receive" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/http_pull_consumers/consumer-err/receive")
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}
		if r.Header.Get("Authorization") != "Bearer token-err" {
			t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer token-err")
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-err", "token-err")
	_, err := client.Receive(context.Background(), 1, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 500") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %q, want status and body", err.Error())
	}
}

func TestClientReceiveInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http_pull_consumers/consumer-json/receive" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/http_pull_consumers/consumer-json/receive")
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}
		if r.Header.Get("Authorization") != "Bearer token-json" {
			t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer token-json")
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_, _ = w.Write([]byte("{"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-json", "token-json")
	_, err := client.Receive(context.Background(), 1, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decode receive response") {
		t.Fatalf("error = %q, want decode receive response", err.Error())
	}
}

func TestClientReceiveContextCancellation(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http_pull_consumers/consumer-cancel/receive" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/http_pull_consumers/consumer-cancel/receive")
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}
		if r.Header.Get("Authorization") != "Bearer token-cancel" {
			t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer token-cancel")
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		select {
		case <-time.After(300 * time.Millisecond):
			_, _ = w.Write([]byte(`{"data":[]}`))
		case <-r.Context().Done():
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-cancel", "token-cancel")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Receive(ctx, 1, 1000)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
}

func TestClientReceiveAuthHeaderSent(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer auth-token" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer auth-token")
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-auth", "auth-token")
	if _, err := client.Receive(context.Background(), 1, 1); err != nil {
		t.Fatalf("Receive returned error: %v", err)
	}
}

func TestClientAckSuccess(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http_pull_consumers/consumer-ack/ack" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/http_pull_consumers/consumer-ack/ack")
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}
		if r.Header.Get("Authorization") != "Bearer token-ack" {
			t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer token-ack")
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		ackIDs, ok := reqBody["ack_ids"].([]any)
		if !ok || len(ackIDs) != 2 {
			t.Fatalf("ack_ids = %v, want two ids", reqBody["ack_ids"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-ack", "token-ack")
	if err := client.Ack(context.Background(), []string{"a1", "a2"}); err != nil {
		t.Fatalf("Ack returned error: %v", err)
	}
}

func TestClientAckEmptyIDsNoop(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-ack-noop", "token")
	if err := client.Ack(context.Background(), nil); err != nil {
		t.Fatalf("Ack returned error: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("server called %d times, want 0", calls.Load())
	}
}

func TestClientAckServerError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http_pull_consumers/consumer-ack-err/ack" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/http_pull_consumers/consumer-ack-err/ack")
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}
		if r.Header.Get("Authorization") != "Bearer token-ack-err" {
			t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer token-ack-err")
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-ack-err", "token-ack-err")
	err := client.Ack(context.Background(), []string{"a1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("error = %q, want status 502", err.Error())
	}
}

func TestClientAckAuthHeaderSent(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ack-auth-token" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer ack-auth-token")
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-ack-auth", "ack-auth-token")
	if err := client.Ack(context.Background(), []string{"a1"}); err != nil {
		t.Fatalf("Ack returned error: %v", err)
	}
}

func TestClientNackSuccess(t *testing.T) {
	t.Parallel()
	handlerErr := make(chan string, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http_pull_consumers/consumer-nack/nack" {
			handlerErr <- "path = " + r.URL.Path
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodPost {
			handlerErr <- "method = " + r.Method
			http.Error(w, "bad method", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			handlerErr <- "Content-Type = " + r.Header.Get("Content-Type")
			http.Error(w, "bad content-type", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer token-nack" {
			handlerErr <- "Authorization = " + r.Header.Get("Authorization")
			http.Error(w, "bad auth", http.StatusBadRequest)
			return
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			handlerErr <- "decode: " + err.Error()
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		ackIDs, ok := reqBody["ack_ids"].([]any)
		if !ok || len(ackIDs) != 2 {
			handlerErr <- "bad ack_ids"
			http.Error(w, "bad ack_ids", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-nack", "token-nack")
	if err := client.Nack(context.Background(), []string{"n1", "n2"}); err != nil {
		t.Fatalf("Nack returned error: %v", err)
	}
	select {
	case msg := <-handlerErr:
		t.Fatalf("handler error: %s", msg)
	default:
	}
}

func TestClientNackEmptyIDsNoop(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-nack-noop", "token")
	if err := client.Nack(context.Background(), []string{}); err != nil {
		t.Fatalf("Nack returned error: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("server called %d times, want 0", calls.Load())
	}
}

func TestClientNackServerError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http_pull_consumers/consumer-nack-err/nack" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/http_pull_consumers/consumer-nack-err/nack")
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}
		if r.Header.Get("Authorization") != "Bearer token-nack-err" {
			t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer token-nack-err")
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-nack-err", "token-nack-err")
	err := client.Nack(context.Background(), []string{"n1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 503") {
		t.Fatalf("error = %q, want status 503", err.Error())
	}
}

func TestIsServerErrorOrNetworkFailure_Status500_True(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: 500}
	if !isServerErrorOrNetworkFailure(resp, nil) {
		t.Fatal("expected true for status 500")
	}
}

func TestIsServerErrorOrNetworkFailure_Status499_False(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: 499}
	if isServerErrorOrNetworkFailure(resp, nil) {
		t.Fatal("expected false for status 499")
	}
}

func TestIsServerErrorOrNetworkFailure_Status501_True(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: 501}
	if !isServerErrorOrNetworkFailure(resp, nil) {
		t.Fatal("expected true for status 501")
	}
}

func TestIsServerErrorOrNetworkFailure_NetworkError_True(t *testing.T) {
	t.Parallel()
	if !isServerErrorOrNetworkFailure(nil, errors.New("connection refused")) {
		t.Fatal("expected true for network error")
	}
}

func TestIsServerErrorOrNetworkFailure_Status200_False(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: 200}
	if isServerErrorOrNetworkFailure(resp, nil) {
		t.Fatal("expected false for status 200")
	}
}

func TestNewClient_DefaultRetryPolicy_RetriesTwice(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-default", "token")
	_, err := client.Receive(context.Background(), 1, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Default MaxRetries=2 means 1 initial + 2 retries = 3 total hits.
	if got := hits.Load(); got != 3 {
		t.Fatalf("hits = %d, want 3 (1 initial + 2 retries)", got)
	}
}

func TestNewClient_InvalidBaseURL_FallsBackToEmptyURL(t *testing.T) {
	t.Parallel()
	client := NewClient("://invalid", "consumer-1", "token")
	_, err := client.Receive(context.Background(), 1, 0)
	if err == nil {
		t.Fatal("expected error for invalid base URL")
	}
	if !strings.Contains(err.Error(), "invalid base url") {
		t.Fatalf("error = %q, want 'invalid base url'", err.Error())
	}
}

func TestNewClient_NoAuthToken_OmitsHeader(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty", got)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-1", "")
	_, err := client.Receive(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("Receive returned error: %v", err)
	}
}

func TestClientSinkConsumerHealth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		statusCode  int
		body        string
		wantErrPart string
	}{
		{
			name:       "active healthy sink",
			statusCode: http.StatusOK,
			body:       `{"name":"consumer-1","status":"active","health":{"status":"healthy"}}`,
		},
		{
			name:       "active waiting sink",
			statusCode: http.StatusOK,
			body:       `{"name":"consumer-1","status":"active","health":{"status":"waiting"}}`,
		},
		{
			name:        "management api rejects token",
			statusCode:  http.StatusUnauthorized,
			body:        `bad token`,
			wantErrPart: "status 401",
		},
		{
			name:        "sink paused",
			statusCode:  http.StatusOK,
			body:        `{"name":"consumer-1","status":"paused","health":{"status":"healthy"}}`,
			wantErrPart: `is paused`,
		},
		{
			name:        "sink health unhealthy",
			statusCode:  http.StatusOK,
			body:        `{"name":"consumer-1","status":"active","health":{"status":"unhealthy"}}`,
			wantErrPart: `health is unhealthy`,
		},
		{
			name:        "unexpected sink name",
			statusCode:  http.StatusOK,
			body:        `{"name":"other-consumer","status":"active","health":{"status":"healthy"}}`,
			wantErrPart: `name mismatch`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/sinks/consumer-1" {
					t.Fatalf("path = %q, want /api/sinks/consumer-1", r.URL.Path)
				}
				if r.Method != http.MethodGet {
					t.Fatalf("method = %q, want GET", r.Method)
				}
				if r.Header.Get("Authorization") != "Bearer token-1" {
					t.Fatalf("Authorization = %q, want Bearer token-1", r.Header.Get("Authorization"))
				}
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer ts.Close()

			client := NewClient(ts.URL, "consumer-1", "token-1")
			err := client.SinkConsumerHealth(context.Background())
			if tt.wantErrPart == "" {
				if err != nil {
					t.Fatalf("SinkConsumerHealth returned error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErrPart) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErrPart)
			}
		})
	}
}

func TestClientSinkConsumerHealth_NoAuthTokenOmitsHeader(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty", got)
		}
		_, _ = w.Write([]byte(`{"name":"consumer-1","status":"active","health":{"status":"healthy"}}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-1", "")
	if err := client.SinkConsumerHealth(context.Background()); err != nil {
		t.Fatalf("SinkConsumerHealth returned error: %v", err)
	}
}

func newTestRetryPolicy() retrypolicy.RetryPolicy[*http.Response] {
	return retrypolicy.NewBuilder[*http.Response]().
		WithMaxRetries(2).
		WithBackoff(time.Millisecond, 10*time.Millisecond).
		HandleIf(func(resp *http.Response, err error) bool {
			if err != nil {
				return true
			}
			return resp.StatusCode >= 500
		}).
		ReturnLastFailure().
		Build()
}

func TestClient_Receive_RetriesOn503(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("unavailable"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"insert","metadata":{"table_schema":"public","table_name":"jobs","commit_timestamp":"2025-01-01T00:00:00Z","idempotency_key":"key-1"}}]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-1", "token-1", WithRetryPolicy(newTestRetryPolicy()), WithCircuitBreaker(nil))
	messages, err := client.Receive(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("Receive returned error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("hits = %d, want 2", got)
	}
}

func TestClient_Receive_NoRetryOn400(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-1", "token-1", WithRetryPolicy(newTestRetryPolicy()), WithCircuitBreaker(nil))
	_, err := client.Receive(context.Background(), 1, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("error = %q, want status 400", err.Error())
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1", got)
	}
}

func TestClient_Receive_ExhaustsRetries(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-1", "token-1", WithRetryPolicy(newTestRetryPolicy()), WithCircuitBreaker(nil))
	_, err := client.Receive(context.Background(), 1, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 503") {
		t.Fatalf("error = %q, want status 503", err.Error())
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("hits = %d, want 3", got)
	}
}

func TestClient_Ack_RetriesOn503(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("unavailable"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-1", "token-1", WithRetryPolicy(newTestRetryPolicy()), WithCircuitBreaker(nil))
	err := client.Ack(context.Background(), []string{"a1"})
	if err != nil {
		t.Fatalf("Ack returned error: %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("hits = %d, want 2", got)
	}
}
