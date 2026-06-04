//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

// Integration tests for soft-delete retention via append-only visibility events.
// These tests complement the existing TestDeleteRunsByOrgOlderThan which
// already asserts RowsAffected — they additionally verify that the rows
// physically remain in the table (masked, not deleted).

func TestMaskRunsByOrgOlderThan_RowsPhysicallyRemain(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-mask-phys-" + newID()
	projectID := "proj-mask-phys-" + newID()
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P"}); err != nil {
		t.Fatalf("project: %v", err)
	}
	job := baseJob(newID(), projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("job: %v", err)
	}
	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("run: %v", err)
	}
	past := time.Now().UTC().Add(-48 * time.Hour)
	if _, err := testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET status='completed', finished_at=$1 WHERE id=$2", past, run.ID); err != nil {
		t.Fatalf("update finished_at: %v", err)
	}

	masked, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	if masked != 1 {
		t.Fatalf("masked = %d, want 1", masked)
	}

	// Physically still there.
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM job_runs WHERE id = $1", run.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("row physically missing after mask: count = %d", count)
	}

	var ledgerVisibleUntil *time.Time
	err = testDB.Pool.QueryRow(ctx, "SELECT visible_until FROM job_runs WHERE id = $1", run.ID).Scan(&ledgerVisibleUntil)
	if err != nil {
		t.Fatalf("query ledger visible_until: %v", err)
	}
	if ledgerVisibleUntil != nil {
		t.Fatalf("ledger visible_until = %v, want nil", *ledgerVisibleUntil)
	}

	var eventVisibleUntil *time.Time
	err = testDB.Pool.QueryRow(ctx, `
		SELECT visible_until
		FROM job_run_visibility_events
		WHERE run_id = $1
		ORDER BY id DESC
		LIMIT 1
	`, run.ID).Scan(&eventVisibleUntil)
	if err != nil {
		t.Fatalf("query visibility event: %v", err)
	}
	if eventVisibleUntil == nil {
		t.Error("visibility event should mask the run")
	}
}

func TestMaskRunsByOrgOlderThan_IdempotentWithinSameCutoff(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-mask-idem-" + newID()
	projectID := "proj-mask-idem-" + newID()
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P"}); err != nil {
		t.Fatalf("project: %v", err)
	}
	job := baseJob(newID(), projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("job: %v", err)
	}
	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("run: %v", err)
	}
	past := time.Now().UTC().Add(-48 * time.Hour)
	_, _ = testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET status='completed', finished_at=$1 WHERE id=$2", past, run.ID)

	first, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	if err != nil {
		t.Fatalf("first mask: %v", err)
	}
	if first != 1 {
		t.Errorf("first = %d, want 1", first)
	}
	// Second call should see the visibility event and skip the row.
	second, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	if err != nil {
		t.Fatalf("second mask: %v", err)
	}
	if second != 0 {
		t.Errorf("second = %d, want 0 (already masked)", second)
	}
}

func TestMaskRunsByOrgOlderThan_DoesNotUpdateLedger(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-mask-hot-" + newID()
	projectID := "proj-mask-hot-" + newID()
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P"}); err != nil {
		t.Fatalf("project: %v", err)
	}
	job := baseJob(newID(), projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("job: %v", err)
	}
	// Enqueue a batch and mark them all as old terminal.
	for range 50 {
		r := baseRun(job, newID())
		r.Status = domain.StatusCompleted
		if err := q.CreateRun(ctx, r); err != nil {
			t.Fatalf("run: %v", err)
		}
	}
	past := time.Now().UTC().Add(-48 * time.Hour)
	_, _ = testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET status='completed', finished_at=$1 WHERE project_id=$2", past, projectID)

	_, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}

	var ledgerMasks int
	err = testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_runs
		WHERE project_id = $1
		  AND visible_until IS NOT NULL
	`, projectID).Scan(&ledgerMasks)
	if err != nil {
		t.Fatalf("query ledger masks: %v", err)
	}
	if ledgerMasks != 0 {
		t.Fatalf("ledger masks = %d, want 0", ledgerMasks)
	}

	var events int
	err = testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_visibility_events
		WHERE run_id IN (SELECT id FROM job_runs WHERE project_id = $1)
	`, projectID).Scan(&events)
	if err != nil {
		t.Fatalf("query visibility events: %v", err)
	}
	if events != 50 {
		t.Fatalf("visibility events = %d, want 50", events)
	}
}
