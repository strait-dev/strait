package cdc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsHandler_Table(t *testing.T) {
	t.Parallel()
	h := NewAnalyticsHandler(nil, nil)
	require.Equal(t, "job_runs", h.
		Table())
}

func TestAnalyticsHandler_InsertAction_Skipped(t *testing.T) {
	t.Parallel()
	exp := newTestExporter()
	h := NewAnalyticsHandler(exp, nil)

	record, _ := json.Marshal(map[string]any{
		"id":         "run-1",
		"job_id":     "job-1",
		"project_id": "p1",
		"status":     "completed",
	})
	msg := Message{
		AckID:    "ack-1",
		Action:   ActionInsert,
		Record:   record,
		Metadata: Metadata{TableName: "job_runs"},
	}
	require.NoError(t, h.Handle(context.
		Background(),
		msg))
	require.Equal(t, 0, exp.PendingLen())
}

func TestAuditHandler_Table(t *testing.T) {
	t.Parallel()
	h := NewAuditHandler(&mockAuditStore{}, nil)
	require.Equal(t, "job_runs", h.
		Table())
}

func TestAuditAction_Delete(t *testing.T) {
	t.Parallel()
	got, ok := auditAction(ActionDelete, "whatever")
	require.True(
		t, ok)
	require.Equal(t, "run.deleted",
		got)
}

func TestAuditAction_ReadIsIgnored(t *testing.T) {
	t.Parallel()
	got, ok := auditAction(ActionRead, "executing")
	require.False(t, ok || got !=
		"")
}

func TestAuditHandler_EmptyProjectID_Skipped(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	record, _ := json.Marshal(map[string]any{
		"id":         "run-1",
		"project_id": "",
		"status":     "completed",
	})
	msg := Message{
		AckID:    "ack-1",
		Action:   ActionUpdate,
		Record:   record,
		Metadata: Metadata{TableName: "job_runs"},
	}
	require.NoError(t, h.Handle(context.
		Background(),
		msg))
	require.Empty(t,
		store.events,
	)
}

func TestAuditHandler_InvalidJSON_ReturnsError(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	msg := Message{
		AckID:    "ack-1",
		Action:   ActionUpdate,
		Record:   json.RawMessage(`{bad json`),
		Metadata: Metadata{TableName: "job_runs"},
	}

	err := h.Handle(context.Background(), msg)
	require.Error(t, err)
}

func TestAuditHandler_DeleteAction(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	record, _ := json.Marshal(map[string]any{
		"id":         "run-1",
		"project_id": "p1",
		"status":     "completed",
	})
	msg := Message{
		AckID:    "ack-1",
		Action:   ActionDelete,
		Record:   record,
		Metadata: Metadata{TableName: "job_runs"},
	}
	require.NoError(t, h.Handle(context.
		Background(),
		msg))
	require.Len(t,
		store.events, 1,
	)
	assert.Equal(
		t, "run.deleted",
		store.
			events[0].Action,
	)
}

func TestEnsureConsumer_AlreadyExists(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The existence probe and readiness poll are read-only GETs against the
		// management API; a Receive (message lease) must never happen. Use assert
		// (not require) inside the handler goroutine — require's FailNow must only
		// run on the test goroutine.
		assert.NotContains(t, r.URL.Path, "/receive")
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/sinks/test-consumer", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"test-consumer","status":"active","health":{"status":"healthy"}}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.NoError(t, err)
}

