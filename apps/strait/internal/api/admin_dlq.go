package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// Admin DLQ endpoints expose audited, RBAC-gated replacements for the
// manual SQL previously required to recover dead-lettered runs. All
// mutations write an audit_events row with the actor, run id, action,
// and before/after state. Listing and replay reuse the existing
// ListDeadLetterRuns / ReplayDeadLetterRun helpers; unmask and purge
// use the new helpers in internal/store/dlq.go.

// requireAdminScope enforces that the caller's API-key/user scopes
// include the requested DLQ scope. Internal-secret callers — and only
// internal-secret callers — bypass the check (fully trusted); those
// requests are marked by a nil scopes slice on the context. An
// explicitly empty slice (len == 0, non-nil) represents an API key that
// was provisioned with no scopes and MUST NOT bypass: the wildcard
// compatibility in domain.HasScope is not appropriate for admin
// endpoints, so we reject such callers with 403.
func (s *Server) requireAdminScope(ctx context.Context, scope string) error {
	if isInternalCaller(ctx) {
		return nil
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return huma.Error403Forbidden("admin scope requires project context")
	}

	actorType := actorTypeFromContext(ctx)
	callerScopes := scopesFromContext(ctx)
	switch actorType {
	case "api_key":
		if !domain.HasScopeStrict(callerScopes, scope) {
			return huma.Error403Forbidden("missing required scope: " + scope)
		}
		return nil
	case "user":
		actorID := actorFromContext(ctx)
		if actorID == "" {
			return huma.Error403Forbidden("admin scope requires actor context")
		}
		if ctx.Value(ctxOIDCScopeClaimPresentKey) == true && len(callerScopes) == 0 {
			return huma.Error403Forbidden("missing required scope: " + scope)
		}
		if len(callerScopes) > 0 && !domain.HasScopeStrict(callerScopes, scope) {
			return huma.Error403Forbidden("missing required scope: " + scope)
		}
		perms, err := s.store.GetUserPermissions(ctx, projectID, actorID)
		if err != nil {
			return huma.Error500InternalServerError("failed to load permissions")
		}
		if !domain.HasScopeStrict(perms, scope) {
			return huma.Error403Forbidden("missing required scope: " + scope)
		}
		return nil
	default:
		return huma.Error403Forbidden("unknown actor type")
	}
}

