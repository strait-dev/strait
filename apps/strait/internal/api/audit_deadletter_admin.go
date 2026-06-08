package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

// Admin-only endpoints for the audit deadletter queue. Access is restricted
// to internal-secret callers via isInternalCaller(ctx), which checks the
// positive ctxInternalCallerKey flag set by internalSecretAuth middleware.
//
// Tenant isolation is structural: every query filters by project_id taken
// from the request context, so even a compromised admin key scoped to
// project A cannot read or mutate project B's DLQ. A missing project
// context yields 400, not a cross-tenant read.
//
// Each access emits its own audit event (audit.deadletter_read,
// audit.deadletter_replayed, audit.deadletter_dropped) — the audit log
// audits its own admin surface.

// DeadletterEntry is the wire representation of a single DLQ row. Matches
// domain.AuditEvent minus chain-internal fields.
type DeadletterEntry struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	ActorID       string          `json:"actor_id"`
	ActorType     string          `json:"actor_type"`
	Action        string          `json:"action"`
	ResourceType  string          `json:"resource_type"`
	ResourceID    string          `json:"resource_id"`
	Details       json.RawMessage `json:"details"`
	CreatedAt     string          `json:"created_at"`
	SchemaVersion uint16          `json:"schema_version"`
}

type ListDeadletterInput struct {
	ProjectID string `query:"project_id"`
	Limit     string `query:"limit"`
	Cursor    string `query:"cursor"`
}

type ListDeadletterOutput struct {
	Body ListDeadletterResponse
}

