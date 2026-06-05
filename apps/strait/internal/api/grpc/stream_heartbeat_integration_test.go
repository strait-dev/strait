//go:build integration

package grpc

import (
	"context"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// TestIntegration_Heartbeat_RenewsStreamLeaseWithoutTouchingLastSeen pins the
// heartbeat contract: the stream renews its cross-replica liveness lease, while
// last_seen_at remains owned by the dbSync loop.
//
// We seed a workers row with an old last_seen_at, fire a heartbeat, and
// assert the timestamp is unchanged.
func TestIntegration_Heartbeat_RenewsStreamLeaseWithoutTouchingLastSeen(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)

	const workerID = "hb-worker"
	const projectID = "proj-hb"

	// Seed a row with last_seen_at far in the past — far enough that any
	// heartbeat-time DB write would jump the timestamp by minutes.
	_, err := env.DB.Pool.Exec(ctx, `
		INSERT INTO workers (id, project_id, queue_name, hostname, version, status, last_seen_at, registered_at)
		VALUES ($1, $2, 'q', 'host1', '1.0', 'active', NOW() - INTERVAL '1 hour', NOW() - INTERVAL '1 hour')
	`, workerID, projectID)
	require.NoError(t,

		err)

	var beforeTS time.Time
	require.NoError(t,

		env.DB.Pool.
			QueryRow(ctx, `SELECT last_seen_at FROM workers WHERE id = $1`,

				workerID).Scan(
			&beforeTS))

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	hb := &workerv1.Heartbeat{}
	require.NoError(t,

		svc.handleHeartbeat(ctx, workerID,
			projectID,

			"", "", hb))

	var afterTS time.Time
	var leaseExpiresAt *time.Time
	require.NoError(t,

		env.DB.Pool.
			QueryRow(ctx, `SELECT last_seen_at, stream_lease_expires_at FROM workers WHERE id = $1`,

				workerID).Scan(
			&afterTS,

			&leaseExpiresAt))
	require.True(t, afterTS.
		Equal(beforeTS))
	require.False(t,
		leaseExpiresAt ==
			nil ||
			!leaseExpiresAt.
				After(time.Now()))

}

// TestIntegration_Heartbeat_DBSyncRefreshesLastSeen confirms that with the
// per-heartbeat write removed, the dbSync loop alone keeps last_seen_at
// fresh. dbSyncOnce calls RegisterWorker (UPSERT, last_seen_at = NOW()),
// which is the authoritative liveness write.
func TestIntegration_Heartbeat_DBSyncRefreshesLastSeen(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)

	reg := NewConnectionRegistry()
	w := makeWorker("dbsync-worker", "proj-dbsync", "key", []string{"q"}, 4)
	require.NoError(t,

		reg.Register(w))

	// First sync: row gets created.
	dbSyncOnce(ctx, reg, q)

	var firstTS time.Time
	require.NoError(t,

		env.DB.Pool.
			QueryRow(ctx, `SELECT last_seen_at FROM workers WHERE id = $1`,

				w.WorkerID).Scan(&firstTS))

	// Postgres NOW() has microsecond resolution but back-to-back calls in
	// the same statement can return identical values. Force a gap.
	time.Sleep(50 * time.Millisecond)

	// Second sync: last_seen_at must advance.
	dbSyncOnce(ctx, reg, q)

	var secondTS time.Time
	require.NoError(t,

		env.DB.Pool.
			QueryRow(ctx, `SELECT last_seen_at FROM workers WHERE id = $1`,

				w.WorkerID).Scan(&secondTS))
	require.True(t, secondTS.
		After(firstTS))

}
