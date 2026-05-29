//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
)

func mustPgQueQueue(t *testing.T) *queue.PgQueQueue {
	t.Helper()
	return queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresQueue(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "test-" + newID(),
		NackDelay:     10 * time.Millisecond,
		ReceiveWindow: 100,
	})
}

func TestQueueEngine_PgQueSelectable(t *testing.T) {
	q, err := queue.NewQueueEngine(testDB.Pool, queue.EnginePgQue, queue.BatchlogConfig{TickInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewQueueEngine(pgque): %v", err)
	}
	if _, ok := q.(*queue.PgQueQueue); !ok {
		t.Fatalf("queue = %T, want *PgQueQueue", q)
	}
}

func TestPgQue_EnqueueInTxRollbackLeavesNoClaimableEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-rollback")
	q := mustPgQueQueue(t)

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.EnqueueInTx(ctx, tx, run); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("EnqueueInTx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	claimed, err := q.DequeueN(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed len = %d, want 0 after rollback", len(claimed))
	}
}

func TestPgQue_CreatesRouteIdempotently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-route")
	q := mustPgQueQueue(t)

	for range 2 {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	var routeRows, queueRows int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM strait_pgque_routes WHERE route_key = 'http'`).Scan(&routeRows); err != nil {
		t.Fatalf("route count: %v", err)
	}
	if routeRows != 1 {
		t.Fatalf("route rows = %d, want 1", routeRows)
	}
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pgque.queue q
		JOIN strait_pgque_routes r ON r.queue_name = q.queue_name
		WHERE r.route_key = 'http'`).Scan(&queueRows); err != nil {
		t.Fatalf("pgque queue count: %v", err)
	}
	if queueRows != 1 {
		t.Fatalf("pgque queue rows = %d, want 1", queueRows)
	}
}
