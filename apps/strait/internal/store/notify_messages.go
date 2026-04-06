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

func (q *Queries) CreateNotificationMessage(ctx context.Context, msg *domain.NotificationMessage) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateNotificationMessage")
	defer span.End()

	if msg.ID == "" {
		msg.ID = uuid.Must(uuid.NewV7()).String()
	}
	if msg.Status == "" {
		msg.Status = domain.NotifyMessageStatusRendering
	}

	query := `
		INSERT INTO notification_messages (
			id, project_id, idempotency_key, recipient_type, recipient_id, tenant_id, workflow_run_id, step_run_id, template_id, category_key,
			channel, provider_id, rendered_content, ai_generated, status, attempts, provider_response, delivered_at, read_at, clicked_at,
			bounced_at, suppression_reason, batch_id, scheduled_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24
		)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		msg.ID,
		msg.ProjectID,
		dbscan.NilIfEmptyString(msg.IdempotencyKey),
		msg.RecipientType,
		msg.RecipientID,
		dbscan.NilIfEmptyString(msg.TenantID),
		dbscan.NilIfEmptyString(msg.WorkflowRunID),
		dbscan.NilIfEmptyString(msg.StepRunID),
		dbscan.NilIfEmptyString(msg.TemplateID),
		dbscan.NilIfEmptyString(msg.CategoryKey),
		msg.Channel,
		dbscan.NilIfEmptyString(msg.ProviderID),
		dbscan.NilIfEmptyRawMessage(msg.RenderedContent),
		msg.AIGenerated,
		msg.Status,
		msg.Attempts,
		dbscan.NilIfEmptyRawMessage(msg.ProviderResponse),
		msg.DeliveredAt,
		msg.ReadAt,
		msg.ClickedAt,
		msg.BouncedAt,
		dbscan.NilIfEmptyString(msg.SuppressionReason),
		dbscan.NilIfEmptyString(msg.BatchID),
		msg.ScheduledAt,
	).Scan(&msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("create notification message: %w", err)
	}

	return nil
}

func (q *Queries) GetNotificationMessage(ctx context.Context, id, projectID string) (*domain.NotificationMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetNotificationMessage")
	defer span.End()

	query := `
		SELECT id, project_id, idempotency_key, recipient_type, recipient_id, tenant_id, workflow_run_id, step_run_id, template_id, category_key,
		       channel, provider_id, rendered_content, ai_generated, status, attempts, provider_response, delivered_at, read_at, clicked_at,
		       bounced_at, suppression_reason, batch_id, scheduled_at, created_at
		FROM notification_messages
		WHERE id = $1 AND project_id = $2`

	msg, err := scanNotificationMessage(q.db.QueryRow(ctx, query, id, projectID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotificationMessageNotFound
		}
		return nil, fmt.Errorf("get notification message: %w", err)
	}

	return msg, nil
}

func (q *Queries) GetNotificationMessageByID(ctx context.Context, id string) (*domain.NotificationMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetNotificationMessageByID")
	defer span.End()

	query := `
		SELECT id, project_id, idempotency_key, recipient_type, recipient_id, tenant_id, workflow_run_id, step_run_id, template_id, category_key,
		       channel, provider_id, rendered_content, ai_generated, status, attempts, provider_response, delivered_at, read_at, clicked_at,
		       bounced_at, suppression_reason, batch_id, scheduled_at, created_at
		FROM notification_messages
		WHERE id = $1`

	msg, err := scanNotificationMessage(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotificationMessageNotFound
		}
		return nil, fmt.Errorf("get notification message by id: %w", err)
	}
	return msg, nil
}

func (q *Queries) ListNotificationMessagesByProject(ctx context.Context, projectID string, status *string, limit int, cursor *time.Time) ([]domain.NotificationMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotificationMessagesByProject")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, project_id, idempotency_key, recipient_type, recipient_id, tenant_id, workflow_run_id, step_run_id, template_id, category_key,
		       channel, provider_id, rendered_content, ai_generated, status, attempts, provider_response, delivered_at, read_at, clicked_at,
		       bounced_at, suppression_reason, batch_id, scheduled_at, created_at
		FROM notification_messages
		WHERE project_id = $1
		  AND ($2::text IS NULL OR status = $2)
		  AND ($3::timestamptz IS NULL OR created_at < $3)
		ORDER BY created_at DESC
		LIMIT $4`

	var statusValue any
	if status != nil {
		statusValue = *status
	}
	var cursorValue any
	if cursor != nil {
		cursorValue = *cursor
	}

	rows, err := q.db.Query(ctx, query, projectID, statusValue, cursorValue, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification messages by project: %w", err)
	}
	defer rows.Close()

	messages := make([]domain.NotificationMessage, 0, limit)
	for rows.Next() {
		msg, scanErr := scanNotificationMessage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list notification messages by project scan: %w", scanErr)
		}
		messages = append(messages, *msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification messages by project rows: %w", err)
	}

	return messages, nil
}

