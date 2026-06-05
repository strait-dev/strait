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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientReceiveSuccess(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/http_pull_consumers/consumer-1/receive",

			r.URL.Path,
		)
		assert.Equal(t, http.MethodPost,
			r.Method,
		)
		assert.Equal(t, "application/json",
			r.Header.
				Get("Content-Type"))
		assert.Equal(t, "Bearer token-1",
			r.Header.
				Get("Authorization"))

		var reqBody map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		assert.InDelta(t, float64(2), reqBody["batch_size"], 1e-9)
		assert.InDelta(t, float64(1000),
			reqBody["wait_for"], 1e-9)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"ack_id":"a1","record":{"id":1},"action":"insert","metadata":{"table_schema":"public","table_name":"job_runs","commit_timestamp":"2025-01-01T00:00:00Z","idempotency_key":"key-1"}},{"ack_id":"a2","record":{"id":2},"action":"update","metadata":{"table_schema":"public","table_name":"jobs","commit_timestamp":"2025-01-01T00:00:01Z","idempotency_key":"key-2"}}]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-1", "token-1")
	messages, err := client.Receive(context.Background(), 2, 1000)
	require.NoError(t, err)
	assert.Len(t,
		messages, 2)
	assert.Equal(t, "a1", messages[0].AckID)
	assert.Equal(t, "consumer-1",
		messages[1].Metadata.
			ConsumerName)
}

func TestClientReceiveEmptyBatch(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/http_pull_consumers/consumer-2/receive",

			r.URL.Path,
		)
		assert.Equal(t, http.MethodPost,
			r.Method,
		)
		assert.Equal(t, "application/json",
			r.Header.
				Get("Content-Type"))
		assert.Equal(t, "Bearer token-2",
			r.Header.
				Get("Authorization"))

		var reqBody map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		assert.InDelta(t, float64(10),
			reqBody["batch_size"], 1e-9)

		if _, ok := reqBody["wait_for"]; ok {
			assert.Failf(t, "test failure",

				"wait_for should be omitted, got %v", reqBody["wait_for"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-2", "token-2")
	messages, err := client.Receive(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Empty(t,
		messages)
}

func TestClientReceiveServerError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/http_pull_consumers/consumer-err/receive",

			r.URL.Path,
		)
		assert.Equal(t, http.MethodPost,
			r.Method,
		)
		assert.Equal(t, "application/json",
			r.Header.
				Get("Content-Type"))
		assert.Equal(t, "Bearer token-err",
			r.Header.
				Get("Authorization"))

		var reqBody map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))

		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-err", "token-err")
	_, err := client.Receive(context.Background(), 1, 1)
	require.Error(t, err)
	assert.False(t, !strings.Contains(err.Error(), "status 500") || !strings.Contains(err.
		Error(), "boom",
	))
}

func TestClientReceiveInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/http_pull_consumers/consumer-json/receive",

			r.URL.
				Path)
		assert.Equal(t, http.MethodPost,
			r.Method,
		)
		assert.Equal(t, "application/json",
			r.Header.
				Get("Content-Type"))
		assert.Equal(t, "Bearer token-json",
			r.
				Header.Get("Authorization"))

		var reqBody map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))

		_, _ = w.Write([]byte("{"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-json", "token-json")
	_, err := client.Receive(context.Background(), 1, 1)
	require.Error(t, err)
	assert.Contains(
		t, err.Error(), "decode receive response")
}

func TestClientReceiveContextCancellation(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/http_pull_consumers/consumer-cancel/receive",

			r.URL.
				Path)
		assert.Equal(t, http.MethodPost,
			r.Method,
		)
		assert.Equal(t, "application/json",
			r.Header.
				Get("Content-Type"))
		assert.Equal(t, "Bearer token-cancel",

			r.Header.Get("Authorization"))

		var reqBody map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))

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
	require.Error(t, err)
	assert.ErrorIs(
		t, err, context.DeadlineExceeded)
}

func TestClientReceiveAuthHeaderSent(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer auth-token",
			r.
				Header.Get("Authorization"))

		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-auth", "auth-token")
	if _, err := client.Receive(context.Background(), 1, 1); err != nil {
		assert.Failf(t, "test failure",

			"Receive returned error: %v", err)
	}
}

func TestClientAckSuccess(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/http_pull_consumers/consumer-ack/ack",

			r.URL.Path)
		assert.Equal(t, http.MethodPost,
			r.Method,
		)
		assert.Equal(t, "application/json",
			r.Header.
				Get("Content-Type"))
		assert.Equal(t, "Bearer token-ack",
			r.Header.
				Get("Authorization"))

		var reqBody map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))

		ackIDs, ok := reqBody["ack_ids"].([]any)
		assert.False(t, !ok || len(ackIDs) != 2)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-ack", "token-ack")
	assert.NoError(t, client.Ack(
		context.Background(),
		[]string{"a1", "a2"}),
	)
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
	require.NoError(t, client.Ack(
		context.Background(),
		nil))
	assert.EqualValues(t, 0, calls.Load())
}

