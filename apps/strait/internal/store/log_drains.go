package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

var ErrLogDrainNotFound = errors.New("log drain not found")

func (q *Queries) CreateLogDrain(ctx context.Context, drain *domain.LogDrain) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateLogDrain")
	defer span.End()

	authConfigJSON, _ := json.Marshal(drain.AuthConfig)

	levelFilter := drain.LevelFilter
	if levelFilter == nil {
		levelFilter = []string{}
	}

	_, err := q.db.Exec(ctx, `
		INSERT INTO log_drains (id, project_id, name, drain_type, endpoint_url, auth_type, auth_config, level_filter, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, drain.ID, drain.ProjectID, drain.Name, drain.DrainType, drain.EndpointURL,
		drain.AuthType, authConfigJSON, levelFilter, drain.Enabled)
	if err != nil {
		return fmt.Errorf("create log drain: %w", err)
	}
	return nil
}

func (q *Queries) GetLogDrain(ctx context.Context, drainID, projectID string) (*domain.LogDrain, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetLogDrain")
	defer span.End()

	var d domain.LogDrain
	var authConfigJSON []byte
	err := q.db.QueryRow(ctx, `
		SELECT id, project_id, name, drain_type, endpoint_url, auth_type, auth_config, level_filter, enabled, created_at, updated_at
		FROM log_drains WHERE id = $1 AND project_id = $2
	`, drainID, projectID).Scan(
		&d.ID, &d.ProjectID, &d.Name, &d.DrainType, &d.EndpointURL,
		&d.AuthType, &authConfigJSON, &d.LevelFilter, &d.Enabled, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrLogDrainNotFound
		}
		return nil, fmt.Errorf("get log drain: %w", err)
	}
	if authConfigJSON != nil {
		_ = json.Unmarshal(authConfigJSON, &d.AuthConfig)
	}
	return &d, nil
}

func (q *Queries) ListLogDrains(ctx context.Context, projectID string) ([]domain.LogDrain, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListLogDrains")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT id, project_id, name, drain_type, endpoint_url, auth_type, auth_config, level_filter, enabled, created_at, updated_at
		FROM log_drains WHERE project_id = $1 ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list log drains: %w", err)
	}
	defer rows.Close()

	drains := make([]domain.LogDrain, 0)
	for rows.Next() {
		var d domain.LogDrain
		var authConfigJSON []byte
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Name, &d.DrainType, &d.EndpointURL,
			&d.AuthType, &authConfigJSON, &d.LevelFilter, &d.Enabled, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list log drains scan: %w", err)
		}
		if authConfigJSON != nil {
			_ = json.Unmarshal(authConfigJSON, &d.AuthConfig)
		}
		drains = append(drains, d)
	}
	return drains, rows.Err()
}

func (q *Queries) ListEnabledLogDrains(ctx context.Context) ([]domain.LogDrain, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEnabledLogDrains")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT id, project_id, name, drain_type, endpoint_url, auth_type, auth_config, level_filter, enabled, created_at, updated_at
		FROM log_drains WHERE enabled = true ORDER BY created_at DESC LIMIT 500
	`)
	if err != nil {
		return nil, fmt.Errorf("list enabled log drains: %w", err)
	}
	defer rows.Close()

	drains := make([]domain.LogDrain, 0)
	for rows.Next() {
		var d domain.LogDrain
		var authConfigJSON []byte
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Name, &d.DrainType, &d.EndpointURL,
			&d.AuthType, &authConfigJSON, &d.LevelFilter, &d.Enabled, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list enabled log drains scan: %w", err)
		}
		if authConfigJSON != nil {
			_ = json.Unmarshal(authConfigJSON, &d.AuthConfig)
		}
		drains = append(drains, d)
	}
	return drains, rows.Err()
}

func (q *Queries) UpdateLogDrain(ctx context.Context, drainID, projectID string, patch map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateLogDrain")
	defer span.End()

	allowedColumns := map[string]struct{}{
		"name":         {},
		"drain_type":   {},
		"endpoint_url": {},
		"auth_type":    {},
		"auth_config":  {},
		"level_filter": {},
		"enabled":      {},
		"updated_at":   {},
	}

	patch["updated_at"] = time.Now()

	setClauses := make([]string, 0, len(patch))
	args := make([]any, 0, 2+len(patch))
	args = append(args, drainID, projectID)
	param := 3
	for k, v := range patch {
		if _, ok := allowedColumns[k]; !ok {
			return &domain.FieldError{Field: k}
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, param))
		args = append(args, v)
		param++
	}

	query := fmt.Sprintf("UPDATE log_drains SET %s WHERE id = $1 AND project_id = $2",
		strings.Join(setClauses, ", "))

	result, err := q.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update log drain: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrLogDrainNotFound
	}
	return nil
}

func (q *Queries) DeleteLogDrain(ctx context.Context, drainID, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteLogDrain")
	defer span.End()

	result, err := q.db.Exec(ctx, `DELETE FROM log_drains WHERE id = $1 AND project_id = $2`, drainID, projectID)
	if err != nil {
		return fmt.Errorf("delete log drain: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrLogDrainNotFound
	}
	return nil
}
