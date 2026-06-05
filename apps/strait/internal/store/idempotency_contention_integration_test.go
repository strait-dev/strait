//go:build integration

package store_test

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/store"
)

// cleanIdempotencyKeys is a per-test cleanup since CleanTables does not
// include the idempotency_keys table.
func cleanIdempotencyKeys(t *testing.T, ctx context.Context) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx, "DELETE FROM idempotency_keys"); err != nil {
		require.Failf(t, "test failure",

			"clean idempotency_keys: %v", err)
	}
}

func TestIdempotency_HotKey_ExactlyOneWinner(t *testing.T) {
	ctx := context.Background()
	cleanIdempotencyKeys(t, ctx)
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")

	const n = 64
	projectID := "proj-hot-" + newID()
	key := "key-hot-" + newID()

	var acquired, pending int64
	var wg conc.WaitGroup
	for range n {
		wg.Go(func() {
			status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Minute)
			assert.NoError(t, err)

			switch status {
			case store.IdempotencyAcquired:
				atomic.AddInt64(&acquired, 1)
			case store.IdempotencyPending:
				atomic.AddInt64(&pending, 1)
			default:
				assert.Failf(t, "test failure", "unexpected status %q", status)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, acquired)
	require.EqualValues(t, n-1,

		pending)

}

func TestIdempotency_HotKey_BoundedLockTimeout(t *testing.T) {
	ctx := context.Background()
	cleanIdempotencyKeys(t, ctx)
	q := mustStore(t)

	// Pin lock_timeout low on every backend to assert advisory locking is what
	// serializes — not row locks. If row-locks were the gate we would see 55P03.
	if _, err := testDB.Pool.Exec(ctx, "SET lock_timeout = '250ms'"); err != nil {
		require.Failf(t, "test failure",

			"set lock_timeout: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testDB.Pool.Exec(ctx, "RESET lock_timeout")
	})

	const n = 32
	projectID := "proj-bounded-" + newID()
	key := "key-bounded-" + newID()

	var errs int64
	var wg conc.WaitGroup
	start := time.Now()
	for range n {
		wg.Go(func() {
			if _, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Minute); err != nil {
				atomic.AddInt64(&errs, 1)
				assert.Failf(t, "test failure",

					"acquire: %v", err)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 0, errs)
	require.LessOrEqual(t,
		time.
			Since(start),
		5*time.Second,
	)

}

func TestIdempotency_DifferentKeys_NoSerialization(t *testing.T) {
	ctx := context.Background()
	cleanIdempotencyKeys(t, ctx)
	q := mustStore(t)

	const n = 100
	projectID := "proj-distinct-" + newID()

	var wg conc.WaitGroup
	start := time.Now()
	for i := range n {
		i := i
		wg.Go(func() {
			key := fmt.Sprintf("key-%d", i)
			status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Minute)
			assert.NoError(t, err)
			assert.Equal(t, store.IdempotencyAcquired,

				status)

		})
	}
	wg.Wait()
	require.LessOrEqual(t,
		time.
			Since(start),
		5*time.Second,
	)

}

func TestIdempotency_ExpiredKeyContention(t *testing.T) {
	ctx := context.Background()
	cleanIdempotencyKeys(t, ctx)
	q := mustStore(t)

	projectID := "proj-expired-" + newID()
	key := "key-expired-" + newID()

	// Seed an expired row.
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO idempotency_keys (project_id, key, status, expires_at)
		VALUES ($1, $2, 'completed', NOW() - INTERVAL '1 hour')`,
		projectID, key,
	); err != nil {
		require.Failf(t, "test failure",

			"seed expired row: %v", err)
	}

	const n = 16
	var acquired, pending int64
	var wg conc.WaitGroup
	for range n {
		wg.Go(func() {
			status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Minute)
			assert.NoError(t, err)

			switch status {
			case store.IdempotencyAcquired:
				atomic.AddInt64(&acquired, 1)
			case store.IdempotencyPending:
				atomic.AddInt64(&pending, 1)
			default:
				assert.Failf(t, "test failure", "unexpected status %q", status)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, acquired)
	require.EqualValues(t, n-1,

		pending)

}

func TestIdempotency_CompletedReplayReturnsCachedResponse(t *testing.T) {
	ctx := context.Background()
	cleanIdempotencyKeys(t, ctx)
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")

	projectID := "proj-replay-" + newID()
	key := "key-replay-" + newID()
	if status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour); err != nil || status != store.IdempotencyAcquired {
		require.Failf(t, "test failure",

			"first acquire: status=%q err=%v", status, err)
	}
	require.NoError(t, q.CompleteIdempotencyKey(ctx, projectID,

		key, 201, nil,
		[]byte(`{"ok":true}`)))

	const n = 32
	var completed, other int64
	var wg conc.WaitGroup
	for range n {
		wg.Go(func() {
			status, rs, _, body, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
			assert.NoError(t, err)

			if status == store.IdempotencyComplete {
				atomic.AddInt64(&completed, 1)
				assert.EqualValues(t, 201, rs)
				assert.False(t, !bytes.
					Contains(body,
						[]byte(`"ok"`)) ||
					!bytes.
						Contains(body,
							[]byte(
								`true`)),
				)

				// JSONB normalizes whitespace; check structural equivalence.

			} else {
				atomic.AddInt64(&other, 1)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, n, completed)

}
