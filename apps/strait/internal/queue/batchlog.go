package queue

import (
	"context"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/google/uuid"
)

const (
	EngineLegacy   = "legacy"
	EngineBatchlog = "batchlog"

	batchlogStatusReady  = "ready"
	batchlogStatusLeased = "leased"
)

type BatchlogConfig struct {
	TickInterval  time.Duration
	LeaseDuration time.Duration
	LeaseOwner    string
}

func (c BatchlogConfig) normalized() BatchlogConfig {
	if c.TickInterval <= 0 {
		c.TickInterval = 100 * time.Millisecond
	}
	if c.LeaseDuration <= 0 {
		c.LeaseDuration = 30 * time.Second
	}
	if c.LeaseOwner == "" {
		c.LeaseOwner = uuid.Must(uuid.NewV7()).String()
	}
	return c
}

func leaseExpired(now time.Time, expiresAt *time.Time) bool {
	return expiresAt != nil && !expiresAt.After(now)
}

type BatchlogQueue struct {
	db     store.DBTX
	legacy *PostgresQueue
	cfg    BatchlogConfig
}

func NewBatchlogQueue(db store.DBTX, legacy *PostgresQueue, cfg BatchlogConfig) *BatchlogQueue {
	if legacy == nil {
		legacy = NewPostgresQueue(db)
	}
	return &BatchlogQueue{db: db, legacy: legacy, cfg: cfg.normalized()}
}

func NewQueueEngine(db store.DBTX, engine string, cfg BatchlogConfig, opts ...PostgresQueueOption) (Queue, error) {
	legacy := NewPostgresQueue(db, opts...)
	switch engine {
	case "", EngineLegacy:
		return legacy, nil
	case EngineBatchlog:
		return NewBatchlogQueue(db, legacy, cfg), nil
	default:
		return nil, fmt.Errorf("unknown queue engine %q", engine)
	}
}

func (q *BatchlogQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	return q.legacy.Enqueue(ctx, run)
}

func (q *BatchlogQueue) EnqueueInTx(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
	return q.legacy.EnqueueInTx(ctx, tx, run)
}

func (q *BatchlogQueue) EnqueueBatch(ctx context.Context, runs []*domain.JobRun) (int64, error) {
	return q.legacy.EnqueueBatch(ctx, runs)
}

func (q *BatchlogQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	runs, err := q.DequeueN(ctx, 1)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return &runs[0], nil
}

func (q *BatchlogQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	return q.dequeueN(ctx, n, "")
}

func (q *BatchlogQueue) DequeueNFair(ctx context.Context, n int) ([]domain.JobRun, error) {
	return q.DequeueN(ctx, n)
}

func (q *BatchlogQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	return q.dequeueN(ctx, n, projectID)
}

func (q *BatchlogQueue) RunTicker(ctx context.Context) {
	ticker := time.NewTicker(q.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = q.ReclaimExpiredLeases(ctx)
			_, _ = q.SealDueBatches(ctx)
		}
	}
}

