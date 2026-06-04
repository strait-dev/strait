//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
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
	if err := q.UpsertDebouncePending(ctx, d); err != nil {
		t.Fatalf("UpsertDebouncePending() error = %v", err)
	}
	if d.ID == "" {
		t.Fatal("UpsertDebouncePending() did not set ID")
	}
	if d.CreatedAt.IsZero() {
		t.Fatal("UpsertDebouncePending() did not set CreatedAt")
	}
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
	if err := q.UpsertDebouncePending(ctx, d1); err != nil {
		t.Fatalf("UpsertDebouncePending(1) error = %v", err)
	}
	initialID := d1.ID
	initialCreatedAt := d1.CreatedAt
	var xminBeforeNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM debounce_pending
		WHERE job_id = $1 AND debounce_key = $2`,
		job.ID,
		"replace-key",
	).Scan(&xminBeforeNoop); err != nil {
		t.Fatalf("query debounce_pending xmin before no-op: %v", err)
	}

	same := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "replace-key",
		Payload:     json.RawMessage(`{"v":1}`),
		Priority:    1,
		TriggeredBy: "api",
		FireAt:      fireAt,
	}
	if err := q.UpsertDebouncePending(ctx, same); err != nil {
		t.Fatalf("UpsertDebouncePending(no-op) error = %v", err)
	}
	var xminAfterNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM debounce_pending
		WHERE job_id = $1 AND debounce_key = $2`,
		job.ID,
		"replace-key",
	).Scan(&xminAfterNoop); err != nil {
		t.Fatalf("query debounce_pending xmin after no-op: %v", err)
	}
	if xminAfterNoop != xminBeforeNoop {
		t.Fatalf("debounce_pending no-op changed xmin from %s to %s", xminBeforeNoop, xminAfterNoop)
	}
	if same.ID != initialID {
		t.Fatalf("debounce_pending no-op id = %q, want %q", same.ID, initialID)
	}
	if !same.CreatedAt.Equal(initialCreatedAt) {
		t.Fatalf("debounce_pending no-op created_at = %v, want %v", same.CreatedAt, initialCreatedAt)
	}

	d2 := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "replace-key",
		Payload:     json.RawMessage(`{"v":2}`),
		Priority:    10,
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(60 * time.Second),
	}
	if err := q.UpsertDebouncePending(ctx, d2); err != nil {
		t.Fatalf("UpsertDebouncePending(2) error = %v", err)
	}
	if d2.ID != initialID {
		t.Fatalf("debounce_pending update id = %q, want %q", d2.ID, initialID)
	}

	// Should have only one row (replaced).
	var rowCount int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM debounce_pending
		WHERE job_id = $1 AND debounce_key = $2`,
		job.ID,
		"replace-key",
	).Scan(&rowCount); err != nil {
		t.Fatalf("query replaced debounce_pending: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("debounce_pending row count = %d, want 1", rowCount)
	}
	var payload json.RawMessage
	var priority int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT payload, priority
		FROM debounce_pending
		WHERE job_id = $1 AND debounce_key = $2`,
		job.ID,
		"replace-key",
	).Scan(&payload, &priority); err != nil {
		t.Fatalf("query replaced debounce_pending fields: %v", err)
	}
	if !jsonEqual(payload, json.RawMessage(`{"v":2}`)) {
		t.Fatalf("debounce_pending payload = %s, want {\"v\":2}", string(payload))
	}
	if priority != 10 {
		t.Fatalf("debounce_pending priority = %d, want 10", priority)
	}
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
	if err := q.UpsertDebouncePending(ctx, due); err != nil {
		t.Fatalf("UpsertDebouncePending(due) error = %v", err)
	}

	// Create a future debounce.
	future := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "future-key",
		Payload:     json.RawMessage(`{}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(10 * time.Minute),
	}
	if err := q.UpsertDebouncePending(ctx, future); err != nil {
		t.Fatalf("UpsertDebouncePending(future) error = %v", err)
	}

	items, err := q.ListDueDebouncePending(ctx)
	if err != nil {
		t.Fatalf("ListDueDebouncePending() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListDueDebouncePending() len = %d, want 1", len(items))
	}
	if items[0].DebounceKey != "due-key" {
		t.Fatalf("ListDueDebouncePending() key = %q, want due-key", items[0].DebounceKey)
	}
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
		if err := q.UpsertDebouncePending(ctx, d); err != nil {
			t.Fatalf("UpsertDebouncePending(projectA %d) error = %v", i, err)
		}
	}
	projectBPending := &domain.DebouncePending{
		JobID:       jobB.ID,
		ProjectID:   projectB,
		DebounceKey: "b-1",
		Payload:     json.RawMessage(`{}`),
		TriggeredBy: "api",
		FireAt:      fireAt.Add(9 * time.Minute),
	}
	if err := q.UpsertDebouncePending(ctx, projectBPending); err != nil {
		t.Fatalf("UpsertDebouncePending(projectB) error = %v", err)
	}

	items, err := q.ListDueDebouncePending(ctx)
	if err != nil {
		t.Fatalf("ListDueDebouncePending() error = %v", err)
	}

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
	if !foundProjectB {
		t.Fatalf("due list omitted later project while one project had many older rows: %+v", items)
	}
	if projectACount > 5 {
		t.Fatalf("due list allowed one project to monopolize batch: projectA count = %d", projectACount)
	}
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
	if err := q.UpsertDebouncePending(ctx, due); err != nil {
		t.Fatalf("UpsertDebouncePending(due) error = %v", err)
	}
	future := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "claim-future",
		Payload:     json.RawMessage(`{"future":true}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(10 * time.Minute),
	}
	if err := q.UpsertDebouncePending(ctx, future); err != nil {
		t.Fatalf("UpsertDebouncePending(future) error = %v", err)
	}

	claimed, ok, err := q.ClaimDueDebouncePending(ctx, due.ID)
	if err != nil {
		t.Fatalf("ClaimDueDebouncePending(due) error = %v", err)
	}
	if !ok || claimed == nil || claimed.ID != due.ID || claimed.DebounceKey != "claim-due" {
		t.Fatalf("claimed due row = %+v ok=%v", claimed, ok)
	}
	stillDue, err := q.ListDueDebouncePending(ctx)
	if err != nil {
		t.Fatalf("ListDueDebouncePending(after claim) error = %v", err)
	}
	if len(stillDue) != 1 || stillDue[0].ID != due.ID {
		t.Fatalf("claim removed due row before completion: %+v", stillDue)
	}

	claimed, ok, err = q.ClaimDueDebouncePending(ctx, future.ID)
	if err != nil {
		t.Fatalf("ClaimDueDebouncePending(future) error = %v", err)
	}
	if ok || claimed != nil {
		t.Fatalf("future row was claimed: %+v ok=%v", claimed, ok)
	}
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
	if err := q.UpsertDebouncePending(ctx, due); err != nil {
		t.Fatalf("UpsertDebouncePending(due) error = %v", err)
	}

	staleUpdated, err := q.RescheduleDebouncePending(ctx, due.ID, originalFireAt.Add(time.Second), time.Now().UTC().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("RescheduleDebouncePending(stale) error = %v", err)
	}
	if staleUpdated {
		t.Fatal("stale fire_at rescheduled pending row")
	}
	stillDue, err := q.ListDueDebouncePending(ctx)
	if err != nil {
		t.Fatalf("ListDueDebouncePending(after stale reschedule) error = %v", err)
	}
	if len(stillDue) != 1 || stillDue[0].ID != due.ID {
		t.Fatalf("stale reschedule changed due row: %+v", stillDue)
	}

	nextFireAt := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Microsecond)
	rescheduled, err := q.RescheduleDebouncePending(ctx, due.ID, originalFireAt, nextFireAt)
	if err != nil {
		t.Fatalf("RescheduleDebouncePending(current) error = %v", err)
	}
	if !rescheduled {
		t.Fatal("current fire_at did not reschedule pending row")
	}
	claimed, ok, err := q.ClaimDueDebouncePending(ctx, due.ID)
	if err != nil {
		t.Fatalf("ClaimDueDebouncePending(rescheduled) error = %v", err)
	}
	if ok || claimed != nil {
		t.Fatalf("rescheduled future row was claimable: %+v ok=%v", claimed, ok)
	}
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
	if err := q.UpsertDebouncePending(ctx, due); err != nil {
		t.Fatalf("UpsertDebouncePending(due) error = %v", err)
	}

	updated := *due
	updated.FireAt = time.Now().UTC().Add(10 * time.Minute).Truncate(time.Microsecond)
	updated.Payload = json.RawMessage(`{"v":2}`)
	if err := q.UpsertDebouncePending(ctx, &updated); err != nil {
		t.Fatalf("UpsertDebouncePending(updated) error = %v", err)
	}

	completed, err := q.CompleteDebouncePending(ctx, due.ID, originalFireAt)
	if err != nil {
		t.Fatalf("CompleteDebouncePending(stale) error = %v", err)
	}
	if completed {
		t.Fatal("stale completion deleted updated pending row")
	}

	completed, err = q.CompleteDebouncePending(ctx, due.ID, updated.FireAt)
	if err != nil {
		t.Fatalf("CompleteDebouncePending(current) error = %v", err)
	}
	if !completed {
		t.Fatal("current completion did not delete pending row")
	}
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
	if err := q.UpsertDebouncePending(ctx, newer); err != nil {
		t.Fatalf("UpsertDebouncePending(newer) error = %v", err)
	}
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
	if err != nil {
		t.Fatalf("InsertDebouncePendingIfAbsent() error = %v", err)
	}
	if inserted {
		t.Fatal("expected old pending restore to be skipped while newer pending exists")
	}

	items, err := q.ListDueDebouncePending(ctx)
	if err != nil {
		t.Fatalf("ListDueDebouncePending() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("old due pending was restored over newer future row: %+v", items)
	}
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
	if err := q.UpsertDebouncePending(ctx, d); err != nil {
		t.Fatalf("UpsertDebouncePending() error = %v", err)
	}

	if err := q.DeleteDebouncePending(ctx, d.ID); err != nil {
		t.Fatalf("DeleteDebouncePending() error = %v", err)
	}

	// Verify it was deleted by listing due items.
	items, err := q.ListDueDebouncePending(ctx)
	if err != nil {
		t.Fatalf("ListDueDebouncePending() error = %v", err)
	}
	for _, item := range items {
		if item.ID == d.ID {
			t.Fatalf("DeleteDebouncePending() did not delete the item")
		}
	}
}
