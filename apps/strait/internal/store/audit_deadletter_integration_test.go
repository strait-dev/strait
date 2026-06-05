//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditDeadletter_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("dlq-test-secret")
	require.NoError(t, err)

	q.SetAuditSigningKey(signingKey)

	ev := &domain.AuditEvent{
		ID:           "dlq-ev-1",
		ProjectID:    "proj-dlq",
		ActorID:      "actor-1",
		ActorType:    "user",
		Action:       domain.AuditActionJobTriggered,
		ResourceType: "job",
		ResourceID:   "job-1",
		Details:      json.RawMessage(`{"run_id":"r1"}`),
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "db down",
		3))

	count, err := q.CountAuditEventsDeadletter(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	// Direct SELECT to verify the stored fields.
	var storedAction, lastErr string
	var retryCount int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT action, last_error, retry_count
		FROM audit_events_deadletter WHERE id = $1
	`,

		"dlq-ev-1").Scan(&storedAction, &lastErr, &retryCount))
	assert.Equal(t, domain.
		AuditActionJobTriggered,

		storedAction)
	assert.Equal(t, "db down",

		lastErr,
	)
	assert.EqualValues(t, 3, retryCount)

	// Round-trip via the main chain is unaffected — deadletter does not
	// participate in the signed chain.
	vc, err := q.VerifyAuditChain(ctx, "proj-dlq")
	require.NoError(t, err)
	assert.True(t, vc.Valid)
	assert.EqualValues(t, 0, vc.EventsChecked)

}

// TestAuditDeadletter_AttemptCountIncrement asserts the per-row attempt
// count starts at zero and IncrementAuditDeadletterAttempt advances it by
// one.
func TestAuditDeadletter_AttemptCountIncrement(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	ev := &domain.AuditEvent{
		ID:        "dlq-attempt-1",
		ProjectID: "proj-attempt",
		ActorID:   "a", ActorType: "user",
		Action:       domain.AuditActionJobTriggered,
		ResourceType: "job", ResourceID: "j1",
		Details:   json.RawMessage(`{"run_id":"r1"}`),
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "down",
		3))

	// Attempt-aware list returns attempt_count=0, reclaimed_event_id=nil.
	_, _, infos, err := q.ListAuditEventsDeadletterWithAttempts(ctx, 100)
	require.NoError(t, err)
	require.False(t, len(infos) != 1 ||
		infos[0].
			AttemptCount !=
			0 || infos[0].ReclaimedEventID !=
		nil,
	)

	// Three increments → attempt_count = 3.
	for range 3 {
		require.NoError(t, q.IncrementAuditDeadletterAttempt(ctx, "dlq-attempt-1"))

	}
	_, _, infos, err = q.ListAuditEventsDeadletterWithAttempts(ctx, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 3, infos[0].AttemptCount)

}

// TestAuditDeadletter_MarkReclaimed_PersistsMarker asserts the
// idempotency marker survives a re-read so the reclaimer can detect a
// previously-reclaimed row and skip the chain insert.
func TestAuditDeadletter_MarkReclaimed_PersistsMarker(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	ev := &domain.AuditEvent{
		ID:        "dlq-marker-1",
		ProjectID: "proj-marker",
		ActorID:   "a", ActorType: "user",
		Action:       domain.AuditActionJobTriggered,
		ResourceType: "job", ResourceID: "j1",
		Details:   json.RawMessage(`{"run_id":"r1"}`),
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "down",
		1))
	require.NoError(t, q.MarkAuditDeadletterReclaimed(ctx, "dlq-marker-1",

		"ev-new-1",
	))

	got, err := q.GetAuditEventDeadletter(ctx, "dlq-marker-1", "proj-marker")
	require.NoError(t, err)
	require.Nil(t, got)

	_, _, infos, err := q.ListAuditEventsDeadletterWithAttempts(ctx, 100)
	require.NoError(t, err)
	require.False(t, len(infos) != 1 ||
		infos[0].
			ReclaimedEventID ==
			nil ||
		*infos[0].ReclaimedEventID !=
			"ev-new-1")

}

func TestReplayAuditEventDeadletter_ConcurrentReplayInsertsOnce(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("dlq-atomic-replay-secret")
	require.NoError(t, err)

	q.SetAuditSigningKey(signingKey)

	ev := &domain.AuditEvent{
		ID:           "dlq-atomic-1",
		ProjectID:    "proj-dlq-atomic",
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobTriggered,
		ResourceType: "job",
		ResourceID:   "job-1",
		Details:      json.RawMessage(`{"run_id":"r1"}`),
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "down",
		1))

	type outcome struct {
		replayed bool
		err      error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, newID := range []string{"audit-replayed-1", "audit-replayed-2"} {
		wg.Add(1)
		{
			newID := newID
			concWG.Go(func() {
				defer wg.Done()
				<-start
				_, replayed, replayErr := q.ReplayAuditEventDeadletter(ctx, ev.ID, ev.ProjectID, newID)
				results <- outcome{replayed: replayed, err: replayErr}
			})
		}
	}
	close(start)
	wg.Wait()
	close(results)

	var replayed, skipped int
	for result := range results {
		require.Nil(t, result.
			err)

		if result.replayed {
			replayed++
		} else {
			skipped++
		}
	}
	require.False(t, replayed !=
		1 ||
		skipped !=
			1)

	var chainRows, dlqRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT
			(SELECT COUNT(*) FROM audit_events WHERE project_id = $1 AND action = $2),
			(SELECT COUNT(*) FROM audit_events_deadletter WHERE project_id = $1)
	`,

		ev.ProjectID, ev.Action).Scan(&chainRows, &dlqRows))
	require.EqualValues(t, 1, chainRows)
	require.EqualValues(t, 0, dlqRows)

	vc, err := q.VerifyAuditChain(ctx, ev.ProjectID)
	require.NoError(t, err)
	require.True(t, vc.Valid)

}

