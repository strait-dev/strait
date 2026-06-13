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

	"github.com/stretchr/testify/require"
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
		require.NoError(t, err)
		require.Equal(t, "NO",
			nullable,
		)
		require.True(t, strings.Contains(
			columnDefault,
			"1",
		))

		if shouldHaveTrigger, ok := triggerTables[table]; ok && !shouldHaveTrigger {
			continue
		}
		var triggerCount int
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`
			SELECT COUNT(*)
			FROM pg_trigger
			WHERE tgrelid = $1::regclass
			  AND tgname = 'cache_version_bump'
			  AND NOT tgisinternal
		`,

			table).Scan(&triggerCount))
		require.EqualValues(t, 1, triggerCount)

	}
}

func TestCacheVersion_DefaultsBumpsAndRollback(t *testing.T) {
	ctx := t.Context()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-cache-version")
	require.NoError(t, q.CreateJob(ctx,
		job))

	assertCacheVersion(t, ctx, "jobs", job.ID, 1)

	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Status:    domain.StatusQueued,
	}
	require.NoError(t, q.CreateRun(ctx,
		run))

	assertRunCacheVersion(t, ctx, run.ID, 1)
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.
		StatusQueued,

		domain.StatusExecuting,

		map[string]any{}))

	assertRunCacheVersion(t, ctx, run.ID, 2)

	if _, err := testDB.Pool.Exec(
		ctx,
		`UPDATE jobs SET description = 'cache-version-bumped' WHERE id = $1`,
		job.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"update job: %v", err)
	}
	assertCacheVersion(t, ctx, "jobs", job.ID, 2)

	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	if _, err := tx.Exec(ctx, `UPDATE jobs SET description = 'rolled-back' WHERE id = $1`, job.ID); err != nil {
		_ = tx.Rollback(ctx)
		require.Failf(t, "test failure",

			"tx update job: %v", err)
	}
	require.NoError(t, tx.Rollback(ctx))

	assertCacheVersion(t, ctx, "jobs", job.ID, 2)
}

