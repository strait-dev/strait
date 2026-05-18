//go:build integration

package grpc

import (
	"context"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/store"
	"strait/internal/testutil"
)

// TestIntegration_Heartbeat_DoesNotWriteDB pins the Phase D contract: the
// gRPC heartbeat handler must NOT write to the workers table. last_seen_at
// is refreshed by the dbSync loop instead. Writing on every heartbeat
// caused N×workers DB writes per HeartbeatInterval without changing
// observability.
//
// We seed a workers row with an old last_seen_at, fire a heartbeat, and
// assert the timestamp is unchanged.
func TestIntegration_Heartbeat_DoesNotWriteDB(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)

	const workerID = "hb-worker"
	const projectID = "proj-hb"

	// Seed a row with last_seen_at far in the past — far enough that any
	// heartbeat-time DB write would jump the timestamp by minutes.
	_, err = env.DB.Pool.Exec(ctx, `
		INSERT INTO workers (id, project_id, queue_name, hostname, version, status, last_seen_at, registered_at)
		VALUES ($1, $2, 'q', 'host1', '1.0', 'active', NOW() - INTERVAL '1 hour', NOW() - INTERVAL '1 hour')
	`, workerID, projectID)
	if err != nil {
		t.Fatalf("seed worker: %v", err)
	}

	var beforeTS time.Time
	if err := env.DB.Pool.QueryRow(ctx,
		`SELECT last_seen_at FROM workers WHERE id = $1`, workerID,
	).Scan(&beforeTS); err != nil {
		t.Fatalf("read seeded last_seen_at: %v", err)
	}

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	hb := &workerv1.Heartbeat{}
	if err := svc.handleHeartbeat(ctx, workerID, "", "", hb); err != nil {
		t.Fatalf("handleHeartbeat: %v", err)
	}

	var afterTS time.Time
	if err := env.DB.Pool.QueryRow(ctx,
		`SELECT last_seen_at FROM workers WHERE id = $1`, workerID,
	).Scan(&afterTS); err != nil {
		t.Fatalf("read post-heartbeat last_seen_at: %v", err)
	}

	if !afterTS.Equal(beforeTS) {
		t.Fatalf("heartbeat wrote to workers table: last_seen_at moved from %v to %v",
			beforeTS, afterTS)
	}
}

// TestIntegration_Heartbeat_DBSyncRefreshesLastSeen confirms that with the
// per-heartbeat write removed, the dbSync loop alone keeps last_seen_at
// fresh. dbSyncOnce calls RegisterWorker (UPSERT, last_seen_at = NOW()),
// which is the post-Phase-D source of truth for liveness.
func TestIntegration_Heartbeat_DBSyncRefreshesLastSeen(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)

	reg := NewConnectionRegistry()
	w := makeWorker("dbsync-worker", "proj-dbsync", "key", []string{"q"}, 4)
	if err := reg.Register(w); err != nil {
		t.Fatalf("register: %v", err)
	}

	// First sync: row gets created.
	dbSyncOnce(ctx, reg, q)

	var firstTS time.Time
	if err := env.DB.Pool.QueryRow(ctx,
		`SELECT last_seen_at FROM workers WHERE id = $1`, w.WorkerID,
	).Scan(&firstTS); err != nil {
		t.Fatalf("read first last_seen_at: %v", err)
	}

	// Postgres NOW() has microsecond resolution but back-to-back calls in
	// the same statement can return identical values. Force a gap.
	time.Sleep(50 * time.Millisecond)

	// Second sync: last_seen_at must advance.
	dbSyncOnce(ctx, reg, q)

	var secondTS time.Time
	if err := env.DB.Pool.QueryRow(ctx,
		`SELECT last_seen_at FROM workers WHERE id = $1`, w.WorkerID,
	).Scan(&secondTS); err != nil {
		t.Fatalf("read second last_seen_at: %v", err)
	}

	if !secondTS.After(firstTS) {
		t.Fatalf("dbSync did not advance last_seen_at: first=%v second=%v",
			firstTS, secondTS)
	}
}
