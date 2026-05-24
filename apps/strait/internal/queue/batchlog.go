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
)

type BatchlogConfig struct {
	TickInterval  time.Duration
	LeaseDuration time.Duration
	LeaseOwner    string
	AckRetention  time.Duration
	CleanupLimit  int
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
	if c.AckRetention <= 0 {
		c.AckRetention = 5 * time.Minute
	}
	if c.CleanupLimit <= 0 {
		c.CleanupLimit = 1000
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
	return q.dequeueHTTP(ctx, n, batchlogDequeueHTTPSQL, nil)
}

func (q *BatchlogQueue) DequeueNFair(ctx context.Context, n int) ([]domain.JobRun, error) {
	return q.dequeueHTTP(ctx, n, batchlogDequeueFairSQL, nil)
}

func (q *BatchlogQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	return q.dequeueHTTP(ctx, n, batchlogDequeueByProjectSQL, []any{projectID})
}

func (q *BatchlogQueue) DequeueNPartitioned(ctx context.Context, n int, projectIDs []string) ([]domain.JobRun, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	return q.dequeueHTTP(ctx, n, batchlogDequeuePartitionedSQL, []any{projectIDs})
}

func (q *BatchlogQueue) DequeueNForWorkerQueues(ctx context.Context, n int, queues []domain.WorkerQueueRef) ([]domain.JobRun, error) {
	projectIDs, queueNames, environmentIDs := workerQueueRefArgs(queues)
	if n <= 0 || len(queueNames) == 0 {
		return nil, nil
	}
	return q.dequeueRows(ctx, batchlogDequeueWorkerSQL, n, []any{projectIDs, queueNames, environmentIDs})
}

var batchlogDequeueHTTPSQL = "/* action=batchlog_dequeue_http */ " + `
		WITH candidate_window AS (
			SELECT qe.run_id, qe.job_id, qe.concurrency_key, qe.batch_id, qe.priority, qe.run_created_at
			FROM queue_entries qe
			WHERE qe.status = 'ready'
			  AND qe.batch_id IS NOT NULL
			  AND qe.available_at <= NOW()
			  AND qe.run_status = $4
			  AND qe.execution_mode = 'http'
			  AND COALESCE(qe.job_enabled, true) = true
			  AND COALESCE(qe.job_paused, false) = false
			  AND (qe.scheduled_at IS NULL OR qe.scheduled_at <= NOW())
			  AND (qe.next_retry_at IS NULL OR qe.next_retry_at <= NOW())
			ORDER BY qe.batch_id ASC, qe.priority DESC, qe.run_created_at ASC
			FOR UPDATE OF qe SKIP LOCKED
			LIMIT $5
		),
		candidate_jobs AS (
			SELECT DISTINCT job_id FROM candidate_window
		),
		candidate_keys AS (
			SELECT DISTINCT job_id, concurrency_key
			FROM candidate_window
			WHERE concurrency_key <> ''
		),
		leased_job_counts AS (
			SELECT leased.job_id, COUNT(*)::int AS count
			FROM queue_entries leased
			JOIN candidate_jobs cj ON cj.job_id = leased.job_id
			WHERE leased.status = 'leased'
			  AND leased.run_status = $4
			  AND leased.lease_expires_at > NOW()
			GROUP BY leased.job_id
		),
		leased_key_counts AS (
			SELECT leased.job_id, leased.concurrency_key, COUNT(*)::int AS count
			FROM queue_entries leased
			JOIN candidate_keys ck
			  ON ck.job_id = leased.job_id
			 AND ck.concurrency_key = leased.concurrency_key
			WHERE leased.status = 'leased'
			  AND leased.run_status = $4
			  AND leased.lease_expires_at > NOW()
			  AND leased.concurrency_key <> ''
			GROUP BY leased.job_id, leased.concurrency_key
		),
		claimed AS (
			SELECT qe.run_id
			FROM candidate_window cw
			JOIN queue_entries qe ON qe.run_id = cw.run_id
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = qe.job_id AND jac_job.concurrency_key = ''
		LEFT JOIN job_active_counts jac_key
		  ON jac_key.job_id = qe.job_id
		  AND jac_key.concurrency_key = qe.concurrency_key
		LEFT JOIN leased_job_counts leased_job
		  ON leased_job.job_id = qe.job_id
		LEFT JOIN leased_key_counts leased_key
		  ON leased_key.job_id = qe.job_id
		  AND leased_key.concurrency_key = qe.concurrency_key
			WHERE (qe.job_max_concurrency IS NULL
			       OR COALESCE(jac_job.count, 0) + COALESCE(leased_job.count, 0) < qe.job_max_concurrency)
			  AND (qe.job_max_concurrency_per_key IS NULL
			       OR qe.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) + COALESCE(leased_key.count, 0) < qe.job_max_concurrency_per_key)
			ORDER BY cw.batch_id ASC, cw.priority DESC, cw.run_created_at ASC
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
	SELECT ` + dequeueColumns + `
		FROM job_runs jr
		JOIN leased l ON l.run_id = jr.id
		ORDER BY jr.created_at ASC`

var batchlogDequeueByProjectSQL = "/* action=batchlog_dequeue_by_project */ " + `
		WITH candidate_window AS (
			SELECT qe.run_id, qe.job_id, qe.concurrency_key, qe.batch_id, qe.priority, qe.run_created_at
			FROM queue_entries qe
			WHERE qe.status = 'ready'
			  AND qe.batch_id IS NOT NULL
			  AND qe.available_at <= NOW()
			  AND qe.run_status = $4
			  AND qe.execution_mode = 'http'
			  AND qe.project_id = $6
			  AND COALESCE(qe.job_enabled, true) = true
			  AND COALESCE(qe.job_paused, false) = false
			  AND (qe.scheduled_at IS NULL OR qe.scheduled_at <= NOW())
			  AND (qe.next_retry_at IS NULL OR qe.next_retry_at <= NOW())
			ORDER BY qe.batch_id ASC, qe.priority DESC, qe.run_created_at ASC
			FOR UPDATE OF qe SKIP LOCKED
			LIMIT $5
		),
		candidate_jobs AS (
			SELECT DISTINCT job_id FROM candidate_window
		),
		candidate_keys AS (
			SELECT DISTINCT job_id, concurrency_key
			FROM candidate_window
			WHERE concurrency_key <> ''
		),
		leased_job_counts AS (
			SELECT leased.job_id, COUNT(*)::int AS count
			FROM queue_entries leased
			JOIN candidate_jobs cj ON cj.job_id = leased.job_id
			WHERE leased.status = 'leased'
			  AND leased.run_status = $4
			  AND leased.lease_expires_at > NOW()
			GROUP BY leased.job_id
		),
		leased_key_counts AS (
			SELECT leased.job_id, leased.concurrency_key, COUNT(*)::int AS count
			FROM queue_entries leased
			JOIN candidate_keys ck
			  ON ck.job_id = leased.job_id
			 AND ck.concurrency_key = leased.concurrency_key
			WHERE leased.status = 'leased'
			  AND leased.run_status = $4
			  AND leased.lease_expires_at > NOW()
			  AND leased.concurrency_key <> ''
			GROUP BY leased.job_id, leased.concurrency_key
		),
		claimed AS (
			SELECT qe.run_id
			FROM candidate_window cw
			JOIN queue_entries qe ON qe.run_id = cw.run_id
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = qe.job_id AND jac_job.concurrency_key = ''
		LEFT JOIN job_active_counts jac_key
		  ON jac_key.job_id = qe.job_id
		  AND jac_key.concurrency_key = qe.concurrency_key
		LEFT JOIN leased_job_counts leased_job
		  ON leased_job.job_id = qe.job_id
		LEFT JOIN leased_key_counts leased_key
		  ON leased_key.job_id = qe.job_id
		  AND leased_key.concurrency_key = qe.concurrency_key
			WHERE (qe.job_max_concurrency IS NULL
			       OR COALESCE(jac_job.count, 0) + COALESCE(leased_job.count, 0) < qe.job_max_concurrency)
			  AND (qe.job_max_concurrency_per_key IS NULL
			       OR qe.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) + COALESCE(leased_key.count, 0) < qe.job_max_concurrency_per_key)
			ORDER BY cw.batch_id ASC, cw.priority DESC, cw.run_created_at ASC
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
	SELECT ` + dequeueColumns + `
		FROM job_runs jr
		JOIN leased l ON l.run_id = jr.id
		ORDER BY jr.created_at ASC`

var batchlogDequeuePartitionedSQL = "/* action=batchlog_dequeue_partitioned */ " + `
		WITH candidate_window AS (
			SELECT qe.run_id, qe.job_id, qe.concurrency_key, qe.batch_id, qe.priority, qe.run_created_at
			FROM queue_entries qe
			WHERE qe.status = 'ready'
			  AND qe.batch_id IS NOT NULL
			  AND qe.available_at <= NOW()
			  AND qe.run_status = $4
			  AND qe.execution_mode = 'http'
			  AND qe.project_id = ANY($6::text[])
			  AND COALESCE(qe.job_enabled, true) = true
			  AND COALESCE(qe.job_paused, false) = false
			  AND (qe.scheduled_at IS NULL OR qe.scheduled_at <= NOW())
			  AND (qe.next_retry_at IS NULL OR qe.next_retry_at <= NOW())
			ORDER BY qe.project_id ASC, qe.batch_id ASC, qe.priority DESC, qe.run_created_at ASC
			FOR UPDATE OF qe SKIP LOCKED
			LIMIT $5
		),
		candidate_jobs AS (
			SELECT DISTINCT job_id FROM candidate_window
		),
		candidate_keys AS (
			SELECT DISTINCT job_id, concurrency_key
			FROM candidate_window
			WHERE concurrency_key <> ''
		),
		leased_job_counts AS (
			SELECT leased.job_id, COUNT(*)::int AS count
			FROM queue_entries leased
			JOIN candidate_jobs cj ON cj.job_id = leased.job_id
			WHERE leased.status = 'leased'
			  AND leased.run_status = $4
			  AND leased.lease_expires_at > NOW()
			GROUP BY leased.job_id
		),
		leased_key_counts AS (
			SELECT leased.job_id, leased.concurrency_key, COUNT(*)::int AS count
			FROM queue_entries leased
			JOIN candidate_keys ck
			  ON ck.job_id = leased.job_id
			 AND ck.concurrency_key = leased.concurrency_key
			WHERE leased.status = 'leased'
			  AND leased.run_status = $4
			  AND leased.lease_expires_at > NOW()
			  AND leased.concurrency_key <> ''
			GROUP BY leased.job_id, leased.concurrency_key
		),
		claimed AS (
			SELECT qe.run_id
			FROM candidate_window cw
			JOIN queue_entries qe ON qe.run_id = cw.run_id
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = qe.job_id AND jac_job.concurrency_key = ''
			LEFT JOIN job_active_counts jac_key
			  ON jac_key.job_id = qe.job_id
			  AND jac_key.concurrency_key = qe.concurrency_key
			LEFT JOIN leased_job_counts leased_job
			  ON leased_job.job_id = qe.job_id
			LEFT JOIN leased_key_counts leased_key
			  ON leased_key.job_id = qe.job_id
			  AND leased_key.concurrency_key = qe.concurrency_key
			WHERE (qe.job_max_concurrency IS NULL
			       OR COALESCE(jac_job.count, 0) + COALESCE(leased_job.count, 0) < qe.job_max_concurrency)
			  AND (qe.job_max_concurrency_per_key IS NULL
			       OR qe.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) + COALESCE(leased_key.count, 0) < qe.job_max_concurrency_per_key)
			ORDER BY cw.batch_id ASC, cw.priority DESC, cw.run_created_at ASC
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
		SELECT ` + dequeueColumns + `
		FROM job_runs jr
		JOIN leased l ON l.run_id = jr.id
		ORDER BY jr.created_at ASC`

var batchlogDequeueFairSQL = "/* action=batchlog_dequeue_fair */ " + `
		WITH candidate_seed AS (
			SELECT DISTINCT ON (qe.job_id) qe.run_id
			FROM queue_entries qe
			WHERE qe.status = 'ready'
			  AND qe.batch_id IS NOT NULL
			  AND qe.available_at <= NOW()
			  AND qe.run_status = $4
			  AND qe.execution_mode = 'http'
			  AND COALESCE(qe.job_enabled, true) = true
			  AND COALESCE(qe.job_paused, false) = false
			  AND (qe.scheduled_at IS NULL OR qe.scheduled_at <= NOW())
			  AND (qe.next_retry_at IS NULL OR qe.next_retry_at <= NOW())
			ORDER BY qe.job_id ASC, qe.batch_id ASC, qe.priority DESC, qe.run_created_at ASC
			LIMIT $5
		),
		candidate_window AS (
			SELECT qe.run_id, qe.job_id, qe.concurrency_key, qe.batch_id, qe.priority, qe.run_created_at
			FROM queue_entries qe
			JOIN candidate_seed cs ON cs.run_id = qe.run_id
			ORDER BY qe.batch_id ASC, qe.priority DESC, qe.run_created_at ASC
			FOR UPDATE OF qe SKIP LOCKED
			LIMIT $5
		),
		candidate_jobs AS (
			SELECT DISTINCT job_id FROM candidate_window
		),
		candidate_keys AS (
			SELECT DISTINCT job_id, concurrency_key
			FROM candidate_window
			WHERE concurrency_key <> ''
		),
		leased_job_counts AS (
			SELECT leased.job_id, COUNT(*)::int AS count
			FROM queue_entries leased
			JOIN candidate_jobs cj ON cj.job_id = leased.job_id
			WHERE leased.status = 'leased'
			  AND leased.run_status = $4
			  AND leased.lease_expires_at > NOW()
			GROUP BY leased.job_id
		),
		leased_key_counts AS (
			SELECT leased.job_id, leased.concurrency_key, COUNT(*)::int AS count
			FROM queue_entries leased
			JOIN candidate_keys ck
			  ON ck.job_id = leased.job_id
			 AND ck.concurrency_key = leased.concurrency_key
			WHERE leased.status = 'leased'
			  AND leased.run_status = $4
			  AND leased.lease_expires_at > NOW()
			  AND leased.concurrency_key <> ''
			GROUP BY leased.job_id, leased.concurrency_key
		),
		claimed AS (
			SELECT qe.run_id
			FROM candidate_window cw
			JOIN queue_entries qe ON qe.run_id = cw.run_id
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = qe.job_id AND jac_job.concurrency_key = ''
			LEFT JOIN job_active_counts jac_key
			  ON jac_key.job_id = qe.job_id
			  AND jac_key.concurrency_key = qe.concurrency_key
			LEFT JOIN leased_job_counts leased_job
			  ON leased_job.job_id = qe.job_id
			LEFT JOIN leased_key_counts leased_key
			  ON leased_key.job_id = qe.job_id
			  AND leased_key.concurrency_key = qe.concurrency_key
			WHERE (qe.job_max_concurrency IS NULL
			       OR COALESCE(jac_job.count, 0) + COALESCE(leased_job.count, 0) < qe.job_max_concurrency)
			  AND (qe.job_max_concurrency_per_key IS NULL
			       OR qe.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) + COALESCE(leased_key.count, 0) < qe.job_max_concurrency_per_key)
			ORDER BY cw.batch_id ASC, cw.priority DESC, cw.run_created_at ASC
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
		SELECT ` + dequeueColumns + `
		FROM job_runs jr
		JOIN leased l ON l.run_id = jr.id
		ORDER BY jr.created_at ASC`

var batchlogDequeueWorkerSQL = "/* action=batchlog_dequeue_worker */ " + `
		WITH candidate_window AS (
			SELECT qe.run_id, qe.job_id, qe.concurrency_key, qe.batch_id, qe.priority, qe.run_created_at
			FROM queue_entries qe
			WHERE qe.status = 'ready'
			  AND qe.batch_id IS NOT NULL
			  AND qe.available_at <= NOW()
			  AND qe.run_status = $4
			  AND qe.execution_mode = 'worker'
			  AND EXISTS (
			      SELECT 1
			      FROM unnest($6::text[], $7::text[], $8::text[]) AS wq(project_id, queue_name, environment_id)
			      WHERE wq.project_id = qe.project_id
			        AND wq.queue_name = qe.queue_name
			        AND (wq.environment_id = '' OR qe.environment_id = wq.environment_id)
			  )
			  AND COALESCE(qe.job_enabled, true) = true
			  AND COALESCE(qe.job_paused, false) = false
			  AND (qe.scheduled_at IS NULL OR qe.scheduled_at <= NOW())
			  AND (qe.next_retry_at IS NULL OR qe.next_retry_at <= NOW())
			ORDER BY qe.batch_id ASC, qe.priority DESC, qe.run_created_at ASC
			FOR UPDATE OF qe SKIP LOCKED
			LIMIT $5
		),
		candidate_jobs AS (
			SELECT DISTINCT job_id FROM candidate_window
		),
		candidate_keys AS (
			SELECT DISTINCT job_id, concurrency_key
			FROM candidate_window
			WHERE concurrency_key <> ''
		),
		leased_job_counts AS (
			SELECT leased.job_id, COUNT(*)::int AS count
			FROM queue_entries leased
			JOIN candidate_jobs cj ON cj.job_id = leased.job_id
			WHERE leased.status = 'leased'
			  AND leased.run_status = $4
			  AND leased.lease_expires_at > NOW()
			GROUP BY leased.job_id
		),
		leased_key_counts AS (
			SELECT leased.job_id, leased.concurrency_key, COUNT(*)::int AS count
			FROM queue_entries leased
			JOIN candidate_keys ck
			  ON ck.job_id = leased.job_id
			 AND ck.concurrency_key = leased.concurrency_key
			WHERE leased.status = 'leased'
			  AND leased.run_status = $4
			  AND leased.lease_expires_at > NOW()
			  AND leased.concurrency_key <> ''
			GROUP BY leased.job_id, leased.concurrency_key
		),
		claimed AS (
			SELECT qe.run_id
			FROM candidate_window cw
			JOIN queue_entries qe ON qe.run_id = cw.run_id
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = qe.job_id AND jac_job.concurrency_key = ''
			LEFT JOIN job_active_counts jac_key
			  ON jac_key.job_id = qe.job_id
			  AND jac_key.concurrency_key = qe.concurrency_key
			LEFT JOIN leased_job_counts leased_job
			  ON leased_job.job_id = qe.job_id
			LEFT JOIN leased_key_counts leased_key
			  ON leased_key.job_id = qe.job_id
			  AND leased_key.concurrency_key = qe.concurrency_key
			WHERE (qe.job_max_concurrency IS NULL
			       OR COALESCE(jac_job.count, 0) + COALESCE(leased_job.count, 0) < qe.job_max_concurrency)
			  AND (qe.job_max_concurrency_per_key IS NULL
			       OR qe.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) + COALESCE(leased_key.count, 0) < qe.job_max_concurrency_per_key)
			ORDER BY cw.batch_id ASC, cw.priority DESC, cw.run_created_at ASC
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
		SELECT ` + dequeueColumns + `
		FROM job_runs jr
		JOIN leased l ON l.run_id = jr.id
		ORDER BY jr.created_at ASC`

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
			_, _ = q.DeleteAckedEntries(ctx, q.cfg.AckRetention, q.cfg.CleanupLimit)
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
				next_retry_at,
				execution_mode,
				queue_name,
				environment_id
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
				jr.next_retry_at,
				COALESCE(NULLIF(jr.execution_mode, ''), 'http'),
				COALESCE(NULLIF(jr.queue_name, ''), 'default'),
				COALESCE(j.environment_id, '')
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
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

func (q *BatchlogQueue) DeleteAckedEntries(ctx context.Context, olderThan time.Duration, limit int) (int64, error) {
	if olderThan < 0 {
		olderThan = 0
	}
	if limit <= 0 {
		limit = 1000
	}
	tag, err := q.db.Exec(ctx, `
		WITH doomed AS (
			SELECT run_id
			FROM queue_entries
			WHERE status = 'acked'
			  AND acked_at IS NOT NULL
			  AND acked_at <= NOW() - $1::interval
			ORDER BY acked_at ASC, run_id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM queue_entries qe
		USING doomed
		WHERE qe.run_id = doomed.run_id
	`, olderThan, limit)
	if err != nil {
		return 0, fmt.Errorf("batchlog delete acked entries: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *BatchlogQueue) dequeueHTTP(ctx context.Context, n int, query string, extraArgs []any) ([]domain.JobRun, error) {
	return q.dequeueRows(ctx, query, n, extraArgs)
}

func (q *BatchlogQueue) dequeueRows(ctx context.Context, query string, n int, extraArgs []any) ([]domain.JobRun, error) {
	if n <= 0 {
		return nil, nil
	}

	windowLimit := n * 8
	windowLimit = max(windowLimit, 64)
	args := []any{n, q.cfg.LeaseOwner, q.cfg.LeaseDuration, domain.StatusQueued, windowLimit}
	args = append(args, extraArgs...)

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
