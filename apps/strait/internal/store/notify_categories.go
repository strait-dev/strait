package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateNotificationCategory(ctx context.Context, cat *domain.NotificationCategory) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateNotificationCategory")
	defer span.End()

	if cat.ID == "" {
		cat.ID = uuid.Must(uuid.NewV7()).String()
	}
	if cat.Type == "" {
		cat.Type = domain.NotifyCategoryTypeProduct
	}

	query := `
		INSERT INTO notification_categories (id, project_id, category_key, name, description, type)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		cat.ID,
		cat.ProjectID,
		cat.CategoryKey,
		cat.Name,
		dbscan.NilIfEmptyString(cat.Description),
		cat.Type,
	).Scan(&cat.CreatedAt)
	if err != nil {
		return fmt.Errorf("create notification category: %w", err)
	}

	return nil
}

func (q *Queries) GetNotificationCategoryByKey(ctx context.Context, projectID, categoryKey string) (*domain.NotificationCategory, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetNotificationCategoryByKey")
	defer span.End()

	query := `
		SELECT id, project_id, category_key, name, description, type, created_at
		FROM notification_categories
		WHERE project_id = $1 AND category_key = $2`

	cat, err := scanNotificationCategory(q.db.QueryRow(ctx, query, projectID, categoryKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotificationCategoryNotFound
		}
		return nil, fmt.Errorf("get notification category by key: %w", err)
	}

	return cat, nil
}

func (q *Queries) ListNotificationCategories(ctx context.Context, projectID string) ([]domain.NotificationCategory, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotificationCategories")
	defer span.End()

	query := `
		SELECT id, project_id, category_key, name, description, type, created_at
		FROM notification_categories
		WHERE project_id = $1
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list notification categories: %w", err)
	}
	defer rows.Close()

	categories := make([]domain.NotificationCategory, 0, 16)
	for rows.Next() {
		cat, scanErr := scanNotificationCategory(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list notification categories scan: %w", scanErr)
		}
		categories = append(categories, *cat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification categories rows: %w", err)
	}

	return categories, nil
}

func scanNotificationCategory(scanner scanTarget) (*domain.NotificationCategory, error) {
	var cat domain.NotificationCategory
	var description *string

	err := scanner.Scan(
		&cat.ID,
		&cat.ProjectID,
		&cat.CategoryKey,
		&cat.Name,
		&description,
		&cat.Type,
		&cat.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if description != nil {
		cat.Description = *description
	}

	return &cat, nil
}
