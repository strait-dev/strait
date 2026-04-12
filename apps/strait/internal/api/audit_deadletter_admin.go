package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

// Admin-only endpoints for the audit deadletter queue. Access is restricted
// to internal-secret callers (scopesFromContext(ctx) == nil) — the same
// pattern used by handleCreateProject and the /internal/admin routes.
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
// endpoints. Any request that presented API-key scopes is rejected. This
// mirrors handleCreateProject's scopesFromContext(ctx) != nil check —
// there is no project-role-based admin middleware in this codebase, and
// the existing admin-only surface (/internal/admin/*) uses the same
// pattern. Returns a 403 error when the caller is not an admin.
func (s *Server) requireAdmin(ctx context.Context) error {
	if scopesFromContext(ctx) != nil {
		return huma.Error403Forbidden("admin access required")
	}
	return nil
}

// redactDeadletterFilter serializes list query params with secret-shaped
// values redacted. The audit event for audit.deadletter_read records this
// string so operators can trace which queries were run without leaking
// anything sensitive even if a caller attempted to inject one.
func redactDeadletterFilter(projectID, limit, cursor string) string {
	redact := func(v string) string {
		lv := strings.ToLower(v)
		for _, marker := range []string{"secret", "token", "password", "bearer", "private", "api_key"} {
			if strings.Contains(lv, marker) {
				return "[redacted]"
			}
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
	if input.ProjectID != "" && input.ProjectID != projectID {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project context")
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
			CreatedAt:     ev.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
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

	// Structural tenant isolation: the store fetch is scoped by project_id,
	// so an admin of project A asking for project B's row gets nil back
	// and we surface a 404 — never a cross-tenant leak.
	ev, err := s.store.GetAuditEventDeadletter(ctx, input.ID, projectID)
	if err != nil {
		slog.Error("failed to fetch audit deadletter", "id", input.ID, "project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to fetch deadletter entry")
	}
	if ev == nil {
		return nil, huma.Error404NotFound("deadletter entry not found")
	}

	// Reclaim: insert into audit_events under a fresh chain-generated id,
	// then delete the DLQ row. If the insert fails the DLQ row stays so
	// a later retry (manual or the scheduler reclaimer) can try again.
	// Self-audit emits only after the replay fully commits, so a failed
	// replay never leaves a misleading audit.deadletter_replayed row.
	newEvent := *ev
	newEvent.ID = uuid.Must(uuid.NewV7()).String()
	if createErr := s.store.CreateAuditEvent(ctx, &newEvent); createErr != nil {
		slog.Error("audit deadletter replay chain insert failed",
			"deadletter_id", input.ID, "project_id", projectID, "error", createErr)
		return nil, huma.Error500InternalServerError("failed to replay deadletter into audit chain")
	}
	if delErr := s.store.DeleteAuditEventDeadletter(ctx, input.ID); delErr != nil {
		// Chain insert succeeded but DLQ delete failed. The chain has the
		// event now, so we return success — the DLQ row will be cleaned up
		// either on a retry of this endpoint (idempotent-ish: a second
		// replay will insert a second chain event, which is why operators
		// should treat replay as at-least-once) or by the next reclaimer
		// tick if the row's last_error is also stale.
		slog.Warn("audit deadletter delete failed after successful chain insert",
			"deadletter_id", input.ID, "new_event_id", newEvent.ID, "error", delErr)
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
type DropDeadletterInput struct {
	ID     string `path:"id"`
	Reason string `query:"reason"`
}

type DropDeadletterOutput struct {
	Body DropDeadletterResponse
}

type DropDeadletterResponse struct {
	DeadletterID string `json:"deadletter_id"`
	Dropped      bool   `json:"dropped"`
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

	// Project-scoped lookup first so a cross-tenant drop is impossible.
	ev, err := s.store.GetAuditEventDeadletter(ctx, input.ID, projectID)
	if err != nil {
		slog.Error("failed to fetch audit deadletter for drop",
			"id", input.ID, "project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to fetch deadletter entry")
	}
	if ev == nil {
		return nil, huma.Error404NotFound("deadletter entry not found")
	}

	if delErr := s.store.DeleteAuditEventDeadletter(ctx, input.ID); delErr != nil {
		slog.Error("audit deadletter drop failed", "id", input.ID, "error", delErr)
		return nil, huma.Error500InternalServerError("failed to drop deadletter entry")
	}

	s.emitAuditEvent(ctx, domain.AuditActionDeadletterDropped, "audit_deadletter", input.ID, map[string]any{
		"deadletter_id": input.ID,
		"reason":        reason,
	})

	return &DropDeadletterOutput{Body: DropDeadletterResponse{
		DeadletterID: input.ID,
		Dropped:      true,
	}}, nil
}
