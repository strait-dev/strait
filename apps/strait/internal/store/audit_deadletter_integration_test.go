//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestAuditDeadletter_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("dlq-test-secret")
	if err != nil {
		t.Fatalf("derive signing key: %v", err)
	}
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

	if err := q.CreateAuditEventDeadletter(ctx, ev, "db down", 3); err != nil {
		t.Fatalf("CreateAuditEventDeadletter: %v", err)
	}

	count, err := q.CountAuditEventsDeadletter(ctx)
	if err != nil {
		t.Fatalf("CountAuditEventsDeadletter: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Direct SELECT to verify the stored fields.
	var storedAction, lastErr string
	var retryCount int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT action, last_error, retry_count
		FROM audit_events_deadletter WHERE id = $1
	`, "dlq-ev-1").Scan(&storedAction, &lastErr, &retryCount); err != nil {
		t.Fatalf("query deadletter row: %v", err)
	}

	if storedAction != domain.AuditActionJobTriggered {
		t.Errorf("action = %q, want %q", storedAction, domain.AuditActionJobTriggered)
	}
	if lastErr != "db down" {
		t.Errorf("last_error = %q, want 'db down'", lastErr)
	}
	if retryCount != 3 {
		t.Errorf("retry_count = %d, want 3", retryCount)
	}

	// Round-trip via the main chain is unaffected — deadletter does not
	// participate in the signed chain.
	vc, err := q.VerifyAuditChain(ctx, "proj-dlq")
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !vc.Valid {
		t.Errorf("main chain should be valid (empty) despite deadletter rows: %s", vc.Error)
	}
	if vc.EventsChecked != 0 {
		t.Errorf("events_checked = %d, want 0 (deadletter is separate)", vc.EventsChecked)
	}

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
	if err := q.CreateAuditEventDeadletter(ctx, ev, "down", 3); err != nil {
		t.Fatalf("CreateAuditEventDeadletter: %v", err)
	}

	// Attempt-aware list returns attempt_count=0, reclaimed_event_id=nil.
	_, _, infos, err := q.ListAuditEventsDeadletterWithAttempts(ctx, 100)
	if err != nil {
		t.Fatalf("ListAuditEventsDeadletterWithAttempts: %v", err)
	}
	if len(infos) != 1 || infos[0].AttemptCount != 0 || infos[0].ReclaimedEventID != nil {
		t.Fatalf("initial info = %+v, want attempt=0 marker=nil", infos[0])
	}

	// Three increments → attempt_count = 3.
	for range 3 {
		if err := q.IncrementAuditDeadletterAttempt(ctx, "dlq-attempt-1"); err != nil {
			t.Fatalf("Increment: %v", err)
		}
	}
	_, _, infos, err = q.ListAuditEventsDeadletterWithAttempts(ctx, 100)
	if err != nil {
		t.Fatalf("re-list: %v", err)
	}
	if infos[0].AttemptCount != 3 {
		t.Errorf("attempt_count after 3 increments = %d, want 3", infos[0].AttemptCount)
	}
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
	if err := q.CreateAuditEventDeadletter(ctx, ev, "down", 1); err != nil {
		t.Fatalf("CreateAuditEventDeadletter: %v", err)
	}
	if err := q.MarkAuditDeadletterReclaimed(ctx, "dlq-marker-1", "ev-new-1"); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	got, err := q.GetAuditEventDeadletter(ctx, "dlq-marker-1", "proj-marker")
	if err != nil {
		t.Fatalf("GetAuditEventDeadletter after reclaim marker: %v", err)
	}
	if got != nil {
		t.Fatalf("GetAuditEventDeadletter returned reclaimed row %+v; replay fetch must hide it", got)
	}

	_, _, infos, err := q.ListAuditEventsDeadletterWithAttempts(ctx, 100)
	if err != nil {
		t.Fatalf("re-list: %v", err)
	}
	if len(infos) != 1 || infos[0].ReclaimedEventID == nil || *infos[0].ReclaimedEventID != "ev-new-1" {
		t.Fatalf("reclaimed_event_id = %+v, want ptr to ev-new-1", infos[0].ReclaimedEventID)
	}
}

func TestReplayAuditEventDeadletter_ConcurrentReplayInsertsOnce(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("dlq-atomic-replay-secret")
	if err != nil {
		t.Fatalf("derive signing key: %v", err)
	}
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
	if err := q.CreateAuditEventDeadletter(ctx, ev, "down", 1); err != nil {
		t.Fatalf("CreateAuditEventDeadletter: %v", err)
	}

	type outcome struct {
		replayed bool
		err      error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, newID := range []string{"audit-replayed-1", "audit-replayed-2"} {
		wg.Add(1)
		go func(newID string) {
			defer wg.Done()
			<-start
			_, replayed, replayErr := q.ReplayAuditEventDeadletter(ctx, ev.ID, ev.ProjectID, newID)
			results <- outcome{replayed: replayed, err: replayErr}
		}(newID)
	}
	close(start)
	wg.Wait()
	close(results)

	var replayed, skipped int
	for result := range results {
		if result.err != nil {
			t.Fatalf("ReplayAuditEventDeadletter concurrent error: %v", result.err)
		}
		if result.replayed {
			replayed++
		} else {
			skipped++
		}
	}
	if replayed != 1 || skipped != 1 {
		t.Fatalf("replayed/skipped = %d/%d, want 1/1", replayed, skipped)
	}

	var chainRows, dlqRows int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM audit_events WHERE project_id = $1 AND action = $2),
			(SELECT COUNT(*) FROM audit_events_deadletter WHERE project_id = $1)
	`, ev.ProjectID, ev.Action).Scan(&chainRows, &dlqRows); err != nil {
		t.Fatalf("count replay results: %v", err)
	}
	if chainRows != 1 {
		t.Fatalf("audit_events replayed rows = %d, want 1", chainRows)
	}
	if dlqRows != 0 {
		t.Fatalf("audit_events_deadletter rows = %d, want 0", dlqRows)
	}

	vc, err := q.VerifyAuditChain(ctx, ev.ProjectID)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !vc.Valid {
		t.Fatalf("chain invalid after atomic replay: %s", vc.Error)
	}
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
		if err := q.CreateAuditEventDeadletter(ctx, ev, "x", 0); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	mk("old-a-1", "proj-a", old)
	mk("old-a-2", "proj-a", old)
	mk("young-a-1", "proj-a", young)
	mk("old-b-1", "proj-b", old)

	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	dropped, err := q.DeleteAuditDeadletterOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeleteAuditDeadletterOlderThan: %v", err)
	}
	if got, want := dropped["proj-a"], int64(2); got != want {
		t.Errorf("proj-a dropped = %d, want %d", got, want)
	}
	if got, want := dropped["proj-b"], int64(1); got != want {
		t.Errorf("proj-b dropped = %d, want %d", got, want)
	}
	// young-a-1 must still be present.
	count, err := q.CountAuditEventsDeadletter(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("remaining = %d, want 1 (young row should survive)", count)
	}
}
