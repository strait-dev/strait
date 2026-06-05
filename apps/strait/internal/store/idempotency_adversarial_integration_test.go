//go:build integration

package store_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/store"
)

// TestIdempotency_SustainedHotKey_NoErrors hammers a single project_id/key
// pair from many goroutines with bounded lock_timeout and asserts that the
// retry loop absorbs any transient failures rather than surfacing them.
func TestIdempotency_SustainedHotKey_NoErrors(t *testing.T) {
	ctx := context.Background()
	cleanIdempotencyKeys(t, ctx)
	q := mustStore(t)

	if _, err := testDB.Pool.Exec(ctx, "SET lock_timeout = '500ms'"); err != nil {
		require.Failf(t, "test failure",

			"set lock_timeout: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testDB.Pool.Exec(ctx, "RESET lock_timeout")
	})

	const concurrent = 32
	const rounds = 4
	projectID := "proj-sustain-" + newID()

	var errs, acquired, pending int64
	var wg conc.WaitGroup
	for round := range rounds {
		round := round
		// Cycle through a small set of hot keys per round so contention is
		// real but does not deadlock against itself across rounds.
		keys := []string{"hot-a", "hot-b", "hot-c", "hot-d"}
		for i := range concurrent {
			i := i
			wg.Go(func() {
				key := keys[(i+round)%len(keys)]
				status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Minute)
				if err != nil {
					atomic.AddInt64(&errs, 1)
					assert.Failf(t, "test failure",

						"round=%d i=%d: %v", round, i, err)
					return
				}
				switch status {
				case store.IdempotencyAcquired:
					atomic.AddInt64(&acquired, 1)
				case store.IdempotencyPending:
					atomic.AddInt64(&pending, 1)
				}
			})
		}
	}
	wg.Wait()
	require.EqualValues(t, 0, errs)
	require.EqualValues(t, 4, acquired)
	require.EqualValues(t, concurrent*
		rounds,
		acquired+
			pending)

	// Exactly len(keys) acquisitions; rest are pending replays.

}

// TestIdempotency_AcquireThenCompleteThenReacquireAcrossExpiry walks the full
// lifecycle under contention: acquire, complete, expire, re-acquire. The
// expiry happens between phases via a manual UPDATE since we can't sleep
// long enough.
func TestIdempotency_AcquireThenCompleteThenReacquireAcrossExpiry(t *testing.T) {
	ctx := context.Background()
	cleanIdempotencyKeys(t, ctx)
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")

	projectID := "proj-lifecycle-" + newID()
	key := "key-lifecycle-" + newID()

	status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	require.False(t, err !=

		nil || status !=
		store.
			IdempotencyAcquired,
	)
	require.NoError(t, q.CompleteIdempotencyKey(
		ctx, projectID,
		key, 200,
		nil, []byte(`"phase1"`)))

	// Replay should see completed. JSONB stores quoted-string scalars with no
	// extra whitespace, so an exact match is safe here.
	status, _, _, body, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	require.False(t, err !=

		nil || status !=
		store.
			IdempotencyComplete ||
		string(body) != `"phase1"`)

	// Force-expire and re-acquire under contention.
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE idempotency_keys SET expires_at = NOW() - INTERVAL '1 hour'
		WHERE project_id = $1 AND key = $2`, projectID, key); err != nil {
		require.Failf(t, "test failure",

			"force-expire: %v", err)
	}

	const concurrent = 16
	var acquired, pending int64
	var wg conc.WaitGroup
	for range concurrent {
		wg.Go(func() {
			s, _, _, _, e := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
			assert.Nil(t, e)

			switch s {
			case store.IdempotencyAcquired:
				atomic.AddInt64(&acquired, 1)
			case store.IdempotencyPending:
				atomic.AddInt64(&pending, 1)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, acquired)
	require.EqualValues(t, concurrent-
		1,
		pending)

}
