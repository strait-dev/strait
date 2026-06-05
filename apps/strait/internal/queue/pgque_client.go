package queue

import (
	"context"
	"fmt"
	"time"

	"strait/internal/store"

	"github.com/jackc/pgx/v5/pgtype"
)

const pgQueReadyEventType = "run.ready"

type pgQueClient struct {
	db           store.DBTX
	consumerName string
}

func (q *PgQueQueue) pgque(db store.DBTX) pgQueClient {
	return pgQueClient{
		db:           db,
		consumerName: q.cfg.ConsumerName,
	}
}

func (c pgQueClient) sendText(ctx context.Context, queueName, eventType, payload string) error {
	if _, err := c.db.Exec(ctx, `SELECT pgque.send($1, $2, $3::text)`, queueName, eventType, payload); err != nil {
		return fmt.Errorf("pgque send: %w", err)
	}
	return nil
}

func (c pgQueClient) sendTextBatch(ctx context.Context, queueName, eventType string, payloads []string) error {
	if len(payloads) == 0 {
		return nil
	}
	if _, err := c.db.Exec(ctx, `SELECT pgque.send_batch($1, $2, $3::text[])`, queueName, eventType, payloads); err != nil {
		return fmt.Errorf("pgque send batch: %w", err)
	}
	return nil
}

func (c pgQueClient) receive(ctx context.Context, queueName string, maxReturn int) ([]pgQueMessage, error) {
	rows, err := c.db.Query(ctx, `
		SELECT msg_id, batch_id, type, payload, retry_count, created_at, extra1, extra2, extra3, extra4
		FROM pgque.receive($1, $2, $3)`, queueName, c.consumerName, maxReturn)
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

func (c pgQueClient) consumerLag(ctx context.Context, queueName string) (int64, error) {
	var lag int64
	if err := c.db.QueryRow(ctx, `
		SELECT GREATEST(
			COALESCE(max(t.tick_id), 0) - COALESCE(max(s.sub_last_tick), 0),
			0
		)
		FROM pgque.queue q
		LEFT JOIN pgque.consumer co
		  ON co.co_name = $2
		LEFT JOIN pgque.subscription s
		  ON s.sub_queue = q.queue_id
		 AND s.sub_consumer = co.co_id
		LEFT JOIN pgque.tick t
		  ON t.tick_queue = q.queue_id
		WHERE q.queue_name = $1`,
		queueName,
		c.consumerName,
	).Scan(&lag); err != nil {
		return 0, fmt.Errorf("pgque consumer lag: %w", err)
	}
	return lag, nil
}

func (c pgQueClient) ack(ctx context.Context, batchID int64) error {
	if _, err := c.db.Exec(ctx, `SELECT pgque.ack($1)`, batchID); err != nil {
		return fmt.Errorf("pgque ack: %w", err)
	}
	return nil
}

func (c pgQueClient) nack(ctx context.Context, msg pgQueMessage, delay time.Duration, reason string) error {
	retryCount := int32(0)
	if msg.RetryCount != nil {
		retryCount = *msg.RetryCount
	}
	interval := pgtype.Interval{Microseconds: delay.Microseconds(), Valid: true}
	if _, err := c.db.Exec(ctx, `
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
		interval,
		reason,
	); err != nil {
		return fmt.Errorf("pgque nack: %w", err)
	}
	return nil
}

func (c pgQueClient) createQueue(ctx context.Context, queueName string) error {
	if _, err := c.db.Exec(ctx, `SELECT pgque.create_queue($1)`, queueName); err != nil {
		return fmt.Errorf("pgque create queue %s: %w", queueName, err)
	}
	return nil
}

func (c pgQueClient) setQueueConfig(ctx context.Context, queueName, key, value string) error {
	if _, err := c.db.Exec(ctx, `SELECT pgque.set_queue_config($1, $2, $3)`, queueName, key, value); err != nil {
		return fmt.Errorf("pgque configure queue %s %s: %w", queueName, key, err)
	}
	return nil
}

func (c pgQueClient) registerConsumer(ctx context.Context, queueName string) error {
	if _, err := c.db.Exec(ctx, `SELECT pgque.register_consumer($1, $2)`, queueName, c.consumerName); err != nil {
		return fmt.Errorf("pgque register consumer %s: %w", queueName, err)
	}
	return nil
}

func (c pgQueClient) ticker(ctx context.Context, queueName string) error {
	if _, err := c.db.Exec(ctx, `SELECT pgque.ticker($1)`, queueName); err != nil {
		return fmt.Errorf("pgque ticker %s: %w", queueName, err)
	}
	return nil
}

func (c pgQueClient) tickerAll(ctx context.Context) error {
	if _, err := c.db.Exec(ctx, `SELECT pgque.ticker()`); err != nil {
		return fmt.Errorf("pgque ticker all: %w", err)
	}
	return nil
}

func (c pgQueClient) forceNextTick(ctx context.Context, queueName string) error {
	if _, err := c.db.Exec(ctx, `SELECT pgque.force_next_tick($1)`, queueName); err != nil {
		return fmt.Errorf("pgque force next tick: %w", err)
	}
	return nil
}

func (c pgQueClient) rotateTablesStep1(ctx context.Context, queueName string) error {
	if _, err := c.db.Exec(ctx, `SELECT pgque.maint_rotate_tables_step1($1)`, queueName); err != nil {
		return fmt.Errorf("pgque maintain rotate step1 %s: %w", queueName, err)
	}
	return nil
}

func (c pgQueClient) rotateTablesStep2(ctx context.Context) error {
	if _, err := c.db.Exec(ctx, `SELECT pgque.maint_rotate_tables_step2()`); err != nil {
		return fmt.Errorf("pgque maintain rotate step2: %w", err)
	}
	return nil
}
