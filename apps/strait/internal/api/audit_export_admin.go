package api

import (
	"context"
	"log/slog"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

// Admin-only endpoint for managing the per-project audit export row cap
// stored in project_quotas.audit_export_row_cap. Access is restricted to
// internal-secret callers (requireAdmin — mirrors the DLQ admin surface
// in audit_deadletter_admin.go). Tenant isolation is structural: the
// project_id always comes from the authenticated request context, so an
// admin key scoped to project A cannot mutate project B's quota even by
// forging a URL or body value.
//
// Setting cap=0 re-inherits the server-wide default
// (config.AuditExportRowCapDefault). Negative values are rejected with
// 400 so operators cannot accidentally disable exports or hit
// surprising int64 wrap-around behavior in the streaming cap check.
//
// Every mutation emits an audit.export_cap_updated event with the old
// and new caps — the audit surface audits its own admin changes.

// UpdateAuditExportCapInput is the PUT body for the admin endpoint.
// RowCap is an int64 because project_quotas.audit_export_row_cap is
// BIGINT; a project legitimately exporting hundreds of millions of
// rows is rare but representable.
type UpdateAuditExportCapInput struct {
	ID   string `path:"id"`
	Body struct {
		RowCap int64 `json:"row_cap"`
	}
}

// UpdateAuditExportCapOutput echoes the applied cap so clients can
// confirm what persisted without a follow-up GET.
type UpdateAuditExportCapOutput struct {
	Body UpdateAuditExportCapResponse
}

type UpdateAuditExportCapResponse struct {
	ProjectID string `json:"project_id"`
	RowCap    int64  `json:"row_cap"`
}

func (s *Server) handleUpdateAuditExportCap(ctx context.Context, input *UpdateAuditExportCapInput) (*UpdateAuditExportCapOutput, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	// Structural tenant isolation: the path id must match the authenticated
	// project context. A compromised admin key on project A cannot rewrite
	// project B's cap via a crafted URL. Return 404 (not 403) so a probing
	// caller cannot distinguish "project B exists but you lack access"
	// from "project B does not exist" — both leak identical information.
	if input.ID != "" && input.ID != projectID {
		return nil, huma.Error404NotFound("audit export cap not found")
	}

	if input.Body.RowCap < 0 {
		return nil, huma.Error400BadRequest("row_cap must be >= 0 (0 re-inherits the default)")
	}

	oldCap, err := s.store.GetAuditExportRowCap(ctx, projectID)
	if err != nil {
		slog.Error("failed to read audit export row cap", "project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to read current cap")
	}

	if err := s.store.SetAuditExportRowCap(ctx, projectID, input.Body.RowCap); err != nil {
		slog.Error("failed to persist audit export row cap", "project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to update cap")
	}
	if s.quotaCache != nil {
		s.quotaCache.Invalidate(projectID)
	}

	s.emitAuditEvent(ctx, domain.AuditActionExportCapUpdated, "project_quotas", projectID, map[string]any{
		"old_cap": oldCap,
		"new_cap": input.Body.RowCap,
	})

	return &UpdateAuditExportCapOutput{Body: UpdateAuditExportCapResponse{
		ProjectID: projectID,
		RowCap:    input.Body.RowCap,
	}}, nil
}
