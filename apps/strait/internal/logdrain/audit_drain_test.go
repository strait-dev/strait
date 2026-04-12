package logdrain

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestAuditSIEMDrain_ForwardBatch_Success(t *testing.T) {
	t.Parallel()

	var received []domain.AuditEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing auth: got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/x-ndjson" {
			t.Errorf("wrong content type: %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("User-Agent") != "Strait-Audit-SIEM/1.0" {
			t.Errorf("wrong user agent: %s", r.Header.Get("User-Agent"))
		}
		body, _ := io.ReadAll(r.Body)
		for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
			var ev domain.AuditEvent
			if err := json.Unmarshal([]byte(line), &ev); err == nil {
				received = append(received, ev)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "test-token")
	events := []domain.AuditEvent{
		{ID: "ev-1", Action: "job.created", ProjectID: "p1"},
		{ID: "ev-2", Action: "job.deleted", ProjectID: "p1"},
	}

	if err := drain.ForwardBatch(context.Background(), events); err != nil {
		t.Fatalf("ForwardBatch: %v", err)
	}
	if len(received) != 2 {
		t.Errorf("received %d events, want 2", len(received))
	}
	if received[0].ID != "ev-1" || received[1].ID != "ev-2" {
		t.Errorf("received IDs = %v, %v", received[0].ID, received[1].ID)
	}
}

func TestAuditSIEMDrain_ForwardBatch_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "token")
	err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{{ID: "ev-1"}})
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestAuditSIEMDrain_ForwardBatch_EmptyBatch(t *testing.T) {
	t.Parallel()
	drain := NewAuditSIEMDrain("https://example.com", "token")
	if err := drain.ForwardBatch(context.Background(), nil); err != nil {
		t.Fatalf("empty batch should not error: %v", err)
	}
}

func TestNewAuditSIEMDrain_EmptyEndpoint(t *testing.T) {
	t.Parallel()
	if drain := NewAuditSIEMDrain("", "token"); drain != nil {
		t.Error("expected nil drain for empty endpoint")
	}
}

func TestAuditSIEMDrain_ForwardBatch_NoAuth(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("expected no auth header, got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "")
	if err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{{ID: "ev-1"}}); err != nil {
		t.Fatalf("ForwardBatch: %v", err)
	}
}