func TestCacheVersion_RunStatusReadReturnsCacheVersion(t *testing.T) {
	ctx := t.Context()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-cache-version-status-read")
	require.NoError(t, q.CreateJob(ctx,
		job))

	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Status:    domain.StatusQueued,
	}
	require.NoError(t, q.CreateRun(ctx,
		run))
	_, initialVersion, err := q.GetRunWithCacheVersion(ctx, run.ID)
	require.NoError(t, err)

	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.
		StatusQueued,

		domain.StatusExecuting,

		map[string]any{}))

	got, version, err := q.GetRunWithCacheVersion(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, got.CacheVersion, version)
	require.Greater(t, version, initialVersion)
	require.Equal(t, domain.
		StatusExecuting,
		got.
			Status,
	)

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
		require.Failf(t, "test failure",

			"insert project quota: %v", err)
	}

	quota, err := q.GetProjectQuota(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, quota)
	require.EqualValues(t, 1, quota.
		CacheVersion,
	)

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE project_quotas SET max_queued_runs = $2 WHERE project_id = $1
	`, projectID, 20); err != nil {
		require.Failf(t, "test failure",

			"update project quota: %v", err)
	}

	quota, err = q.GetProjectQuota(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, quota)
	require.EqualValues(t, 2, quota.
		CacheVersion,
	)

}

func TestCacheNamespaceVersion_BumpEnsureAndRollback(t *testing.T) {
	ctx := t.Context()
	q := mustStore(t)
	mustClean(t, ctx)

	cacheKey := "project-1:user-1"

	version, err := q.EnsureCacheNamespaceVersion(ctx, "permission", cacheKey)
	require.NoError(t, err)
	require.EqualValues(t, 1, version)

	version, err = q.BumpCacheNamespaceVersion(ctx, "permission", cacheKey)
	require.NoError(t, err)
	require.EqualValues(t, 2, version)

	errRollback := q.WithTxQueries(ctx, func(tx *store.Queries) error {
		if _, err := tx.BumpCacheNamespaceVersion(ctx, "permission", cacheKey); err != nil {
			return err
		}
		return fmt.Errorf("force rollback")
	})
	require.NotNil(t, errRollback)

	version, err = q.GetCacheNamespaceVersion(ctx, "permission", cacheKey)
	require.NoError(t, err)
	require.EqualValues(t, 2, version)

}

func TestCacheVersion_JobDependencyRoundTripAndBump(t *testing.T) {
	ctx := t.Context()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-cache-version-job-deps")
	require.NoError(t, q.CreateJob(ctx,
		job))

	dependencyJob := baseJob(newID(), job.ProjectID)
	require.NoError(t, q.CreateJob(ctx,
		dependencyJob,
	))

	dep := &domain.JobDependency{
		JobID:          job.ID,
		DependsOnJobID: dependencyJob.ID,
		Condition:      "completed",
	}
	require.NoError(t, q.CreateJobDependency(ctx,
		dep))
	require.EqualValues(t, 2, dep.
		CacheVersion,
	)

	listVersion, err := q.GetJobDependencyListVersion(ctx, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, listVersion)

	got, err := q.GetJobDependency(ctx, dep.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, got.
		CacheVersion,
	)

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_dependencies SET condition = 'failed' WHERE id = $1
	`, dep.ID); err != nil {
		require.Failf(t, "test failure",

			"update job dependency: %v", err)
	}

	got, err = q.GetJobDependency(ctx, dep.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, got.
		CacheVersion,
	)

	deps, err := q.ListJobDependencies(ctx, job.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, deps, 1)
	require.EqualValues(t, 2, deps[0].CacheVersion)

	dependents, err := q.ListDependentsByDependencyJob(ctx, dependencyJob.ID)
	require.NoError(t, err)
	require.Len(t, dependents,

		1)
	require.EqualValues(t, 2, dependents[0].
		CacheVersion,
	)
	require.NoError(t, q.DeleteJobDependency(ctx,
		dep.ID,
	))

	listVersion, err = q.GetJobDependencyListVersion(ctx, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 3, listVersion)

	deps, err = q.ListJobDependencies(ctx, job.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, deps, 0)

}

func TestCacheVersion_ConcurrentUpdatesProduceMonotonicVersion(t *testing.T) {
	ctx := t.Context()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-cache-version-concurrent")
	require.NoError(t, q.CreateJob(ctx,
		job))

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
		require.NoError(t, err)

	}
	assertCacheVersion(t, ctx, "jobs", job.ID, 1+updates)
}

func assertCacheVersion(t *testing.T, ctx context.Context, table, id string, want int64) {
	t.Helper()
	var got int64
	query := fmt.Sprintf("SELECT cache_version FROM %s WHERE id = $1", table)
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		query,
		id).Scan(&got))
	require.Equal(t, want,
		got,
	)

}

func assertRunCacheVersion(t *testing.T, ctx context.Context, runID string, want int64) {
	t.Helper()
	var got int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
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

		runID).Scan(&got),
	)
	require.Equal(t, want,
		got,
	)

}

func TestCacheVersion_RunSideTableAppendsVersions(t *testing.T) {
	ctx := t.Context()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cache-version-append")
	run := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run))

	for i := range 3 {
		from := domain.StatusQueued
		to := domain.StatusExecuting
		if i%2 == 1 {
			from, to = domain.StatusExecuting, domain.StatusQueued
		}
		err := q.UpdateRunStatus(ctx, run.ID, from, to, map[string]any{
			"started_at": nil,
		})
		require.NoError(t, err)

	}

	var versions []int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COALESCE(ARRAY_AGG(cache_version ORDER BY id ASC), '{}'::bigint[])
		FROM job_run_cache_versions
		WHERE run_id = $1`,

		run.ID).Scan(&versions))
	require.Len(t, versions, 3)
	require.Greater(t, versions[0], int64(1))
	require.Greater(t, versions[1], versions[0])
	require.Greater(t, versions[2], versions[1])

	latestVersion := versions[len(versions)-1]
	assertRunCacheVersion(t, ctx, run.ID, latestVersion)
}
