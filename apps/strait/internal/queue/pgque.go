package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"
	"strait/internal/store"

	"go.opentelemetry.io/otel"
)

const pgQueConsumerName = "strait"

type PgQueConfig struct {
	TickInterval  time.Duration
	ConsumerName  string
	NackDelay     time.Duration
	ReceiveWindow int
}

func (c PgQueConfig) normalized() PgQueConfig {
	if c.TickInterval <= 0 {
		c.TickInterval = 50 * time.Millisecond
	}
	if c.ConsumerName == "" {
		c.ConsumerName = pgQueConsumerName
	}
	if c.NackDelay <= 0 {
		c.NackDelay = time.Second
	}
	if c.ReceiveWindow <= 0 {
		c.ReceiveWindow = 1000
	}
	return c
}

type PgQueQueue struct {
	db      store.DBTX
	legacy  *PostgresQueue
	cfg     PgQueConfig
	mu      sync.Mutex
	pending map[string][]domain.JobRun
}

type pgQueReadyEvent struct {
	RunID      string `json:"run_id"`
	RouteKey   string `json:"route_key"`
	Generation int64  `json:"generation"`
	Priority   int    `json:"priority"`
}

type pgQueMessage struct {
	ID         int64
	BatchID    int64
	Type       string
	Payload    string
	RetryCount *int32
	CreatedAt  time.Time
	Extra1     *string
	Extra2     *string
	Extra3     *string
	Extra4     *string
}

type pgQueClaimFilter struct {
	ProjectID     string
	ExecutionMode domain.ExecutionMode
	WorkerRefs    []domain.WorkerQueueRef
}

const pgQueClaimDequeueColumns = `jr.id, jr.job_id, jr.project_id, u.status, u.attempt, jr.payload, jr.result, jr.metadata, jr.error, jr.error_class,
		          jr.triggered_by, u.scheduled_at, u.started_at, u.finished_at, u.heartbeat_at,
		          u.next_retry_at, u.expires_at, jr.parent_run_id, u.priority, jr.idempotency_key, jr.job_version, jr.created_at, jr.workflow_step_run_id, jr.execution_trace, jr.debug_mode, jr.continuation_of, jr.lineage_depth, jr.tags, jr.job_version_id, jr.created_by, jr.batch_id, u.concurrency_key, u.execution_mode, jr.is_rollback, jr.replayed_run_id`

func NewPgQueQueue(db store.DBTX, legacy *PostgresQueue, cfg PgQueConfig) *PgQueQueue {
	if legacy == nil {
		legacy = NewPostgresQueue(db)
	}
	return &PgQueQueue{db: db, legacy: legacy, cfg: cfg.normalized(), pending: make(map[string][]domain.JobRun)}
}

func (q *PgQueQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		if err := q.legacy.Enqueue(ctx, run); err != nil {
			return err
		}
		if run.Status == domain.StatusQueued {
			return q.sendReadyEvent(ctx, q.db, run)
		}
		return nil
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgque enqueue: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := q.EnqueueInTx(ctx, tx, run); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgque enqueue: commit: %w", err)
	}
	return nil
}

func (q *PgQueQueue) EnqueueInTx(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
	if err := q.legacy.EnqueueInTx(ctx, tx, run); err != nil {
		return err
	}
	if run.Status != domain.StatusQueued {
		return nil
	}
	return q.sendReadyEvent(ctx, tx, run)
}

func (q *PgQueQueue) EnqueueBatch(ctx context.Context, runs []*domain.JobRun) (int64, error) {
	if len(runs) == 0 {
		return 0, nil
	}
	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		inserted, err := q.legacy.EnqueueBatch(ctx, runs)
		if err != nil {
			return 0, err
		}
		if err := q.sendReadyEvents(ctx, q.db, runs); err != nil {
			return 0, err
		}
		return inserted, nil
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("pgque enqueue batch: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	legacy := NewPostgresQueue(tx)
	inserted, err := legacy.EnqueueBatch(ctx, runs)
	if err != nil {
		return 0, err
	}
	if err := q.sendReadyEvents(ctx, tx, runs); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("pgque enqueue batch: commit: %w", err)
	}
	return inserted, nil
}

func (q *PgQueQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	runs, err := q.DequeueN(ctx, 1)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return &runs[0], nil
}

func (q *PgQueQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	return q.dequeueFromRoute(ctx, n, pgQueHTTPRouteKey, pgQueClaimFilter{ExecutionMode: domain.ExecutionModeHTTP})
}

