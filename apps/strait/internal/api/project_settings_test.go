package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
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
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
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
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
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
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
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
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
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
	if w.Code == http.StatusForbidden {
		t.Fatalf("expected non-403, got 403: %s", w.Body.String())
	}
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
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without project context, got %d: %s", w.Code, w.Body.String())
	}
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
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
