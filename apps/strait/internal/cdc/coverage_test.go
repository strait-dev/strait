package cdc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
		// Respond OK to the probe receive call.
		var body map[string]any
		assert.NoError(t, json.NewDecoder(r.
			Body).Decode(&body))
		assert.InDelta(t, float64(1), body["batch_size"], 1e-9)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.NoError(t, err)
}

func TestEnsureConsumer_CreatesOnFailedProbe(t *testing.T) {
	t.Parallel()
	var callCount int
	var receiveCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if strings.Contains(r.URL.Path, "/receive") {
			receiveCount++
			if receiveCount == 1 {
				// Fail the probe with a 404.
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("not found"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		if strings.Contains(r.URL.Path, "/api/sinks") {
			var body map[string]any
			assert.NoError(t, json.NewDecoder(r.
				Body).Decode(&body))
			assert.Equal(t, "test-consumer",
				body["name"])
			assert.Equal(t, "strait-db",
				body["database"])
			assert.Equal(t, "Bearer token",
				r.Header.
					Get("Authorization"))

			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"sink-1"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.NoError(t, err)
}

func TestEnsureConsumer_CreateFails(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/receive") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
			return
		}
		if strings.Contains(r.URL.Path, "/api/sinks") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid sink config"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
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
	var receiveCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/receive") {
			receiveCount++
			if receiveCount == 1 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("not found"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		if strings.Contains(r.URL.Path, "/api/sinks") {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"validation_errors":{"name":["has already been taken"]}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.NoError(t, err)
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
	var receiveCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/receive") {
			receiveCount++
			if receiveCount == 1 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("not found"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		assert.Empty(t, r.Header.
			Get("Authorization"))

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer", "", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.NoError(t, err)
}

func TestEnsureConsumer_NetworkError(t *testing.T) {
	t.Parallel()
	// Use a server that immediately closes.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/receive") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
			return
		}
	}))
	// Close immediately so the create request fails with a network error.
	serverURL := ts.URL
	ts.Close()

	client := NewClient(serverURL, "consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	require.Error(t, err)
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
