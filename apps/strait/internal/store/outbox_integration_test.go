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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListQuarantinedOutbox_ReturnsNewestFirst(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-list")
	require.NoError(t, q.CreateJob(ctx,
		job))

	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-older", time.Now().Add(-2*time.Minute), "older")
	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-newer", time.Now().Add(-time.Minute), "newer")

	rows, err := q.ListQuarantinedOutbox(ctx, job.ProjectID, 10, nil, "")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.False(t, rows[0].
		ID != "outbox-newer" ||
		rows[1].ID !=
			"outbox-older",
	)
	require.Equal(t, "newer",

		rows[0].
			Error)
	require.Nil(t, rows[0].RetryOfOutboxID)

}

func TestListQuarantinedOutbox_PaginatesWithCompositeCursor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-pagination")
	require.NoError(t, q.CreateJob(ctx,
		job))

	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-a", time.Now().Add(-3*time.Minute), "a")
	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-b", time.Now().Add(-2*time.Minute), "b")
	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-c", time.Now().Add(-time.Minute), "c")

	firstPage, err := q.ListQuarantinedOutbox(ctx, job.ProjectID, 2, nil, "")
	require.NoError(t, err)
	require.Len(t, firstPage,

		2)

	cursorConsumedAt := firstPage[1].ConsumedAt
	secondPage, err := q.ListQuarantinedOutbox(ctx, job.ProjectID, 2, &cursorConsumedAt, firstPage[1].ID)
	require.NoError(t, err)
	require.Len(t, secondPage,

		1)
	require.Equal(t, "outbox-a",

		secondPage[0].ID,
	)

}

func TestGetQuarantinedOutbox_ReturnsStoredRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-get")
	require.NoError(t, q.CreateJob(ctx,
		job))

	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-get", time.Now().Add(-time.Minute), "terminal failure")

	row, err := q.GetQuarantinedOutbox(ctx, job.ProjectID, "outbox-get")
	require.NoError(t, err)
	require.Equal(t, "terminal failure",

		row.Error,
	)
	require.Nil(t, row.
		RetryOfOutboxID,
	)

}

func TestGetQuarantinedOutbox_NotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	_, err := q.GetQuarantinedOutbox(ctx, "project-outbox-missing", "missing")
	require.True(t, errors.Is(err, store.
		ErrOutboxRowNotFound,
	))

}

func TestRetryQuarantinedOutbox_CopiesEnqueueFieldsAndTracksLineage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-retry")
	require.NoError(t, q.CreateJob(ctx,
		job))

	idempotencyKey := "retry-key"
	scheduledAt := time.Now().UTC().Truncate(time.Second)
	payload := json.RawMessage(`{"hello":"world"}`)
	metadata := map[string]any{"source": "quarantine", "attempt": 1}
	writeCustomQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-source", time.Now().Add(-time.Minute), "terminal failure", payload, metadata, &idempotencyKey, &scheduledAt, 7, nil)

	cloned, err := q.RetryQuarantinedOutbox(ctx, job.ProjectID, "outbox-source")
	require.NoError(t, err)
	require.False(t, cloned.
		ID == "" ||
		cloned.ID ==
			"outbox-source",
	)
	require.False(t, cloned.
		ProjectID !=
		job.ProjectID ||
		cloned.
			JobID != job.
			ID)
	require.True(t, jsonEqual(cloned.
		Payload, payload,
	))

	wantMetadata, _ := json.Marshal(metadata)
	require.True(t, jsonEqual(cloned.
		Metadata, wantMetadata,
	))
	require.False(t, cloned.
		IdempotencyKey ==
		nil ||
		*cloned.IdempotencyKey !=
			idempotencyKey,
	)
	require.False(t, cloned.
		ScheduledAt ==
		nil ||
		!cloned.ScheduledAt.
			Equal(scheduledAt))
	require.EqualValues(t, 7, cloned.
		Priority,
	)
	require.False(t, cloned.
		RetryOfOutboxID ==
		nil ||
		*cloned.RetryOfOutboxID !=
			"outbox-source",
	)

	rows, err := q.ClaimUnconsumedOutbox(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, cloned.
		ID, rows[0].ID)
	require.False(t, rows[0].
		RetryOfOutboxID ==
		nil || *rows[0].RetryOfOutboxID !=
		"outbox-source",
	)

	source, err := q.GetQuarantinedOutbox(ctx, job.ProjectID, "outbox-source")
	require.NoError(t, err)
	require.Equal(t, "terminal failure",

		source.
			Error)
	require.Nil(t, source.
		RetryOfOutboxID,
	)

}

func TestRetryQuarantinedOutbox_ConflictsWhenActiveCloneExists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-retry-conflict")
	require.NoError(t, q.CreateJob(ctx,
		job))

	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-source", time.Now().Add(-time.Minute), "terminal failure")

	first, err := q.RetryQuarantinedOutbox(ctx, job.ProjectID, "outbox-source")
	require.NoError(t, err)
	require.NotEqual(t, "",

		first.ID)

	_, err = q.RetryQuarantinedOutbox(ctx, job.ProjectID, "outbox-source")
	require.True(t, errors.Is(err, store.
		ErrOutboxRowConflict,
	))

}

