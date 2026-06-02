//go:build integration

package store_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestCacheVersion_SchemaCoversStrongAndStatusTables(t *testing.T) {
	ctx := t.Context()

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
		"job_run_cache_versions",
		"job_runs_history",
		"workflow_runs",
		"workflow_step_runs",
	}
	triggerTables := map[string]bool{
		"job_run_cache_versions": false,
		"job_runs_history":       false,
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
	ctx := t.Context()
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
	assertRunCacheVersion(t, ctx, run.ID, 1)

	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusExecuting, map[string]any{}); err != nil {
		t.Fatalf("UpdateRunStatus() error = %v", err)
	}
	assertRunCacheVersion(t, ctx, run.ID, 2)

	if _, err := testDB.Pool.Exec(
		ctx,
		`UPDATE jobs SET description = 'cache-version-bumped' WHERE id = $1`,
		job.ID,
	); err != nil {
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

func TestCacheVersion_RunStatusReadReturnsCacheVersion(t *testing.T) {
	ctx := t.Context()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-cache-version-status-read")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Status:    domain.StatusQueued,
	}
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusExecuting, map[string]any{}); err != nil {
		t.Fatalf("UpdateRunStatus() error = %v", err)
	}

	got, version, err := q.GetRunWithCacheVersion(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRunWithCacheVersion() error = %v", err)
	}
	if got.CacheVersion != 2 || version != 2 {
		t.Fatalf("GetRunWithCacheVersion() version = %d/%d, want 2/2", got.CacheVersion, version)
	}
	if got.Status != domain.StatusExecuting {
		t.Fatalf("GetRunWithCacheVersion() status = %s, want executing", got.Status)
	}
}

func TestCacheVersion_ProjectQuotaRoundTripAndBump(t *testing.T) {
	ctx := t.Context()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-cache-version-quota-" + newID()
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO project_quotas (project_id, max_queued_runs)
		VALUES ($1, $2)
	`, projectID, 10); err != nil {
		t.Fatalf("insert project quota: %v", err)
	}

	quota, err := q.GetProjectQuota(ctx, projectID)
	if err != nil {
		t.Fatalf("GetProjectQuota() error = %v", err)
	}
	if quota == nil {
		t.Fatal("GetProjectQuota() = nil, want quota")
	}
	if quota.CacheVersion != 1 {
		t.Fatalf("initial quota CacheVersion = %d, want 1", quota.CacheVersion)
	}

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE project_quotas SET max_queued_runs = $2 WHERE project_id = $1
	`, projectID, 20); err != nil {
		t.Fatalf("update project quota: %v", err)
	}

	quota, err = q.GetProjectQuota(ctx, projectID)
	if err != nil {
		t.Fatalf("GetProjectQuota(after update) error = %v", err)
	}
	if quota == nil {
		t.Fatal("GetProjectQuota(after update) = nil, want quota")
	}
	if quota.CacheVersion != 2 {
		t.Fatalf("updated quota CacheVersion = %d, want 2", quota.CacheVersion)
	}
}

func TestCacheNamespaceVersion_BumpEnsureAndRollback(t *testing.T) {
	ctx := t.Context()
	q := mustStore(t)
	mustClean(t, ctx)

	cacheKey := "project-1:user-1"

	version, err := q.EnsureCacheNamespaceVersion(ctx, "permission", cacheKey)
	if err != nil {
		t.Fatalf("EnsureCacheNamespaceVersion() error = %v", err)
	}
	if version != 1 {
		t.Fatalf("initial version = %d, want 1", version)
	}
	version, err = q.BumpCacheNamespaceVersion(ctx, "permission", cacheKey)
	if err != nil {
		t.Fatalf("BumpCacheNamespaceVersion() error = %v", err)
	}
	if version != 2 {
		t.Fatalf("bumped version = %d, want 2", version)
	}

	errRollback := q.WithTxQueries(ctx, func(tx *store.Queries) error {
		if _, err := tx.BumpCacheNamespaceVersion(ctx, "permission", cacheKey); err != nil {
			return err
		}
		return fmt.Errorf("force rollback")
	})
	if errRollback == nil {
		t.Fatal("WithTxQueries() error = nil, want forced rollback")
	}
	version, err = q.GetCacheNamespaceVersion(ctx, "permission", cacheKey)
	if err != nil {
		t.Fatalf("GetCacheNamespaceVersion() error = %v", err)
	}
	if version != 2 {
		t.Fatalf("version after rollback = %d, want 2", version)
	}
}

