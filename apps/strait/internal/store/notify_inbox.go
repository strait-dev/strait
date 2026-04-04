package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateInboxItem(ctx context.Context, item *domain.InboxItem) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateInboxItem")
	defer span.End()

	if item.ID == "" {
		item.ID = uuid.Must(uuid.NewV7()).String()
	}
	if item.Priority == "" {
		item.Priority = "normal"
	}
	if item.State == "" {
		item.State = domain.NotifyInboxStateUnread
	}
	if item.DedupCount <= 0 {
		item.DedupCount = 1
	}
	if len(item.Actions) == 0 {
		item.Actions = []byte("[]")
	}

	query := `
		INSERT INTO inbox_items (
			id, recipient_type, recipient_id, project_id, tenant_id, workflow_id, workflow_run_id, category_key,
			title, body, avatar, priority, state, actions, dedup_key, dedup_count,
			read_at, archived_at, actioned_at, action_result
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16,
			$17, $18, $19, $20
		)
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(
		ctx,
		query,
		item.ID,
		item.RecipientType,
		item.RecipientID,
		item.ProjectID,
		dbscan.NilIfEmptyString(item.TenantID),
		dbscan.NilIfEmptyString(item.WorkflowID),
		dbscan.NilIfEmptyString(item.WorkflowRunID),
		dbscan.NilIfEmptyString(item.CategoryKey),
		item.Title,
		dbscan.NilIfEmptyString(item.Body),
		dbscan.NilIfEmptyString(item.Avatar),
		item.Priority,
		item.State,
		item.Actions,
		dbscan.NilIfEmptyString(item.DedupKey),
		item.DedupCount,
		item.ReadAt,
		item.ArchivedAt,
		item.ActionedAt,
		dbscan.NilIfEmptyRawMessage(item.ActionResult),
	).Scan(&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create inbox item: %w", err)
	}

	return nil
}

func (q *Queries) GetInboxItem(ctx context.Context, id, recipientType, recipientID string) (*domain.InboxItem, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetInboxItem")
	defer span.End()

	query := `
		SELECT id, recipient_type, recipient_id, project_id, tenant_id, workflow_id, workflow_run_id, category_key,
		       title, body, avatar, priority, state, actions, dedup_key, dedup_count,
		       read_at, archived_at, actioned_at, action_result, created_at, updated_at
		FROM inbox_items
		WHERE id = $1 AND recipient_type = $2 AND recipient_id = $3`

	item, err := scanInboxItem(q.db.QueryRow(ctx, query, id, recipientType, recipientID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInboxItemNotFound
		}
		return nil, fmt.Errorf("get inbox item: %w", err)
	}

	return item, nil
}

func (q *Queries) ListInboxItems(ctx context.Context, recipientType, recipientID string, state *string, limit int, cursor *time.Time) ([]domain.InboxItem, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListInboxItems")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, recipient_type, recipient_id, project_id, tenant_id, workflow_id, workflow_run_id, category_key,
		       title, body, avatar, priority, state, actions, dedup_key, dedup_count,
		       read_at, archived_at, actioned_at, action_result, created_at, updated_at
		FROM inbox_items
		WHERE recipient_type = $1
		  AND recipient_id = $2
		  AND ($3::text IS NULL OR state = $3)
		  AND ($4::timestamptz IS NULL OR created_at < $4)
		ORDER BY created_at DESC
		LIMIT $5`

	var stateValue any
	if state != nil {
		stateValue = *state
	}
	var cursorValue any
	if cursor != nil {
		cursorValue = *cursor
	}

	rows, err := q.db.Query(ctx, query, recipientType, recipientID, stateValue, cursorValue, limit)
	if err != nil {
		return nil, fmt.Errorf("list inbox items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.InboxItem, 0, limit)
	for rows.Next() {
		item, scanErr := scanInboxItem(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list inbox items scan: %w", scanErr)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list inbox items rows: %w", err)
	}

	return items, nil
}

func (q *Queries) CountInboxUnread(ctx context.Context, recipientType, recipientID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountInboxUnread")
	defer span.End()

	query := `
		SELECT COUNT(*)
		FROM inbox_items
		WHERE recipient_type = $1 AND recipient_id = $2 AND state = 'unread'`

	var count int
	if err := q.db.QueryRow(ctx, query, recipientType, recipientID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count inbox unread: %w", err)
	}
	return count, nil
}

func (q *Queries) MarkAllInboxItemsRead(ctx context.Context, recipientType, recipientID string, readAt time.Time) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkAllInboxItemsRead")
	defer span.End()

	query := `
		UPDATE inbox_items
		SET state = 'read',
			read_at = COALESCE(read_at, $3),
			updated_at = NOW()
		WHERE recipient_type = $1
		  AND recipient_id = $2
		  AND state = 'unread'`

	tag, err := q.db.Exec(ctx, query, recipientType, recipientID, readAt)
	if err != nil {
		return 0, fmt.Errorf("mark all inbox items read: %w", err)
	}

	return tag.RowsAffected(), nil
}

