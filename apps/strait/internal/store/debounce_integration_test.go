//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestUpsertDebouncePending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-upsert")

	d := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "key-1",
		Payload:     json.RawMessage(`{"hello":"world"}`),
		Priority:    5,
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(30 * time.Second),
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		d))
	require.NotEqual(t, "",

		d.ID)
	require.False(t, d.CreatedAt.
		IsZero())

}

func TestUpsertDebouncePending_UpsertReplaces(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-replace")
	fireAt := time.Date(2026, 6, 3, 12, 0, 0, 123456000, time.UTC)

	d1 := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "replace-key",
		Payload:     json.RawMessage(`{"v":1}`),
		Priority:    1,
		TriggeredBy: "api",
		FireAt:      fireAt,
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		d1))

	initialID := d1.ID
	initialCreatedAt := d1.CreatedAt
	var xminBeforeNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM debounce_pending
		WHERE job_id = $1 AND debounce_key = $2`,

		job.ID, "replace-key").
		Scan(&xminBeforeNoop))

	same := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "replace-key",
		Payload:     json.RawMessage(`{"v":1}`),
		Priority:    1,
		TriggeredBy: "api",
		FireAt:      fireAt,
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		same))

	var xminAfterNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM debounce_pending
		WHERE job_id = $1 AND debounce_key = $2`,

		job.ID, "replace-key").
		Scan(&xminAfterNoop))
	require.Equal(t, xminBeforeNoop,

		xminAfterNoop,
	)
	require.Equal(t, initialID,

		same.
			ID)
	require.True(t, same.CreatedAt.
		Equal(initialCreatedAt))

	d2 := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "replace-key",
		Payload:     json.RawMessage(`{"v":2}`),
		Priority:    10,
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(60 * time.Second),
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		d2))
	require.Equal(t, initialID,

		d2.ID,
	)

	// Should have only one row (replaced).
	var rowCount int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)::int
		FROM debounce_pending
		WHERE job_id = $1 AND debounce_key = $2`,

		job.ID, "replace-key",
	).Scan(&rowCount))
	require.EqualValues(t, 1, rowCount)

	var payload json.RawMessage
	var priority int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT payload, priority
		FROM debounce_pending
		WHERE job_id = $1 AND debounce_key = $2`,

		job.ID, "replace-key",
	).Scan(&payload, &priority))
	require.True(t, jsonEqual(payload,
		json.RawMessage(`{"v":2}`),
	))
	require.EqualValues(t, 10, priority)

}

func TestListDueDebouncePending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-list-due")

	// Create a due debounce (fire_at in the past).
	due := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "due-key",
		Payload:     json.RawMessage(`{}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(-1 * time.Minute),
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		due))

	// Create a future debounce.
	future := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "future-key",
		Payload:     json.RawMessage(`{}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(10 * time.Minute),
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		future))

	items, err := q.ListDueDebouncePending(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "due-key",

		items[0].DebounceKey,
	)

}

func TestListDueDebouncePending_FairAcrossProjects(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectA := "project-debounce-fair-a"
	projectB := "project-debounce-fair-b"
	jobA := mustCreateJob(t, ctx, q, projectA)
	jobB := mustCreateJob(t, ctx, q, projectB)
	fireAt := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	for i := range 101 {
		d := &domain.DebouncePending{
			JobID:       jobA.ID,
			ProjectID:   projectA,
			DebounceKey: "a-" + newID(),
			Payload:     json.RawMessage(`{}`),
			TriggeredBy: "api",
			FireAt:      fireAt.Add(time.Duration(i) * time.Microsecond),
		}
		require.NoError(t, q.UpsertDebouncePending(ctx,
			d))

	}
	projectBPending := &domain.DebouncePending{
		JobID:       jobB.ID,
		ProjectID:   projectB,
		DebounceKey: "b-1",
		Payload:     json.RawMessage(`{}`),
		TriggeredBy: "api",
		FireAt:      fireAt.Add(9 * time.Minute),
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		projectBPending,
	))

	items, err := q.ListDueDebouncePending(ctx)
	require.NoError(t, err)

	var projectACount int
	foundProjectB := false
	for _, item := range items {
		switch item.ProjectID {
		case projectA:
			projectACount++
		case projectB:
			if item.ID == projectBPending.ID {
				foundProjectB = true
			}
		}
	}
	require.True(t, foundProjectB)
	require.LessOrEqual(t,
		projectACount,

		5)

}

