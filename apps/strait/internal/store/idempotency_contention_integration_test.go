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

	"strait/internal/store"
)

// cleanIdempotencyKeys is a per-test cleanup since CleanTables does not
// include the idempotency_keys table.
func cleanIdempotencyKeys(t *testing.T, ctx context.Context) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx, "DELETE FROM idempotency_keys"); err != nil {
		t.Fatalf("clean idempotency_keys: %v", err)
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
			if err != nil {
				t.Errorf("TryAcquireIdempotencyKey: %v", err)
				return
			}
			switch status {
			case store.IdempotencyAcquired:
				atomic.AddInt64(&acquired, 1)
			case store.IdempotencyPending:
				atomic.AddInt64(&pending, 1)
			default:
				t.Errorf("unexpected status %q", status)
			}
		})
	}
	wg.Wait()

	if acquired != 1 {
		t.Fatalf("acquired = %d, want 1", acquired)
	}
	if pending != n-1 {
		t.Fatalf("pending = %d, want %d", pending, n-1)
	}
}

func TestIdempotency_HotKey_BoundedLockTimeout(t *testing.T) {
	ctx := context.Background()
	cleanIdempotencyKeys(t, ctx)
	q := mustStore(t)

	// Pin lock_timeout low on every backend to assert advisory locking is what
	// serializes — not row locks. If row-locks were the gate we would see 55P03.
	if _, err := testDB.Pool.Exec(ctx, "SET lock_timeout = '250ms'"); err != nil {
		t.Fatalf("set lock_timeout: %v", err)
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
				t.Errorf("acquire: %v", err)
			}
		})
	}
	wg.Wait()
	if errs != 0 {
		t.Fatalf("got %d errors under bounded lock_timeout", errs)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("contention took too long: %v (advisory lock acquire should be fast)", d)
	}
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
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			if status != store.IdempotencyAcquired {
				t.Errorf("status = %q, want acquired", status)
			}
		})
	}
	wg.Wait()
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("100 distinct keys took %v (advisory locks should not serialize distinct buckets)", d)
	}
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
		t.Fatalf("seed expired row: %v", err)
	}

	const n = 16
	var acquired, pending int64
	var wg conc.WaitGroup
	for range n {
		wg.Go(func() {
			status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Minute)
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			switch status {
			case store.IdempotencyAcquired:
				atomic.AddInt64(&acquired, 1)
			case store.IdempotencyPending:
				atomic.AddInt64(&pending, 1)
			default:
				t.Errorf("unexpected status %q", status)
			}
		})
	}
	wg.Wait()

	if acquired != 1 {
		t.Fatalf("expired-row race acquired = %d, want exactly 1", acquired)
	}
	if pending != n-1 {
		t.Fatalf("expired-row race pending = %d, want %d", pending, n-1)
	}
}

func TestIdempotency_CompletedReplayReturnsCachedResponse(t *testing.T) {
	ctx := context.Background()
	cleanIdempotencyKeys(t, ctx)
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")

	projectID := "proj-replay-" + newID()
	key := "key-replay-" + newID()
	if status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour); err != nil || status != store.IdempotencyAcquired {
		t.Fatalf("first acquire: status=%q err=%v", status, err)
	}
	if err := q.CompleteIdempotencyKey(ctx, projectID, key, 201, nil, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("complete: %v", err)
	}

	const n = 32
	var completed, other int64
	var wg conc.WaitGroup
	for range n {
		wg.Go(func() {
			status, rs, _, body, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
			if err != nil {
				t.Errorf("replay acquire: %v", err)
				return
			}
			if status == store.IdempotencyComplete {
				atomic.AddInt64(&completed, 1)
				if rs != 201 {
					t.Errorf("replay response status = %d, want 201", rs)
				}
				// JSONB normalizes whitespace; check structural equivalence.
				if !bytes.Contains(body, []byte(`"ok"`)) || !bytes.Contains(body, []byte(`true`)) {
					t.Errorf("replay body = %q, want cached {ok:true}", body)
				}
			} else {
				atomic.AddInt64(&other, 1)
			}
		})
	}
	wg.Wait()
	if completed != n {
		t.Fatalf("completed replays = %d, want %d (other=%d)", completed, n, other)
	}
}
