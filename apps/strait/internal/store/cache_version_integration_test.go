//go:build integration

package store_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"strait/internal/domain"
)

func TestCacheVersion_SchemaCoversStrongAndStatusTables(t *testing.T) {
	ctx := context.Background()

	tables := []string{
		"api_keys",
		"project_roles",
		"project_member_roles",
		"resource_policies",
		"tag_policies",
		"project_quotas",
		"organization_subscriptions",
		"jobs",
		"job_dependencies",
		"job_runs",
		"job_runs_history",
		"workflow_runs",
		"workflow_step_runs",
	}
	triggerTables := map[string]bool{
		"job_runs_history": false,
	}
	for _, table := range tables {
		var nullable, columnDefault string
		err := testDB.Pool.QueryRow(ctx, `
			SELECT is_nullable, column_default
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = $1
			  AND column_name = 'cache_version'
		`, table).Scan(&nullable, &columnDefault)
		if err != nil {
			t.Fatalf("%s cache_version column missing: %v", table, err)
		}
		if nullable != "NO" {
			t.Fatalf("%s cache_version nullable = %s, want NO", table, nullable)
		}
		if !strings.Contains(columnDefault, "1") {
			t.Fatalf("%s cache_version default = %q, want 1", table, columnDefault)
		}
		if shouldHaveTrigger, ok := triggerTables[table]; ok && !shouldHaveTrigger {
			continue
		}
		var triggerCount int
		if err := testDB.Pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM pg_trigger
			WHERE tgrelid = $1::regclass
			  AND tgname = 'cache_version_bump'
			  AND NOT tgisinternal
		`, table).Scan(&triggerCount); err != nil {
			t.Fatalf("%s trigger lookup: %v", table, err)
		}
		if triggerCount != 1 {
			t.Fatalf("%s cache_version_bump trigger count = %d, want 1", table, triggerCount)
		}
	}
}

func TestCacheVersion_DefaultsBumpsAndRollback(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-cache-version")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	assertCacheVersion(t, ctx, "jobs", job.ID, 1)

	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Status:    domain.StatusQueued,
	}
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	assertCacheVersion(t, ctx, "job_runs", run.ID, 1)

	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusExecuting, map[string]any{}); err != nil {
		t.Fatalf("UpdateRunStatus() error = %v", err)
	}
	assertCacheVersion(t, ctx, "job_runs", run.ID, 2)

	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET description = 'cache-version-bumped' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("update job: %v", err)
	}
	assertCacheVersion(t, ctx, "jobs", job.ID, 2)

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET description = 'rolled-back' WHERE id = $1`, job.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("tx update job: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	assertCacheVersion(t, ctx, "jobs", job.ID, 2)
}

func TestCacheVersion_ConcurrentUpdatesProduceMonotonicVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-cache-version-concurrent")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	const updates = 8
	errs := make(chan error, updates)
	var wg sync.WaitGroup
	for i := range updates {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := testDB.Pool.Exec(ctx,
				`UPDATE jobs SET description = COALESCE(description, '') || $2 WHERE id = $1`,
				job.ID,
				fmt.Sprintf("-%d", i),
			)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent update error = %v", err)
		}
	}
	assertCacheVersion(t, ctx, "jobs", job.ID, 1+updates)
}

func assertCacheVersion(t *testing.T, ctx context.Context, table, id string, want int64) {
	t.Helper()
	var got int64
	query := fmt.Sprintf("SELECT cache_version FROM %s WHERE id = $1", table)
	if err := testDB.Pool.QueryRow(ctx, query, id).Scan(&got); err != nil {
		t.Fatalf("select %s cache_version: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s cache_version = %d, want %d", table, got, want)
	}
}