func (q *Queries) UpdateInboxItemState(ctx context.Context, id, recipientType, recipientID, state string, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateInboxItemState")
	defer span.End()

	if state == "" {
		return &domain.FieldError{Field: "state"}
	}

	allowedColumns := map[string]struct{}{
		"read_at":       {},
		"archived_at":   {},
		"actioned_at":   {},
		"action_result": {},
		"dedup_count":   {},
	}

	setClauses := []string{"state = $1", "updated_at = NOW()"}
	args := []any{state, id, recipientType, recipientID}
	param := 5

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := fields[key]
		if _, ok := allowedColumns[key]; !ok {
			return &domain.FieldError{Field: key}
		}
		if raw, ok := value.(json.RawMessage); ok {
			value = dbscan.NilIfEmptyRawMessage(raw)
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, param))
		args = append(args, value)
		param++
	}

	query := fmt.Sprintf(
		"UPDATE inbox_items SET %s WHERE id = $2 AND recipient_type = $3 AND recipient_id = $4",
		strings.Join(setClauses, ", "),
	)

	tag, err := q.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update inbox item state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInboxItemNotFound
	}

	return nil
}

func scanInboxItem(scanner scanTarget) (*domain.InboxItem, error) {
	var item domain.InboxItem
	var tenantID *string
	var workflowID *string
	var workflowRunID *string
	var categoryKey *string
	var body *string
	var avatar *string
	var dedupKey *string
	var actionResult []byte

	err := scanner.Scan(
		&item.ID,
		&item.RecipientType,
		&item.RecipientID,
		&item.ProjectID,
		&tenantID,
		&workflowID,
		&workflowRunID,
		&categoryKey,
		&item.Title,
		&body,
		&avatar,
		&item.Priority,
		&item.State,
		&item.Actions,
		&dedupKey,
		&item.DedupCount,
		&item.ReadAt,
		&item.ArchivedAt,
		&item.ActionedAt,
		&actionResult,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if tenantID != nil {
		item.TenantID = *tenantID
	}
	if workflowID != nil {
		item.WorkflowID = *workflowID
	}
	if workflowRunID != nil {
		item.WorkflowRunID = *workflowRunID
	}
	if categoryKey != nil {
		item.CategoryKey = *categoryKey
	}
	if body != nil {
		item.Body = *body
	}
	if avatar != nil {
		item.Avatar = *avatar
	}
	if dedupKey != nil {
		item.DedupKey = *dedupKey
	}
	if len(actionResult) > 0 {
		item.ActionResult = actionResult
	}
	if len(item.Actions) == 0 {
		item.Actions = []byte("[]")
	}
	if item.DedupCount == 0 {
		item.DedupCount = 1
	}

	return &item, nil
}
