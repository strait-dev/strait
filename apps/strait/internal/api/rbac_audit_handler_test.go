package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAuditEventOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		order         string
		wantAscending bool
		wantError     bool
	}{
		{name: "default desc"},
		{name: "desc", order: "desc"},
		{name: "asc", order: "asc", wantAscending: true},
		{name: "invalid", order: "sideways", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseAuditEventOrder(tt.order)
			if tt.wantError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantAscending, got)
		})
	}
}

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
