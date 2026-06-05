package api

import (
	"context"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	require.Equal(t, "proj-abc",

		deletedWithProject)

}

func TestDropDeadletter_DeleteScopedByProject(t *testing.T) {
	t.Parallel()
	var deletedWithProject string
	base := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) (*domain.AuditEvent, error) {
			return &domain.AuditEvent{
				ID: id, ProjectID: projectID, Action: "test.action",
			}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	ms := &atomicDropAPIStore{APIStoreMock: base, drop: func(_ context.Context, id, projectID string, _ *domain.AuditEvent) (bool, error) {
		require.Equal(t, "dlq-2",

			id)

		deletedWithProject = projectID
		return true, nil
	}}
	srv := newTestServer(t, ms, nil, nil)
	_, err := srv.handleDropDeadletter(adminCtx("proj-xyz"), &DropDeadletterInput{ID: "dlq-2"})
	require.NoError(t, err)
	require.Equal(t, "proj-xyz",

		deletedWithProject)

}
