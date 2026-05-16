package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) UpsertDebouncePending(ctx context.Context, d *domain.DebouncePending) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertDebouncePending")
	defer span.End()

	if d.ID == "" {
		d.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO debounce_pending (id, job_id, project_id, debounce_key, payload, tags, priority, concurrency_key, ttl_secs, triggered_by, created_by, fire_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (job_id, debounce_key)
		DO UPDATE SET payload = EXCLUDED.payload, tags = EXCLUDED.tags, priority = EXCLUDED.priority,
		              concurrency_key = EXCLUDED.concurrency_key, ttl_secs = EXCLUDED.ttl_secs,
		              triggered_by = EXCLUDED.triggered_by, created_by = EXCLUDED.created_by,
		              fire_at = EXCLUDED.fire_at
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query,
		d.ID, d.JobID, d.ProjectID, d.DebounceKey, d.Payload, d.Tags,
		d.Priority, nilIfEmpty(d.ConcurrencyKey), d.TTLSecs, d.TriggeredBy, nilIfEmpty(d.CreatedBy), d.FireAt,
	).Scan(&d.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert debounce pending: %w", err)
	}
	return nil
}

func (q *Queries) ListDueDebouncePending(ctx context.Context) ([]domain.DebouncePending, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDueDebouncePending")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, debounce_key, payload, tags, priority, concurrency_key, ttl_secs, triggered_by, created_by, fire_at, created_at
		FROM debounce_pending
		WHERE fire_at <= NOW()
		ORDER BY fire_at ASC
		LIMIT 100`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list due debounce pending: %w", err)
	}
	defer rows.Close()

	items := make([]domain.DebouncePending, 0, 16)
	for rows.Next() {
		d, scanErr := scanDebouncePending(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list due debounce pending scan: %w", scanErr)
		}
		items = append(items, *d)
	}
	return items, rows.Err()
}

func (q *Queries) DeleteDebouncePending(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteDebouncePending")
	defer span.End()

	_, err := q.db.Exec(ctx, `DELETE FROM debounce_pending WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete debounce pending: %w", err)
	}
	return nil
}

func (q *Queries) ClaimDueDebouncePending(ctx context.Context, id string) (*domain.DebouncePending, bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimDueDebouncePending")
	defer span.End()

	row := q.db.QueryRow(ctx, `
		DELETE FROM debounce_pending
		WHERE id = $1
		  AND fire_at <= NOW()
		RETURNING id, job_id, project_id, debounce_key, payload, tags, priority, concurrency_key, ttl_secs, triggered_by, created_by, fire_at, created_at
	`, id)
	d, err := scanDebouncePending(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("claim due debounce pending: %w", err)
	}
	return d, true, nil
}

func (q *Queries) InsertDebouncePendingIfAbsent(ctx context.Context, d *domain.DebouncePending) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.InsertDebouncePendingIfAbsent")
	defer span.End()

	if d.ID == "" {
		d.ID = uuid.Must(uuid.NewV7()).String()
	}
	tag, err := q.db.Exec(ctx, `
		INSERT INTO debounce_pending (id, job_id, project_id, debounce_key, payload, tags, priority, concurrency_key, ttl_secs, triggered_by, created_by, fire_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (job_id, debounce_key) DO NOTHING
	`, d.ID, d.JobID, d.ProjectID, d.DebounceKey, d.Payload, d.Tags,
		d.Priority, nilIfEmpty(d.ConcurrencyKey), d.TTLSecs, d.TriggeredBy, nilIfEmpty(d.CreatedBy), d.FireAt,
	)
	if err != nil {
		return false, fmt.Errorf("insert debounce pending if absent: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func scanDebouncePending(row pgx.Row) (*domain.DebouncePending, error) {
	var d domain.DebouncePending
	var concurrencyKey *string
	var createdBy *string
	var tags []byte
	err := row.Scan(
		&d.ID, &d.JobID, &d.ProjectID, &d.DebounceKey, &d.Payload, &tags,
		&d.Priority, &concurrencyKey, &d.TTLSecs, &d.TriggeredBy, &createdBy, &d.FireAt, &d.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if concurrencyKey != nil {
		d.ConcurrencyKey = *concurrencyKey
	}
	if createdBy != nil {
		d.CreatedBy = *createdBy
	}
	if len(tags) > 0 {
		d.Tags = tags
	}
	return &d, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// DebounceStore defines debounce operations for the scheduler.
type DebounceStore interface {
	ListDueDebouncePending(ctx context.Context) ([]domain.DebouncePending, error)
	DeleteDebouncePending(ctx context.Context, id string) error
	ClaimDueDebouncePending(ctx context.Context, id string) (*domain.DebouncePending, bool, error)
	InsertDebouncePendingIfAbsent(ctx context.Context, d *domain.DebouncePending) (bool, error)
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetProjectQuota(ctx context.Context, projectID string) (*ProjectQuota, error)
	CountProjectQueuedRuns(ctx context.Context, projectID string) (int, error)
	CountProjectActiveRuns(ctx context.Context, projectID string) (int, error)
	CountRunsForJobSince(ctx context.Context, jobID string, since time.Time) (int, error)
	SumProjectDailyCostMicrousd(ctx context.Context, projectID, timezone string) (int64, error)
	CreateRun(ctx context.Context, run *domain.JobRun) error
	TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error)
	ReleaseAdvisoryLock(ctx context.Context, lockID int64) error
}

// BatchStore defines batch buffer operations for the scheduler.
type BatchStore interface {
	ListFlushableBatches(ctx context.Context) ([]FlushableBatch, error)
	DrainBatchBuffer(ctx context.Context, jobID, batchKey string, limit int) ([]domain.BatchBufferItem, error)
	ListBatchBufferItems(ctx context.Context, jobID, batchKey string, limit int) ([]domain.BatchBufferItem, error)
	DeleteBatchBufferItems(ctx context.Context, ids []string) error
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	CreateRun(ctx context.Context, run *domain.JobRun) error
	TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error)
	ReleaseAdvisoryLock(ctx context.Context, lockID int64) error
}

// FlushableBatch identifies a batch group that is ready to be flushed.
type FlushableBatch struct {
	JobID     string    `json:"job_id"`
	ProjectID string    `json:"project_id"`
	BatchKey  string    `json:"batch_key"`
	ItemCount int       `json:"item_count"`
	OldestAt  time.Time `json:"oldest_at"`
}
