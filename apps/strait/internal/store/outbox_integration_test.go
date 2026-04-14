//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
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
	if rows[0].RetryOfOutboxID != nil {
		t.Fatalf("first row RetryOfOutboxID = %v, want nil", *rows[0].RetryOfOutboxID)
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
	if row.RetryOfOutboxID != nil {
		t.Fatalf("row.RetryOfOutboxID = %v, want nil", *row.RetryOfOutboxID)
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

func TestRetryQuarantinedOutbox_CopiesEnqueueFieldsAndTracksLineage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-retry")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	idempotencyKey := "retry-key"
	scheduledAt := time.Now().UTC().Truncate(time.Second)
	payload := json.RawMessage(`{"hello":"world"}`)
	metadata := map[string]any{"source": "quarantine", "attempt": 1}
	writeCustomQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-source", time.Now().Add(-time.Minute), "terminal failure", payload, metadata, &idempotencyKey, &scheduledAt, 7, nil)

	cloned, err := q.RetryQuarantinedOutbox(ctx, job.ProjectID, "outbox-source")
	if err != nil {
		t.Fatalf("RetryQuarantinedOutbox() error = %v", err)
	}
	if cloned.ID == "" || cloned.ID == "outbox-source" {
		t.Fatalf("cloned.ID = %q, want fresh id", cloned.ID)
	}
	if cloned.ProjectID != job.ProjectID || cloned.JobID != job.ID {
		t.Fatalf("cloned identifiers = (%s,%s), want (%s,%s)", cloned.ProjectID, cloned.JobID, job.ProjectID, job.ID)
	}
	if !jsonEqual(cloned.Payload, payload) {
		t.Fatalf("cloned.Payload = %s, want %s", cloned.Payload, payload)
	}
	wantMetadata, _ := json.Marshal(metadata)
	if !jsonEqual(cloned.Metadata, wantMetadata) {
		t.Fatalf("cloned.Metadata = %s, want %s", cloned.Metadata, wantMetadata)
	}
	if cloned.IdempotencyKey == nil || *cloned.IdempotencyKey != idempotencyKey {
		t.Fatalf("cloned.IdempotencyKey = %v, want %q", cloned.IdempotencyKey, idempotencyKey)
	}
	if cloned.ScheduledAt == nil || !cloned.ScheduledAt.Equal(scheduledAt) {
		t.Fatalf("cloned.ScheduledAt = %v, want %v", cloned.ScheduledAt, scheduledAt)
	}
	if cloned.Priority != 7 {
		t.Fatalf("cloned.Priority = %d, want 7", cloned.Priority)
	}
	if cloned.RetryOfOutboxID == nil || *cloned.RetryOfOutboxID != "outbox-source" {
		t.Fatalf("cloned.RetryOfOutboxID = %v, want outbox-source", cloned.RetryOfOutboxID)
	}

	rows, err := q.ClaimUnconsumedOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimUnconsumedOutbox() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("unconsumed rows len = %d, want 1", len(rows))
	}
	if rows[0].ID != cloned.ID {
		t.Fatalf("unconsumed row id = %s, want %s", rows[0].ID, cloned.ID)
	}
	if rows[0].RetryOfOutboxID == nil || *rows[0].RetryOfOutboxID != "outbox-source" {
		t.Fatalf("claimed RetryOfOutboxID = %v, want outbox-source", rows[0].RetryOfOutboxID)
	}

	source, err := q.GetQuarantinedOutbox(ctx, job.ProjectID, "outbox-source")
	if err != nil {
		t.Fatalf("GetQuarantinedOutbox(source) error = %v", err)
	}
	if source.Error != "terminal failure" {
		t.Fatalf("source.Error = %q, want terminal failure", source.Error)
	}
	if source.RetryOfOutboxID != nil {
		t.Fatalf("source.RetryOfOutboxID = %v, want nil", source.RetryOfOutboxID)
	}
}

func TestRetryQuarantinedOutbox_ConflictsWhenActiveCloneExists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-retry-conflict")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-source", time.Now().Add(-time.Minute), "terminal failure")

	first, err := q.RetryQuarantinedOutbox(ctx, job.ProjectID, "outbox-source")
	if err != nil {
		t.Fatalf("first RetryQuarantinedOutbox() error = %v", err)
	}
	if first.ID == "" {
		t.Fatal("first retry clone id should not be empty")
	}

	_, err = q.RetryQuarantinedOutbox(ctx, job.ProjectID, "outbox-source")
	if !errors.Is(err, store.ErrOutboxRowConflict) {
		t.Fatalf("second RetryQuarantinedOutbox() error = %v, want %v", err, store.ErrOutboxRowConflict)
	}
}