func (q *Queries) ClaimDueScheduledNotificationMessages(ctx context.Context, limit int) ([]domain.NotificationMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimDueScheduledNotificationMessages")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}

	query := `
		WITH due AS (
			SELECT id
			FROM notification_messages
			WHERE status = 'scheduled'
			  AND scheduled_at IS NOT NULL
			  AND scheduled_at <= NOW()
			ORDER BY scheduled_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE notification_messages nm
		SET status = $2,
		    attempts = nm.attempts + 1
		FROM due
		WHERE nm.id = due.id
		RETURNING nm.id, nm.project_id, nm.idempotency_key, nm.recipient_type, nm.recipient_id, nm.tenant_id,
		          nm.workflow_run_id, nm.step_run_id, nm.template_id, nm.category_key,
		          nm.channel, nm.provider_id, nm.rendered_content, nm.ai_generated, nm.status, nm.attempts,
		          nm.provider_response, nm.delivered_at, nm.read_at, nm.clicked_at,
		          nm.bounced_at, nm.suppression_reason, nm.batch_id, nm.scheduled_at, nm.created_at`

	rows, err := q.db.Query(ctx, query, limit, domain.NotifyMessageStatusProcessing)
	if err != nil {
		return nil, fmt.Errorf("claim due scheduled notification messages: %w", err)
	}
	defer rows.Close()

	messages := make([]domain.NotificationMessage, 0, limit)
	for rows.Next() {
		msg, scanErr := scanNotificationMessage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("claim due scheduled notification messages scan: %w", scanErr)
		}
		messages = append(messages, *msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim due scheduled notification messages rows: %w", err)
	}

	return messages, nil
}

func (q *Queries) UpdateNotificationMessageStatus(ctx context.Context, id, projectID, fromStatus, toStatus string, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateNotificationMessageStatus")
	defer span.End()

	if toStatus == "" {
		return &domain.FieldError{Field: "status"}
	}

	allowedColumns := map[string]struct{}{
		"attempts":           {},
		"provider_id":        {},
		"rendered_content":   {},
		"provider_response":  {},
		"delivered_at":       {},
		"read_at":            {},
		"clicked_at":         {},
		"bounced_at":         {},
		"suppression_reason": {},
		"batch_id":           {},
		"scheduled_at":       {},
	}

	setClauses := []string{"status = $1"}
	args := []any{toStatus, id, projectID}
	param := 4

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
		if key == "provider_id" || key == "suppression_reason" || key == "batch_id" {
			if text, ok := value.(string); ok {
				value = dbscan.NilIfEmptyString(text)
			}
		}

		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, param))
		args = append(args, value)
		param++
	}

	query := fmt.Sprintf("UPDATE notification_messages SET %s WHERE id = $2 AND project_id = $3", strings.Join(setClauses, ", "))
	if fromStatus != "" {
		query = fmt.Sprintf("%s AND status = $%d", query, param)
		args = append(args, fromStatus)
	}

	tag, err := q.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update notification message status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if fromStatus == "" {
			return ErrNotificationMessageNotFound
		}
		return fmt.Errorf("%w: id %s from %s", ErrNotificationMessageStatusConflict, id, fromStatus)
	}

	return nil
}

func scanNotificationMessage(scanner scanTarget) (*domain.NotificationMessage, error) {
	var msg domain.NotificationMessage
	var idempotencyKey *string
	var tenantID *string
	var workflowRunID *string
	var stepRunID *string
	var templateID *string
	var categoryKey *string
	var providerID *string
	var renderedContent []byte
	var providerResponse []byte
	var suppressionReason *string
	var batchID *string

	err := scanner.Scan(
		&msg.ID,
		&msg.ProjectID,
		&idempotencyKey,
		&msg.RecipientType,
		&msg.RecipientID,
		&tenantID,
		&workflowRunID,
		&stepRunID,
		&templateID,
		&categoryKey,
		&msg.Channel,
		&providerID,
		&renderedContent,
		&msg.AIGenerated,
		&msg.Status,
		&msg.Attempts,
		&providerResponse,
		&msg.DeliveredAt,
		&msg.ReadAt,
		&msg.ClickedAt,
		&msg.BouncedAt,
		&suppressionReason,
		&batchID,
		&msg.ScheduledAt,
		&msg.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if idempotencyKey != nil {
		msg.IdempotencyKey = *idempotencyKey
	}
	if tenantID != nil {
		msg.TenantID = *tenantID
	}
	if workflowRunID != nil {
		msg.WorkflowRunID = *workflowRunID
	}
	if stepRunID != nil {
		msg.StepRunID = *stepRunID
	}
	if templateID != nil {
		msg.TemplateID = *templateID
	}
	if categoryKey != nil {
		msg.CategoryKey = *categoryKey
	}
	if providerID != nil {
		msg.ProviderID = *providerID
	}
	if len(renderedContent) > 0 {
		msg.RenderedContent = renderedContent
	}
	if len(providerResponse) > 0 {
		msg.ProviderResponse = providerResponse
	}
	if suppressionReason != nil {
		msg.SuppressionReason = *suppressionReason
	}
	if batchID != nil {
		msg.BatchID = *batchID
	}

	return &msg, nil
}
