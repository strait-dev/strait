package billing

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPolarEventIngester_IngestComputeUsage(t *testing.T) {
	t.Parallel()

	var received atomic.Int32
	var lastBody polarIngestRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)

		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/events/ingest" {
			t.Errorf("expected /v1/events/ingest, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %s", got)
		}

		if err := json.NewDecoder(r.Body).Decode(&lastBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ingester := NewPolarEventIngester(srv.URL, "test-token", slog.Default())

	err := ingester.IngestComputeUsage(context.Background(), "cust-123", "run-456", 1700)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Load() != 1 {
		t.Fatalf("expected 1 request, got %d", received.Load())
	}

	if len(lastBody.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(lastBody.Events))
	}

	evt := lastBody.Events[0]
	if evt.Name != "compute_overage" {
		t.Errorf("expected meter name compute_overage, got %s", evt.Name)
	}
	if evt.ExternalCustomerID != "cust-123" {
		t.Errorf("expected customer cust-123, got %s", evt.ExternalCustomerID)
	}
	if evt.ExternalID != "run-456" {
		t.Errorf("expected external_id run-456, got %s", evt.ExternalID)
	}
	if evt.Metadata["amount"] != "1700" {
		t.Errorf("expected amount 1700, got %s", evt.Metadata["amount"])
	}
}

func TestPolarEventIngester_SkipsWhenNotConfigured(t *testing.T) {
	t.Parallel()

	// Empty token should not make any requests.
	ingester := NewPolarEventIngester("http://localhost:9999", "", slog.Default())
	err := ingester.IngestComputeUsage(context.Background(), "cust-123", "run-456", 1700)
	if err != nil {
		t.Fatalf("expected nil error for empty token, got: %v", err)
	}

	// Empty customer ID should not make any requests.
	ingester2 := NewPolarEventIngester("http://localhost:9999", "token", slog.Default())
	err = ingester2.IngestComputeUsage(context.Background(), "", "run-456", 1700)
	if err != nil {
		t.Fatalf("expected nil error for empty customer, got: %v", err)
	}
}

func TestPolarEventIngester_ReturnsErrorOnFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ingester := NewPolarEventIngester(srv.URL, "test-token", slog.Default())
	err := ingester.IngestComputeUsage(context.Background(), "cust-123", "run-456", 1700)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestPolarEventIngester_IngestBatch(t *testing.T) {
	t.Parallel()

	var lastBody polarIngestRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&lastBody); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ingester := NewPolarEventIngester(srv.URL, "test-token", slog.Default())

	events := []polarEvent{
		{Name: "compute_overage", ExternalCustomerID: "cust-1", Metadata: map[string]string{"amount": "100"}, ExternalID: "run-1"},
		{Name: "compute_overage", ExternalCustomerID: "cust-1", Metadata: map[string]string{"amount": "200"}, ExternalID: "run-2"},
	}

	err := ingester.IngestBatch(context.Background(), events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lastBody.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(lastBody.Events))
	}
}
