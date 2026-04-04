package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateNotificationTemplate(ctx context.Context, tmpl *domain.NotificationTemplate) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateNotificationTemplate")
	defer span.End()

	if tmpl.ID == "" {
		tmpl.ID = uuid.Must(uuid.NewV7()).String()
	}
	if tmpl.Version == 0 {
		tmpl.Version = 1
	}
	if tmpl.DefaultLocale == "" {
		tmpl.DefaultLocale = "en"
	}
	if tmpl.Status == "" {
		tmpl.Status = "active"
	}
	if len(tmpl.Variables) == 0 {
		tmpl.Variables = []byte("[]")
	}
	if len(tmpl.LocaleTemplates) == 0 {
		tmpl.LocaleTemplates = []byte("{}")
	}

	query := `
		INSERT INTO notification_templates (
			id, project_id, template_key, name, description, version, channels, variables, locale_templates, default_locale, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(
		ctx,
		query,
		tmpl.ID,
		tmpl.ProjectID,
		tmpl.TemplateKey,
		tmpl.Name,
		dbscan.NilIfEmptyString(tmpl.Description),
		tmpl.Version,
		tmpl.Channels,
		tmpl.Variables,
		tmpl.LocaleTemplates,
		tmpl.DefaultLocale,
		tmpl.Status,
	).Scan(&tmpl.CreatedAt, &tmpl.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create notification template: %w", err)
	}

	return nil
}

func (q *Queries) GetNotificationTemplateByID(ctx context.Context, id, projectID string) (*domain.NotificationTemplate, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetNotificationTemplateByID")
	defer span.End()

	query := `
		SELECT id, project_id, template_key, name, description, version, channels, variables, locale_templates, default_locale, status, created_at, updated_at
		FROM notification_templates
		WHERE id = $1 AND project_id = $2`

	tmpl, err := scanNotificationTemplate(q.db.QueryRow(ctx, query, id, projectID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotificationTemplateNotFound
		}
		return nil, fmt.Errorf("get notification template by ID: %w", err)
	}

	return tmpl, nil
}

func (q *Queries) GetLatestNotificationTemplateByKey(ctx context.Context, projectID, templateKey string) (*domain.NotificationTemplate, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetLatestNotificationTemplateByKey")
	defer span.End()

	query := `
		SELECT id, project_id, template_key, name, description, version, channels, variables, locale_templates, default_locale, status, created_at, updated_at
		FROM notification_templates
		WHERE project_id = $1 AND template_key = $2
		ORDER BY version DESC
		LIMIT 1`

	tmpl, err := scanNotificationTemplate(q.db.QueryRow(ctx, query, projectID, templateKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotificationTemplateNotFound
		}
		return nil, fmt.Errorf("get latest notification template by key: %w", err)
	}

	return tmpl, nil
}

func (q *Queries) ListNotificationTemplates(ctx context.Context, projectID string, status *string, limit int, cursor *time.Time) ([]domain.NotificationTemplate, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotificationTemplates")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, project_id, template_key, name, description, version, channels, variables, locale_templates, default_locale, status, created_at, updated_at
		FROM notification_templates
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
		return nil, fmt.Errorf("list notification templates: %w", err)
	}
	defer rows.Close()

	templates := make([]domain.NotificationTemplate, 0, limit)
	for rows.Next() {
		tmpl, scanErr := scanNotificationTemplate(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list notification templates scan: %w", scanErr)
		}
		templates = append(templates, *tmpl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification templates rows: %w", err)
	}

	return templates, nil
}

func scanNotificationTemplate(scanner scanTarget) (*domain.NotificationTemplate, error) {
	var tmpl domain.NotificationTemplate
	var description *string
	var variables []byte
	var localeTemplates []byte

	err := scanner.Scan(
		&tmpl.ID,
		&tmpl.ProjectID,
		&tmpl.TemplateKey,
		&tmpl.Name,
		&description,
		&tmpl.Version,
		&tmpl.Channels,
		&variables,
		&localeTemplates,
		&tmpl.DefaultLocale,
		&tmpl.Status,
		&tmpl.CreatedAt,
		&tmpl.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if description != nil {
		tmpl.Description = *description
	}
	if len(variables) == 0 {
		tmpl.Variables = []byte("[]")
	} else {
		tmpl.Variables = variables
	}
	if len(localeTemplates) == 0 {
		tmpl.LocaleTemplates = []byte("{}")
	} else {
		tmpl.LocaleTemplates = localeTemplates
	}

	return &tmpl, nil
}
