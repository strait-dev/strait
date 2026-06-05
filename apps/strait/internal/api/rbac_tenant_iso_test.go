package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// TestTenantIso_RBAC_UpdateRole_RejectsCrossProject verifies that callers
// authenticated against proj-A cannot mutate a role owned by proj-B.
// The regression path ignored the GetProjectRole error and silently proceeded
// to UpdateProjectRole, allowing a cross-project write whenever the role id
// was guessable.
func TestTenantIso_RBAC_UpdateRole_RejectsCrossProject(t *testing.T) {
	t.Parallel()

	updated := false
	ms := &APIStoreMock{
		GetProjectRoleFunc: func(_ context.Context, id string) (*domain.ProjectRole, error) {
			return &domain.ProjectRole{ID: id, ProjectID: "proj-other", Name: "old", Permissions: []string{"jobs:read"}}, nil
		},
		UpdateProjectRoleFunc: func(_ context.Context, _ *domain.ProjectRole) error {
			updated = true
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"hacked","permissions":["*"]}`
	req := authedProjectRequest(http.MethodPatch, "/v1/roles/role_other", body, "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,

		w.Code)
	require.False(t, updated)
}

// TestTenantIso_RBAC_UpdateRole_RejectsStoreError verifies that an
// unexpected GetProjectRole error surfaces as 500 instead of being
// swallowed. The regression path used `_` for the err and proceeded to the
// unconditional update, so a transient DB error would silently drop the
// previous-state guard.
func TestTenantIso_RBAC_UpdateRole_RejectsStoreError(t *testing.T) {
	t.Parallel()

	updated := false
	ms := &APIStoreMock{
		GetProjectRoleFunc: func(_ context.Context, _ string) (*domain.ProjectRole, error) {
			return nil, errors.New("transient db error")
		},
		UpdateProjectRoleFunc: func(_ context.Context, _ *domain.ProjectRole) error {
			updated = true
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"x","permissions":["jobs:read"]}`
	req := authedProjectRequest(http.MethodPatch, "/v1/roles/role_x", body, "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
	require.False(t, updated)
}

// TestTenantIso_RBAC_GetRole_LineageStopsAtForeignParent verifies that the
// lineage walk truncates as soon as it reaches a parent role that does not
// belong to the caller's project. System roles (empty ProjectID) remain
// visible.
func TestTenantIso_RBAC_GetRole_LineageStopsAtForeignParent(t *testing.T) {
	t.Parallel()

	roles := map[string]*domain.ProjectRole{
		"role_child":     {ID: "role_child", ProjectID: "proj-mine", ParentRoleID: "role_p1"},
		"role_p1":        {ID: "role_p1", ProjectID: "proj-mine", ParentRoleID: "role_p2"},
		"role_p2":        {ID: "role_p2", ProjectID: "proj-other", ParentRoleID: "role_p3"},
		"role_p3":        {ID: "role_p3", ProjectID: "proj-other", ParentRoleID: ""},
		"role_sys_child": {ID: "role_sys_child", ProjectID: "proj-mine", ParentRoleID: "role_sys"},
		"role_sys":       {ID: "role_sys", ProjectID: "", ParentRoleID: ""},
	}
	ms := &APIStoreMock{
		GetProjectRoleFunc: func(_ context.Context, id string) (*domain.ProjectRole, error) {
			r, ok := roles[id]
			if !ok {
				return nil, store.ErrRoleNotFound
			}
			return r, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/roles/role_child?include_lineage=true", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	body := w.Body.String()
	require.Contains(
		t, body, `"id":"role_p1"`,
	)
	require.False(t, strings.Contains(body,
		`"id":"role_p2"`,
	) || strings.Contains(
		body,
		`"id":"role_p3"`))

	// role_p1 is in the caller project and must appear; role_p2 / role_p3
	// belong to a different project and must be truncated.

	// System roles stay visible from a same-project descendant.
	req2 := authedProjectRequest(http.MethodGet, "/v1/roles/role_sys_child?include_lineage=true", "", "proj-mine")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK,
		w2.
			Code)
	require.Contains(
		t, w2.
			Body.String(), `"id":"role_sys"`)
}