func TestCacheVersion_JobDependencyRoundTripAndBump(t *testing.T) {
	ctx := t.Context()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-cache-version-job-deps")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	dependencyJob := baseJob(newID(), job.ProjectID)
	if err := q.CreateJob(ctx, dependencyJob); err != nil {
		t.Fatalf("CreateJob(dependency) error = %v", err)
	}

	dep := &domain.JobDependency{
		JobID:          job.ID,
		DependsOnJobID: dependencyJob.ID,
		Condition:      "completed",
	}
	if err := q.CreateJobDependency(ctx, dep); err != nil {
		t.Fatalf("CreateJobDependency() error = %v", err)
	}
	if dep.CacheVersion != 2 {
		t.Fatalf("created dependency list CacheVersion = %d, want 2", dep.CacheVersion)
	}
	listVersion, err := q.GetJobDependencyListVersion(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJobDependencyListVersion() error = %v", err)
	}
	if listVersion != 2 {
		t.Fatalf("GetJobDependencyListVersion() = %d, want 2", listVersion)
	}

	got, err := q.GetJobDependency(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetJobDependency() error = %v", err)
	}
	if got.CacheVersion != 1 {
		t.Fatalf("GetJobDependency() CacheVersion = %d, want 1", got.CacheVersion)
	}

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_dependencies SET condition = 'failed' WHERE id = $1
	`, dep.ID); err != nil {
		t.Fatalf("update job dependency: %v", err)
	}

	got, err = q.GetJobDependency(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetJobDependency(after update) error = %v", err)
	}
	if got.CacheVersion != 2 {
		t.Fatalf("updated GetJobDependency() CacheVersion = %d, want 2", got.CacheVersion)
	}
	deps, err := q.ListJobDependencies(ctx, job.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobDependencies() error = %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("ListJobDependencies() len = %d, want 1", len(deps))
	}
	if deps[0].CacheVersion != 2 {
		t.Fatalf("ListJobDependencies()[0] CacheVersion = %d, want 2", deps[0].CacheVersion)
	}

	dependents, err := q.ListDependentsByDependencyJob(ctx, dependencyJob.ID)
	if err != nil {
		t.Fatalf("ListDependentsByDependencyJob() error = %v", err)
	}
	if len(dependents) != 1 {
		t.Fatalf("ListDependentsByDependencyJob() len = %d, want 1", len(dependents))
	}
	if dependents[0].CacheVersion != 2 {
		t.Fatalf("ListDependentsByDependencyJob()[0] CacheVersion = %d, want 2", dependents[0].CacheVersion)
	}

	if err := q.DeleteJobDependency(ctx, dep.ID); err != nil {
		t.Fatalf("DeleteJobDependency() error = %v", err)
	}
	listVersion, err = q.GetJobDependencyListVersion(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJobDependencyListVersion(after delete) error = %v", err)
	}
	if listVersion != 3 {
		t.Fatalf("GetJobDependencyListVersion(after delete) = %d, want 3", listVersion)
	}
	deps, err = q.ListJobDependencies(ctx, job.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobDependencies(after delete) error = %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("ListJobDependencies(after delete) len = %d, want 0", len(deps))
	}
}

func TestCacheVersion_ConcurrentUpdatesProduceMonotonicVersion(t *testing.T) {
	ctx := t.Context()
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

func assertRunCacheVersion(t *testing.T, ctx context.Context, runID string, want int64) {
	t.Helper()
	var got int64
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COALESCE(v.cache_version, jr.cache_version)
		FROM job_runs jr
		LEFT JOIN LATERAL (
			SELECT cache_version
			FROM job_run_cache_versions v
			WHERE v.run_id = jr.id
			ORDER BY v.id DESC
			LIMIT 1
		) v ON true
		WHERE jr.id = $1`,
		runID,
	).Scan(&got); err != nil {
		t.Fatalf("select run cache_version: %v", err)
	}
	if got != want {
		t.Fatalf("run cache_version = %d, want %d", got, want)
	}
}

func TestCacheVersion_RunSideTableAppendsVersions(t *testing.T) {
	ctx := t.Context()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cache-version-append")
	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	for i := range 3 {
		from := domain.StatusQueued
		to := domain.StatusExecuting
		if i%2 == 1 {
			from, to = domain.StatusExecuting, domain.StatusQueued
		}
		err := q.UpdateRunStatus(ctx, run.ID, from, to, map[string]any{
			"started_at": nil,
		})
		if err != nil {
			t.Fatalf("UpdateRunStatus(%d) error = %v", i, err)
		}
	}

	var rawRows int
	var latestVersion int64
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cache_version ORDER BY id DESC))[1], 0)
		FROM job_run_cache_versions
		WHERE run_id = $1`, run.ID).Scan(&rawRows, &latestVersion); err != nil {
		t.Fatalf("query cache version history: %v", err)
	}
	if rawRows != 3 {
		t.Fatalf("cache version rows = %d, want append-only history", rawRows)
	}
	if latestVersion != 4 {
		t.Fatalf("latest cache version = %d, want 4", latestVersion)
	}
	assertRunCacheVersion(t, ctx, run.ID, latestVersion)
}
