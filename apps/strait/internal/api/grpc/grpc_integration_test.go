//go:build integration

package grpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/pubsub"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/sourcegraph/conc"
)

// TestIntegration_DBSync_WorkerVisible verifies that after the DB sync interval fires,
// a registered worker is visible in the workers table.
func TestIntegration_DBSync_WorkerVisible(t *testing.T) {
	ctx := context.Background()

	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	reg := NewConnectionRegistry()
	w := makeWorker("sync-worker", "proj-1", "key-sync", []string{"q1"}, 4)
	require.NoError(t,

		reg.Register(w))

	// Upsert directly to simulate what dbSyncOnce does.
	dbSyncOnce(ctx, reg, q)

	// Verify the worker row exists in the workers table.
	var id string
	err := env.DB.Pool.QueryRow(ctx,
		`SELECT id FROM workers WHERE id = $1 AND project_id = $2`,
		"sync-worker", "proj-1",
	).Scan(&id)
	require.NoError(t,

		err)
	assert.Equal(t, "sync-worker",

		id)

}

// TestIntegration_Sweep_EvictsStaleWorkers verifies that EvictStaleWorkers marks stale workers offline.
func TestIntegration_Sweep_EvictsStaleWorkers(t *testing.T) {
	ctx := context.Background()

	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)

	// Insert a worker with a very old last_seen_at directly (workers table has no FK to projects).
	_, err := env.DB.Pool.Exec(ctx, `
		INSERT INTO workers (id, project_id, queue_name, hostname, version, status, last_seen_at, registered_at)
		VALUES ('stale-worker', 'proj-stale', 'q1', 'host1', '1.0', 'active', NOW() - INTERVAL '1 hour', NOW() - INTERVAL '1 hour')
		ON CONFLICT (project_id, id) DO UPDATE SET last_seen_at = NOW() - INTERVAL '1 hour', status = 'active'
	`)
	require.NoError(t,

		err)

	// Cutoff: 5 minutes ago — the worker's last_seen_at is 1 hour ago so it's stale.
	cutoff := time.Now().Add(-5 * time.Minute)
	n, err := q.EvictStaleWorkers(ctx, cutoff)
	require.NoError(t,

		err)
	assert.NotEqual(t,

		0, n)

	// Verify the worker status is now 'offline'.
	var status string
	err = env.DB.Pool.QueryRow(ctx,
		`SELECT status FROM workers WHERE id = $1 AND project_id = $2`,
		"stale-worker", "proj-stale",
	).Scan(&status)
	require.NoError(t,

		err)
	assert.Equal(t, "offline",

		status)

}

// TestIntegration_CloseByAPIKey_ViaRevokeCh verifies that CloseByAPIKey closes
// the revokeCh for all workers under the given API key, allowing the stream goroutine
// to detect the signal and exit.
func TestIntegration_CloseByAPIKey_ViaRevokeCh(t *testing.T) {
	ctx := context.Background()

	_ = cleanIntegrationEnv(t, ctx)

	reg := NewConnectionRegistry()

	// Register workers under the same API key.
	w1 := makeWorker("w1", "proj-a", "key-revoke", []string{"q"}, 4)
	w2 := makeWorker("w2", "proj-a", "key-revoke", []string{"q"}, 4)
	require.NoError(t,

		reg.Register(w1))
	require.NoError(t,

		reg.Register(w2))

	// Simulate revocation signal.
	reg.CloseByAPIKey("key-revoke")

	// Both workers' revokeCh must be closed.
	for _, w := range []*ConnectedWorker{w1, w2} {
		select {
		case <-w.revokeCh:
			// closed — expected
		case <-time.After(100 * time.Millisecond):
			assert.Failf(t, "test failure", "revokeCh for worker %s was not closed within timeout", w.WorkerID)
		}
	}
	_ = ctx
}