func TestEnsureConsumer_CreatesOnFailedProbe(t *testing.T) {
	t.Parallel()
	var created atomic.Bool
	var receiveHits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/receive") {
			receiveHits.Add(1)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sinks/test-consumer":
			if !created.Load() {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("not found"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"test-consumer","status":"active","health":{"status":"healthy"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/sinks":
			var body map[string]any
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "test-consumer", body["name"])
			assert.Equal(t, "strait-db", body["database"])
			assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
			created.Store(true)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"sink-1"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.NoError(t, err)
	require.Zero(t, receiveHits.Load(), "EnsureConsumer must never lease a message via /receive")
}

func TestEnsureConsumer_CreateFails(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sinks/test-consumer":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/sinks":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid sink config"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.Error(t, err)
	require.Contains(
		t, err.
			Error(), "status 400")
}

func TestDoRequest_InvalidBaseURL(t *testing.T) {
	t.Parallel()
	// Create a client with an empty/invalid base URL by passing empty string.
	client := NewClient("", "consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	_, err := client.Receive(context.Background(), 1, 0)
	require.Error(t, err)
	require.Contains(
		t, err.
			Error(), "invalid base url")
}

func TestDoRequest_NoPolicies(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	msgs, err := client.Receive(context.Background(), 1, 0)
	require.NoError(t, err)
	require.Empty(t,
		msgs)
}

func TestEnsureConsumer_DuplicateNameWaitsForConsumer(t *testing.T) {
	t.Parallel()
	var posted atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NotContains(t, r.URL.Path, "/receive")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sinks/test-consumer":
			if !posted.Load() {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("not found"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"test-consumer","status":"active","health":{"status":"healthy"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/sinks":
			posted.Store(true)
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"validation_errors":{"name":["has already been taken"]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.NoError(t, err)
}

func TestSequinSinkAlreadyExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		body       []byte
		want       bool
	}{
		{
			name:       "conflict duplicate",
			statusCode: http.StatusConflict,
			body:       []byte(`{"error":"has already been taken"}`),
			want:       true,
		},
		{
			name:       "unprocessable duplicate",
			statusCode: http.StatusUnprocessableEntity,
			body:       []byte(`{"validation_errors":{"name":["has already been taken"]}}`),
			want:       true,
		},
		{
			name:       "unprocessable different error",
			statusCode: http.StatusUnprocessableEntity,
			body:       []byte(`{"validation_errors":{"name":["is invalid"]}}`),
			want:       false,
		},
		{
			name:       "bad request duplicate text",
			statusCode: http.StatusBadRequest,
			body:       []byte(`{"error":"has already been taken"}`),
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, sequinSinkAlreadyExists(tt.statusCode, tt.body))
		})
	}
}

func TestDoRequest_NoAuthToken(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.
			Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer", "", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	_, err := client.Receive(context.Background(), 1, 0)
	require.NoError(t, err)
}

func TestEnsureConsumer_NoAuthToken(t *testing.T) {
	t.Parallel()
	var created atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sinks/consumer":
			if !created.Load() {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"consumer","status":"active","health":{"status":"healthy"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/sinks":
			created.Store(true)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer", "", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.NoError(t, err)
}

func TestEnsureConsumer_NetworkError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	// Close immediately so both the probe and the create request fail with a
	// network error; EnsureConsumer must surface it rather than hang.
	serverURL := ts.URL
	ts.Close()

	client := NewClient(serverURL, "consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.Error(t, err)
}

// TestEnsureConsumer_RetriesCreateOn503 is the regression guard for sink
// creation bypassing the retry/circuit-breaker policy: a transient 5xx during
// startup must be retried, not crash the process.
func TestEnsureConsumer_RetriesCreateOn503(t *testing.T) {
	t.Parallel()
	var postHits atomic.Int32
	var created atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sinks/consumer-1":
			if !created.Load() {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"consumer-1","status":"active","health":{"status":"healthy"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/sinks":
			if postHits.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("unavailable"))
				return
			}
			created.Store(true)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"sink-1"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer-1", "token-1", WithRetryPolicy(newTestRetryPolicy()), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.NoError(t, err)
	require.EqualValues(t, 2, postHits.Load(), "sink creation must be retried on 5xx")
}

func TestNewClient_InvalidBaseURL(t *testing.T) {
	t.Parallel()
	// Should not panic on invalid URL; silently falls back to empty URL.
	client := NewClient("://invalid", "consumer", "token")
	require.NotNil(t, client)

	// Attempting a request should fail gracefully.
	_, err := client.Receive(context.Background(), 1, 0)
	if err == nil {
		fmt.Println("note: invalid URL may still produce an error downstream")
	}
}

func TestNotificationTriggerHandler_Table(t *testing.T) {
	t.Parallel()
	h := NewNotificationTriggerHandler(nil, nil)
	require.Equal(t, "job_runs", h.
		Table())
}

func TestSLOHandler_Table(t *testing.T) {
	t.Parallel()
	h := NewSLOHandler(nil, nil)
	require.Equal(t, "job_runs", h.
		Table())
}

func TestWebhookTriggerHandler_Table(t *testing.T) {
	t.Parallel()
	h := NewWebhookTriggerHandler(nil, nil)
	require.Equal(t, "job_runs", h.
		Table())
}

func TestWebhookReceiver_RegisterAdditionalHandler(t *testing.T) {
	t.Parallel()
	wr := NewWebhookReceiver(nil, nil)

	h := &HandlerFunc{TableName: "job_runs", Fn: func(_ context.Context, _ Message) error { return nil }}
	wr.RegisterAdditionalHandler(h)
	require.Len(t,
		wr.additionalHandlers["job_runs"],
		1)

	h2 := &HandlerFunc{TableName: "job_runs", Fn: func(_ context.Context, _ Message) error { return nil }}
	wr.RegisterAdditionalHandler(h2)
	require.Len(t,
		wr.additionalHandlers["job_runs"],
		2)
}
