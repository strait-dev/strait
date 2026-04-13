package api

import (
	"context"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
)

// Admin-only endpoint for rotating the per-project audit HMAC signing key.
// Access is restricted to internal-secret callers (requireAdmin) — the same
// pattern used by the DLQ and retention admin surfaces. Tenant isolation is
// structural: the project_id used for rotation is taken from the
// authenticated request context, so an admin key scoped to project A cannot
// rotate project B's key by forging a URL.
//
// The store's RotateAuditSigningKey performs the full rotation atomically:
// advisory-lock → derive new epoch key → store encrypted → emit
// is_anchor=TRUE audit.key_rotated event under the new key. The handler
// therefore emits no additional audit event — doing so would either
// duplicate the anchor or log under the old key.

// RotateAuditSigningKeyInput carries the path-bound project id. The
// authenticated project id from context is authoritative; the path id is
// only used to reject cross-tenant requests.
type RotateAuditSigningKeyInput struct {
	ID   string `path:"id"`
	Body struct{}
}

type RotateAuditSigningKeyOutput struct {
	Body RotateAuditSigningKeyResponse
}

// RotateAuditSigningKeyResponse echoes the epoch transition so operators
// can confirm the rotation landed without a follow-up read. The anchor
// event for the new epoch is discoverable via the audit-events list.
type RotateAuditSigningKeyResponse struct {
	PreviousEpoch int `json:"previous_epoch"`
	NewEpoch      int `json:"new_epoch"`
}

func (s *Server) handleRotateAuditSigningKey(ctx context.Context, input *RotateAuditSigningKeyInput) (*RotateAuditSigningKeyOutput, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.ID != "" && input.ID != projectID {
		return nil, huma.Error403Forbidden("path project id does not match authenticated project context")
	}

	actorID := actorFromContext(ctx)
	newEpoch, err := s.store.RotateAuditSigningKey(ctx, projectID, actorID)
	if err != nil {
		slog.Error("failed to rotate audit signing key", "project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to rotate audit signing key")
	}

	return &RotateAuditSigningKeyOutput{Body: RotateAuditSigningKeyResponse{
		PreviousEpoch: newEpoch - 1,
		NewEpoch:      newEpoch,
	}}, nil
}
