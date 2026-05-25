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
)

func TestOutbox_WriteInTxHappyPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-outbox-happy")

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	entries := []queue.OutboxEntry{{
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"key":"value"}`),
		Metadata:  map[string]any{"source": "test"},
		Priority:  5,
	}}
	if err := queue.WriteOutboxInTx(ctx, tx, entries); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Verify the row is present and unconsumed.
	count, err := st.CountUnconsumedOutbox(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("unconsumed = %d, want 1", count)
	}
}

func TestOutbox_RollbackLeavesNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-outbox-rollback")

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{{
		ProjectID: job.ProjectID,
		JobID:     job.ID,
	}}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	count, _ := st.CountUnconsumedOutbox(ctx)
	if count != 0 {
		t.Errorf("count = %d, want 0 after rollback", count)
	}
}

func TestOutbox_IdempotentOnIDConflict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-outbox-idem")

	fixedID := newID()
	for i := range 3 {
		tx, err := testDB.Pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{{
			ID:        fixedID,
			ProjectID: job.ProjectID,
			JobID:     job.ID,
		}}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
	}
	count, _ := st.CountUnconsumedOutbox(ctx)
	if count != 1 {
		t.Errorf("count = %d, want 1 (idempotent)", count)
	}
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
	if err != nil {
		t.Fatalf("flush begin: %v", err)
	}
	rows, err := st.ClaimUnconsumedOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("claim = %d, want 3", len(rows))
	}
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	if err := st.MarkOutboxConsumed(ctx, ids); err != nil {
		t.Fatalf("mark: %v", err)
	}
	_ = flushTx.Commit(ctx)

	// All consumed now.
	count, _ := st.CountUnconsumedOutbox(ctx)
	if count != 0 {
		t.Errorf("count after mark = %d", count)
	}
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
	if err != nil {
		t.Fatalf("age: %v", err)
	}
	if age < 50*time.Millisecond {
		t.Errorf("age = %v, want >= 50ms", age)
	}
}

func TestOutbox_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	cases := []queue.OutboxEntry{
		{JobID: "j1"},     // missing project
		{ProjectID: "p1"}, // missing job
	}
	for i, e := range cases {
		if err := queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{e}); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestOutbox_EmptyWriteNoOp(t *testing.T) {
	ctx := context.Background()
	tx, _ := testDB.Pool.Begin(ctx)
	defer tx.Rollback(ctx)
	if err := queue.WriteOutboxInTx(ctx, tx, nil); err != nil {
		t.Errorf("nil entries: %v", err)
	}
	if err := queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{}); err != nil {
		t.Errorf("empty slice: %v", err)
	}
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
			if seen[id] {
				t.Errorf("duplicate claim of %s", id)
			}
			seen[id] = true
		}
	}
	if len(seen) != 20 {
		t.Errorf("claimed %d unique, want 20", len(seen))
	}
	count, _ := st.CountUnconsumedOutbox(ctx)
	if count != 0 {
		t.Errorf("remaining = %d, want 0", count)
	}
}
