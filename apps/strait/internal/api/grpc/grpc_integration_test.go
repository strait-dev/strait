//go:build integration

package grpc

import (
	"context"
	"testing"
	"time"

	"strait/internal/pubsub"
	"strait/internal/store"
	"strait/internal/testutil"
)

// TestIntegration_DBSync_WorkerVisible verifies that after the DB sync interval fires,
// a registered worker is visible in the workers table.
func TestIntegration_DBSync_WorkerVisible(t *testing.T) {
	ctx := context.Background()

	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })

	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean tables: %v", err)
	}

	q := store.New(env.DB.Pool)
	reg := NewConnectionRegistry()
	w := makeWorker("sync-worker", "proj-1", "key-sync", []string{"q1"}, 4)
	if err := reg.Register(w); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Upsert directly to simulate what dbSyncOnce does.
	dbSyncOnce(ctx, reg, q)

	// Verify the worker row exists in the workers table.
	var id string
	err = env.DB.Pool.QueryRow(ctx,
		`SELECT id FROM workers WHERE id = $1 AND project_id = $2`,
		"sync-worker", "proj-1",
	).Scan(&id)
	if err != nil {
		t.Fatalf("worker not found in DB after dbSync: %v", err)
	}
	if id != "sync-worker" {
		t.Errorf("expected sync-worker, got %s", id)
	}
}

// TestIntegration_Sweep_EvictsStaleWorkers verifies that EvictStaleWorkers marks stale workers offline.
func TestIntegration_Sweep_EvictsStaleWorkers(t *testing.T) {
	ctx := context.Background()

	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })

	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean tables: %v", err)
	}

	q := store.New(env.DB.Pool)

	// Insert a worker with a very old last_seen_at directly (workers table has no FK to projects).
	_, err = env.DB.Pool.Exec(ctx, `
		INSERT INTO workers (id, project_id, queue_name, hostname, version, status, last_seen_at, registered_at)
		VALUES ('stale-worker', 'proj-stale', 'q1', 'host1', '1.0', 'active', NOW() - INTERVAL '1 hour', NOW() - INTERVAL '1 hour')
		ON CONFLICT (id) DO UPDATE SET last_seen_at = NOW() - INTERVAL '1 hour', status = 'active'
	`)
	if err != nil {
		t.Fatalf("insert stale worker: %v", err)
	}

	// Cutoff: 5 minutes ago — the worker's last_seen_at is 1 hour ago so it's stale.
	cutoff := time.Now().Add(-5 * time.Minute)
	n, err := q.EvictStaleWorkers(ctx, cutoff)
	if err != nil {
		t.Fatalf("evict stale workers: %v", err)
	}
	if n == 0 {
		t.Error("expected at least 1 worker to be evicted")
	}

	// Verify the worker status is now 'offline'.
	var status string
	err = env.DB.Pool.QueryRow(ctx,
		`SELECT status FROM workers WHERE id = $1`,
		"stale-worker",
	).Scan(&status)
	if err != nil {
		t.Fatalf("query worker status: %v", err)
	}
	if status != "offline" {
		t.Errorf("expected offline status, got %s", status)
	}
}

// TestIntegration_CloseByAPIKey_ViaRevokeCh verifies that CloseByAPIKey closes
// the revokeCh for all workers under the given API key, allowing the stream goroutine
// to detect the signal and exit.
func TestIntegration_CloseByAPIKey_ViaRevokeCh(t *testing.T) {
	ctx := context.Background()

	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })

	reg := NewConnectionRegistry()

	// Register workers under the same API key.
	w1 := makeWorker("w1", "proj-a", "key-revoke", []string{"q"}, 4)
	w2 := makeWorker("w2", "proj-a", "key-revoke", []string{"q"}, 4)

	if err := reg.Register(w1); err != nil {
		t.Fatalf("register w1: %v", err)
	}
	if err := reg.Register(w2); err != nil {
		t.Fatalf("register w2: %v", err)
	}

	// Simulate revocation signal.
	reg.CloseByAPIKey("key-revoke")

	// Both workers' revokeCh must be closed.
	for _, w := range []*ConnectedWorker{w1, w2} {
		select {
		case <-w.revokeCh:
			// closed — expected
		case <-time.After(100 * time.Millisecond):
			t.Errorf("revokeCh for worker %s was not closed within timeout", w.WorkerID)
		}
	}
	_ = ctx
}

// TestIntegration_Registry_DBSync_Roundtrip verifies full roundtrip: register → dbSync → query.
func TestIntegration_Registry_DBSync_Roundtrip(t *testing.T) {
	ctx := context.Background()

	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })

	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean tables: %v", err)
	}

	q := store.New(env.DB.Pool)
	reg := NewConnectionRegistry()

	// Register multiple workers (workers table has no FK constraint to projects).
	for i := range 3 {
		w := makeWorker(
			workerIDFromIndex(i),
			"proj-rt",
			keyIDFromIndex(i),
			[]string{"q"},
			4,
		)
		if err := reg.Register(w); err != nil {
			t.Fatalf("register worker %d: %v", i, err)
		}
	}

	// Run DB sync.
	dbSyncOnce(ctx, reg, q)

	// All 3 workers should be in the DB.
	var count int
	err = env.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM workers WHERE project_id = 'proj-rt'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count workers: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 workers in DB, got %d", count)
	}
}

// TestIntegration_Noop_Publisher verifies that a no-op publisher satisfies the interface.
func TestIntegration_Noop_Publisher(t *testing.T) {
	pub := &noopPublisher{}
	ctx := context.Background()

	if err := pub.Publish(ctx, "channel", []byte("data")); err != nil {
		t.Errorf("Publish failed: %v", err)
	}
	if err := pub.PublishBatch(ctx, nil); err != nil {
		t.Errorf("PublishBatch failed: %v", err)
	}
	sub, err := pub.Subscribe(ctx, "channel")
	if err != nil {
		t.Errorf("Subscribe failed: %v", err)
	}
	if sub == nil {
		t.Error("expected non-nil subscription")
	}
	if err := pub.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// Helper functions for integration tests.

func workerIDFromIndex(i int) string {
	return "rt-worker-" + itoa(i)
}

func keyIDFromIndex(i int) string {
	return "rt-key-" + itoa(i)
}

func itoa(i int) string {
	return [...]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}[i]
}

// noopPublisher is a no-op implementation of pubsub.Publisher for integration tests
// that don't require real pub/sub behavior.
type noopPublisher struct{}

func (n *noopPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (n *noopPublisher) PublishBatch(_ context.Context, _ []pubsub.PubSubMessage) error {
	return nil
}
func (n *noopPublisher) Subscribe(ctx context.Context, _ string) (*pubsub.Subscription, error) {
	ch := make(chan []byte)
	ctx2, cancel := context.WithCancel(ctx)
	go func() {
		<-ctx2.Done()
		close(ch)
	}()
	return pubsub.NewSubscription(ch, cancel), nil
}
func (n *noopPublisher) Close() error { return nil }