func newDLQAuditEvent(ctx context.Context, projectID, action, runID string, before, after any) (*domain.AuditEvent, error) {
	details := map[string]any{
		"run_id": runID,
		"before": before,
		"after":  after,
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return nil, err
	}
	actorID := actorFromContext(ctx)
	actorType, _ := ctx.Value(ctxActorTypeKey).(string)
	if actorType == "" {
		actorType = "api_key"
	}
	return &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      actorID,
		ActorType:    actorType,
		Action:       action,
		ResourceType: "job_run",
		ResourceID:   runID,
		Details:      raw,
	}, nil
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

	// Parse the optional masked filter. Accept "true"/"false" explicitly;
	// empty string means "no filter". Reject everything else so callers
	// don't silently get unfiltered results from a typo.
	var maskedFilter *bool
	switch input.Masked {
	case "true":
		v := true
		maskedFilter = &v
	case "false":
		v := false
		maskedFilter = &v
	case "":
		// no filter
	default:
		return nil, huma.Error400BadRequest("masked must be 'true', 'false', or omitted")
	}

	var jobFilter *string
	if input.JobID != "" {
		job := input.JobID
		jobFilter = &job
	}

	var runs []domain.JobRun
	if environmentIDFromContext(ctx) != "" {
		runs, err = s.listAdminDeadLetterRunsForEnvironment(ctx, projectID, jobFilter, maskedFilter, limit+1, cursor)
	} else if jobFilter != nil || maskedFilter != nil {
		runs, err = s.store.ListDeadLetterRunsFiltered(ctx, projectID, jobFilter, maskedFilter, limit+1, cursor)
	} else {
		runs, err = s.store.ListDeadLetterRuns(ctx, projectID, limit+1, cursor)
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list dead letter runs")
	}

	return &ListAdminDLQOutput{Body: paginatedResult(runs, limit, func(run domain.JobRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

func (s *Server) listAdminDeadLetterRunsForEnvironment(ctx context.Context, projectID string, jobFilter *string, maskedFilter *bool, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	jobEnvCache := make(map[string]bool)
	filtered := make([]domain.JobRun, 0, limit)
	pageCursor := cursor
	fetchLimit := max(limit, 25)

	for {
		var (
			page []domain.JobRun
			err  error
		)
		if jobFilter != nil || maskedFilter != nil {
			page, err = s.store.ListDeadLetterRunsFiltered(ctx, projectID, jobFilter, maskedFilter, fetchLimit, pageCursor)
		} else {
			page, err = s.store.ListDeadLetterRuns(ctx, projectID, fetchLimit, pageCursor)
		}
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return filtered, nil
		}

		for _, run := range page {
			allowed, err := s.runMatchesEnvironment(ctx, run, jobEnvCache)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
			filtered = append(filtered, run)
			if len(filtered) >= limit {
				return filtered, nil
			}
		}

		if len(page) < fetchLimit {
			return filtered, nil
		}
		lastCreatedAt := page[len(page)-1].CreatedAt
		pageCursor = &lastCreatedAt
	}
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
	ctx, span := otel.Tracer("api").Start(ctx, "api.AdminDLQReplay")
	defer span.End()

	actorID := actorFromContext(ctx)
	span.SetAttributes(
		attribute.String("run.id", input.RunID),
		attribute.String("actor.id", actorID),
	)

	if err := s.requireAdminScope(ctx, domain.ScopeDLQReplay); err != nil {
		return nil, err
	}
	if err := s.requireRunAccess(ctx, input.RunID); err != nil {
		return nil, err
	}

	// Single-transaction replay: CAS the row back to queued, stamp
	// replayed_run_id, and write the audit event in one round-trip tx.
	// The project id on the audit row is derived from the CAS result, so
	// we pre-build the audit envelope with the actor/action and let the
	// store fill in details + project id after the update returns.
	actorType, _ := ctx.Value(ctxActorTypeKey).(string)
	if actorType == "" {
		actorType = "api_key"
	}
	audit := &domain.AuditEvent{
		ActorID:      actorID,
		ActorType:    actorType,
		Action:       "dlq.replay",
		ResourceType: "job_run",
		ResourceID:   input.RunID,
	}

	run, err := s.store.ReplayDeadLetterRunWithAudit(ctx, input.RunID, audit)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrRunNotFound):
			return nil, huma.Error404NotFound("run not found")
		case errors.Is(err, store.ErrRunConflict):
			return nil, huma.Error409Conflict("run is not in dead_letter status")
		default:
			slog.Error("dlq replay failed",
				"action", "dlq.replay",
				"run_id", input.RunID,
				"actor_id", actorID,
				"err", err,
			)
			return nil, huma.Error500InternalServerError("failed to replay dead letter run")
		}
	}
	span.SetAttributes(attribute.String("project.id", run.ProjectID))

	slog.Info("dlq replay",
		"action", "dlq.replay",
		"run_id", input.RunID,
		"project_id", run.ProjectID,
		"actor_id", actorID,
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
	ctx, span := otel.Tracer("api").Start(ctx, "api.AdminDLQUnmask")
	defer span.End()

	actorID := actorFromContext(ctx)
	span.SetAttributes(
		attribute.String("run.id", input.RunID),
		attribute.String("actor.id", actorID),
	)

	if err := s.requireAdminScope(ctx, domain.ScopeDLQReplay); err != nil {
		return nil, err
	}

	original, err := s.getRunForAccess(ctx, input.RunID)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("project.id", original.ProjectID))

	audit, err := newDLQAuditEvent(ctx, original.ProjectID, "dlq.unmask", input.RunID,
		map[string]any{"masked": true},
		map[string]any{"masked": false},
	)
	if err != nil {
		slog.Error("dlq audit marshal failed", "action", "dlq.unmask", "run_id", input.RunID, "project_id", original.ProjectID, "actor_id", actorID, "err", err)
		return nil, huma.Error500InternalServerError("failed to audit dlq unmask")
	}

	if err := s.runInTx(ctx, func(txStore APIStore) error {
		if err := txStore.UnmaskDLQRun(ctx, input.RunID); err != nil {
			return err
		}
		if err := txStore.CreateAuditEvent(ctx, audit); err != nil {
			return err
		}
		return nil
	}); err != nil {
		switch {
		case errors.Is(err, store.ErrRunNotFound):
			return nil, huma.Error404NotFound("run not found")
		case errors.Is(err, store.ErrRunConflict):
			return nil, huma.Error409Conflict("run is not in dead_letter status")
		default:
			slog.Error("dlq unmask failed",
				"action", "dlq.unmask",
				"run_id", input.RunID,
				"project_id", original.ProjectID,
				"actor_id", actorID,
				"err", err,
			)
			return nil, huma.Error500InternalServerError("failed to unmask dlq run")
		}
	}

	slog.Info("dlq unmask",
		"action", "dlq.unmask",
		"run_id", input.RunID,
		"project_id", original.ProjectID,
		"actor_id", actorID,
	)

	out := &AdminDLQAckOutput{}
	out.Body.RunID = input.RunID
	out.Body.OK = true
	return out, nil
}

