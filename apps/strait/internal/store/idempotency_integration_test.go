//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"

	"strait/internal/store"
)

// Integration tests for the idempotency foursome, which had zero
// integration coverage prior to this commit despite implementing a
// three-way race (acquire/pending/completed) that's critical for
// distributed request deduplication.
// TryAcquireIdempotencyKey

func TestIdempotency_TryAcquire_NewKey_Acquired(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	status, code, _, body, err := q.TryAcquireIdempotencyKey(ctx, "proj-idem-"+newID(), "key-"+newID(), time.Hour)
	if err != nil {
		t.Fatalf("TryAcquireIdempotencyKey error = %v", err)
	}
	if status != store.IdempotencyAcquired {
		t.Fatalf("status = %q, want acquired", status)
	}
	if code != 0 || body != nil {
		t.Fatalf("fresh key returned code=%d body=%q, want zero values", code, body)
	}
}

func TestIdempotency_TryAcquire_Pending_ReturnsPending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-idem-" + newID()
	key := "key-" + newID()

	// First call wins and acquires.
	status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	if err != nil {
		t.Fatalf("first TryAcquire: %v", err)
	}
	if status != store.IdempotencyAcquired {
		t.Fatalf("first status = %q, want acquired", status)
	}

	// Second call sees a pending row.
	status, _, _, _, err = q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	if err != nil {
		t.Fatalf("second TryAcquire: %v", err)
	}
	if status != store.IdempotencyPending {
		t.Fatalf("second status = %q, want pending", status)
	}
}

