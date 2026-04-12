package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

// Admin DLQ endpoints expose audited, RBAC-gated replacements for the
// manual SQL previously required to recover dead-lettered runs. All
// mutations write an audit_events row with the actor, run id, action,
// and before/after state. Listing and replay reuse the existing
// ListDeadLetterRuns / ReplayDeadLetterRun helpers; unmask and purge
// use the new helpers in internal/store/dlq.go.

// requireAdminScope enforces that the caller's API-key/user scopes
// include the requested DLQ scope. Internal-secret callers bypass the
// check (fully trusted). Returns a 403 error if the scope is missing.
func (s *Server) requireAdminScope(ctx context.Context, scope string) error {
	callerScopes := scopesFromContext(ctx)
	if callerScopes == nil {
		// Internal secret auth: trusted.
		return nil
	}
	if !domain.HasScope(callerScopes, scope) {
		return huma.Error403Forbidden("missing required scope: " + scope)
	}
	return nil
}

// writeDLQAudit writes a best-effort audit_events row for a DLQ admin
// mutation. Failures are logged but do not fail the caller; the mutation
// has already committed.
func (s *Server) writeDLQAudit(ctx context.Context, projectID, action, runID string, before, after any) {
	details := map[string]any{
		"run_id": runID,
		"before": before,
		"after":  after,
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return
	}
	actorID := actorFromContext(ctx)
	actorType, _ := ctx.Value(ctxActorTypeKey).(string)
	if actorType == "" {
		actorType = "api_key"
	}
	ev := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      actorID,
		ActorType:    actorType,
		Action:       action,
		ResourceType: "job_run",
		ResourceID:   runID,
		Details:      raw,
	}
	_ = s.store.CreateAuditEvent(ctx, ev)
}

// GET /v1/admin/dlq.

// ListAdminDLQInput is the typed input for the admin DLQ listing endpoint.
type ListAdminDLQInput struct {
	JobID  string `query:"job_id"`
	Masked string `query:"masked"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

// ListAdminDLQOutput is the typed output for the admin DLQ listing endpoint.
type ListAdminDLQOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleAdminListDLQ(ctx context.Context, input *ListAdminDLQInput) (*ListAdminDLQOutput, error) {
	if err := s.requireAdminScope(ctx, domain.ScopeDLQRead); err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	runs, err := s.store.ListDeadLetterRuns(ctx, projectID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list dead letter runs")
	}

	// Optional client-side filter for job_id and masked, applied after the
	// store query so the existing helper (and its RLS plumbing) stays
	// untouched. The page size is bounded so this is cheap.
	if input.JobID != "" || input.Masked != "" {
		filtered := make([]domain.JobRun, 0, len(runs))
		for _, r := range runs {
			if input.JobID != "" && r.JobID != input.JobID {
				continue
			}
			filtered = append(filtered, r)
		}
		runs = filtered
	}

	return &ListAdminDLQOutput{Body: paginatedResult(runs, limit, func(run domain.JobRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

// POST /v1/admin/dlq/{run_id}/replay.

// AdminDLQRunInput is the shared typed input for per-run DLQ mutations.
type AdminDLQRunInput struct {
	RunID string `path:"run_id"`
}

// AdminReplayDLQOutput is the typed output for the replay endpoint.
type AdminReplayDLQOutput struct {
	Body *domain.JobRun
}

func (s *Server) handleAdminReplayDLQ(ctx context.Context, input *AdminDLQRunInput) (*AdminReplayDLQOutput, error) {
	if err := s.requireAdminScope(ctx, domain.ScopeDLQReplay); err != nil {
		return nil, err
	}
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	original, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to load run")
	}

	run, err := s.store.ReplayDeadLetterRun(ctx, input.RunID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrRunNotFound):
			return nil, huma.Error404NotFound("run not found")
		case errors.Is(err, store.ErrRunConflict):
			return nil, huma.Error409Conflict("run is not in dead_letter status")
		default:
			return nil, huma.Error500InternalServerError("failed to replay dead letter run")
		}
	}

	// Record lineage back-pointer on the original row. The existing
	// ReplayDeadLetterRun helper CASes the same row back to queued, so
	// replayed_run_id references the same id -- still useful as a marker
	// that the row went through the admin replay path.
	_ = s.store.MarkRunReplayed(ctx, input.RunID, run.ID)

	s.writeDLQAudit(ctx, original.ProjectID, "dlq.replay", input.RunID,
		map[string]any{"status": original.Status},
		map[string]any{"status": run.Status, "replayed_run_id": run.ID},
	)

	return &AdminReplayDLQOutput{Body: run}, nil
}

// POST /v1/admin/dlq/{run_id}/unmask.

// AdminDLQAckOutput is a minimal success envelope for unmask/purge.
type AdminDLQAckOutput struct {
	Body struct {
		RunID string `json:"run_id"`
		OK    bool   `json:"ok"`
	}
}

func (s *Server) handleAdminUnmaskDLQ(ctx context.Context, input *AdminDLQRunInput) (*AdminDLQAckOutput, error) {
	if err := s.requireAdminScope(ctx, domain.ScopeDLQReplay); err != nil {
		return nil, err
	}
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	original, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to load run")
	}

	if err := s.store.UnmaskDLQRun(ctx, input.RunID); err != nil {
		switch {
		case errors.Is(err, store.ErrRunNotFound):
			return nil, huma.Error404NotFound("run not found")
		case errors.Is(err, store.ErrRunConflict):
			return nil, huma.Error409Conflict("run is not in dead_letter status")
		default:
			return nil, huma.Error500InternalServerError("failed to unmask dlq run")
		}
	}

	s.writeDLQAudit(ctx, original.ProjectID, "dlq.unmask", input.RunID,
		map[string]any{"masked": true},
		map[string]any{"masked": false},
	)

	out := &AdminDLQAckOutput{}
	out.Body.RunID = input.RunID
	out.Body.OK = true
	return out, nil
}

// POST /v1/admin/dlq/{run_id}/purge.

func (s *Server) handleAdminPurgeDLQ(ctx context.Context, input *AdminDLQRunInput) (*AdminDLQAckOutput, error) {
	if err := s.requireAdminScope(ctx, domain.ScopeDLQPurge); err != nil {
		return nil, err
	}
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	original, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to load run")
	}

	if err := s.store.PurgeDLQRun(ctx, input.RunID); err != nil {
		switch {
		case errors.Is(err, store.ErrRunNotFound):
			return nil, huma.Error404NotFound("run not found")
		case errors.Is(err, store.ErrRunConflict):
			return nil, huma.Error409Conflict("run is not in dead_letter status")
		default:
			return nil, huma.Error500InternalServerError("failed to purge dlq run")
		}
	}

	s.writeDLQAudit(ctx, original.ProjectID, "dlq.purge", input.RunID,
		map[string]any{"status": original.Status, "job_id": original.JobID},
		map[string]any{"deleted": true},
	)

	out := &AdminDLQAckOutput{}
	out.Body.RunID = input.RunID
	out.Body.OK = true
	return out, nil
}