func TestPurgeQuarantinedOutbox_RemovesOnlyTargetedRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-purge")
	require.NoError(t, q.CreateJob(ctx,
		job))

	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-delete", time.Now().Add(-2*time.Minute), "delete me")
	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-keep", time.Now().Add(-time.Minute), "keep me")

	deleted, err := q.PurgeQuarantinedOutbox(ctx, job.ProjectID, "outbox-delete")
	require.NoError(t, err)
	require.Equal(t, "outbox-delete",

		deleted.ID,
	)

	rows, err := q.ListQuarantinedOutbox(ctx, job.ProjectID, 10, nil, "")
	require.NoError(t, err)
	require.False(t, len(rows) != 1 ||
		rows[0].ID !=
			"outbox-keep",
	)

}

func TestPurgeQuarantinedOutbox_RejectsNonQuarantinedRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-purge-conflict")
	require.NoError(t, q.CreateJob(ctx,
		job))

	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	defer func() { _ = tx.Rollback(ctx) }()
	require.NoError(t, queue.
		WriteOutboxInTx(ctx,
			tx, []queue.OutboxEntry{{
				ID: "outbox-pending",

				ProjectID: job.ProjectID,
				JobID:     job.ID, Payload: json.
						RawMessage(`{"ok":true}`), Metadata: map[string]any{"source": "test"}}}))
	require.NoError(t, tx.Commit(ctx))

	_, err = q.PurgeQuarantinedOutbox(ctx, job.ProjectID, "outbox-pending")
	require.True(t, errors.Is(err, store.
		ErrOutboxRowConflict,
	))

}

func TestListAndGetQuarantinedOutbox_IncludeRetryLineage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-lineage")
	require.NoError(t, q.CreateJob(ctx,
		job))

	lineage := "source-outbox"
	writeCustomQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "outbox-lineage", time.Now().Add(-time.Minute), "terminal failure", json.RawMessage(`{"ok":true}`), map[string]any{"source": "test"}, nil, nil, 0, &lineage)

	rows, err := q.ListQuarantinedOutbox(ctx, job.ProjectID, 10, nil, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.False(t, rows[0].
		RetryOfOutboxID ==
		nil || *rows[0].RetryOfOutboxID !=
		lineage,
	)

	row, err := q.GetQuarantinedOutbox(ctx, job.ProjectID, "outbox-lineage")
	require.NoError(t, err)
	require.False(t, row.RetryOfOutboxID ==
		nil ||
		*row.RetryOfOutboxID !=
			lineage)

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
	require.NoError(t, err)

	defer func() { _ = tx.Rollback(ctx) }()

	var idempotencyValue string
	if idempotencyKey != nil {
		idempotencyValue = *idempotencyKey
	}
	require.NoError(t, queue.
		WriteOutboxInTx(ctx,
			tx, []queue.OutboxEntry{{
				ID: id, ProjectID: projectID,
				JobID: jobID, Payload: payload, Metadata: metadata,
				IdempotencyKey: idempotencyValue, ScheduledAt: scheduledAt,
				Priority: priority}}))

	if retryOf != nil {
		if _, err := tx.Exec(ctx, `UPDATE enqueue_outbox SET retry_of_outbox_id = $2 WHERE id = $1`, id, *retryOf); err != nil {
			require.Failf(t, "test failure",

				"set retry_of_outbox_id: %v", err)
		}
	}
	require.NoError(t, store.
		MarkOutboxErroredInTx(ctx, tx, id, errText))

	if _, err := tx.Exec(ctx, `UPDATE enqueue_outbox SET consumed_at = $2 WHERE id = $1`, id, consumedAt.UTC()); err != nil {
		require.Failf(t, "test failure",

			"force consumed_at: %v", err)
	}
	require.NoError(t, tx.Commit(ctx))

}

func TestArchiveConsumedOutboxBatch_PreservesQuarantinedRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)

	q := mustStore(t)
	job := baseJob(newID(), "project-outbox-archive")
	require.NoError(t, q.CreateJob(ctx,
		job))

	past := time.Now().Add(-10 * time.Minute)

	// Write a quarantined row (consumed with an error).
	writeQuarantinedOutboxRow(t, ctx, job.ProjectID, job.ID, "quarantined-1", past, "some error")

	// Write a normally consumed row (consumed without error).
	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	if err := queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{{
		ID:        "consumed-1",
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"ok":true}`),
	}}); err != nil {
		_ = tx.Rollback(ctx)
		require.Failf(t, "test failure",

			"WriteOutboxInTx: %v", err)
	}
	if err := store.MarkOutboxConsumedInTx(ctx, tx, []string{"consumed-1"}); err != nil {
		_ = tx.Rollback(ctx)
		require.Failf(t, "test failure",

			"MarkOutboxConsumedInTx: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE enqueue_outbox SET consumed_at = $2 WHERE id = $1`, "consumed-1", past.UTC()); err != nil {
		_ = tx.Rollback(ctx)
		require.Failf(t, "test failure",

			"force consumed_at: %v", err)
	}
	require.NoError(t, tx.Commit(ctx))

	// Archive: should move consumed-1 but NOT quarantined-1.
	archived, err := q.ArchiveConsumedOutboxBatch(ctx, time.Minute, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 1, archived)

	// Quarantined row should still be visible to the admin API.
	rows, err := q.ListQuarantinedOutbox(ctx, job.ProjectID, 10, nil, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "quarantined-1",

		rows[0].ID)

	// Consumed row should be gone from the hot table.
	var hotCount int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM enqueue_outbox WHERE id = $1`,

		"consumed-1").Scan(&hotCount))
	assert.EqualValues(t, 0, hotCount)

}
