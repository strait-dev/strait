package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestAdminEndpointsReachability guards against the failure mode where an
// admin operation is registered with Huma (for OpenAPI docs) but never
// mounted on the chi router that actually serves requests. Every admin
// path below is hit with an authenticated request; the test only requires
// that the response is NOT a "route not found" 404, proving the endpoint
// exists. 401/403/400/500 are all acceptable — they prove the handler
// ran.
func TestAdminEndpointsReachability(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		// Stub enough of the store surface that handlers can run without
		// panicking. Return values do not matter — the test only asserts
		// reachability, not behavior.
		ListAuditEventsDeadletterByProjectFunc: func(_ context.Context, _ string, _ int, _ string) ([]domain.AuditEvent, []string, []string, error) {
			return nil, nil, nil, nil
		},
		GetAuditEventDeadletterFunc: func(_ context.Context, _, _ string) (*domain.AuditEvent, error) {
			return nil, nil
		},
		GetAuditRetentionDaysFunc: func(_ context.Context, _ string) (int, bool, error) {
			return 0, false, nil
		},
		SetAuditRetentionDaysFunc: func(_ context.Context, _ string, _ int) error { return nil },
		GetAuditExportRowCapFunc:  func(_ context.Context, _ string) (int64, error) { return 0, nil },
		SetAuditExportRowCapFunc:  func(_ context.Context, _ string, _ int64) error { return nil },
		RotateAuditSigningKeyFunc: func(_ context.Context, _, _ string) (int, error) { return 1, nil },
		GetAuditEventFunc: func(_ context.Context, projectID, id string) (*domain.AuditEvent, error) {
			return &domain.AuditEvent{ID: id, ProjectID: projectID}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"list-audit-deadletter", http.MethodGet, "/v1/audit/deadletter", ""},
		{"replay-audit-deadletter", http.MethodPost, "/v1/audit/deadletter/dlq-1/replay", "{}"},
		{"drop-audit-deadletter", http.MethodDelete, "/v1/audit/deadletter/dlq-1", ""},
		{"get-audit-retention", http.MethodGet, "/v1/projects/proj-a/audit/retention", ""},
		{"set-audit-retention", http.MethodPut, "/v1/projects/proj-a/audit/retention", `{"days":30}`},
		{"rotate-audit-signing-key", http.MethodPost, "/v1/projects/proj-a/audit/rotate-key", "{}"},
		{"update-audit-export-cap", http.MethodPut, "/v1/projects/proj-a/quotas/audit-export-cap", `{"row_cap":100000}`},
		{"get-audit-event", http.MethodGet, "/v1/audit-events/evt-1", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := authedProjectRequest(tc.method, tc.path, tc.body, "proj-a")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			require.False(t, w.Code ==
				http.StatusNotFound &&
				strings.Contains(w.
					Body.String(), "404 page not found",
				))

			// A chi "route not found" returns 404 with a body that chi
			// writes directly. The Huma/TypedHandler surface produces
			// structured JSON error bodies. Treat a 404 whose body
			// indicates chi's default "404 page not found" as the fail
			// case; any 4xx/5xx from the handler itself is fine.

		})
	}
}
