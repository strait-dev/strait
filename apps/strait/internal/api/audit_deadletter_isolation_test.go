package api

import (
	"context"
	"testing"

	"strait/internal/domain"
)

func TestReplayDeadletter_DeleteScopedByProject(t *testing.T) {
	t.Parallel()
	var deletedWithProject string
	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) (*domain.AuditEvent, error) {
			return &domain.AuditEvent{
				ID: id, ProjectID: projectID, Action: "test.action",
			}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
		DeleteAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) error {
			deletedWithProject = projectID
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	_, err := srv.handleReplayDeadletter(adminCtx("proj-abc"), &ReplayDeadletterInput{ID: "dlq-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedWithProject != "proj-abc" {
		t.Fatalf("expected delete scoped to proj-abc, got %q", deletedWithProject)
	}
}

func TestDropDeadletter_DeleteScopedByProject(t *testing.T) {
	t.Parallel()
	var deletedWithProject string
	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) (*domain.AuditEvent, error) {
			return &domain.AuditEvent{
				ID: id, ProjectID: projectID, Action: "test.action",
			}, nil
		},
		DeleteAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) error {
			deletedWithProject = projectID
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	srv := newTestServer(t, ms, nil, nil)
	_, err := srv.handleDropDeadletter(adminCtx("proj-xyz"), &DropDeadletterInput{ID: "dlq-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedWithProject != "proj-xyz" {
		t.Fatalf("expected delete scoped to proj-xyz, got %q", deletedWithProject)
	}
}