func (q *PgQueQueue) DequeueNFair(ctx context.Context, n int) ([]domain.JobRun, error) {
	return q.DequeueN(ctx, n)
}

func (q *PgQueQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	return q.dequeueFromRoute(ctx, n, pgQueHTTPRouteKey, pgQueClaimFilter{ProjectID: projectID, ExecutionMode: domain.ExecutionModeHTTP})
}

func (q *PgQueQueue) DequeueNForWorkerQueues(ctx context.Context, n int, queues []domain.WorkerQueueRef) ([]domain.JobRun, error) {
	refs := normalizePgQueWorkerQueueRefs(queues)
	if n <= 0 || len(refs) == 0 {
		return nil, nil
	}
	routes, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		return nil, err
	}
	claimed := make([]domain.JobRun, 0, n)
	for _, routeKey := range routes {
		if len(claimed) >= n {
			break
		}
		batch, err := q.dequeueFromRoute(ctx, n-len(claimed), routeKey, pgQueClaimFilter{
			ExecutionMode: domain.ExecutionModeWorker,
			WorkerRefs:    refs,
		})
		if err != nil {
			return claimed, err
		}
		claimed = append(claimed, batch...)
	}
	return claimed, nil
}

func (q *PgQueQueue) ForceTick(ctx context.Context, routeKey string) error {
	queueName := pgQueQueueName(routeKey)
	if _, err := q.db.Exec(ctx, `SELECT pgque.force_tick($1)`, queueName); err != nil {
		return fmt.Errorf("pgque force tick: %w", err)
	}
	if _, err := q.db.Exec(ctx, `SELECT pgque.ticker($1)`, queueName); err != nil {
		return fmt.Errorf("pgque force tick: %w", err)
	}
	return nil
}

func (q *PgQueQueue) RunTicker(ctx context.Context) {
	ticker := time.NewTicker(q.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = q.db.Exec(ctx, `SELECT pgque.ticker()`)
		}
	}
}

func (q *PgQueQueue) sendReadyEvent(ctx context.Context, db store.DBTX, run *domain.JobRun) error {
	routeKey, err := q.routeKeyForRun(ctx, db, run)
	if err != nil {
		return err
	}
	queueName := pgQueQueueName(routeKey)
	if err := q.ensureRoute(ctx, db, routeKey, queueName); err != nil {
		return err
	}
	generation, err := q.readyGeneration(ctx, db, run.ID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(pgQueReadyEvent{
		RunID:      run.ID,
		RouteKey:   routeKey,
		Generation: generation,
		Priority:   run.Priority,
	})
	if err != nil {
		return fmt.Errorf("pgque ready event: marshal: %w", err)
	}
	if _, err := db.Exec(ctx, `SELECT pgque.send($1, 'run.ready', $2::text)`, queueName, string(payload)); err != nil {
		return fmt.Errorf("pgque send ready event: %w", err)
	}
	return nil
}

func (q *PgQueQueue) sendReadyEvents(ctx context.Context, db store.DBTX, runs []*domain.JobRun) error {
	byRoute := make(map[string][]string)
	for _, run := range runs {
		if run.Status != domain.StatusQueued {
			continue
		}
		routeKey, err := q.routeKeyForRun(ctx, db, run)
		if err != nil {
			return err
		}
		generation, err := q.readyGeneration(ctx, db, run.ID)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(pgQueReadyEvent{
			RunID:      run.ID,
			RouteKey:   routeKey,
			Generation: generation,
			Priority:   run.Priority,
		})
		if err != nil {
			return fmt.Errorf("pgque ready event: marshal: %w", err)
		}
		byRoute[routeKey] = append(byRoute[routeKey], string(payload))
	}
	for routeKey, payloads := range byRoute {
		queueName := pgQueQueueName(routeKey)
		if err := q.ensureRoute(ctx, db, routeKey, queueName); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `SELECT pgque.send_batch($1, 'run.ready', $2::text[])`, queueName, payloads); err != nil {
			return fmt.Errorf("pgque send ready event batch: %w", err)
		}
	}
	return nil
}

