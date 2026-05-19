package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type outboxAdminStore interface {
	ListQuarantinedOutbox(ctx context.Context, projectID string, limit int, cursorConsumedAt *time.Time, cursorID string) ([]store.QuarantinedOutboxRow, error)
	GetQuarantinedOutbox(ctx context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error)
	RetryQuarantinedOutbox(ctx context.Context, projectID, id string) (*store.OutboxRow, error)
	PurgeQuarantinedOutbox(ctx context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error)
}

func resolveOutboxAdminStore(s APIStore) outboxAdminStore {
	if s == nil {
		return nil
	}
	if outboxStore, ok := any(s).(outboxAdminStore); ok {
		return outboxStore
	}
	return nil
}

type AdminOutboxRow struct {
	ID              string          `json:"id"`
	ProjectID       string          `json:"project_id"`
	JobID           string          `json:"job_id"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	IdempotencyKey  *string         `json:"idempotency_key,omitempty"`
	ScheduledAt     *time.Time      `json:"scheduled_at,omitempty"`
	Priority        int             `json:"priority"`
	CreatedAt       time.Time       `json:"created_at"`
	ConsumedAt      time.Time       `json:"consumed_at"`
	Error           string          `json:"error"`
	RetryOfOutboxID *string         `json:"retry_of_outbox_id,omitempty"`
}

type ListAdminOutboxInput struct {
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListAdminOutboxOutput struct {
	Body PaginatedResponse
}

type GetAdminOutboxInput struct {
	OutboxID string `path:"outbox_id"`
}

type GetAdminOutboxOutput struct {
	Body *AdminOutboxRow
}

type AdminOutboxMutationInput struct {
	OutboxID string `path:"outbox_id"`
}

type AdminRetryOutboxOutput struct {
	Body struct {
		OutboxID      string `json:"outbox_id"`
		RetryOutboxID string `json:"retry_outbox_id"`
		OK            bool   `json:"ok"`
	}
}

type AdminOutboxAckOutput struct {
	Body struct {
		OutboxID string `json:"outbox_id"`
		OK       bool   `json:"ok"`
	}
}

func (s *Server) handleAdminListOutbox(ctx context.Context, input *ListAdminOutboxInput) (*ListAdminOutboxOutput, error) {
	if err := s.requireAdminScope(ctx, domain.ScopeOutboxRead); err != nil {
		return nil, err
	}
	if err := requireProjectWideOutboxAccess(ctx); err != nil {
		return nil, err
	}
	if s.outboxAdminStore == nil {
		return nil, huma.Error503ServiceUnavailable("outbox admin store unavailable")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	limit, _, err := parsePaginationFromStrings(input.Limit, "")
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	cursorConsumedAt, cursorID, err := parseOutboxCursor(input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	rows, err := s.outboxAdminStore.ListQuarantinedOutbox(ctx, projectID, limit+1, cursorConsumedAt, cursorID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list quarantined outbox rows")
	}

	items := make([]AdminOutboxRow, len(rows))
	for i, row := range rows {
		items[i] = adminOutboxRow(row)
	}

	return &ListAdminOutboxOutput{Body: paginatedResult(items, limit, func(row AdminOutboxRow) string {
		return formatOutboxCursor(row.ConsumedAt, row.ID)
	})}, nil
}

func (s *Server) handleAdminGetOutbox(ctx context.Context, input *GetAdminOutboxInput) (*GetAdminOutboxOutput, error) {
	if err := s.requireAdminScope(ctx, domain.ScopeOutboxRead); err != nil {
		return nil, err
	}
	if err := requireProjectWideOutboxAccess(ctx); err != nil {
		return nil, err
	}
	if s.outboxAdminStore == nil {
		return nil, huma.Error503ServiceUnavailable("outbox admin store unavailable")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	row, err := s.outboxAdminStore.GetQuarantinedOutbox(ctx, projectID, input.OutboxID)
	if err != nil {
		if errors.Is(err, store.ErrOutboxRowNotFound) {
			return nil, huma.Error404NotFound("outbox row not found")
		}
		return nil, huma.Error500InternalServerError("failed to get quarantined outbox row")
	}
	out := adminOutboxRow(*row)
	return &GetAdminOutboxOutput{Body: &out}, nil
}

func (s *Server) handleAdminRetryOutbox(ctx context.Context, input *AdminOutboxMutationInput) (*AdminRetryOutboxOutput, error) {
	if err := s.requireAdminScope(ctx, domain.ScopeOutboxRetry); err != nil {
		return nil, err
	}
	if err := requireProjectWideOutboxAccess(ctx); err != nil {
		return nil, err
	}
	if s.outboxAdminStore == nil {
		return nil, huma.Error503ServiceUnavailable("outbox admin store unavailable")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	source, err := s.outboxAdminStore.GetQuarantinedOutbox(ctx, projectID, input.OutboxID)
	if err != nil {
		if errors.Is(err, store.ErrOutboxRowNotFound) {
			return nil, huma.Error404NotFound("outbox row not found")
		}
		return nil, huma.Error500InternalServerError("failed to load quarantined outbox row")
	}

	cloned, err := s.outboxAdminStore.RetryQuarantinedOutbox(ctx, projectID, input.OutboxID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrOutboxRowNotFound):
			return nil, huma.Error404NotFound("outbox row not found")
		case errors.Is(err, store.ErrOutboxRowConflict):
			return nil, huma.Error409Conflict("an active retry already exists for this outbox row")
		default:
			return nil, huma.Error500InternalServerError("failed to retry quarantined outbox row")
		}
	}

	s.writeOutboxAudit(ctx, projectID, "outbox.retry", input.OutboxID, outboxAuditSnapshot(*source), map[string]any{
		"retry_outbox_id": cloned.ID,
	})

	out := &AdminRetryOutboxOutput{}
	out.Body.OutboxID = input.OutboxID
	out.Body.RetryOutboxID = cloned.ID
	out.Body.OK = true
	return out, nil
}

func (s *Server) handleAdminPurgeOutbox(ctx context.Context, input *AdminOutboxMutationInput) (*AdminOutboxAckOutput, error) {
	if err := s.requireAdminScope(ctx, domain.ScopeOutboxPurge); err != nil {
		return nil, err
	}
	if err := requireProjectWideOutboxAccess(ctx); err != nil {
		return nil, err
	}
	if s.outboxAdminStore == nil {
		return nil, huma.Error503ServiceUnavailable("outbox admin store unavailable")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	row, err := s.outboxAdminStore.PurgeQuarantinedOutbox(ctx, projectID, input.OutboxID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrOutboxRowNotFound):
			return nil, huma.Error404NotFound("outbox row not found")
		case errors.Is(err, store.ErrOutboxRowConflict):
			return nil, huma.Error409Conflict("outbox row state changed during purge")
		default:
			return nil, huma.Error500InternalServerError("failed to purge quarantined outbox row")
		}
	}

	s.writeOutboxAudit(ctx, projectID, "outbox.purge", input.OutboxID, outboxAuditSnapshot(*row), map[string]any{
		"purged": true,
	})

	out := &AdminOutboxAckOutput{}
	out.Body.OutboxID = input.OutboxID
	out.Body.OK = true
	return out, nil
}

func requireProjectWideOutboxAccess(ctx context.Context) error {
	if environmentIDFromContext(ctx) != "" {
		return huma.Error403Forbidden("outbox admin requires a project-wide key")
	}
	return nil
}

func adminOutboxRow(row store.QuarantinedOutboxRow) AdminOutboxRow {
	return AdminOutboxRow{
		ID:              row.ID,
		ProjectID:       row.ProjectID,
		JobID:           row.JobID,
		Payload:         row.Payload,
		Metadata:        row.Metadata,
		IdempotencyKey:  row.IdempotencyKey,
		ScheduledAt:     row.ScheduledAt,
		Priority:        row.Priority,
		CreatedAt:       row.CreatedAt,
		ConsumedAt:      row.ConsumedAt,
		Error:           row.Error,
		RetryOfOutboxID: row.RetryOfOutboxID,
	}
}

func outboxAuditSnapshot(row store.QuarantinedOutboxRow) map[string]any {
	snapshot := map[string]any{
		"id":                      row.ID,
		"project_id":              row.ProjectID,
		"job_id":                  row.JobID,
		"priority":                row.Priority,
		"created_at":              row.CreatedAt,
		"consumed_at":             row.ConsumedAt,
		"payload_bytes":           len(row.Payload),
		"metadata_bytes":          len(row.Metadata),
		"idempotency_key_present": row.IdempotencyKey != nil && *row.IdempotencyKey != "",
		"error_present":           strings.TrimSpace(row.Error) != "",
		"error_bytes":             len(row.Error),
	}
	if len(row.Payload) > 0 {
		snapshot["payload_sha256"] = outboxAuditHash(row.Payload)
	}
	if len(row.Metadata) > 0 {
		snapshot["metadata_sha256"] = outboxAuditHash(row.Metadata)
	}
	if row.ScheduledAt != nil {
		snapshot["scheduled_at"] = *row.ScheduledAt
	}
	if row.RetryOfOutboxID != nil {
		snapshot["retry_of_outbox_id"] = *row.RetryOfOutboxID
	}
	return snapshot
}

func outboxAuditHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Server) writeOutboxAudit(ctx context.Context, projectID, action, outboxID string, before, after any) {
	details := map[string]any{
		"outbox_id": outboxID,
		"before":    before,
		"after":     after,
	}
	raw, err := s.marshalAndCapDetails(ctx, action, details)
	if err != nil {
		slog.Error("outbox audit marshal failed", "action", action, "outbox_id", outboxID, "err", err)
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
		ResourceType: "enqueue_outbox",
		ResourceID:   outboxID,
		Details:      raw,
	}
	if err := s.store.CreateAuditEvent(ctx, ev); err != nil {
		slog.Error("outbox audit write failed",
			"action", action,
			"outbox_id", outboxID,
			"project_id", projectID,
			"err", err,
		)
	}
}

func formatOutboxCursor(consumedAt time.Time, id string) string {
	return consumedAt.UTC().Format(time.RFC3339Nano) + "|" + id
}

func parseOutboxCursor(cursor string) (*time.Time, string, error) {
	if strings.TrimSpace(cursor) == "" {
		return nil, "", nil
	}
	parts := strings.SplitN(cursor, "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil, "", &paginationError{msg: "invalid outbox cursor"}
	}
	consumedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, "", &paginationError{msg: "invalid outbox cursor"}
	}
	return &consumedAt, parts[1], nil
}