func TestClientAckServerError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/http_pull_consumers/consumer-ack-err/ack",

			r.URL.Path,
		)
		assert.Equal(t, http.MethodPost,
			r.Method,
		)
		assert.Equal(t, "application/json",
			r.Header.
				Get("Content-Type"))
		assert.Equal(t, "Bearer token-ack-err",

			r.Header.Get("Authorization"))

		var reqBody map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))

		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-ack-err", "token-ack-err")
	err := client.Ack(context.Background(), []string{"a1"})
	require.Error(t, err)
	assert.Contains(
		t, err.Error(), "status 502")
}

func TestClientAckAuthHeaderSent(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer ack-auth-token",

			r.Header.Get("Authorization"))

		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-ack-auth", "ack-auth-token")
	assert.NoError(t, client.Ack(
		context.Background(),
		[]string{"a1"}))
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
	require.NoError(t, client.Nack(context.Background(),
		[]string{"n1", "n2"},
	))

	select {
	case msg := <-handlerErr:
		assert.Failf(t, "test failure", "handler error: %s", msg)
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
	require.NoError(t, client.Nack(context.Background(),
		[]string{}))
	assert.EqualValues(t, 0, calls.Load())
}

func TestClientNackServerError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/http_pull_consumers/consumer-nack-err/nack",

			r.URL.
				Path)
		assert.Equal(t, http.MethodPost,
			r.Method,
		)
		assert.Equal(t, "application/json",
			r.Header.
				Get("Content-Type"))
		assert.Equal(t, "Bearer token-nack-err",

			r.Header.Get("Authorization"))

		var reqBody map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))

		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-nack-err", "token-nack-err")
	err := client.Nack(context.Background(), []string{"n1"})
	require.Error(t, err)
	assert.Contains(
		t, err.Error(), "status 503")
}

func TestIsServerErrorOrNetworkFailure_Status500_True(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: 500}
	assert.True(
		t, isServerErrorOrNetworkFailure(resp,
			nil))
}

func TestIsServerErrorOrNetworkFailure_Status499_False(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: 499}
	assert.False(t, isServerErrorOrNetworkFailure(resp,
		nil))
}

func TestIsServerErrorOrNetworkFailure_Status501_True(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: 501}
	assert.True(
		t, isServerErrorOrNetworkFailure(resp,
			nil))
}

func TestIsServerErrorOrNetworkFailure_NetworkError_True(t *testing.T) {
	t.Parallel()
	assert.True(
		t, isServerErrorOrNetworkFailure(nil, errors.New("connection refused")),
	)
}

func TestIsServerErrorOrNetworkFailure_Status200_False(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: 200}
	assert.False(t, isServerErrorOrNetworkFailure(resp,
		nil))
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
	require.Error(t, err)
	assert.EqualValues(t, 3, hits.Load())

	// Default MaxRetries=2 means 1 initial + 2 retries = 3 total hits.
}

func TestNewClient_InvalidBaseURL_FallsBackToEmptyURL(t *testing.T) {
	t.Parallel()
	client := NewClient("://invalid", "consumer-1", "token")
	_, err := client.Receive(context.Background(), 1, 0)
	require.Error(t, err)
	assert.Contains(
		t, err.Error(), "invalid base url")
}

func TestNewClient_NoAuthToken_OmitsHeader(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.
			Get("Authorization"))

		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-1", "")
	_, err := client.Receive(context.Background(), 1, 0)
	assert.NoError(t, err)
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
				assert.Equal(t, "/api/sinks/consumer-1",

					r.URL.Path,
				)
				assert.Equal(t, http.MethodGet,
					r.Method,
				)
				assert.Equal(t, "Bearer token-1",
					r.Header.
						Get("Authorization"))

				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer ts.Close()

			client := NewClient(ts.URL, "consumer-1", "token-1")
			err := client.SinkConsumerHealth(context.Background())
			if tt.wantErrPart == "" {
				assert.NoError(t, err)

				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(),
				tt.wantErrPart,
			)
		})
	}
}

func TestClientSinkConsumerHealth_NoAuthTokenOmitsHeader(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.
			Get("Authorization"))

		_, _ = w.Write([]byte(`{"name":"consumer-1","status":"active","health":{"status":"healthy"}}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-1", "")
	assert.NoError(t, client.SinkConsumerHealth(context.
		Background()))
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
	require.NoError(t, err)
	assert.Len(t,
		messages, 1)
	assert.EqualValues(t, 2, hits.Load())
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
	require.Error(t, err)
	assert.Contains(
		t, err.Error(), "status 400")
	assert.EqualValues(t, 1, hits.Load())
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
	require.Error(t, err)
	assert.Contains(
		t, err.Error(), "status 503")
	assert.EqualValues(t, 3, hits.Load())
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
	require.NoError(t, err)
	assert.EqualValues(t, 2, hits.Load())
}
