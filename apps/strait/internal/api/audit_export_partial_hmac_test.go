package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

// TestAuditExport_StreamError_OmitsHMACTrailer verifies that when the DB
// stream errors mid-export, the X-Audit-Signature trailer is NOT set. Before
// the fix, a partial export still carried a valid HMAC over truncated output,
// misleading consumers into trusting an incomplete payload.
func TestAuditExport_StreamError_OmitsHMACTrailer(t *testing.T) {
	t.Parallel()

	callCount := 0
	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			// Emit one event successfully, then return an error to simulate
			// a mid-stream DB failure.
			ev := &domain.AuditEvent{
				ID: "ev-1", ProjectID: "proj-1", ActorID: "user-1", ActorType: "user",
				Action: "job.created", ResourceType: "job", ResourceID: "job-1",
				CreatedAt: time.Now(),
			}
			if err := fn(ev); err != nil {
				return err
			}
			callCount++
			return errors.New("connection lost mid-stream")
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet,
		"/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson",
		"", "proj-1")
	srv.ServeHTTP(w, r)

	sig := w.Header().Get("X-Audit-Signature")
	assert.Equal(
		t, "", sig,
	)

}

// TestAuditExport_CleanStream_IncludesHMACTrailer is the positive control:
// a clean stream should produce the HMAC trailer.
func TestAuditExport_CleanStream_IncludesHMACTrailer(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			return fn(&domain.AuditEvent{
				ID: "ev-1", ProjectID: "proj-1", ActorID: "user-1", ActorType: "user",
				Action: "job.created", ResourceType: "job", ResourceID: "job-1",
				CreatedAt: time.Now(),
			})
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet,
		"/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson",
		"", "proj-1")
	srv.ServeHTTP(w, r)

	sig := w.Header().Get("X-Audit-Signature")
	assert.NotEqual(t, "",
		sig)

}