func TestIdempotency_TryAcquire_Completed_ReturnsCachedResponse(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	projectID := "proj-idem-" + newID()
	key := "key-" + newID()

	if _, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	wantBody := []byte(`{"id":"abc"}`)
	if err := q.CompleteIdempotencyKey(ctx, projectID, key, 201, nil, wantBody); err != nil {
		t.Fatalf("complete: %v", err)
	}
	var rawBody []byte
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT response_body FROM idempotency_keys WHERE project_id = $1 AND key = $2`,
		projectID, key,
	).Scan(&rawBody); err != nil {
		t.Fatalf("read raw response body: %v", err)
	}
	if string(rawBody) == string(wantBody) {
		t.Fatalf("response_body stored plaintext: %s", rawBody)
	}

	status, code, _, body, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	if err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	if status != store.IdempotencyComplete {
		t.Fatalf("status = %q, want completed", status)
	}
	if code != 201 {
		t.Fatalf("cached status code = %d, want 201", code)
	}
	// Postgres reformats JSONB on ingestion (adds a space after colons
	// and reorders keys), so byte-equal comparison is too strict. Parse
	// and compare semantically.
	var got, want map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal cached body: %v", err)
	}
	if err := json.Unmarshal(wantBody, &want); err != nil {
		t.Fatalf("unmarshal want body: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cached body = %v, want %v", got, want)
	}
}

func TestIdempotency_TryAcquire_ExpiredReacquire(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-idem-" + newID()
	key := "key-" + newID()

	// Acquire with a TTL that's already in the past by the time the row
	// is committed — the expires_at check in the SELECT will trigger
	// the stale-row cleanup and retry path.
	if _, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, -time.Second); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// Second call should delete the expired row and reacquire.
	status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	if status != store.IdempotencyAcquired {
		t.Fatalf("status = %q, want acquired (expired reacquire path)", status)
	}
}

func TestIdempotency_TryAcquire_RaceBetweenGoroutines(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-idem-race-" + newID()
	key := "key-race-" + newID()

	const goroutines = 50
	var (
		wg          conc.WaitGroup
		acquired    atomic.Int32
		pending     atomic.Int32
		errors      atomic.Int32
		firstErrMsg atomic.Pointer[string]
	)
	start := make(chan struct{})

	for range goroutines {
		wg.Go(func() {
			<-start
			status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
			if err != nil {
				errors.Add(1)
				msg := err.Error()
				firstErrMsg.CompareAndSwap(nil, &msg)
				return
			}
			switch status {
			case store.IdempotencyAcquired:
				acquired.Add(1)
			case store.IdempotencyPending:
				pending.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()

	if errors.Load() > 0 {
		if p := firstErrMsg.Load(); p != nil {
			t.Fatalf("race produced %d errors, first: %s", errors.Load(), *p)
		}
		t.Fatalf("race produced %d errors", errors.Load())
	}
	if got := acquired.Load(); got != 1 {
		t.Fatalf("acquired = %d, want exactly 1 (race not exclusive)", got)
	}
	if got := pending.Load(); got != goroutines-1 {
		t.Fatalf("pending = %d, want %d", got, goroutines-1)
	}
}

// CompleteIdempotencyKey

func TestIdempotency_Complete_UpdatesRow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	projectID := "proj-idem-" + newID()
	key := "key-" + newID()

	if _, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := q.CompleteIdempotencyKey(ctx, projectID, key, 200, nil, []byte(`"ok"`)); err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Read back directly via TryAcquire's completed branch.
	status, code, _, body, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if status != store.IdempotencyComplete {
		t.Fatalf("status = %q, want completed", status)
	}
	if code != 200 {
		t.Fatalf("code = %d, want 200", code)
	}
	var got any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got != "ok" {
		t.Fatalf("body = %v, want \"ok\"", got)
	}
}

func TestIdempotency_Complete_NotFound_NoError(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	// Completing a non-existent key should be a no-op (UPDATE affects 0
	// rows). The function does not differentiate this from "row with
	// status != pending" — both are silently ignored, which is the
	// correct behavior for an idempotent retry path.
	err := q.CompleteIdempotencyKey(ctx, "proj-missing-"+newID(), "key-missing", 200, nil, []byte(`"ok"`))
	if err != nil {
		t.Fatalf("complete(not found) should be no-op, got %v", err)
	}
}

// DeleteIdempotencyKey

func TestIdempotency_Delete_RemovesRow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-idem-" + newID()
	key := "key-" + newID()

	if _, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	rows, err := q.DeleteIdempotencyKey(ctx, projectID, key)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rows != 1 {
		t.Fatalf("deleted rows = %d, want 1", rows)
	}

	// After delete, the key should be reacquirable.
	status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	if err != nil {
		t.Fatalf("reacquire after delete: %v", err)
	}
	if status != store.IdempotencyAcquired {
		t.Fatalf("status = %q, want acquired", status)
	}
}

func TestIdempotency_Delete_NotFound_Zero(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	rows, err := q.DeleteIdempotencyKey(ctx, "proj-missing-"+newID(), "key-missing")
	if err != nil {
		t.Fatalf("delete(not found): %v", err)
	}
	if rows != 0 {
		t.Fatalf("deleted rows = %d, want 0", rows)
	}
}

// CleanExpiredIdempotencyKeys

func TestIdempotency_CleanExpired_DeletesOnlyExpired(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-idem-cleanup-" + newID()

	// 25 expired rows.
	for i := range 25 {
		if _, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, "expired-"+newID(), -time.Minute); err != nil {
			t.Fatalf("insert expired %d: %v", i, err)
		}
	}
	// 10 non-expired rows.
	for i := range 10 {
		if _, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, "fresh-"+newID(), time.Hour); err != nil {
			t.Fatalf("insert fresh %d: %v", i, err)
		}
	}

	deleted, err := q.CleanExpiredIdempotencyKeys(ctx)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted < 25 {
		t.Fatalf("cleanup deleted %d, want at least 25 expired", deleted)
	}

	// Verify 10 fresh rows remain by directly counting under the test
	// project. idempotency_keys has no RLS policy so a direct pool
	// count works.
	var remaining int
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM idempotency_keys WHERE project_id = $1`, projectID,
	).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 10 {
		t.Fatalf("remaining fresh rows = %d, want 10", remaining)
	}
}
