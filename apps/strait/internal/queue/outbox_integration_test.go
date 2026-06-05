//go:build integration

package queue_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/queue"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutbox_WriteInTxHappyPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-outbox-happy")

	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	defer tx.Rollback(ctx)

	entries := []queue.OutboxEntry{{
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"key":"value"}`),
		Metadata:  map[string]any{"source": "test"},
		Priority:  5,
	}}
	require.NoError(t, queue.
		WriteOutboxInTx(ctx,
			tx, entries))
	require.NoError(t, tx.Commit(ctx))

	// Verify the row is present and unconsumed.
	count, err := st.CountUnconsumedOutbox(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

}

func TestOutbox_RollbackLeavesNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-outbox-rollback")

	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, queue.
		WriteOutboxInTx(ctx,
			tx, []queue.OutboxEntry{{ProjectID: job.ProjectID, JobID: job.
				ID}}))
	require.NoError(t, tx.Rollback(ctx))

	count, _ := st.CountUnconsumedOutbox(ctx)
	assert.EqualValues(t, 0, count)

}

func TestOutbox_IdempotentOnIDConflict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-outbox-idem")

	fixedID := newID()
	for range 3 {
		tx, err := testDB.Pool.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, queue.
			WriteOutboxInTx(ctx,
				tx, []queue.OutboxEntry{{ID: fixedID,

					ProjectID: job.ProjectID,
					JobID:     job.ID}}))
		require.NoError(t, tx.Commit(ctx))

	}
	count, _ := st.CountUnconsumedOutbox(ctx)
	assert.EqualValues(t, 1, count)

}

func TestOutbox_ClaimMarkCycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-outbox-claim")

	// Write 3 entries.
	tx, _ := testDB.Pool.Begin(ctx)
	_ = queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{
		{ProjectID: job.ProjectID, JobID: job.ID},
		{ProjectID: job.ProjectID, JobID: job.ID},
		{ProjectID: job.ProjectID, JobID: job.ID},
	})
	_ = tx.Commit(ctx)

	// Flusher-style claim + mark in the same transaction.
	flushTx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	rows, err := st.ClaimUnconsumedOutbox(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, rows, 3)

	var ids []string
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	require.NoError(t, st.MarkOutboxConsumed(ctx,
		ids))

	_ = flushTx.Commit(ctx)

	// All consumed now.
	count, _ := st.CountUnconsumedOutbox(ctx)
	assert.EqualValues(t, 0, count)

}

func TestOutbox_OldestAge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-outbox-age")

	tx, _ := testDB.Pool.Begin(ctx)
	_ = queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{{
		ProjectID: job.ProjectID, JobID: job.ID,
	}})
	_ = tx.Commit(ctx)

	time.Sleep(100 * time.Millisecond)
	age, err := st.OldestUnconsumedOutboxAge(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t,

		age, 50*
			time.Millisecond,
	)

}

func TestOutbox_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	defer tx.Rollback(ctx)

	cases := []queue.OutboxEntry{
		{JobID: "j1"},     // missing project
		{ProjectID: "p1"}, // missing job
	}
	for _, e := range cases {
		assert.Error(t, queue.WriteOutboxInTx(ctx, tx,
			[]queue.OutboxEntry{e}))

	}
}

func TestOutbox_EmptyWriteNoOp(t *testing.T) {
	ctx := context.Background()
	tx, _ := testDB.Pool.Begin(ctx)
	defer tx.Rollback(ctx)
	assert.NoError(t, queue.
		WriteOutboxInTx(ctx,
			tx, nil))
	assert.NoError(t, queue.
		WriteOutboxInTx(ctx,
			tx, []queue.OutboxEntry{}))

}

func TestOutbox_ConcurrentFlushersDoNotDuplicate(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-outbox-concurrent")

	// Write 20 entries.
	tx, _ := testDB.Pool.Begin(ctx)
	var entries []queue.OutboxEntry
	for range 20 {
		entries = append(entries, queue.OutboxEntry{
			ProjectID: job.ProjectID, JobID: job.ID,
		})
	}
	_ = queue.WriteOutboxInTx(ctx, tx, entries)
	_ = tx.Commit(ctx)
	_ = st

	// Two concurrent flushers claiming with SKIP LOCKED. Each flusher
	// must hold its own transaction so the row locks persist across the
	// claim-then-mark-consumed-then-commit sequence.
	type result struct {
		ids []string
	}
	ch := make(chan result, 2)
	for range 2 {
		concWG.Go(func() {
			ftx, _ := testDB.Pool.Begin(ctx)
			defer ftx.Rollback(ctx)
			rows, err := store.ClaimUnconsumedOutboxInTx(ctx, ftx, 20)
			if err != nil {
				ch <- result{}
				return
			}
			var ids []string
			for _, r := range rows {
				ids = append(ids, r.ID)
			}
			_ = store.MarkOutboxConsumedInTx(ctx, ftx, ids)
			_ = ftx.Commit(ctx)
			ch <- result{ids: ids}
		})
	}

	seen := map[string]bool{}
	for range 2 {
		r := <-ch
		for _, id := range r.ids {
			assert.False(t, seen[id])

			seen[id] = true
		}
	}
	assert.Len(t, seen, 20)

	count, _ := st.CountUnconsumedOutbox(ctx)
	assert.EqualValues(t, 0, count)

}
