package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleListAuditEvents_InvalidOrder(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &mockAPIStore{}, nil, nil)
	req := authedRequest(http.MethodGet, "/v1/audit-events?project_id=proj_1&order=sideways", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleListAuditEvents_InvalidFrom(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &mockAPIStore{}, nil, nil)
	req := authedRequest(http.MethodGet, "/v1/audit-events?project_id=proj_1&from=bad-time", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
