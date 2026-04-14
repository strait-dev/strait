package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type outboxAdminStore interface {
	ListQuarantinedOutbox(ctx context.Context, projectID string, limit int, cursorConsumedAt *time.Time, cursorID string) ([]store.QuarantinedOutboxRow, error)
	GetQuarantinedOutbox(ctx context.Context, projectID, id string) (*store.QuarantinedOutboxRow, error)
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
	ID             string          `json:"id"`
	ProjectID      string          `json:"project_id"`
	JobID          string          `json:"job_id"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
	ScheduledAt    *time.Time      `json:"scheduled_at,omitempty"`
	Priority       int             `json:"priority"`
	CreatedAt      time.Time       `json:"created_at"`
	ConsumedAt     time.Time       `json:"consumed_at"`
	Error          string          `json:"error"`
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

func (s *Server) handleAdminListOutbox(ctx context.Context, input *ListAdminOutboxInput) (*ListAdminOutboxOutput, error) {
	if err := s.requireAdminScope(ctx, domain.ScopeOutboxRead); err != nil {
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

func adminOutboxRow(row store.QuarantinedOutboxRow) AdminOutboxRow {
	return AdminOutboxRow{
		ID:             row.ID,
		ProjectID:      row.ProjectID,
		JobID:          row.JobID,
		Payload:        row.Payload,
		Metadata:       row.Metadata,
		IdempotencyKey: row.IdempotencyKey,
		ScheduledAt:    row.ScheduledAt,
		Priority:       row.Priority,
		CreatedAt:      row.CreatedAt,
		ConsumedAt:     row.ConsumedAt,
		Error:          row.Error,
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
