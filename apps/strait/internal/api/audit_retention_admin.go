package api

import (
	"context"
	"log/slog"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

// Admin-only endpoints for managing the per-project audit retention
// override stored in project_quotas.audit_retention_days. Access is
// restricted to internal-secret callers (requireAdmin) and tenant
// isolation is structural: the project_id always comes from the
// authenticated request context, so an admin key scoped to project A
// cannot read or mutate project B's retention by forging a URL.
//
// Semantics:
//   - No row (or NULL) = inherit config.AuditRetentionDefaultDays. GET
//     returns inherited_from_default = true and the default days value.
//   - days > 0       = retain for that many days, overriding default.
//   - days == 0      = explicit "disable trim" — the reaper skips this
//     project entirely (Phase 2). GET reports this as an explicit
//     override (inherited_from_default = false).
//   - days < 0       = 400 from the PUT handler.
//
// Every mutation emits an audit.retention_updated event with old/new.

// GetAuditRetentionInput carries the path-bound project id. The
// authenticated project id from context is still the authoritative
// value; the path id is only used to reject cross-tenant requests.
type GetAuditRetentionInput struct {
	ID string `path:"id"`
}

// GetAuditRetentionOutput wraps the response body.
type GetAuditRetentionOutput struct {
	Body GetAuditRetentionResponse
}

// GetAuditRetentionResponse is the GET body. InheritedFromDefault
// distinguishes "no override present" from "explicit override of N" —
// including the important N = 0 case (explicitly disabled).
type GetAuditRetentionResponse struct {
	ProjectID            string `json:"project_id"`
	Days                 int    `json:"days"`
	InheritedFromDefault bool   `json:"inherited_from_default"`
}

// UpdateAuditRetentionInput is the PUT body. Days is an int (project
// quota column is INT) and matches the reaper's in-memory override type.
type UpdateAuditRetentionInput struct {
	ID   string `path:"id"`
	Body struct {
		Days int `json:"days"`
	}
}

// UpdateAuditRetentionOutput echoes the persisted override so clients
// can confirm what landed without a follow-up GET.
type UpdateAuditRetentionOutput struct {
	Body UpdateAuditRetentionResponse
}

type UpdateAuditRetentionResponse struct {
	ProjectID string `json:"project_id"`
	Days      int    `json:"days"`
}

func (s *Server) handleGetAuditRetention(ctx context.Context, input *GetAuditRetentionInput) (*GetAuditRetentionOutput, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	// 404 (not 403) on cross-tenant to avoid enumeration of project ids.
	if input.ID != "" && input.ID != projectID {
		return nil, huma.Error404NotFound("retention override not found")
	}

	days, present, err := s.store.GetAuditRetentionDays(ctx, projectID)
	if err != nil {
		slog.Error("failed to read audit retention override", "project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to read retention override")
	}

	if present {
		return &GetAuditRetentionOutput{Body: GetAuditRetentionResponse{
			ProjectID:            projectID,
			Days:                 days,
			InheritedFromDefault: false,
		}}, nil
	}

	// No override: report the server default with the inherited flag set.
	defaultDays := 0
	if s.config != nil {
		defaultDays = s.config.AuditRetentionDefaultDays
	}
	return &GetAuditRetentionOutput{Body: GetAuditRetentionResponse{
		ProjectID:            projectID,
		Days:                 defaultDays,
		InheritedFromDefault: true,
	}}, nil
}

func (s *Server) handleSetAuditRetention(ctx context.Context, input *UpdateAuditRetentionInput) (*UpdateAuditRetentionOutput, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	// 404 (not 403) on cross-tenant to avoid enumeration of project ids.
	if input.ID != "" && input.ID != projectID {
		return nil, huma.Error404NotFound("retention override not found")
	}

	if input.Body.Days < 0 {
		return nil, huma.Error400BadRequest("days must be >= 0 (0 disables retention trimming for this project)")
	}
	if input.Body.Days > domain.MaxAuditRetentionDays {
		return nil, huma.Error400BadRequest("days exceeds maximum audit retention")
	}

	// Capture the previous value for the self-audit payload. If no
	// override is present, record the server default as old_days so
	// operators can reconstruct the effective change.
	oldDays, present, err := s.store.GetAuditRetentionDays(ctx, projectID)
	if err != nil {
		slog.Error("failed to read audit retention override", "project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to read current retention")
	}
	if !present && s.config != nil {
		oldDays = s.config.AuditRetentionDefaultDays
	}

	audit, err := s.buildAuditEvent(ctx, domain.AuditActionRetentionUpdated, "project_quotas", projectID, map[string]any{
		"old_days": oldDays,
		"new_days": input.Body.Days,
	})
	if err != nil {
		slog.Error("failed to build audit retention update event", "project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to audit retention update")
	}
	if audit == nil {
		return nil, huma.Error500InternalServerError("failed to audit retention update")
	}

	if err := s.runInTx(ctx, func(txStore APIStore) error {
		if err := txStore.SetAuditRetentionDays(ctx, projectID, input.Body.Days); err != nil {
			return err
		}
		return txStore.CreateAuditEvent(ctx, audit)
	}); err != nil {
		slog.Error("failed to persist audited audit retention override", "project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to update retention")
	}

	if s.quotaCache != nil {
		s.quotaCache.Invalidate(projectID)
	}

	return &UpdateAuditRetentionOutput{Body: UpdateAuditRetentionResponse{
		ProjectID: projectID,
		Days:      input.Body.Days,
	}}, nil
}