func TestPurgeQuarantinedOutbox_RemovesOnlyTargetedRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-purge")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-delete", time.Now().Add(-2*time.Minute), "delete me")
	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-keep", time.Now().Add(-time.Minute), "keep me")

	deleted, err := q.PurgeQuarantinedOutbox(ctx, job.ProjectID, "outbox-delete")
	if err != nil {
		t.Fatalf("PurgeQuarantinedOutbox() error = %v", err)
	}
	if deleted.ID != "outbox-delete" {
		t.Fatalf("deleted.ID = %s, want outbox-delete", deleted.ID)
	}

	rows, err := q.ListQuarantinedOutbox(ctx, job.ProjectID, 10, nil, "")
	if err != nil {
		t.Fatalf("ListQuarantinedOutbox() error = %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "outbox-keep" {
		t.Fatalf("remaining rows = %+v, want only outbox-keep", rows)
	}
}

func TestPurgeQuarantinedOutbox_RejectsNonQuarantinedRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-purge-conflict")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{{
		ID:        "outbox-pending",
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"ok":true}`),
		Metadata:  map[string]any{"source": "test"},
	}}); err != nil {
		t.Fatalf("WriteOutboxInTx() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	_, err = q.PurgeQuarantinedOutbox(ctx, job.ProjectID, "outbox-pending")
	if !errors.Is(err, store.ErrOutboxRowConflict) {
		t.Fatalf("PurgeQuarantinedOutbox() error = %v, want %v", err, store.ErrOutboxRowConflict)
	}
}

func TestListAndGetQuarantinedOutbox_IncludeRetryLineage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-lineage")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	lineage := "source-outbox"
	writeCustomQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-lineage", time.Now().Add(-time.Minute), "terminal failure", json.RawMessage(`{"ok":true}`), map[string]any{"source": "test"}, nil, nil, 0, &lineage)

	rows, err := q.ListQuarantinedOutbox(ctx, job.ProjectID, 10, nil, "")
	if err != nil {
		t.Fatalf("ListQuarantinedOutbox() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].RetryOfOutboxID == nil || *rows[0].RetryOfOutboxID != lineage {
		t.Fatalf("rows[0].RetryOfOutboxID = %v, want %q", rows[0].RetryOfOutboxID, lineage)
	}

	row, err := q.GetQuarantinedOutbox(ctx, job.ProjectID, "outbox-lineage")
	if err != nil {
		t.Fatalf("GetQuarantinedOutbox() error = %v", err)
	}
	if row.RetryOfOutboxID == nil || *row.RetryOfOutboxID != lineage {
		t.Fatalf("row.RetryOfOutboxID = %v, want %q", row.RetryOfOutboxID, lineage)
	}
}

func writeQuarantinedOutboxRow(t *testing.T, ctx context.Context, projectID, jobID, id string, consumedAt time.Time, errText string) {
	t.Helper()
	writeCustomQuarantinedOutboxRow(t, ctx, projectID, jobID, id, consumedAt, errText, json.RawMessage(`{"ok":true}`), map[string]any{"source": "test"}, nil, nil, 0, nil)
}

func writeCustomQuarantinedOutboxRow(
	t *testing.T,
	ctx context.Context,
	projectID, jobID, id string,
	consumedAt time.Time,
	errText string,
	payload json.RawMessage,
	metadata map[string]any,
	idempotencyKey *string,
	scheduledAt *time.Time,
	priority int,
	retryOf *string,
) {
	t.Helper()

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var idempotencyValue string
	if idempotencyKey != nil {
		idempotencyValue = *idempotencyKey
	}

	if err := queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{{
		ID:             id,
		ProjectID:      projectID,
		JobID:          jobID,
		Payload:        payload,
		Metadata:       metadata,
		IdempotencyKey: idempotencyValue,
		ScheduledAt:    scheduledAt,
		Priority:       priority,
	}}); err != nil {
		t.Fatalf("WriteOutboxInTx() error = %v", err)
	}
	if retryOf != nil {
		if _, err := tx.Exec(ctx, `UPDATE enqueue_outbox SET retry_of_outbox_id = $2 WHERE id = $1`, id, *retryOf); err != nil {
			t.Fatalf("set retry_of_outbox_id: %v", err)
		}
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