func (q *PgQueQueue) ensureRoute(ctx context.Context, db store.DBTX, routeKey, queueName string) error {
	if _, err := db.Exec(ctx, `
		INSERT INTO strait_pgque_routes (route_key, queue_name)
		VALUES ($1, $2)
		ON CONFLICT (route_key) DO NOTHING`, routeKey, queueName); err != nil {
		return fmt.Errorf("pgque route upsert: %w", err)
	}
	if _, err := db.Exec(ctx, `SELECT pgque.create_queue($1)`, queueName); err != nil {
		return fmt.Errorf("pgque create queue %s: %w", queueName, err)
	}
	if _, err := db.Exec(ctx, `SELECT pgque.register_consumer($1, $2)`, queueName, q.cfg.ConsumerName); err != nil {
		return fmt.Errorf("pgque register consumer %s: %w", queueName, err)
	}
	return nil
}

func normalizePgQueWorkerQueueRefs(refs []domain.WorkerQueueRef) []domain.WorkerQueueRef {
	if len(refs) == 0 {
		return nil
	}
	normalized := make([]domain.WorkerQueueRef, 0, len(refs))
	seen := make(map[domain.WorkerQueueRef]struct{}, len(refs))
	for _, ref := range refs {
		if ref.ProjectID == "" || ref.QueueName == "" {
			continue
		}
		ref.QueueName = runQueueName(ref.QueueName)
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		normalized = append(normalized, ref)
	}
	return normalized
}

func (q *PgQueQueue) routeKeyForRun(ctx context.Context, db store.DBTX, run *domain.JobRun) (string, error) {
	if run == nil || run.ExecutionMode != domain.ExecutionModeWorker {
		return pgQueHTTPRouteKey, nil
	}
	var queueName, environmentID string
	if err := db.QueryRow(ctx, `
		SELECT COALESCE(NULLIF($2, ''), NULLIF(j.queue_name, ''), 'default'),
		       COALESCE(j.environment_id, '')
		FROM jobs j
		WHERE j.id = $1`, run.JobID, run.QueueName).Scan(&queueName, &environmentID); err != nil {
		return "", fmt.Errorf("pgque worker route lookup: %w", err)
	}
	return pgQueWorkerRouteKey(run.ProjectID, queueName, environmentID), nil
}

func (q *PgQueQueue) readyGeneration(ctx context.Context, db store.DBTX, runID string) (int64, error) {
	var generation int64
	if err := db.QueryRow(ctx, `SELECT ready_generation FROM job_run_state WHERE run_id = $1`, runID).Scan(&generation); err != nil {
		return 0, fmt.Errorf("pgque ready generation: %w", err)
	}
	return generation, nil
}

func (q *PgQueQueue) workerRouteKeys(ctx context.Context, refs []domain.WorkerQueueRef) ([]string, error) {
	routes := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		queueName := runQueueName(ref.QueueName)
		if ref.EnvironmentID != "" {
			key := pgQueWorkerRouteKey(ref.ProjectID, queueName, ref.EnvironmentID)
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				routes = append(routes, key)
			}
			continue
		}
		prefix := pgQueWorkerRouteKey(ref.ProjectID, queueName, "")
		rows, err := q.db.Query(ctx, `
			SELECT route_key
			FROM strait_pgque_routes
			WHERE route_key = $1 OR route_key LIKE $2
			ORDER BY route_key`, prefix, prefix+"%")
		if err != nil {
			return nil, fmt.Errorf("pgque worker route lookup: %w", err)
		}
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				rows.Close()
				return nil, fmt.Errorf("pgque worker route scan: %w", err)
			}
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				routes = append(routes, key)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("pgque worker route rows: %w", err)
		}
		rows.Close()
		if _, ok := seen[prefix]; !ok {
			seen[prefix] = struct{}{}
			routes = append(routes, prefix)
		}
	}
	return routes, nil
}

