package api

import (
	"context"
	"errors"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
)

// GetAuditEventInput is the typed input for reading a single audit event.
type GetAuditEventInput struct {
	ID string `path:"id"`
}

// GetAuditEventOutput is the typed output for reading a single audit event.
type GetAuditEventOutput struct {
	Body *domain.AuditEvent
}

// handleGetAuditEvent returns a single audit event scoped to the caller's
// project. Cross-tenant reads return 404 to avoid leaking existence.
// The read is itself audited as audit.single_read with target_id = event id.
func (s *Server) handleGetAuditEvent(ctx context.Context, input *GetAuditEventInput) (*GetAuditEventOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	if err := s.checkFeatureAllowed(ctx, projectID, billing.FeatureAuditLogs, "Audit logs"); err != nil {
		return nil, err
	}

	ev, err := s.store.GetAuditEvent(ctx, projectID, input.ID)
	if err != nil {
		if errors.Is(err, store.ErrAuditEventNotFound) {
			return nil, huma.Error404NotFound("audit event not found")
		}
		slog.Error("failed to get audit event", "project_id", projectID, "id", input.ID, "error", err)
		return nil, huma.Error500InternalServerError("failed to get audit event")
	}

	s.emitAuditEvent(ctx, domain.AuditActionAuditSingleRead, "audit", ev.ID, map[string]any{
		"target_id": ev.ID,
	})

	return &GetAuditEventOutput{Body: ev}, nil
}
