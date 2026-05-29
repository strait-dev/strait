package queue

import (
	"context"
	"encoding/json"
	"fmt"
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
	db     store.DBTX
	legacy *PostgresQueue
	cfg    PgQueConfig
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

const pgQueDequeueColumns = `jr.id, jr.job_id, jr.project_id, jr.status, jr.attempt, jr.payload, jr.result, jr.metadata, jr.error, jr.error_class,
		          jr.triggered_by, jr.scheduled_at, jr.started_at, jr.finished_at, jr.heartbeat_at,
		          jr.next_retry_at, jr.expires_at, jr.parent_run_id, jr.priority, jr.idempotency_key, jr.job_version, jr.created_at, jr.workflow_step_run_id, jr.execution_trace, jr.debug_mode, jr.continuation_of, jr.lineage_depth, jr.tags, jr.job_version_id, jr.created_by, jr.batch_id, jr.concurrency_key, jr.execution_mode, jr.is_rollback, jr.replayed_run_id`

func NewPgQueQueue(db store.DBTX, legacy *PostgresQueue, cfg PgQueConfig) *PgQueQueue {
	if legacy == nil {
		legacy = NewPostgresQueue(db)
	}
	return &PgQueQueue{db: db, legacy: legacy, cfg: cfg.normalized()}
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
	return q.dequeueFromRoute(ctx, n, pgQueHTTPRouteKey, "")
}

func (q *PgQueQueue) DequeueNFair(ctx context.Context, n int) ([]domain.JobRun, error) {
	return q.DequeueN(ctx, n)
}

func (q *PgQueQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	return q.dequeueFromRoute(ctx, n, pgQueHTTPRouteKey, projectID)
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
	routeKey := pgQueRouteKeyForRun(run)
	queueName := pgQueQueueName(routeKey)
	if err := q.ensureRoute(ctx, db, routeKey, queueName); err != nil {
		return err
	}
	payload, err := json.Marshal(pgQueReadyEvent{
		RunID:    run.ID,
		RouteKey: routeKey,
		Priority: run.Priority,
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
		routeKey := pgQueRouteKeyForRun(run)
		payload, err := json.Marshal(pgQueReadyEvent{
			RunID:    run.ID,
			RouteKey: routeKey,
			Priority: run.Priority,
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

func (q *PgQueQueue) dequeueFromRoute(ctx context.Context, n int, routeKey, projectID string) ([]domain.JobRun, error) {
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

	for attempt := 0; attempt < 3; attempt++ {
		messages, err := q.receive(ctx, queueName, max(n, q.cfg.ReceiveWindow))
		if err != nil {
			return nil, err
		}
		if len(messages) == 0 {
			return nil, nil
		}

		ids := make([]string, 0, len(messages))
		for _, msg := range messages {
			var event pgQueReadyEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil || event.RunID == "" {
				continue
			}
			ids = append(ids, event.RunID)
		}

		runs, err := q.claimRuns(ctx, ids, n, projectID)
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
				_ = q.nack(ctx, msg, q.cfg.NackDelay, "not claimable")
			}
		}
		if err := q.ack(ctx, messages[0].BatchID); err != nil {
			return runs, err
		}
		if len(runs) > 0 {
			return runs, nil
		}
	}
	return nil, nil
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

func (q *PgQueQueue) claimRuns(ctx context.Context, ids []string, limit int, projectID string) ([]domain.JobRun, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx, fmt.Sprintf(`
		WITH input AS (
			SELECT *
			FROM unnest($1::text[]) WITH ORDINALITY AS u(id, ord)
		),
		candidates AS (
			SELECT jr.id,
			       input.ord,
			       jr.priority AS claim_priority,
			       jr.created_at AS claim_created_at
			FROM input
			JOIN job_runs jr ON jr.id = input.id
			WHERE jr.status = $3
			  AND ($4::text = '' OR jr.project_id = $4)
			  AND COALESCE(jr.job_enabled, true) = true
			  AND COALESCE(jr.job_paused, false) = false
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			ORDER BY jr.priority DESC, jr.created_at ASC, input.ord
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $2
		),
		updated AS (
			UPDATE job_runs jr
			SET status = $5, started_at = NOW()
			FROM candidates
			WHERE jr.id = candidates.id
			RETURNING %s, candidates.claim_priority, candidates.claim_created_at, candidates.ord
		)
		SELECT %s FROM updated ORDER BY claim_priority DESC, claim_created_at ASC, ord`,
		pgQueDequeueColumns,
		dequeueColumns,
	), ids, limit, domain.StatusQueued, projectID, domain.StatusDequeued)
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
