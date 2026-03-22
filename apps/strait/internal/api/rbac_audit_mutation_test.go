package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"strait/internal/domain"
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

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if captured == nil {
		t.Fatal("expected CreateAuditEvent to be called")
	}
	if captured.Action != "role.created" {
		t.Fatalf("action = %q, want %q", captured.Action, "role.created")
	}
	if captured.ResourceType != "role" {
		t.Fatalf("resource_type = %q, want %q", captured.ResourceType, "role")
	}
	if captured.ResourceID != "role-1" {
		t.Fatalf("resource_id = %q, want %q", captured.ResourceID, "role-1")
	}
	if captured.ProjectID != "proj-1" {
		t.Fatalf("project_id = %q, want %q", captured.ProjectID, "proj-1")
	}
	if captured.ActorID != "user-1" {
		t.Fatalf("actor_id = %q, want %q", captured.ActorID, "user-1")
	}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	if captured == nil {
		t.Fatal("expected CreateAuditEvent to be called")
	}
	if captured.Action != "permission.granted" {
		t.Fatalf("action = %q, want %q", captured.Action, "permission.granted")
	}
	if captured.ResourceType != "role" {
		t.Fatalf("resource_type = %q, want %q", captured.ResourceType, "role")
	}
	if captured.ResourceID != "role-2" {
		t.Fatalf("resource_id = %q, want %q", captured.ResourceID, "role-2")
	}

	var details map[string]any
	if err := json.Unmarshal(captured.Details, &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	if details["user_id"] != "user-2" {
		t.Fatalf("details.user_id = %v, want %q", details["user_id"], "user-2")
	}
}
