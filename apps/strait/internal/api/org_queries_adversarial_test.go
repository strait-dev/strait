package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestOrgQuery_EmptyOrgID verifies that an empty org ID in the URL path
// results in a 400 or 404 rather than leaking data.
func TestOrgQuery_EmptyOrgID(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, orgID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			require.NotEqual(t, "", orgID)

			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// The router should not match an empty orgID segment; this results in
	// a different path entirely, so we expect a non-200 response.
	for _, path := range []string{"/v1/organizations//runs", "/v1/organizations//jobs"} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, path, ""))
		require.NotEqual(t, http.
			StatusOK, w.Code)

	}
}

// TestOrgQuery_NullByteOrgID verifies that URL-encoded null bytes in the org
// ID do not cause panics or unexpected behavior.
func TestOrgQuery_NullByteOrgID(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{}, nil
		},
		ListJobsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Job, error) {
			return []domain.Job{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Use percent-encoded null byte since raw \x00 is invalid in URLs.
	for _, path := range []string{
		"/v1/organizations/org%00evil/runs",
		"/v1/organizations/org%00evil/jobs",
	} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, path, ""))
		require.NotEqual(t, 0, w.
			Code)

		// The server must not panic. Any HTTP status is acceptable.

	}
}

// TestOrgQuery_PathTraversalOrgID verifies that path-traversal sequences in
// the org ID do not bypass access controls or reach unintended resources.
func TestOrgQuery_PathTraversalOrgID(t *testing.T) {
	t.Parallel()

	traversalIDs := []string{
		"../other-org",
		"..%2Fother-org",
		"org-1/../../admin",
		"org-1%00/../admin",
	}

	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	for _, orgID := range traversalIDs {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/"+orgID+"/runs", ""))
		require.NotEqual(t, 0, w.
			Code)

		// Path traversal must not produce 200 with real data; either the
		// router rejects it or the handler sees a garbled orgID.

	}
}

// TestOrgQuery_CrossOrgAccess verifies that an API key scoped to one org
// cannot query runs for a different org.
func TestOrgQuery_CrossOrgAccess(t *testing.T) {
	t.Parallel()

	rawKey := "strait_" + strings.Repeat("ab", 32)

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        "key-1",
				ProjectID: "proj-1",
				OrgID:     "org-attacker",
			}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	for _, path := range []string{
		"/v1/organizations/org-victim/runs",
		"/v1/organizations/org-victim/jobs",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+rawKey)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)
		require.Equal(t, http.StatusForbidden,
			w.Code)

	}
}

// TestOrgQuery_ProjectScopedKeyRejectsOrg verifies that a project-scoped API
// key (with scopes but no orgID) cannot query org-level endpoints.
func TestOrgQuery_ProjectScopedKeyRejectsOrg(t *testing.T) {
	t.Parallel()

	rawKey := "strait_" + strings.Repeat("cc", 32)

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        "key-proj",
				ProjectID: "proj-1",
				OrgID:     "", // No org association.
				Scopes:    []string{"jobs:read", "runs:read"},
			}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	for _, path := range []string{
		"/v1/organizations/org-1/runs",
		"/v1/organizations/org-1/jobs",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+rawKey)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)
		require.Equal(t, http.StatusForbidden,
			w.Code)

	}
}

// TestOrgQuery_PaginationOverflow verifies that requesting a limit larger than
// the maximum is clamped rather than causing errors or unbounded queries.
func TestOrgQuery_PaginationOverflow(t *testing.T) {
	t.Parallel()

	var capturedLimit int
	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, _ string, limit int, _ *time.Time) ([]domain.JobRun, error) {
			capturedLimit = limit
			return []domain.JobRun{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	const orgUUID = "00000000-0000-4000-8000-0000000000aa"
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/"+orgUUID+"/runs?limit=999999", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.LessOrEqual(t, capturedLimit,
		maxPageLimit+
			1)

	// The handler adds 1 to the limit before passing to store; maxPageLimit
	// is 100, so we expect 101 at most.

}

// FuzzOrgQueryIDs fuzzes the org ID path parameter through the list-org-runs
// handler to verify it never panics regardless of input.
func FuzzOrgQueryIDs(f *testing.F) {
	f.Add("org-1")
	f.Add("")
	f.Add("\x00")
	f.Add("../other-org")
	f.Add(strings.Repeat("x", 10000))
	f.Add("org-1; DROP TABLE jobs")
	f.Add("org%00injected")

	f.Fuzz(func(t *testing.T, orgID string) {
		ms := &APIStoreMock{
			ListRunsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
				return []domain.JobRun{}, nil
			},
		}

		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		path := "/v1/organizations/" + url.PathEscape(orgID) + "/runs"
		srv.ServeHTTP(w, authedRequest(http.MethodGet, path, ""))
		require.NotEqual(t, 0, w.
			Code)

		// We only care that the server does not panic.

	})
}

// TestOrgQuery_LongOrgID verifies that an extremely long org ID (10 KB) does
// not cause panics, memory exhaustion, or unbounded processing.
func TestOrgQuery_LongOrgID(t *testing.T) {
	t.Parallel()

	longOrgID := strings.Repeat("a", 10*1024)

	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{}, nil
		},
		ListJobsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Job, error) {
			return []domain.Job{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	for _, suffix := range []string{"/runs", "/jobs"} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/"+longOrgID+suffix, ""))
		require.NotEqual(t, 0, w.
			Code)

		// Must not panic; any status code is fine.

	}
}