func (q *PgQueQueue) dequeueFromRoute(ctx context.Context, n int, routeKey string, filter pgQueClaimFilter) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.PgQueDequeue")
	defer span.End()

	if n <= 0 {
		return nil, nil
	}
	queueName := pgQueQueueName(routeKey)
	if err := q.ensureRoute(ctx, q.db, routeKey, queueName); err != nil {
		return nil, err
	}
	_ = q.ForceTick(ctx, routeKey)

	q.mu.Lock()
	defer q.mu.Unlock()

	if runs := q.popPending(routeKey, n); len(runs) > 0 {
		return runs, nil
	}

	for attempt := 0; attempt < 3; attempt++ {
		messages, err := q.receive(ctx, queueName, max(n, q.cfg.ReceiveWindow))
		if err != nil {
			return nil, err
		}
		if len(messages) == 0 {
			return nil, nil
		}

		ids := make([]string, 0, len(messages))
		generations := make([]int64, 0, len(messages))
		for _, msg := range messages {
			var event pgQueReadyEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil || event.RunID == "" {
				continue
			}
			ids = append(ids, event.RunID)
			generations = append(generations, event.Generation)
		}

		runs, err := q.claimRuns(ctx, ids, generations, len(ids), filter)
		if err != nil {
			return nil, err
		}
		claimed := make(map[string]struct{}, len(runs))
		for i := range runs {
			claimed[runs[i].ID] = struct{}{}
			q.legacy.recordClaimMetrics(ctx, &runs[i])
		}

		for _, msg := range messages {
			var event pgQueReadyEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil || event.RunID == "" {
				_ = q.nack(ctx, msg, q.cfg.NackDelay, "invalid ready event")
				continue
			}
			if _, ok := claimed[event.RunID]; !ok {
				delay := q.cfg.NackDelay
				if len(runs) > 0 {
					delay = 0
				}
				_ = q.nack(ctx, msg, delay, "not claimable")
			}
		}
		if err := q.ack(ctx, messages[0].BatchID); err != nil {
			return runs, err
		}
		if len(runs) > 0 {
			if len(runs) > n {
				q.pending[routeKey] = append(q.pending[routeKey], runs[n:]...)
				return runs[:n], nil
			}
			return runs, nil
		}
	}
	return nil, nil
}

func (q *PgQueQueue) popPending(routeKey string, n int) []domain.JobRun {
	pending := q.pending[routeKey]
	if len(pending) == 0 {
		return nil
	}
	if len(pending) <= n {
		delete(q.pending, routeKey)
		return pending
	}
	out := append([]domain.JobRun(nil), pending[:n]...)
	q.pending[routeKey] = append([]domain.JobRun(nil), pending[n:]...)
	return out
}