func TestReplayAuditEventDeadletter_ContextRoutedStoreUsesAmbientTx(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := store.NewWithContextRouting(testDB.Pool)
	signingKey, err := store.DeriveAuditSigningKey("dlq-context-routed-secret")
	require.NoError(t, err)

	q.SetAuditSigningKey(signingKey)

	ev := &domain.AuditEvent{
		ID:           "dlq-context-routed-1",
		ProjectID:    "proj-dlq-context",
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobTriggered,
		ResourceType: "job",
		ResourceID:   "job-1",
		Details:      json.RawMessage(`{"run_id":"r1"}`),
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "down",
		1))

	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	defer tx.Rollback(ctx) //nolint:errcheck
	txCtx := store.ContextWithTx(ctx, tx)

	replayedEvent, replayed, err := q.ReplayAuditEventDeadletter(txCtx, ev.ID, ev.ProjectID, "audit-context-routed-1")
	require.NoError(t, err)
	require.False(t, !replayed ||
		replayedEvent ==
			nil || replayedEvent.
		ID !=
		"audit-context-routed-1",
	)
	require.NoError(t, tx.Commit(ctx))

	var chainRows, dlqRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT
			(SELECT COUNT(*) FROM audit_events WHERE id = $1 AND project_id = $2),
			(SELECT COUNT(*) FROM audit_events_deadletter WHERE id = $3 AND project_id = $2)
	`,

		"audit-context-routed-1",
		ev.ProjectID, ev.ID).
		Scan(&chainRows, &dlqRows))
	require.False(t, chainRows !=
		1 ||
		dlqRows !=
			0)

}

func TestListAuditEventsDeadletterByProject_PaginatesSameQueuedAtRows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	projectID := "proj-dlq-page-tie"
	queuedAt := time.Now().UTC().Truncate(time.Microsecond)
	ids := []string{"dlq-page-tie-1", "dlq-page-tie-2", "dlq-page-tie-3"}
	for _, id := range ids {
		ev := &domain.AuditEvent{
			ID:           id,
			ProjectID:    projectID,
			ActorID:      "actor",
			ActorType:    "user",
			Action:       domain.AuditActionJobTriggered,
			ResourceType: "job",
			ResourceID:   "job-1",
			Details:      json.RawMessage(`{}`),
			CreatedAt:    time.Now().UTC(),
		}
		require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "down",
			0))

		if _, err := testDB.Pool.Exec(ctx, `UPDATE audit_events_deadletter SET queued_at = $2 WHERE id = $1`, id, queuedAt); err != nil {
			require.Failf(t, "test failure",

				"pin queued_at(%s): %v", id, err)
		}
	}

	page1, _, cursors, err := q.ListAuditEventsDeadletterByProject(ctx, projectID, 2, "")
	require.NoError(t, err)
	require.Len(t, page1, 2)

	page2, _, _, err := q.ListAuditEventsDeadletterByProject(ctx, projectID, 2, cursors[len(cursors)-1])
	require.NoError(t, err)
	require.Len(t, page2, 1)

	got := []string{page1[0].ID, page1[1].ID, page2[0].ID}
	for i, want := range ids {
		require.Equal(t, want,
			got[i])

	}
}

func TestDropAuditEventDeadletterWithAudit_InsertsAuditAndDeletesRow(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("dlq-atomic-drop-secret")
	require.NoError(t, err)

	q.SetAuditSigningKey(signingKey)

	projectID := "proj-dlq-atomic-drop"
	dlqID := "dlq-atomic-drop-1"
	ev := &domain.AuditEvent{
		ID:           dlqID,
		ProjectID:    projectID,
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobTriggered,
		ResourceType: "job",
		ResourceID:   "job-1",
		Details:      json.RawMessage(`{}`),
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "down",
		0))

	auditEvent := &domain.AuditEvent{
		ID:           "audit-dlq-drop-1",
		ProjectID:    projectID,
		ActorID:      "internal:admin",
		ActorType:    "internal",
		Action:       domain.AuditActionDeadletterDropped,
		ResourceType: "audit_deadletter",
		ResourceID:   dlqID,
		Details:      json.RawMessage(`{"deadletter_id":"dlq-atomic-drop-1","reason":"corrupt_payload"}`),
		CreatedAt:    time.Now().UTC(),
	}
	dropped, err := q.DropAuditEventDeadletterWithAudit(ctx, dlqID, projectID, auditEvent)
	require.NoError(t, err)
	require.True(t, dropped)

	var dlqRows, auditRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT
			(SELECT COUNT(*) FROM audit_events_deadletter WHERE id = $1 AND project_id = $2),
			(SELECT COUNT(*) FROM audit_events WHERE id = $3 AND project_id = $2 AND action = $4 AND resource_id = $1)
	`,

		dlqID, projectID,
		auditEvent.
			ID,
		domain.AuditActionDeadletterDropped,
	).Scan(&dlqRows, &auditRows))
	require.False(t, dlqRows !=
		0 ||
		auditRows !=
			1)

}