func (q *BatchlogQueue) BackfillDue(ctx context.Context) (int64, error) {
	tag, err := q.db.Exec(ctx, `
		INSERT INTO queue_entries (
			run_id,
			job_id,
			project_id,
			priority,
			run_created_at,
			available_at,
			status,
			concurrency_key,
			run_status,
			job_enabled,
			job_paused,
			job_max_concurrency,
			job_max_concurrency_per_key,
			scheduled_at,
			next_retry_at
		)
		SELECT
			jr.id,
			jr.job_id,
			jr.project_id,
			jr.priority,
			jr.created_at,
			GREATEST(
				COALESCE(jr.scheduled_at, '-infinity'::timestamptz),
				COALESCE(jr.next_retry_at, '-infinity'::timestamptz),
				jr.created_at
			),
			'ready',
			COALESCE(jr.concurrency_key, ''),
			jr.status,
			jr.job_enabled,
			jr.job_paused,
			jr.job_max_concurrency,
			jr.job_max_concurrency_per_key,
			jr.scheduled_at,
			jr.next_retry_at
		FROM job_runs jr
		WHERE jr.status = $1
		  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
		  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
		ON CONFLICT (run_id) DO NOTHING
	`, domain.StatusQueued)
	if err != nil {
		return 0, fmt.Errorf("batchlog backfill due: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *BatchlogQueue) SealDueBatches(ctx context.Context) (int64, error) {
	var batchID int64
	var sealed int64
	err := q.db.QueryRow(ctx, `
		WITH due AS (
			SELECT qe.run_id
			FROM queue_entries qe
			WHERE qe.status = 'ready'
			  AND qe.batch_id IS NULL
			  AND qe.available_at <= NOW()
			  AND qe.run_status = $1
			  AND (qe.scheduled_at IS NULL OR qe.scheduled_at <= NOW())
			  AND (qe.next_retry_at IS NULL OR qe.next_retry_at <= NOW())
			LIMIT 10000
		),
		next_batch AS (
			SELECT nextval('queue_batch_id_seq') AS id
			WHERE EXISTS (SELECT 1 FROM due)
		),
		updated AS (
			UPDATE queue_entries qe
			SET batch_id = nb.id, updated_at = NOW()
			FROM next_batch nb
			WHERE qe.run_id IN (SELECT run_id FROM due)
			RETURNING qe.run_id, nb.id
		)
		SELECT COALESCE(MAX(id), 0), COUNT(*) FROM updated
	`, domain.StatusQueued).Scan(&batchID, &sealed)
	if err != nil {
		return 0, fmt.Errorf("batchlog seal due batches: %w", err)
	}
	_ = batchID
	return sealed, nil
}

func (q *BatchlogQueue) ReclaimExpiredLeases(ctx context.Context) (int64, error) {
	tag, err := q.db.Exec(ctx, `
		UPDATE queue_entries qe
		SET status = 'ready',
		    lease_owner = NULL,
		    lease_expires_at = NULL,
		    updated_at = NOW()
		WHERE qe.status = 'leased'
		  AND qe.lease_expires_at <= NOW()
		  AND qe.run_status = $1
	`, domain.StatusQueued)
	if err != nil {
		return 0, fmt.Errorf("batchlog reclaim expired leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *BatchlogQueue) dequeueN(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	if n <= 0 {
		return nil, nil
	}

	projectClause := ""
	args := []any{n, q.cfg.LeaseOwner, q.cfg.LeaseDuration}
	if projectID != "" {
		projectClause = "AND qe.project_id = $4"
		args = append(args, projectID)
	}

	query := fmt.Sprintf(`
		WITH claimed AS (
			SELECT qe.run_id
			FROM queue_entries qe
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = qe.job_id AND jac_job.concurrency_key = ''
			LEFT JOIN job_active_counts jac_key
			  ON jac_key.job_id = qe.job_id
			  AND jac_key.concurrency_key = qe.concurrency_key
			LEFT JOIN job_batchlog_lease_counts jlc_job
			  ON jlc_job.job_id = qe.job_id AND jlc_job.concurrency_key = ''
			LEFT JOIN job_batchlog_lease_counts jlc_key
			  ON jlc_key.job_id = qe.job_id
			  AND jlc_key.concurrency_key = qe.concurrency_key
			WHERE qe.status = 'ready'
			  AND qe.batch_id IS NOT NULL
			  AND qe.available_at <= NOW()
			  AND qe.run_status = '%s'
			  AND COALESCE(qe.job_enabled, true) = true
			  AND COALESCE(qe.job_paused, false) = false
			  AND (qe.scheduled_at IS NULL OR qe.scheduled_at <= NOW())
			  AND (qe.next_retry_at IS NULL OR qe.next_retry_at <= NOW())
			  AND (qe.job_max_concurrency IS NULL
			       OR COALESCE(jac_job.count, 0) + COALESCE(jlc_job.count, 0) < qe.job_max_concurrency)
			  AND (qe.job_max_concurrency_per_key IS NULL
			       OR qe.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) + COALESCE(jlc_key.count, 0) < qe.job_max_concurrency_per_key)
			  %s
			ORDER BY qe.batch_id ASC, qe.priority DESC, qe.run_created_at ASC
			FOR UPDATE OF qe SKIP LOCKED
			LIMIT $1
		),
		leased AS (
			UPDATE queue_entries qe
			SET status = 'leased',
			    lease_owner = $2,
			    lease_expires_at = NOW() + $3,
			    claimed_at = NOW(),
			    attempts = attempts + 1,
			    updated_at = NOW()
			FROM claimed
			WHERE qe.run_id = claimed.run_id
			RETURNING qe.run_id
		)
		SELECT %s
		FROM job_runs jr
		JOIN leased l ON l.run_id = jr.id
		ORDER BY jr.created_at ASC
	`, domain.StatusQueued, projectClause, dequeueColumns)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("batchlog dequeue: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, n)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("batchlog dequeue scan: %w", err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("batchlog dequeue rows: %w", err)
	}
	for i := range runs {
		q.legacy.recordClaimMetrics(ctx, &runs[i])
	}
	return runs, nil
}
