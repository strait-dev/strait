package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetProjectSettings_InternalSecret_CrossOrgForbidden(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
			"proj-B": "org-B",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	req := internalSecretRequestWithProject(http.MethodGet, "/v1/projects/proj-B/settings", "", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(
		t, http.StatusForbidden,
		w.Code)
}

func TestGetProjectSettings_InternalSecret_SameOrgAllowed(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	req := internalSecretRequestWithProject(http.MethodGet, "/v1/projects/proj-A/settings", "", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(
		t, http.StatusOK, w.Code,
	)
}

func TestGetProjectSettings_APIKey_CrossOrgForbidden(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
			"proj-B": "org-B",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	req := apiKeyRequest(http.MethodGet, "/v1/projects/proj-B/settings", "", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(
		t, http.StatusForbidden,
		w.Code)
}

func TestGetProjectSettings_APIKey_SameOrgAllowed(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	req := apiKeyRequest(http.MethodGet, "/v1/projects/proj-A/settings", "", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(
		t, http.StatusOK, w.Code,
	)
}

func TestUpdateProjectSettings_InternalSecret_CrossOrgForbidden(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
			"proj-B": "org-B",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	body := `{"max_key_lifetime_days":30}`
	req := internalSecretRequestWithProject(http.MethodPut, "/v1/projects/proj-B/settings", body, "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(
		t, http.StatusForbidden,
		w.Code)
}

func TestUpdateProjectSettings_InternalSecret_SameOrgAllowed(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	body := `{"max_key_lifetime_days":30}`
	req := internalSecretRequestWithProject(http.MethodPut, "/v1/projects/proj-A/settings", body, "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.NotEqual(t, http.StatusForbidden,
		w.Code,
	)
}

func TestGetProjectSettings_NoProjectContext_Forbidden(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-A": "org-A"},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	req := authedRequest(http.MethodGet, "/v1/projects/proj-A/settings", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(
		t, http.StatusForbidden,
		w.Code)
}

func TestUpdateProjectSettings_InvalidBody_BadRequest(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{"proj-A": "org-A"},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	req := internalSecretRequestWithProject(http.MethodPut, "/v1/projects/proj-A/settings", "not-json", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(
		t, http.StatusBadRequest,
		w.Code,
	)
}
