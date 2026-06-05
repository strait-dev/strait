package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleListAuditEvents_InvalidOrder(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/audit-events?order=sideways", "", "proj_1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(
		t, http.StatusBadRequest,
		w.Code,
	)
}

func TestHandleListAuditEvents_InvalidFrom(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/audit-events?from=bad-time", "", "proj_1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(
		t, http.StatusBadRequest,
		w.Code,
	)
}