// POST /v1/admin/dlq/{run_id}/purge.

func (s *Server) handleAdminPurgeDLQ(ctx context.Context, input *AdminDLQRunInput) (*AdminDLQAckOutput, error) {
	ctx, span := otel.Tracer("api").Start(ctx, "api.AdminDLQPurge")
	defer span.End()

	actorID := actorFromContext(ctx)
	span.SetAttributes(
		attribute.String("run.id", input.RunID),
		attribute.String("actor.id", actorID),
	)

	if err := s.requireAdminScope(ctx, domain.ScopeDLQPurge); err != nil {
		return nil, err
	}

	original, err := s.getRunForAccess(ctx, input.RunID)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("project.id", original.ProjectID))

	audit, err := newDLQAuditEvent(ctx, original.ProjectID, "dlq.purge", input.RunID,
		map[string]any{"status": original.Status, "job_id": original.JobID},
		map[string]any{"deleted": true},
	)
	if err != nil {
		slog.Error("dlq audit marshal failed", "action", "dlq.purge", "run_id", input.RunID, "project_id", original.ProjectID, "actor_id", actorID, "err", err)
		return nil, huma.Error500InternalServerError("failed to audit dlq purge")
	}

	if err := s.runInTx(ctx, func(txStore APIStore) error {
		if err := txStore.PurgeDLQRun(ctx, input.RunID); err != nil {
			return err
		}
		if err := txStore.CreateAuditEvent(ctx, audit); err != nil {
			return err
		}
		return nil
	}); err != nil {
		switch {
		case errors.Is(err, store.ErrRunNotFound):
			return nil, huma.Error404NotFound("run not found")
		case errors.Is(err, store.ErrRunConflict):
			return nil, huma.Error409Conflict("run is not in dead_letter status")
		default:
			slog.Error("dlq purge failed",
				"action", "dlq.purge",
				"run_id", input.RunID,
				"project_id", original.ProjectID,
				"actor_id", actorID,
				"err", err,
			)
			return nil, huma.Error500InternalServerError("failed to purge dlq run")
		}
	}

	slog.Info("dlq purge",
		"action", "dlq.purge",
		"run_id", input.RunID,
		"project_id", original.ProjectID,
		"actor_id", actorID,
	)

	out := &AdminDLQAckOutput{}
	out.Body.RunID = input.RunID
	out.Body.OK = true
	return out, nil
}