func (q *PgQueQueue) receive(ctx context.Context, queueName string, maxReturn int) ([]pgQueMessage, error) {
	rows, err := q.db.Query(ctx, `
		SELECT msg_id, batch_id, type, payload, retry_count, created_at, extra1, extra2, extra3, extra4
		FROM pgque.receive($1, $2, $3)`, queueName, q.cfg.ConsumerName, maxReturn)
	if err != nil {
		return nil, fmt.Errorf("pgque receive: %w", err)
	}
	defer rows.Close()

	var messages []pgQueMessage
	for rows.Next() {
		var msg pgQueMessage
		if err := rows.Scan(
			&msg.ID,
			&msg.BatchID,
			&msg.Type,
			&msg.Payload,
			&msg.RetryCount,
			&msg.CreatedAt,
			&msg.Extra1,
			&msg.Extra2,
			&msg.Extra3,
			&msg.Extra4,
		); err != nil {
			return nil, fmt.Errorf("pgque receive scan: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque receive rows: %w", err)
	}
	return messages, nil
}

func (q *PgQueQueue) claimRuns(ctx context.Context, ids []string, generations []int64, limit int, filter pgQueClaimFilter) ([]domain.JobRun, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) != len(generations) {
		return nil, fmt.Errorf("pgque claim runs: mismatched id/generation counts")
	}
	projectIDs, queueNames, environmentIDs := workerQueueRefArgs(filter.WorkerRefs)
	rows, err := q.db.Query(ctx, fmt.Sprintf(`
		WITH input AS (
			SELECT *
			FROM unnest($1::text[], $2::bigint[]) WITH ORDINALITY AS u(id, generation, ord)
		),
		raw_candidates AS (
			SELECT s.run_id,
			       input.ord,
			       s.job_id,
			       s.concurrency_key,
			       s.job_max_concurrency,
			       s.job_max_concurrency_per_key,
			       s.priority AS claim_priority,
			       jr.created_at AS claim_created_at,
			       (
			           SELECT COUNT(*)
			           FROM job_run_state active
			           WHERE active.job_id = s.job_id
			             AND active.status IN ('dequeued', 'executing')
			       ) AS active_count,
			       (
			           SELECT COUNT(*)
			           FROM job_run_state active
			           WHERE active.job_id = s.job_id
			             AND active.concurrency_key = s.concurrency_key
			             AND active.status IN ('dequeued', 'executing')
			       ) AS key_active_count
			FROM input
			JOIN job_run_state s ON s.run_id = input.id
			JOIN job_runs jr ON jr.id = s.run_id
			WHERE s.status = $4
			  AND s.ready_generation = input.generation
			  AND s.execution_mode = $6
			  AND ($7::text = '' OR s.project_id = $7)
			  AND (
			      $6 <> 'worker'
			      OR EXISTS (
			          SELECT 1
			          FROM unnest($8::text[], $9::text[], $10::text[]) AS wq(project_id, queue_name, environment_id)
			          WHERE wq.project_id = s.project_id
			            AND wq.queue_name = s.queue_name
			            AND (wq.environment_id = '' OR s.environment_id = wq.environment_id)
			      )
			  )
			  AND COALESCE(s.job_enabled, true) = true
			  AND COALESCE(s.job_paused, false) = false
			  AND (s.scheduled_at IS NULL OR s.scheduled_at <= NOW())
			  AND (s.next_retry_at IS NULL OR s.next_retry_at <= NOW())
			ORDER BY s.priority DESC, jr.created_at ASC, input.ord
			FOR UPDATE OF s SKIP LOCKED
		),
		ranked_candidates AS (
			SELECT raw_candidates.*,
			       ROW_NUMBER() OVER (PARTITION BY job_id ORDER BY claim_priority DESC, claim_created_at ASC, ord) AS job_rank,
			       ROW_NUMBER() OVER (PARTITION BY job_id, concurrency_key ORDER BY claim_priority DESC, claim_created_at ASC, ord) AS key_rank
			FROM raw_candidates
		),
		candidates AS (
			SELECT *
			FROM ranked_candidates
			WHERE (job_max_concurrency IS NULL OR job_rank <= GREATEST(job_max_concurrency - active_count, 0))
			  AND (
			      job_max_concurrency_per_key IS NULL
			      OR concurrency_key = ''
			      OR key_rank <= GREATEST(job_max_concurrency_per_key - key_active_count, 0)
			  )
			ORDER BY claim_priority DESC, claim_created_at ASC, ord
			LIMIT $3
		),
		updated_state AS (
			UPDATE job_run_state s
			SET status = $5,
			    started_at = NOW(),
			    updated_at = NOW()
			FROM candidates
			WHERE s.run_id = candidates.run_id
			RETURNING s.run_id,
			          s.status,
			          s.attempt,
			          s.scheduled_at,
			          s.started_at,
			          s.finished_at,
			          s.heartbeat_at,
			          s.next_retry_at,
			          s.expires_at,
			          s.priority,
			          s.concurrency_key,
			          s.execution_mode,
			          candidates.claim_priority,
			          candidates.claim_created_at,
			          candidates.ord
		)
		SELECT %s
		FROM updated_state u
		JOIN job_runs jr ON jr.id = u.run_id
		ORDER BY u.claim_priority DESC, u.claim_created_at ASC, u.ord`,
		pgQueClaimDequeueColumns,
	), ids, generations, limit, domain.StatusQueued, domain.StatusDequeued, filter.ExecutionMode, filter.ProjectID, projectIDs, queueNames, environmentIDs)
	if err != nil {
		return nil, fmt.Errorf("pgque claim runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("pgque claim scan: %w", err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque claim rows: %w", err)
	}
	return runs, nil
}

func (q *PgQueQueue) ack(ctx context.Context, batchID int64) error {
	if _, err := q.db.Exec(ctx, `SELECT pgque.ack($1)`, batchID); err != nil {
		return fmt.Errorf("pgque ack: %w", err)
	}
	return nil
}

func (q *PgQueQueue) nack(ctx context.Context, msg pgQueMessage, delay time.Duration, reason string) error {
	retryCount := int32(0)
	if msg.RetryCount != nil {
		retryCount = *msg.RetryCount
	}
	if _, err := q.db.Exec(ctx, `
		SELECT pgque.nack(
			$1,
			ROW($2, $1, $3, $4, $5, $6, $7, $8, $9, $10)::pgque.message,
			$11::interval,
			$12
		)`,
		msg.BatchID,
		msg.ID,
		msg.Type,
		msg.Payload,
		retryCount,
		msg.CreatedAt,
		msg.Extra1,
		msg.Extra2,
		msg.Extra3,
		msg.Extra4,
		delay.String(),
		reason,
	); err != nil {
		return fmt.Errorf("pgque nack: %w", err)
	}
	return nil
}

var _ Queue = (*PgQueQueue)(nil)
var _ interface {
	RunTicker(context.Context)
} = (*PgQueQueue)(nil)
