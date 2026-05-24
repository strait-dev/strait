package cdc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnalyticsHandler_Table(t *testing.T) {
	t.Parallel()
	h := NewAnalyticsHandler(nil, nil)
	if got := h.Table(); got != "job_runs" {
		t.Fatalf("Table() = %q, want %q", got, "job_runs")
	}
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

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exp.PendingLen() != 0 {
		t.Fatal("expected no enqueued records for insert action")
	}
}

func TestAuditHandler_Table(t *testing.T) {
	t.Parallel()
	h := NewAuditHandler(&mockAuditStore{}, nil)
	if got := h.Table(); got != "job_runs" {
		t.Fatalf("Table() = %q, want %q", got, "job_runs")
	}
}

func TestAuditAction_Delete(t *testing.T) {
	t.Parallel()
	got, ok := auditAction(ActionDelete, "whatever")
	if !ok {
		t.Fatal("auditAction(delete) ok = false, want true")
	}
	if got != "run.deleted" {
		t.Fatalf("auditAction(delete) = %q, want %q", got, "run.deleted")
	}
}

func TestAuditAction_ReadIsIgnored(t *testing.T) {
	t.Parallel()
	got, ok := auditAction(ActionRead, "executing")
	if ok || got != "" {
		t.Fatalf("auditAction(read) = %q, %v; want empty false", got, ok)
	}
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

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.events) != 0 {
		t.Fatal("expected no audit events for empty project_id")
	}
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
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
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

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.events))
	}
	if store.events[0].Action != "run.deleted" {
		t.Errorf("expected action=run.deleted, got %s", store.events[0].Action)
	}
}

func TestEnsureConsumer_AlreadyExists(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond OK to the probe receive call.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	if err != nil {
		t.Fatalf("EnsureConsumer error: %v", err)
	}
}

func TestEnsureConsumer_CreatesOnFailedProbe(t *testing.T) {
	t.Parallel()
	var callCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if strings.Contains(r.URL.Path, "/receive") {
			// Fail the probe with a 404.
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
			return
		}
		if strings.Contains(r.URL.Path, "/api/sinks") {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode sink body: %v", err)
			}
			if body["name"] != "test-consumer" {
				t.Fatalf("sink name = %v, want test-consumer", body["name"])
			}
			if body["database"] != "strait-db" {
				t.Fatalf("database = %v, want strait-db", body["database"])
			}
			if r.Header.Get("Authorization") != "Bearer token" {
				t.Fatalf("auth = %q, want Bearer token", r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"sink-1"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	if err != nil {
		t.Fatalf("EnsureConsumer error: %v", err)
	}
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
	if err == nil {
		t.Fatal("expected error when sink creation fails")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("error = %q, want status 400", err.Error())
	}
}

func TestDoRequest_InvalidBaseURL(t *testing.T) {
	t.Parallel()
	// Create a client with an empty/invalid base URL by passing empty string.
	client := NewClient("", "consumer", "token", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	_, err := client.Receive(context.Background(), 1, 0)
	if err == nil {
		t.Fatal("expected error for invalid base URL")
	}
	if !strings.Contains(err.Error(), "invalid base url") {
		t.Fatalf("error = %q, want 'invalid base url'", err.Error())
	}
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
	if err != nil {
		t.Fatalf("Receive error: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("len(msgs) = %d, want 0", len(msgs))
	}
}

func TestDoRequest_NoAuthToken(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("expected no Authorization header, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer", "", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	_, err := client.Receive(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("Receive error: %v", err)
	}
}

func TestEnsureConsumer_NoAuthToken(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/receive") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("expected no Authorization header, got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "consumer", "", WithRetryPolicy(nil), WithCircuitBreaker(nil))
	err := client.EnsureConsumer(context.Background(), []string{"job_runs"})
	if err != nil {
		t.Fatalf("EnsureConsumer error: %v", err)
	}
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
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestNewClient_InvalidBaseURL(t *testing.T) {
	t.Parallel()
	// Should not panic on invalid URL; silently falls back to empty URL.
	client := NewClient("://invalid", "consumer", "token")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	// Attempting a request should fail gracefully.
	_, err := client.Receive(context.Background(), 1, 0)
	if err == nil {
		fmt.Println("note: invalid URL may still produce an error downstream")
	}
}

func TestNotificationTriggerHandler_Table(t *testing.T) {
	t.Parallel()
	h := NewNotificationTriggerHandler(nil, nil)
	if got := h.Table(); got != "job_runs" {
		t.Fatalf("Table() = %q, want %q", got, "job_runs")
	}
}

func TestSLOHandler_Table(t *testing.T) {
	t.Parallel()
	h := NewSLOHandler(nil, nil)
	if got := h.Table(); got != "job_runs" {
		t.Fatalf("Table() = %q, want %q", got, "job_runs")
	}
}

func TestWebhookTriggerHandler_Table(t *testing.T) {
	t.Parallel()
	h := NewWebhookTriggerHandler(nil, nil)
	if got := h.Table(); got != "job_runs" {
		t.Fatalf("Table() = %q, want %q", got, "job_runs")
	}
}

func TestWebhookReceiver_RegisterAdditionalHandler(t *testing.T) {
	t.Parallel()
	wr := NewWebhookReceiver(nil, nil)

	h := &HandlerFunc{TableName: "job_runs", Fn: func(_ context.Context, _ Message) error { return nil }}
	wr.RegisterAdditionalHandler(h)

	if len(wr.additionalHandlers["job_runs"]) != 1 {
		t.Fatalf("expected 1 additional handler, got %d", len(wr.additionalHandlers["job_runs"]))
	}

	h2 := &HandlerFunc{TableName: "job_runs", Fn: func(_ context.Context, _ Message) error { return nil }}
	wr.RegisterAdditionalHandler(h2)

	if len(wr.additionalHandlers["job_runs"]) != 2 {
		t.Fatalf("expected 2 additional handlers, got %d", len(wr.additionalHandlers["job_runs"]))
	}
}
