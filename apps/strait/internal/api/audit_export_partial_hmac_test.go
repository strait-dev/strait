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

// TestAuditExport_StreamError_OmitsSignatureAndBody verifies that when the DB
// stream errors mid-export, no signature is emitted and no partial export body
// is leaked. Because signed exports are now buffered before sending, a mid-stream
// failure surfaces as a real 500 instead of a truncated, wrongly-signed payload.
func TestAuditExport_StreamError_OmitsSignatureAndBody(t *testing.T) {
	t.Parallel()

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
			return errors.New("connection lost mid-stream")
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet,
		"/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson",
		"", "proj-1")
	srv.ServeHTTP(w, r)

	assert.Empty(t, w.Header().Get("X-Audit-Signature"))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "job.created",
		"a failed signed export must not leak a partial body")
}

// TestAuditExport_CleanStream_DeliversSignatureHeader is the positive control:
// a clean signed export delivers the HMAC as a normal response header (resilient
// to buffering proxies that strip trailers) alongside the full body.
func TestAuditExport_CleanStream_DeliversSignatureHeader(t *testing.T) {
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

	assert.Equal(t, http.StatusOK, w.Code)
	sig := w.Header().Get("X-Audit-Signature")
	assert.Contains(t, sig, "sha256=")
	// The Trailer announcement is gone; the signature is a real header now.
	assert.Empty(t, w.Header().Get("Trailer"))
	assert.Contains(t, w.Body.String(), "job.created", "the full body must still be delivered")
}
