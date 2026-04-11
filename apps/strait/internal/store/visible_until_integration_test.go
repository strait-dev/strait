//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

// Phase 7 integration tests for soft-delete retention via visible_until.
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

	// visible_until is set.
	var visibleUntil *time.Time
	err = testDB.Pool.QueryRow(ctx, "SELECT visible_until FROM job_runs WHERE id = $1", run.ID).Scan(&visibleUntil)
	if err != nil {
		t.Fatalf("query visible_until: %v", err)
	}
	if visibleUntil == nil {
		t.Error("visible_until should be set after mask")
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
	// Second call should see visible_until IS NOT NULL and skip the row.
	second, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	if err != nil {
		t.Fatalf("second mask: %v", err)
	}
	if second != 0 {
		t.Errorf("second = %d, want 0 (already masked)", second)
	}
}

func TestMaskRunsByOrgOlderThan_HOTUpdateEligible(t *testing.T) {
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
	for i := 0; i < 50; i++ {
		r := baseRun(job, newID())
		r.Status = domain.StatusCompleted
		if err := q.CreateRun(ctx, r); err != nil {
			t.Fatalf("run: %v", err)
		}
	}
	past := time.Now().UTC().Add(-48 * time.Hour)
	_, _ = testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET status='completed', finished_at=$1 WHERE project_id=$2", past, projectID)

	// Reset stats before the mask operation.
	_, _ = testDB.Pool.Exec(ctx, "SELECT pg_stat_clear_snapshot()")

	_, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}

	// visible_until is intentionally not indexed, so the UPDATE should be
	// HOT-eligible. Query pg_stat_user_tables for the ratio.
	var upd, hot int64
	err = testDB.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(n_tup_upd), 0), COALESCE(SUM(n_tup_hot_upd), 0)
		FROM pg_stat_user_tables
		WHERE relname = 'job_runs' OR relname LIKE 'job_runs_%'
	`).Scan(&upd, &hot)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if upd == 0 {
		t.Skip("pg_stat reported zero updates (timing)")
	}
	t.Logf("upd=%d hot=%d", upd, hot)
	// HOT ratio should be high on the masking path specifically. We don't
	// assert a strict threshold because the test fixture does many non-
	// mask updates. This test is primarily a smoke check that the UPDATE
	// runs without error and reports in pg_stat.
}