// TestAuditDeadletter_DeleteOlderThan_PerProjectCounts asserts the
// retention reaper removes only old rows and returns counts grouped by
// project.
func TestAuditDeadletter_DeleteOlderThan_PerProjectCounts(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	old := time.Now().UTC().Add(-90 * 24 * time.Hour)
	young := time.Now().UTC().Add(-1 * 24 * time.Hour)

	mk := func(id, project string, when time.Time) {
		ev := &domain.AuditEvent{
			ID: id, ProjectID: project, ActorID: "a", ActorType: "user",
			Action:       domain.AuditActionJobTriggered,
			ResourceType: "job", ResourceID: "j",
			Details:   json.RawMessage(`{}`),
			CreatedAt: when,
		}
		require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "x",
			0))

	}
	mk("old-a-1", "proj-a", old)
	mk("old-a-2", "proj-a", old)
	mk("young-a-1", "proj-a", young)
	mk("old-b-1", "proj-b", old)

	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	dropped, err := q.DeleteAuditDeadletterOlderThan(ctx, cutoff)
	require.NoError(t, err)

	if got, want := dropped["proj-a"], int64(2); got != want {
		assert.Failf(t, "test failure",

			"proj-a dropped = %d, want %d", got, want)
	}
	if got, want := dropped["proj-b"], int64(1); got != want {
		assert.Failf(t, "test failure",

			"proj-b dropped = %d, want %d", got, want)
	}
	// young-a-1 must still be present.
	count, err := q.CountAuditEventsDeadletter(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

}

