package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

var ErrAgentMessageNotFound = errors.New("agent message not found")

func (q *Queries) CreateAgentMessage(ctx context.Context, msg *domain.AgentMessage) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAgentMessage")
	defer span.End()

	if msg.ID == "" {
		msg.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO agent_messages (
			id, project_id, source_agent_id, target_agent_id,
			source_run_id, chain_id, chain_depth, payload, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at`

	return q.db.QueryRow(ctx, query,
		msg.ID, msg.ProjectID, msg.SourceAgentID, msg.TargetAgentID,
		msg.SourceRunID, msg.ChainID, msg.ChainDepth, msg.Payload, msg.Status,
	).Scan(&msg.CreatedAt)
}

func (q *Queries) GetAgentMessage(ctx context.Context, id string) (*domain.AgentMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAgentMessage")
	defer span.End()

	query := `
		SELECT id, project_id, source_agent_id, target_agent_id,
			source_run_id, chain_id, chain_depth, payload, status,
			created_at, delivered_at, error
		FROM agent_messages WHERE id = $1`

	var msg domain.AgentMessage
	err := q.db.QueryRow(ctx, query, id).Scan(
		&msg.ID, &msg.ProjectID, &msg.SourceAgentID, &msg.TargetAgentID,
		&msg.SourceRunID, &msg.ChainID, &msg.ChainDepth, &msg.Payload, &msg.Status,
		&msg.CreatedAt, &msg.DeliveredAt, &msg.Error,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgentMessageNotFound
		}
		return nil, err
	}
	return &msg, nil
}

func (q *Queries) UpdateAgentMessageStatus(ctx context.Context, id string, status domain.AgentMessageStatus, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateAgentMessageStatus")
	defer span.End()

	query := `UPDATE agent_messages SET status = $2`
	args := []any{id, status}
	argIdx := 3

	if deliveredAt, ok := fields["delivered_at"]; ok {
		query += `, delivered_at = ` + argPlaceholder(argIdx)
		args = append(args, deliveredAt)
		argIdx++
	}
	if errMsg, ok := fields["error"]; ok {
		query += `, error = ` + argPlaceholder(argIdx)
		args = append(args, errMsg)
		_ = argIdx // lint: argIdx may be incremented for future fields
	}

	query += ` WHERE id = $1`
	_, err := q.db.Exec(ctx, query, args...)
	return err
}

func (q *Queries) ListAgentMessagesByChain(ctx context.Context, chainID string) ([]domain.AgentMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAgentMessagesByChain")
	defer span.End()

	query := `
		SELECT id, project_id, source_agent_id, target_agent_id,
			source_run_id, chain_id, chain_depth, payload, status,
			created_at, delivered_at, error
		FROM agent_messages
		WHERE chain_id = $1
		ORDER BY chain_depth ASC`

	rows, err := q.db.Query(ctx, query, chainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.AgentMessage
	for rows.Next() {
		var msg domain.AgentMessage
		if err := rows.Scan(
			&msg.ID, &msg.ProjectID, &msg.SourceAgentID, &msg.TargetAgentID,
			&msg.SourceRunID, &msg.ChainID, &msg.ChainDepth, &msg.Payload, &msg.Status,
			&msg.CreatedAt, &msg.DeliveredAt, &msg.Error,
		); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (q *Queries) ListPendingAgentMessages(ctx context.Context, limit int) ([]domain.AgentMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListPendingAgentMessages")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, project_id, source_agent_id, target_agent_id,
			source_run_id, chain_id, chain_depth, payload, status,
			created_at, delivered_at, error
		FROM agent_messages
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1`

	rows, err := q.db.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.AgentMessage
	for rows.Next() {
		var msg domain.AgentMessage
		if err := rows.Scan(
			&msg.ID, &msg.ProjectID, &msg.SourceAgentID, &msg.TargetAgentID,
			&msg.SourceRunID, &msg.ChainID, &msg.ChainDepth, &msg.Payload, &msg.Status,
			&msg.CreatedAt, &msg.DeliveredAt, &msg.Error,
		); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (q *Queries) ListAgentMessagesByAgent(ctx context.Context, agentID string, limit int, cursor *time.Time) ([]domain.AgentMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAgentMessagesByAgent")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, project_id, source_agent_id, target_agent_id,
			source_run_id, chain_id, chain_depth, payload, status,
			created_at, delivered_at, error
		FROM agent_messages
		WHERE (source_agent_id = $1 OR target_agent_id = $1)`

	args := []any{agentID}
	if cursor != nil {
		query += ` AND created_at < $2 ORDER BY created_at DESC LIMIT $3`
		args = append(args, *cursor, limit)
	} else {
		query += ` ORDER BY created_at DESC LIMIT $2`
		args = append(args, limit)
	}

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.AgentMessage
	for rows.Next() {
		var msg domain.AgentMessage
		if err := rows.Scan(
			&msg.ID, &msg.ProjectID, &msg.SourceAgentID, &msg.TargetAgentID,
			&msg.SourceRunID, &msg.ChainID, &msg.ChainDepth, &msg.Payload, &msg.Status,
			&msg.CreatedAt, &msg.DeliveredAt, &msg.Error,
		); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func argPlaceholder(n int) string {
	return "$" + strconv.Itoa(n)
}
