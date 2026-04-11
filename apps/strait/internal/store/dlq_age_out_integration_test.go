//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestMaskOldDLQRows_MasksOnlyStale(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-age-" + newID()
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org", Name: "p"}); err != nil {
		t.Fatalf("project: %v", err)
	}
	job := baseJob(newID(), projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("job: %v", err)
	}

	// Insert 3 DLQ rows: one old, one borderline, one fresh.
	ages := []time.Duration{48 * time.Hour, 26 * time.Hour, 1 * time.Hour}
	var ids []string
	for i, age := range ages {
		id := newID()
		_, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
			VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW() - $4::interval)
		`, id, job.ID, projectID, age.String())
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		ids = append(ids, id)
	}

	// Mask older than 24h.
	n, err := q.MaskOldDLQRows(ctx, 24*time.Hour, 100)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	if n != 2 {
		t.Errorf("masked = %d, want 2", n)
	}

	// Verify visible_until set on the two old rows, null on the fresh one.
	for i, id := range ids {
		var visible *time.Time
		err := testDB.Pool.QueryRow(ctx, `SELECT visible_until FROM job_runs WHERE id = $1`, id).Scan(&visible)
		if err != nil {
			t.Fatalf("query %d: %v", i, err)
		}
		if i < 2 {
			if visible == nil {
				t.Errorf("row %d should be masked", i)
			}
		} else {
			if visible != nil {
				t.Errorf("row %d should still be visible", i)
			}
		}
	}
}

func TestMaskOldDLQRows_Idempotent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-idem-" + newID()
	_ = q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org", Name: "p"})
	job := baseJob(newID(), projectID)
	_ = q.CreateJob(ctx, job)

	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
		VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW() - INTERVAL '48 hours')
	`, newID(), job.ID, projectID)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	first, _ := q.MaskOldDLQRows(ctx, 24*time.Hour, 100)
	second, _ := q.MaskOldDLQRows(ctx, 24*time.Hour, 100)
	if first != 1 || second != 0 {
		t.Errorf("first=%d second=%d, want 1,0", first, second)
	}
}

func TestMaskOldDLQRows_DecrementsCounterViaTrigger(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-count-" + newID()
	_ = q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org", Name: "p"})
	job := baseJob(newID(), projectID)
	_ = q.CreateJob(ctx, job)

	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
		VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW() - INTERVAL '48 hours')
	`, newID(), job.ID, projectID)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var before, after int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COALESCE(count, 0) FROM dlq_counts WHERE job_id = $1`, job.ID).Scan(&before)
	if before != 1 {
		t.Fatalf("dlq counter before = %d, want 1", before)
	}

	if _, err := q.MaskOldDLQRows(ctx, 24*time.Hour, 100); err != nil {
		t.Fatalf("mask: %v", err)
	}

	_ = testDB.Pool.QueryRow(ctx, `SELECT COALESCE(count, 0) FROM dlq_counts WHERE job_id = $1`, job.ID).Scan(&after)
	if after != 0 {
		t.Errorf("dlq counter after = %d, want 0", after)
	}
}

func TestMaskOldDLQRows_RespectsLimit(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-limit-" + newID()
	_ = q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org", Name: "p"})
	job := baseJob(newID(), projectID)
	_ = q.CreateJob(ctx, job)

	for i := 0; i < 10; i++ {
		_, _ = testDB.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
			VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW() - INTERVAL '48 hours')
		`, newID(), job.ID, projectID)
	}

	first, _ := q.MaskOldDLQRows(ctx, 24*time.Hour, 3)
	if first != 3 {
		t.Errorf("first tick masked %d, want 3", first)
	}
	second, _ := q.MaskOldDLQRows(ctx, 24*time.Hour, 3)
	if second != 3 {
		t.Errorf("second tick = %d, want 3", second)
	}
}