type ListDeadletterResponse struct {
	Entries    []DeadletterEntry `json:"entries"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// deadletterMaxLimit bounds how many rows a single list call can fetch.
// Matches other audit admin surfaces; pagination via cursor is the
// supported way to walk a larger DLQ backlog.
const deadletterMaxLimit = 200

// requireAdmin enforces the internal-secret auth pattern for DLQ admin
// endpoints. Only requests positively identified as internal-secret callers
// are admitted. Checking isInternalCaller(ctx) is more secure than checking
// scopesFromContext(ctx) == nil: nil scopes are also present on
// unauthenticated requests that bypassed auth middleware, whereas
// ctxInternalCallerKey is only set after X-Internal-Secret passes
// constant-time comparison in internalSecretAuth. Returns 403 for any
// caller that is not a verified internal-secret caller.
func (s *Server) requireAdmin(ctx context.Context) error {
	if !isInternalCaller(ctx) {
		return huma.Error403Forbidden("admin access required")
	}
	return nil
}

// redactDeadletterFilter serializes list query params with secret-shaped
// values stripped. The audit event for audit.deadletter_read records
// this string so operators can trace which queries were run without
// leaking anything sensitive even if a caller attempted to inject one.
//
// The previous implementation was a substring allow-list on a handful
// of key-name markers ("secret", "token", ...) — trivial to bypass by
// encoding the secret under a different name in the value. We instead
// run the same scanAndRedact used by audit emit, which matches shape
// regardless of context and replaces the match with a typed marker
// so operators still see that a secret was attempted.
func redactDeadletterFilter(projectID, limit, cursor string) string {
	redact := func(v string) string {
		if v == "" {
			return v
		}
		redacted, _ := scanAndRedact(v)
		if s, ok := redacted.(string); ok {
			return s
		}
		return v
	}
	return fmt.Sprintf("project_id=%s&limit=%s&cursor=%s",
		redact(projectID), redact(limit), redact(cursor))
}

func (s *Server) handleListDeadletter(ctx context.Context, input *ListDeadletterInput) (*ListDeadletterOutput, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	// Reject cross-tenant requests: if the caller specifies project_id
	// in the query, it must match the authenticated project context.
	// Return 404 (not 403) so a probing admin key cannot distinguish
	// "project B exists but is cross-tenant" from "project B does not
	// exist" — either response would leak the same information.
	if input.ProjectID != "" && input.ProjectID != projectID {
		return nil, huma.Error404NotFound("deadletter queue not found")
	}

	limit := 50
	if input.Limit != "" {
		n, err := strconv.Atoi(input.Limit)
		if err != nil || n <= 0 {
			return nil, huma.Error400BadRequest("limit must be a positive integer")
		}
		if n > deadletterMaxLimit {
			n = deadletterMaxLimit
		}
		limit = n
	}

	events, _, cursors, err := s.store.ListAuditEventsDeadletterByProject(ctx, projectID, limit, input.Cursor)
	if err != nil {
		slog.Error("failed to list audit deadletter", "project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to list deadletter")
	}

	entries := make([]DeadletterEntry, 0, len(events))
	for _, ev := range events {
		entries = append(entries, DeadletterEntry{
			ID:            ev.ID,
			ProjectID:     ev.ProjectID,
			ActorID:       ev.ActorID,
			ActorType:     ev.ActorType,
			Action:        ev.Action,
			ResourceType:  ev.ResourceType,
			ResourceID:    ev.ResourceID,
			Details:       ev.Details,
			CreatedAt:     ev.CreatedAt.UTC().Format(time.RFC3339Nano),
			SchemaVersion: ev.SchemaVersion,
		})
	}

	var next string
	if len(cursors) == limit && len(cursors) > 0 {
		next = cursors[len(cursors)-1]
	}

	s.emitAuditEvent(ctx, domain.AuditActionDeadletterRead, "audit_deadletter", projectID, map[string]any{
		"filter": redactDeadletterFilter(input.ProjectID, input.Limit, input.Cursor),
		"count":  len(entries),
	})

	return &ListDeadletterOutput{Body: ListDeadletterResponse{Entries: entries, NextCursor: next}}, nil
}

// ReplayDeadletterInput identifies a single DLQ row to move into the chain.
type ReplayDeadletterInput struct {
	ID string `path:"id"`
}

type ReplayDeadletterOutput struct {
	Body ReplayDeadletterResponse
}

type ReplayDeadletterResponse struct {
	DeadletterID string `json:"deadletter_id"`
	NewEventID   string `json:"new_event_id"`
}

type auditDeadletterAtomicReplayer interface {
	ReplayAuditEventDeadletter(ctx context.Context, id, projectID, newEventID string) (*domain.AuditEvent, bool, error)
}

func (s *Server) handleReplayDeadletter(ctx context.Context, input *ReplayDeadletterInput) (*ReplayDeadletterOutput, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.ID == "" {
		return nil, huma.Error400BadRequest("id is required")
	}

	// Replay must be atomic: inserting into the tamper-evident audit chain,
	// marking the DLQ row reclaimed, and deleting it have to commit or roll back
	// together. The previous non-transactional fallback could insert a DUPLICATE
	// chain event on retry when the insert succeeded but the mark/delete failed.
	// Require the atomic replayer rather than risk a corrupted chain.
	replayer, ok := s.store.(auditDeadletterAtomicReplayer)
	if !ok {
		slog.Error("audit deadletter replay requires an atomic replayer store",
			"deadletter_id", input.ID, "project_id", projectID)
		return nil, huma.Error500InternalServerError("audit deadletter replay is not supported")
	}

	// Structural tenant isolation: the atomic replay is scoped by project_id, so
	// an admin of project A asking for project B's row gets a 404 — never a leak.
	newEventID := uuid.Must(uuid.NewV7()).String()
	newEvent, replayed, replayErr := replayer.ReplayAuditEventDeadletter(ctx, input.ID, projectID, newEventID)
	if replayErr != nil {
		slog.Error("audit deadletter atomic replay failed",
			"deadletter_id", input.ID, "project_id", projectID, "error", replayErr)
		return nil, huma.Error500InternalServerError("failed to replay deadletter into audit chain")
	}
	if !replayed || newEvent == nil {
		return nil, huma.Error404NotFound("deadletter entry not found")
	}
	s.emitAuditEvent(ctx, domain.AuditActionDeadletterReplayed, "audit_deadletter", input.ID, map[string]any{
		"deadletter_id": input.ID,
		"new_event_id":  newEvent.ID,
	})
	return &ReplayDeadletterOutput{Body: ReplayDeadletterResponse{
		DeadletterID: input.ID,
		NewEventID:   newEvent.ID,
	}}, nil
}

// DropDeadletterInput identifies a DLQ row to permanently delete.
// Reason is free-form and recorded in the self-audit event; bound it to
// 1024 chars so an operator mistake or script loop cannot wedge a huge
// blob in the audit chain via the drop endpoint.
type DropDeadletterInput struct {
	ID     string `path:"id"`
	Reason string `query:"reason" maxLength:"1024"`
}

type DropDeadletterOutput struct {
	Body DropDeadletterResponse
}

type DropDeadletterResponse struct {
	DeadletterID string `json:"deadletter_id"`
	Dropped      bool   `json:"dropped"`
}

type auditDeadletterAtomicDropper interface {
	DropAuditEventDeadletterWithAudit(ctx context.Context, id, projectID string, auditEvent *domain.AuditEvent) (bool, error)
}

func (s *Server) handleDropDeadletter(ctx context.Context, input *DropDeadletterInput) (*DropDeadletterOutput, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.ID == "" {
		return nil, huma.Error400BadRequest("id is required")
	}
	reason := input.Reason
	if reason == "" {
		reason = "operator_drop"
	}

	dropper, ok := s.store.(auditDeadletterAtomicDropper)
	if !ok {
		slog.Error("audit deadletter drop requires atomic audit-capable store")
		return nil, huma.Error500InternalServerError("failed to drop deadletter entry")
	}
	auditEvent, auditErr := s.buildAuditEvent(ctx, domain.AuditActionDeadletterDropped, "audit_deadletter", input.ID, map[string]any{
		"deadletter_id": input.ID,
		"reason":        reason,
	})
	if auditErr != nil {
		slog.Error("failed to build audit deadletter drop event", "id", input.ID, "project_id", projectID, "error", auditErr)
		return nil, huma.Error500InternalServerError("failed to build audit event")
	}

	dropped, dropErr := dropper.DropAuditEventDeadletterWithAudit(ctx, input.ID, projectID, auditEvent)
	if dropErr != nil {
		slog.Error("audit deadletter atomic drop failed", "id", input.ID, "project_id", projectID, "error", dropErr)
		return nil, huma.Error500InternalServerError("failed to drop deadletter entry")
	}
	if !dropped {
		return nil, huma.Error404NotFound("deadletter entry not found")
	}

	return &DropDeadletterOutput{Body: DropDeadletterResponse{
		DeadletterID: input.ID,
		Dropped:      true,
	}}, nil
}
