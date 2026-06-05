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
	"github.com/stretchr/testify/require"

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
	require.NoError(t, err)
	require.Equal(t, store.
		IdempotencyAcquired,

		status)
	require.False(t, code !=

		0 || body !=
		nil)

}

func TestIdempotency_TryAcquire_Pending_ReturnsPending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-idem-" + newID()
	key := "key-" + newID()

	// First call wins and acquires.
	status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	require.NoError(t, err)
	require.Equal(t, store.
		IdempotencyAcquired,

		status)

	// Second call sees a pending row.
	status, _, _, _, err = q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	require.NoError(t, err)
	require.Equal(t, store.
		IdempotencyPending,

		status,
	)

}

func TestIdempotency_TryAcquire_Completed_ReturnsCachedResponse(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	projectID := "proj-idem-" + newID()
	key := "key-" + newID()

	if _, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour); err != nil {
		require.Failf(t, "test failure",

			"acquire: %v", err)
	}
	wantBody := []byte(`{"id":"abc"}`)
	require.NoError(t, q.CompleteIdempotencyKey(
		ctx, projectID, key,
		201, nil,
		wantBody))

	status, code, _, body, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	require.NoError(t, err)
	require.Equal(t, store.
		IdempotencyComplete,

		status)
	require.EqualValues(t, 201, code)

	// Postgres reformats JSONB on ingestion (adds a space after colons
	// and reorders keys), so byte-equal comparison is too strict. Parse
	// and compare semantically.
	var got, want map[string]any
	require.NoError(t, json.
		Unmarshal(body, &got))
	require.NoError(t, json.
		Unmarshal(wantBody,
			&want))
	require.True(t, reflect.
		DeepEqual(got, want),
	)

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
		require.Failf(t, "test failure",

			"first acquire: %v", err)
	}

	// Second call should delete the expired row and reacquire.
	status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	require.NoError(t, err)
	require.Equal(t, store.
		IdempotencyAcquired,

		status)

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
		require.Nil(t, firstErrMsg.
			Load())
		require.Failf(t, "test failure",

			"race produced %d errors", errors.Load())
	}
	require.EqualValues(t, 1, acquired.
		Load())
	require.EqualValues(t, goroutines-
		1,
		pending.Load())

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
		require.Failf(t, "test failure",

			"acquire: %v", err)
	}
	require.NoError(t, q.CompleteIdempotencyKey(
		ctx, projectID, key,
		200, nil,
		[]byte(`"ok"`)))

	// Read back directly via TryAcquire's completed branch.
	status, code, _, body, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	require.NoError(t, err)
	require.Equal(t, store.
		IdempotencyComplete,

		status)
	require.EqualValues(t, 200, code)

	var got any
	require.NoError(t, json.
		Unmarshal(body, &got))
	require.Equal(t, "ok",
		got,
	)

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
	require.NoError(t, err)

}

// DeleteIdempotencyKey

func TestIdempotency_Delete_RemovesRow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-idem-" + newID()
	key := "key-" + newID()

	if _, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour); err != nil {
		require.Failf(t, "test failure",

			"acquire: %v", err)
	}

	rows, err := q.DeleteIdempotencyKey(ctx, projectID, key)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	// After delete, the key should be reacquirable.
	status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, time.Hour)
	require.NoError(t, err)
	require.Equal(t, store.
		IdempotencyAcquired,

		status)

}

func TestIdempotency_Delete_NotFound_Zero(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	rows, err := q.DeleteIdempotencyKey(ctx, "proj-missing-"+newID(), "key-missing")
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)

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
			require.Failf(t, "test failure",

				"insert expired %d: %v", i, err)
		}
	}
	// 10 non-expired rows.
	for i := range 10 {
		if _, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, "fresh-"+newID(), time.Hour); err != nil {
			require.Failf(t, "test failure",

				"insert fresh %d: %v", i, err)
		}
	}

	deleted, err := q.CleanExpiredIdempotencyKeys(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(
		t,
		deleted,
		int64(25))

	// Verify 10 fresh rows remain by directly counting under the test
	// project. idempotency_keys has no RLS policy so a direct pool
	// count works.
	var remaining int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM idempotency_keys WHERE project_id = $1`,

		projectID).Scan(&remaining))
	require.EqualValues(t, 10, remaining)

}
