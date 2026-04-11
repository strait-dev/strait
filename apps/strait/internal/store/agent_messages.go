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

func scanAgentMessage(scanner interface{ Scan(dest ...any) error }) (*domain.AgentMessage, error) {
	var msg domain.AgentMessage
	var sourceRunID *string
	var errMsg *string
	if err := scanner.Scan(
		&msg.ID, &msg.ProjectID, &msg.SourceAgentID, &msg.TargetAgentID,
		&sourceRunID, &msg.ChainID, &msg.ChainDepth, &msg.Payload, &msg.Status,
		&msg.CreatedAt, &msg.DeliveredAt, &errMsg,
	); err != nil {
		return nil, err
	}
	if sourceRunID != nil {
		msg.SourceRunID = *sourceRunID
	}
	if errMsg != nil {
		msg.Error = *errMsg
	}
	return &msg, nil
}

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

	msg, err := scanAgentMessage(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgentMessageNotFound
		}
		return nil, err
	}
	return msg, nil
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

// maxAgentMessagesPerChain bounds ListAgentMessagesByChain so a
// pathological writer cannot inflate a chain and stall a reader.
// The chain_depth check constraint on agent_messages caps real chains
// at 20, so 100 is a generous ceiling that leaves room for parallel
// siblings at the same depth.
const maxAgentMessagesPerChain = 100

func (q *Queries) ListAgentMessagesByChain(ctx context.Context, chainID string) ([]domain.AgentMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAgentMessagesByChain")
	defer span.End()

	query := `
		SELECT id, project_id, source_agent_id, target_agent_id,
			source_run_id, chain_id, chain_depth, payload, status,
			created_at, delivered_at, error
		FROM agent_messages
		WHERE chain_id = $1
		ORDER BY chain_depth ASC
		LIMIT $2`

	rows, err := q.db.Query(ctx, query, chainID, maxAgentMessagesPerChain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.AgentMessage
	for rows.Next() {
		msg, scanErr := scanAgentMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		messages = append(messages, *msg)
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
		msg, scanErr := scanAgentMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		messages = append(messages, *msg)
	}
	return messages, rows.Err()
}

// AgentMessageCursor is a compound cursor for paginating
// ListAgentMessagesByAgent. Pagination-by-timestamp alone isn't safe
// because two messages in the same agent chain can share a single
// millisecond (back-to-back inserts in a tight loop). The `ID`
// tie-breaker guarantees no-duplicate no-gap paging even under
// collision. Callers construct a cursor from the last row of the
// previous page: `&AgentMessageCursor{CreatedAt: last.CreatedAt, ID: last.ID}`.
type AgentMessageCursor struct {
	CreatedAt time.Time
	ID        string
}

// ListAgentMessagesByAgent returns messages where the agent appears as
// either source or target, newest first. Rewritten as a UNION ALL of
// two index-backed subqueries because the original
// WHERE (source_agent_id = $1 OR target_agent_id = $1) clause couldn't
// use either of idx_agent_messages_source_created or
// idx_agent_messages_target. Each branch contributes up to `limit` rows;
// the outer ORDER BY + LIMIT merges and trims to the final `limit`.
//
// Cursor pagination uses a row-value comparison on (created_at, id)
// so two messages with identical created_at values still paginate
// deterministically — see AgentMessageCursor for the rationale.
func (q *Queries) ListAgentMessagesByAgent(ctx context.Context, agentID string, limit int, cursor *AgentMessageCursor) ([]domain.AgentMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAgentMessagesByAgent")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	const columns = `id, project_id, source_agent_id, target_agent_id,
		source_run_id, chain_id, chain_depth, payload, status,
		created_at, delivered_at, error`

	var query string
	args := []any{agentID}
	if cursor != nil {
		// Row-value comparison: (created_at, id) < ($2, $3) is the
		// Postgres idiom for "strictly earlier than this row". Works
		// tuple-wise: if created_at < $2, the row matches; if
		// created_at = $2, then id < $3 must hold.
		query = `
			(SELECT ` + columns + `
			 FROM agent_messages
			 WHERE source_agent_id = $1 AND (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC, id DESC
			 LIMIT $4)
			UNION ALL
			(SELECT ` + columns + `
			 FROM agent_messages
			 WHERE target_agent_id = $1 AND (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC, id DESC
			 LIMIT $4)
			ORDER BY created_at DESC, id DESC
			LIMIT $4`
		args = append(args, cursor.CreatedAt, cursor.ID, limit)
	} else {
		query = `
			(SELECT ` + columns + `
			 FROM agent_messages
			 WHERE source_agent_id = $1
			 ORDER BY created_at DESC, id DESC
			 LIMIT $2)
			UNION ALL
			(SELECT ` + columns + `
			 FROM agent_messages
			 WHERE target_agent_id = $1
			 ORDER BY created_at DESC, id DESC
			 LIMIT $2)
			ORDER BY created_at DESC, id DESC
			LIMIT $2`
		args = append(args, limit)
	}

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.AgentMessage
	for rows.Next() {
		msg, scanErr := scanAgentMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		messages = append(messages, *msg)
	}
	return messages, rows.Err()
}

// AgentMessageEdge represents a directed edge between two agents with a message count.
type AgentMessageEdge struct {
	SourceAgentID string `json:"source_agent_id"`
	TargetAgentID string `json:"target_agent_id"`
	MessageCount  int    `json:"message_count"`
}

// maxAgentTopologyEdges caps GetAgentTopologyEdges. The endpoint is a
// visualization query (network graph of agent-to-agent message flow);
// callers that need an exhaustive count should iterate on
// ListAgentMessagesByAgent with a cursor instead.
const maxAgentTopologyEdges = 500

// GetAgentTopologyEdges returns the directed edges between agents for a
// project, grouped by source/target with message counts. Bounded to
// maxAgentTopologyEdges to keep the UI graph render cheap on chatty
// projects; order is by message_count DESC so the busiest edges win.
func (q *Queries) GetAgentTopologyEdges(ctx context.Context, projectID string) ([]AgentMessageEdge, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAgentTopologyEdges")
	defer span.End()

	query := `
		SELECT source_agent_id, target_agent_id, COUNT(*) AS message_count
		FROM agent_messages
		WHERE project_id = $1 AND source_agent_id != ''
		GROUP BY source_agent_id, target_agent_id
		ORDER BY message_count DESC
		LIMIT $2`

	rows, err := q.db.Query(ctx, query, projectID, maxAgentTopologyEdges)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []AgentMessageEdge
	for rows.Next() {
		var e AgentMessageEdge
		if err := rows.Scan(&e.SourceAgentID, &e.TargetAgentID, &e.MessageCount); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

func argPlaceholder(n int) string {
	return "$" + strconv.Itoa(n)
}