func TestClaimDueDebouncePending_OnlyClaimsDueRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-claim")
	due := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "claim-due",
		Payload:     json.RawMessage(`{"due":true}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(-1 * time.Minute),
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		due))

	future := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "claim-future",
		Payload:     json.RawMessage(`{"future":true}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(10 * time.Minute),
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		future))

	claimed, ok, err := q.ClaimDueDebouncePending(ctx, due.ID)
	require.NoError(t, err)
	require.False(t, !ok ||

		claimed ==
			nil || claimed.
		ID != due.ID ||
		claimed.
			DebounceKey !=
			"claim-due")

	stillDue, err := q.ListDueDebouncePending(ctx)
	require.NoError(t, err)
	require.False(t, len(stillDue) !=
		1 || stillDue[0].ID != due.
		ID)

	claimed, ok, err = q.ClaimDueDebouncePending(ctx, future.ID)
	require.NoError(t, err)
	require.False(t, ok ||
		claimed !=
			nil)

}

func TestRescheduleDebouncePending_OnlyIfFireAtUnchanged(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-reschedule")
	originalFireAt := time.Now().UTC().Add(-1 * time.Minute).Truncate(time.Microsecond)
	due := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "reschedule-key",
		Payload:     json.RawMessage(`{}`),
		TriggeredBy: "api",
		FireAt:      originalFireAt,
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		due))

	staleUpdated, err := q.RescheduleDebouncePending(ctx, due.ID, originalFireAt.Add(time.Second), time.Now().UTC().Add(5*time.Minute))
	require.NoError(t, err)
	require.False(t, staleUpdated)

	stillDue, err := q.ListDueDebouncePending(ctx)
	require.NoError(t, err)
	require.False(t, len(stillDue) !=
		1 || stillDue[0].ID != due.
		ID)

	nextFireAt := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Microsecond)
	rescheduled, err := q.RescheduleDebouncePending(ctx, due.ID, originalFireAt, nextFireAt)
	require.NoError(t, err)
	require.True(t, rescheduled)

	claimed, ok, err := q.ClaimDueDebouncePending(ctx, due.ID)
	require.NoError(t, err)
	require.False(t, ok ||
		claimed !=
			nil)

}

func TestCompleteDebouncePending_DeletesOnlyUnchangedClaim(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-complete")
	originalFireAt := time.Now().UTC().Add(-1 * time.Minute).Truncate(time.Microsecond)
	due := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "complete-key",
		Payload:     json.RawMessage(`{"v":1}`),
		TriggeredBy: "api",
		FireAt:      originalFireAt,
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		due))

	updated := *due
	updated.FireAt = time.Now().UTC().Add(10 * time.Minute).Truncate(time.Microsecond)
	updated.Payload = json.RawMessage(`{"v":2}`)
	require.NoError(t, q.UpsertDebouncePending(ctx,
		&updated))

	completed, err := q.CompleteDebouncePending(ctx, due.ID, originalFireAt)
	require.NoError(t, err)
	require.False(t, completed)

	completed, err = q.CompleteDebouncePending(ctx, due.ID, updated.FireAt)
	require.NoError(t, err)
	require.True(t, completed)

}

func TestInsertDebouncePendingIfAbsent_PreservesNewerPending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-restore")
	newer := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "restore-key",
		Payload:     json.RawMessage(`{"v":2}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(10 * time.Minute),
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		newer))

	oldClaim := &domain.DebouncePending{
		ID:          "old-claim",
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "restore-key",
		Payload:     json.RawMessage(`{"v":1}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(-1 * time.Minute),
	}
	inserted, err := q.InsertDebouncePendingIfAbsent(ctx, oldClaim)
	require.NoError(t, err)
	require.False(t, inserted)

	items, err := q.ListDueDebouncePending(ctx)
	require.NoError(t, err)
	require.Len(t, items, 0)

}

func TestDeleteDebouncePending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-delete")

	d := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "delete-key",
		Payload:     json.RawMessage(`{}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(-1 * time.Minute),
	}
	require.NoError(t, q.UpsertDebouncePending(ctx,
		d))
	require.NoError(t, q.DeleteDebouncePending(ctx,
		d.ID))

	// Verify it was deleted by listing due items.
	items, err := q.ListDueDebouncePending(ctx)
	require.NoError(t, err)

	for _, item := range items {
		require.NotEqual(t, d.ID,

			item.ID,
		)

	}
}
