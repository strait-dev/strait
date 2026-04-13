package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func uniqueJobIDs(entries []OutboxEntry) []string {
	seen := make(map[string]struct{}, len(entries))
	var out []string
	for _, e := range entries {
		if _, ok := seen[e.JobID]; ok {
			continue
		}
		seen[e.JobID] = struct{}{}
		out = append(out, e.JobID)
	}
	return out
}

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

// ErrOutboxJobNotFound is returned by WriteOutboxInTx when a job_id
// in the entry batch does not exist in the jobs table.
var ErrOutboxJobNotFound = errors.New("outbox: job not found")

// WriteOutboxInTx inserts outbox entries within the caller's transaction.
// Auto-generates IDs if unset; idempotent on primary-key conflict so the
// caller's retry loop doesn't double-write. Validates that every
// referenced job_id exists.
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
	// Validate all unique job_ids exist in a single round trip.
	jobIDs := uniqueJobIDs(entries)
	rows, err := tx.Query(ctx, `SELECT id FROM jobs WHERE id = ANY($1)`, jobIDs)
	if err != nil {
		return fmt.Errorf("validate job_ids: %w", err)
	}
	defer rows.Close()
	found := make(map[string]struct{}, len(jobIDs))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan job_id: %w", err)
		}
		found[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("validate job_ids: %w", err)
	}
	for _, jid := range jobIDs {
		if _, ok := found[jid]; !ok {
			return fmt.Errorf("%w: %s", ErrOutboxJobNotFound, jid)
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
