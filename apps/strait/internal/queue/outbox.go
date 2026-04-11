package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// R3 Phase 8: transactional outbox primitive.
//
// Callers that have their own DB transaction can write an OutboxEntry
// to the enqueue_outbox table as part of that transaction, guaranteeing
// atomic commit of user state and enqueue intent. The scheduler's
// OutboxFlusher then moves those rows into job_runs asynchronously
// with SKIP LOCKED so multiple flushers are safe.

// OutboxEntry describes a future enqueue.
type OutboxEntry struct {
	ID             string
	ProjectID      string
	JobID          string
	Payload        json.RawMessage
	Metadata       map[string]any
	IdempotencyKey string
	ScheduledAt    *time.Time
	Priority       int
}

// WriteOutboxInTx inserts outbox entries within the caller's transaction.
// Auto-generates IDs if unset; idempotent on primary-key conflict so the
// caller's retry loop doesn't double-write.
func WriteOutboxInTx(ctx context.Context, tx pgx.Tx, entries []OutboxEntry) error {
	if len(entries) == 0 {
		return nil
	}
	for i := range entries {
		if entries[i].ID == "" {
			entries[i].ID = uuid.Must(uuid.NewV7()).String()
		}
		if entries[i].ProjectID == "" {
			return fmt.Errorf("outbox entry %d: project_id required", i)
		}
		if entries[i].JobID == "" {
			return fmt.Errorf("outbox entry %d: job_id required", i)
		}
	}
	const sql = `
		INSERT INTO enqueue_outbox (
			id, project_id, job_id, payload, metadata,
			idempotency_key, scheduled_at, priority, created_at
		) VALUES (
			$1, $2, $3, $4, $5::jsonb, $6, $7, $8, NOW()
		)
		ON CONFLICT (id) DO NOTHING`
	for _, e := range entries {
		metaBytes, err := json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		if len(e.Metadata) == 0 {
			metaBytes = []byte("{}")
		}
		_, err = tx.Exec(ctx, sql,
			e.ID, e.ProjectID, e.JobID, jsonBytes(e.Payload), metaBytes,
			nullableString(e.IdempotencyKey), e.ScheduledAt, e.Priority,
		)
		if err != nil {
			return fmt.Errorf("write outbox entry %s: %w", e.ID, err)
		}
	}
	return nil
}

// jsonBytes returns nil when the payload is empty, so the INSERT binds
// SQL NULL rather than the string "null".
func jsonBytes(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return []byte(b)
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
