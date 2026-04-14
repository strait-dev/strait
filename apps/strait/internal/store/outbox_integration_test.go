//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/queue"
	"strait/internal/store"
)

func TestListQuarantinedOutbox_ReturnsNewestFirst(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-list")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-older", time.Now().Add(-2*time.Minute), "older")
	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-newer", time.Now().Add(-time.Minute), "newer")

	rows, err := q.ListQuarantinedOutbox(ctx, job.ProjectID, 10, nil, "")
	if err != nil {
		t.Fatalf("ListQuarantinedOutbox() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}
	if rows[0].ID != "outbox-newer" || rows[1].ID != "outbox-older" {
		t.Fatalf("row order = [%s %s], want [outbox-newer outbox-older]", rows[0].ID, rows[1].ID)
	}
	if rows[0].Error != "newer" {
		t.Fatalf("first row error = %q, want %q", rows[0].Error, "newer")
	}
}

func TestListQuarantinedOutbox_PaginatesWithCompositeCursor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-pagination")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-a", time.Now().Add(-3*time.Minute), "a")
	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-b", time.Now().Add(-2*time.Minute), "b")
	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-c", time.Now().Add(-time.Minute), "c")

	firstPage, err := q.ListQuarantinedOutbox(ctx, job.ProjectID, 2, nil, "")
	if err != nil {
		t.Fatalf("first ListQuarantinedOutbox() error = %v", err)
	}
	if len(firstPage) != 2 {
		t.Fatalf("first page len = %d, want 2", len(firstPage))
	}

	cursorConsumedAt := firstPage[1].ConsumedAt
	secondPage, err := q.ListQuarantinedOutbox(ctx, job.ProjectID, 2, &cursorConsumedAt, firstPage[1].ID)
	if err != nil {
		t.Fatalf("second ListQuarantinedOutbox() error = %v", err)
	}
	if len(secondPage) != 1 {
		t.Fatalf("second page len = %d, want 1", len(secondPage))
	}
	if secondPage[0].ID != "outbox-a" {
		t.Fatalf("second page id = %s, want outbox-a", secondPage[0].ID)
	}
}

func TestGetQuarantinedOutbox_ReturnsStoredRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-get")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-get", time.Now().Add(-time.Minute), "terminal failure")

	row, err := q.GetQuarantinedOutbox(ctx, job.ProjectID, "outbox-get")
	if err != nil {
		t.Fatalf("GetQuarantinedOutbox() error = %v", err)
	}
	if row.Error != "terminal failure" {
		t.Fatalf("row.Error = %q, want %q", row.Error, "terminal failure")
	}
}

func TestGetQuarantinedOutbox_NotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	_, err := q.GetQuarantinedOutbox(ctx, "project-outbox-missing", "missing")
	if err != store.ErrOutboxRowNotFound {
		t.Fatalf("GetQuarantinedOutbox() error = %v, want %v", err, store.ErrOutboxRowNotFound)
	}
}

func writeQuarantinedOutboxRow(t *testing.T, ctx context.Context, projectID, jobID, id string, consumedAt time.Time, errText string) {
	t.Helper()

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{{
		ID:        id,
		ProjectID: projectID,
		JobID:     jobID,
		Payload:   json.RawMessage(`{"ok":true}`),
		Metadata:  map[string]any{"source": "test"},
	}}); err != nil {
		t.Fatalf("WriteOutboxInTx() error = %v", err)
	}
	if err := store.MarkOutboxErroredInTx(ctx, tx, id, errText); err != nil {
		t.Fatalf("MarkOutboxErroredInTx() error = %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE enqueue_outbox SET consumed_at = $2 WHERE id = $1`, id, consumedAt.UTC()); err != nil {
		t.Fatalf("force consumed_at: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
}
