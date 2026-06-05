package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestRBACMutations_CreateRole_EmitsAuditEvent(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.CreateProjectRoleFunc = func(_ context.Context, role *domain.ProjectRole) error {
		role.ID = "role-1"
		return nil
	}

	var (
		mu       sync.Mutex
		captured *domain.AuditEvent
	)
	ms.CreateAuditEventFunc = func(_ context.Context, ev *domain.AuditEvent) error {
		mu.Lock()
		defer mu.Unlock()
		clone := *ev
		captured = &clone
		return nil
	}

	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodPost, "/v1/roles", `{"name":"deployer","permissions":["jobs:write"]}`)
	req.Header.Set("X-Project-Id", "proj-1")
	req.Header.Set("X-Actor-Id", "user-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, captured)
	require.Equal(t, "role.created",
		captured.
			Action)
	require.Equal(t, "role", captured.
		ResourceType,
	)
	require.Equal(t, "role-1", captured.
		ResourceID,
	)
	require.Equal(t, "proj-1", captured.
		ProjectID,
	)
	require.Equal(t, "user-1", captured.
		ActorID,
	)
}

func TestRBACMutations_AssignMember_EmitsPermissionGrantedAuditEvent(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: id, Name: "admin"}, nil
	}
	ms.AssignMemberRoleFunc = func(_ context.Context, m *domain.ProjectMemberRole) error {
		m.ID = "member-1"
		return nil
	}

	var captured *domain.AuditEvent
	ms.CreateAuditEventFunc = func(_ context.Context, ev *domain.AuditEvent) error {
		clone := *ev
		captured = &clone
		return nil
	}

	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodPost, "/v1/members", `{"user_id":"user-2","role_id":"role-2"}`)
	req.Header.Set("X-Project-Id", "proj-2")
	req.Header.Set("X-Actor-Id", "user-admin")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, captured)
	require.Equal(t, "permission.granted",
		captured.
			Action)
	require.Equal(t, "role", captured.
		ResourceType,
	)
	require.Equal(t, "role-2", captured.
		ResourceID,
	)

	var details map[string]any
	require.NoError(t, json.Unmarshal(captured.
		Details, &details))
	require.Equal(t, "user-2", details["user_id"])
}