// TestIntegration_Registry_DBSync_Roundtrip verifies full roundtrip: register → dbSync → query.
func TestIntegration_Registry_DBSync_Roundtrip(t *testing.T) {
	ctx := context.Background()

	env := cleanIntegrationEnv(t, ctx)

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
		require.NoError(t,

			reg.Register(w))

	}

	// Run DB sync.
	dbSyncOnce(ctx, reg, q)

	// All 3 workers should be in the DB.
	var count int
	err := env.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM workers WHERE project_id = 'proj-rt'`,
	).Scan(&count)
	require.NoError(t,

		err)
	assert.EqualValues(t, 3,

		count)

}

// TestIntegration_Noop_Publisher verifies that a no-op publisher satisfies the interface.
func TestIntegration_Noop_Publisher(t *testing.T) {
	pub := &noopPublisher{}
	ctx := context.Background()
	assert.NoError(t,

		pub.Publish(ctx, "channel",

			[]byte("data")))
	assert.NoError(t,

		pub.PublishBatch(ctx,
			nil))

	sub, err := pub.Subscribe(ctx, "channel")
	assert.NoError(t,

		err)
	assert.NotNil(t,
		sub,
	)
	assert.NoError(t,

		pub.Close())

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

// TestIntegration_CrossReplica_WorkerDisconnect verifies that publishing to the
// project-scoped worker disconnect Redis channel causes a subscribed stream to
// receive the signal. This exercises the cross-replica disconnect path
// end-to-end: the DELETE /v1/workers/:id handler publishes to this channel;
// the stream goroutine subscribes and closes the stream on receipt.
func TestIntegration_CrossReplica_WorkerDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r, err := testutil.SetupSharedTestRedis(ctx, "api-grpc-redis")
	require.NoError(t,

		err)

	t.Cleanup(func() { r.Cleanup(context.Background()) })

	// Build a publisher and subscriber backed by the same Redis container.
	client := redis.NewClient(r.Options())
	t.Cleanup(func() { _ = client.Close() })
	pub := pubsub.NewRedisPublisher(client)

	workerID := "cross-replica-worker-1"
	projectID := "proj-cross-replica"
	channel := workerDisconnectChannel(projectID, workerID)

	sub, err := pub.Subscribe(ctx, channel)
	require.NoError(t,

		err)

	defer sub.Close()
	require.NoError(t,

		pub.Publish(ctx, channel,

			[]byte(workerID)))

	// Publish the disconnect signal (what DELETE /v1/workers/:id does).

	// The stream goroutine selects on sub.Ch; verify the message arrives.
	select {
	case msg := <-sub.Ch:
		if string(msg) != workerID {
			assert.Failf(t, "test failure",

				"received payload %q, want %q", msg, workerID)
		}
	case <-ctx.Done():
		require.Fail(t, "timed out waiting for disconnect signal on Redis channel")
	}
}

// TestIntegration_CrossReplica_APIKeyRevoke verifies that publishing to the
// apikey:revoked:<id> Redis channel causes a subscribed stream to receive the
// signal. This exercises the cross-replica revocation path: the POST
// /v1/api-keys/:id/revoke handler publishes to this channel; the stream goroutine
// subscribes and calls registry.CloseByAPIKey on receipt.
func TestIntegration_CrossReplica_APIKeyRevoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r, err := testutil.SetupSharedTestRedis(ctx, "api-grpc-redis")
	require.NoError(t,

		err)

	t.Cleanup(func() { r.Cleanup(context.Background()) })

	client := redis.NewClient(r.Options())
	t.Cleanup(func() { _ = client.Close() })
	pub := pubsub.NewRedisPublisher(client)

	apiKeyID := "key-revoke-cross-replica"
	channel := fmt.Sprintf("apikey:revoked:%s", apiKeyID)

	sub, err := pub.Subscribe(ctx, channel)
	require.NoError(t,

		err)

	defer sub.Close()
	require.NoError(t,

		pub.Publish(ctx, channel,

			[]byte(apiKeyID)))

	// Publish the revocation signal (what POST /v1/api-keys/:id/revoke does).

	select {
	case msg := <-sub.Ch:
		if string(msg) != apiKeyID {
			assert.Failf(t, "test failure",

				"received payload %q, want %q", msg, apiKeyID)
		}
	case <-ctx.Done():
		require.Fail(t, "timed out waiting for revoke signal on Redis channel")
	}
}

// TestIntegration_CrossReplica_APIKeyRevoke_RegistryCloseByAPIKey verifies the full
// revocation flow: publish to Redis, receive in subscriber, call CloseByAPIKey,
// observe all matching streams' revokeCh closed.
func TestIntegration_CrossReplica_APIKeyRevoke_RegistryCloseByAPIKey(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r, err := testutil.SetupSharedTestRedis(ctx, "api-grpc-redis")
	require.NoError(t,

		err)

	t.Cleanup(func() { r.Cleanup(context.Background()) })

	client := redis.NewClient(r.Options())
	t.Cleanup(func() { _ = client.Close() })
	pub := pubsub.NewRedisPublisher(client)

	apiKeyID := "key-full-revoke-flow"
	channel := fmt.Sprintf("apikey:revoked:%s", apiKeyID)

	// Set up registry with workers under the key being revoked.
	reg := NewConnectionRegistry()
	w1 := makeWorker("w-full-1", "proj-b", apiKeyID, []string{"q"}, 2)
	w2 := makeWorker("w-full-2", "proj-b", apiKeyID, []string{"q"}, 2)
	require.NoError(t,

		reg.Register(w1))
	require.NoError(t,

		reg.Register(w2))

	// Subscribe to the revocation channel (simulating what the stream goroutine does).
	sub, err := pub.Subscribe(ctx, channel)
	require.NoError(t,

		err)

	// Start a goroutine that simulates the stream goroutine's select on the revokeCh.
	done := make(chan struct{})
	concWG.Go(func() {
		defer close(done)
		select {
		case <-sub.Ch:
			// Signal received from Redis — close all matching streams locally.
			reg.CloseByAPIKey(apiKeyID)
		case <-ctx.Done():
		}
	})
	require.NoError(t,

		pub.Publish(ctx, channel,

			[]byte(apiKeyID)))

	// Publish revocation (simulating POST /v1/api-keys/:id/revoke).

	// Wait for the goroutine to process the signal.
	select {
	case <-done:
	case <-ctx.Done():
		require.Fail(t, "timed out waiting for revoke goroutine")
	}
	sub.Close()

	// Both workers' revokeCh must be closed.
	for _, w := range []*ConnectedWorker{w1, w2} {
		select {
		case <-w.revokeCh:
			// closed as expected
		case <-time.After(100 * time.Millisecond):
			assert.Failf(t, "test failure", "revokeCh for worker %s was not closed", w.WorkerID)
		}
	}
}

// noopPublisher is a no-op implementation of pubsub.Publisher for integration tests
// that don't require real pub/sub behavior.
type noopPublisher struct{}

func (n *noopPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (n *noopPublisher) PublishBatch(_ context.Context, _ []pubsub.PubSubMessage) error {
	return nil
}
func (n *noopPublisher) Subscribe(ctx context.Context, _ string) (*pubsub.Subscription, error) {
	var concWG conc.WaitGroup
	ch := make(chan []byte)
	ctx2, cancel := context.WithCancel(ctx)
	concWG.Go(func() {
		<-ctx2.Done()
		close(ch)
	})
	return pubsub.NewSubscription(ch, func() {
		cancel()
		concWG.Wait()
	}), nil
}
func (n *noopPublisher) Close() error { return nil }
