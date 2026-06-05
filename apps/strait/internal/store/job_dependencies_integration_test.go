//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestCreateJobDependency(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-job-dep-create"
	job := mustCreateJob(t, ctx, q, projectID)
	depJob := mustCreateJob(t, ctx, q, projectID)

	dep := &domain.JobDependency{
		JobID:          job.ID,
		DependsOnJobID: depJob.ID,
	}
	require.NoError(t, q.CreateJobDependency(ctx,
		dep))
	require.NotEqual(t, "",

		dep.ID)
	require.Equal(t, "completed",

		dep.
			Condition)

}

func TestCreateJobDependency_SelfDependency(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-dep-self")

	dep := &domain.JobDependency{
		JobID:          job.ID,
		DependsOnJobID: job.ID,
	}
	require.Error(t, q.CreateJobDependency(ctx,
		dep))

}

func TestListJobDependencies(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-job-dep-list"
	job := mustCreateJob(t, ctx, q, projectID)
	depA := mustCreateJob(t, ctx, q, projectID)
	depB := mustCreateJob(t, ctx, q, projectID)

	for _, depJob := range []*domain.Job{depA, depB} {
		dep := &domain.JobDependency{
			JobID:          job.ID,
			DependsOnJobID: depJob.ID,
		}
		require.NoError(t, q.CreateJobDependency(ctx,
			dep))

	}

	deps, err := q.ListJobDependencies(ctx, job.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, deps, 2)

}

func TestDeleteJobDependency(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-job-dep-delete"
	job := mustCreateJob(t, ctx, q, projectID)
	depJob := mustCreateJob(t, ctx, q, projectID)

	dep := &domain.JobDependency{
		JobID:          job.ID,
		DependsOnJobID: depJob.ID,
	}
	require.NoError(t, q.CreateJobDependency(ctx,
		dep))
	require.NoError(t, q.DeleteJobDependency(ctx,
		dep.ID))

	deps, err := q.ListJobDependencies(ctx, job.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, deps, 0)

}

func TestListDependentsByDependencyJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-job-dep-dependents"
	depJob := mustCreateJob(t, ctx, q, projectID)
	jobA := mustCreateJob(t, ctx, q, projectID)
	jobB := mustCreateJob(t, ctx, q, projectID)

	for _, j := range []*domain.Job{jobA, jobB} {
		dep := &domain.JobDependency{
			JobID:          j.ID,
			DependsOnJobID: depJob.ID,
		}
		require.NoError(t, q.CreateJobDependency(ctx,
			dep))

	}

	dependents, err := q.ListDependentsByDependencyJob(ctx, depJob.ID)
	require.NoError(t, err)
	require.Len(t, dependents,

		2)

	// Empty case.
	empty, err := q.ListDependentsByDependencyJob(ctx, newID())
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestListWaitingRunsByJobIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-waiting-runs-by-job-ids"
	job := mustCreateJob(t, ctx, q, projectID)

	run := baseRun(job, newID())
	run.Status = domain.StatusWaiting
	require.NoError(t, q.CreateRun(ctx,
		run))

	runs, err := q.ListWaitingRunsByJobIDs(ctx, []string{job.ID}, 100)
	require.NoError(t, err)
	require.Len(t, runs, 1)

	// Empty job IDs.
	nilRuns, err := q.ListWaitingRunsByJobIDs(ctx, []string{}, 100)
	require.NoError(t, err)
	require.Nil(t, nilRuns)

}

func TestAreJobDependenciesSatisfied_NoDeps(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-deps-satisfied-no-deps")
	run := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run))

	satisfied, err := q.AreJobDependenciesSatisfied(ctx, run)
	require.NoError(t, err)
	require.True(t, satisfied)

}