func TestAuditDeadletter_DeleteOlderThanWithAudit_WritesMarkersBeforeDeleting(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("dlq-retention-with-audit-secret")
	require.NoError(t, err)

	q.SetAuditSigningKey(signingKey)

	old := time.Now().UTC().Add(-90 * 24 * time.Hour)
	young := time.Now().UTC().Add(-1 * 24 * time.Hour)

	mk := func(id, project string, when time.Time) {
		ev := &domain.AuditEvent{
			ID:           id,
			ProjectID:    project,
			ActorID:      "a",
			ActorType:    "user",
			Action:       domain.AuditActionJobTriggered,
			ResourceType: "job",
			ResourceID:   "j",
			Details:      json.RawMessage(`{}`),
			CreatedAt:    when,
		}
		require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "x",
			0))

	}
	mk("old-a-with-audit-1", "proj-retention-a", old)
	mk("old-a-with-audit-2", "proj-retention-a", old)
	mk("young-a-with-audit-1", "proj-retention-a", young)
	mk("old-b-with-audit-1", "proj-retention-b", old)

	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	dropped, err := q.DeleteAuditDeadletterOlderThanWithAudit(ctx, cutoff, 30)
	require.NoError(t, err)

	if got, want := dropped["proj-retention-a"], int64(2); got != want {
		assert.Failf(t, "test failure",

			"proj-retention-a dropped = %d, want %d", got, want)
	}
	if got, want := dropped["proj-retention-b"], int64(1); got != want {
		assert.Failf(t, "test failure",

			"proj-retention-b dropped = %d, want %d", got, want)
	}

	count, err := q.CountAuditEventsDeadletter(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	rows, err := testDB.Pool.Query(ctx, `
		SELECT project_id, details
		FROM audit_events
		WHERE action = $1
		  AND resource_type = 'audit_events_deadletter'
		  AND resource_id = 'retention'
		ORDER BY project_id
	`, domain.AuditActionDeadletterAged)
	require.NoError(t, err)

	defer rows.Close()

	markers := map[string]struct {
		droppedCount float64
		maxAgeDays   float64
		reason       string
	}{}
	for rows.Next() {
		var projectID string
		var raw json.RawMessage
		require.NoError(t, rows.
			Scan(&projectID,
				&raw,
			))

		var details map[string]any
		require.NoError(t, json.
			Unmarshal(raw, &details))

		markers[projectID] = struct {
			droppedCount float64
			maxAgeDays   float64
			reason       string
		}{
			droppedCount: details["dropped_count"].(float64),
			maxAgeDays:   details["max_age_days"].(float64),
			reason:       details["reason"].(string),
		}
	}
	require.NoError(t, rows.
		Err())

	if got, want := len(markers), 2; got != want {
		require.Failf(t, "test failure",

			"marker count = %d, want %d", got, want)
	}
	if got, want := markers["proj-retention-a"].droppedCount, float64(2); got != want {
		assert.Failf(t, "test failure",

			"proj-retention-a marker dropped_count = %v, want %v", got, want)
	}
	if got, want := markers["proj-retention-b"].droppedCount, float64(1); got != want {
		assert.Failf(t, "test failure",

			"proj-retention-b marker dropped_count = %v, want %v", got, want)
	}
	for _, marker := range markers {
		assert.EqualValues(t, 30, marker.
			maxAgeDays,
		)
		assert.Equal(t, "max_age_exceeded",

			marker.reason,
		)

	}
}

func TestAuditDeadletter_DeleteOlderThanWithAudit_RollsBackWhenMarkerFails(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("dlq-retention-rollback-secret")
	require.NoError(t, err)

	q.SetAuditSigningKey(signingKey)

	ev := &domain.AuditEvent{
		ID:           "old-rollback-with-audit-1",
		ProjectID:    "proj-retention-rollback",
		ActorID:      "a",
		ActorType:    "user",
		Action:       domain.AuditActionJobTriggered,
		ResourceType: "job",
		ResourceID:   "j",
		Details:      json.RawMessage(`{}`),
		CreatedAt:    time.Now().UTC().Add(-90 * 24 * time.Hour),
	}
	require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "x",
		0))

	forced := errors.New("forced audit marker failure")
	store.SetAuditEventPostInsertHookForTest(q, func(context.Context) error {
		return forced
	})
	t.Cleanup(func() { store.SetAuditEventPostInsertHookForTest(q, nil) })

	_, err = q.DeleteAuditDeadletterOlderThanWithAudit(ctx, time.Now().UTC().Add(-30*24*time.Hour), 30)
	require.Error(t, err)
	require.True(t, errors.Is(err, forced))

	var dlqRows, markerRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT
			(SELECT COUNT(*) FROM audit_events_deadletter WHERE id = $1),
			(SELECT COUNT(*) FROM audit_events WHERE project_id = $2 AND action = $3)
	`,

		ev.ID, ev.ProjectID, domain.AuditActionDeadletterAged,
	).Scan(&dlqRows,
		&markerRows),
	)
	require.EqualValues(t, 1, dlqRows)
	require.EqualValues(t, 0, markerRows)

}

func TestAuditDeadletter_DeleteOlderThan_SkipsZeroCreatedAtRows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)

	ev := &domain.AuditEvent{
		ID: "zero-created-at-dlq", ProjectID: "proj-zero-created", ActorID: "a", ActorType: "user",
		Action:       domain.AuditActionJobTriggered,
		ResourceType: "job", ResourceID: "j",
		Details: json.RawMessage(`{}`),
	}
	require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "x",
		0))

	if _, err := testDB.Pool.Exec(ctx, `UPDATE audit_events_deadletter SET created_at = TIMESTAMPTZ '0001-01-01 00:00:00+00' WHERE id = $1`, ev.ID); err != nil {
		require.Failf(t, "test failure",

			"force zero created_at: %v", err)
	}

	dropped, err := q.DeleteAuditDeadletterOlderThan(ctx, time.Now().UTC().Add(-30*24*time.Hour))
	require.NoError(t, err)
	require.Len(t, dropped,

		0)

	count, err := q.CountAuditEventsDeadletter(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

}
