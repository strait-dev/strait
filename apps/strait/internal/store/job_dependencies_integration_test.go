//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
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
	if err := q.CreateJobDependency(ctx, dep); err != nil {
		t.Fatalf("CreateJobDependency() error = %v", err)
	}
	if dep.ID == "" {
		t.Fatal("CreateJobDependency() did not set ID")
	}
	if dep.Condition != "completed" {
		t.Fatalf("CreateJobDependency() condition = %q, want completed", dep.Condition)
	}
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
	if err := q.CreateJobDependency(ctx, dep); err == nil {
		t.Fatal("CreateJobDependency(self) expected error, got nil")
	}
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
		if err := q.CreateJobDependency(ctx, dep); err != nil {
			t.Fatalf("CreateJobDependency() error = %v", err)
		}
	}

	deps, err := q.ListJobDependencies(ctx, job.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobDependencies() error = %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("ListJobDependencies() len = %d, want 2", len(deps))
	}
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
	if err := q.CreateJobDependency(ctx, dep); err != nil {
		t.Fatalf("CreateJobDependency() error = %v", err)
	}

	if err := q.DeleteJobDependency(ctx, dep.ID); err != nil {
		t.Fatalf("DeleteJobDependency() error = %v", err)
	}

	deps, err := q.ListJobDependencies(ctx, job.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobDependencies() error = %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("ListJobDependencies() len = %d, want 0", len(deps))
	}
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
		if err := q.CreateJobDependency(ctx, dep); err != nil {
			t.Fatalf("CreateJobDependency() error = %v", err)
		}
	}

	dependents, err := q.ListDependentsByDependencyJob(ctx, depJob.ID)
	if err != nil {
		t.Fatalf("ListDependentsByDependencyJob() error = %v", err)
	}
	if len(dependents) != 2 {
		t.Fatalf("ListDependentsByDependencyJob() len = %d, want 2", len(dependents))
	}

	// Empty case.
	empty, err := q.ListDependentsByDependencyJob(ctx, newID())
	if err != nil {
		t.Fatalf("ListDependentsByDependencyJob(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListDependentsByDependencyJob(empty) len = %d, want 0", len(empty))
	}
}

func TestListWaitingRunsByJobIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-waiting-runs-by-job-ids"
	job := mustCreateJob(t, ctx, q, projectID)

	run := baseRun(job, newID())
	run.Status = domain.StatusWaiting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	runs, err := q.ListWaitingRunsByJobIDs(ctx, []string{job.ID}, 100)
	if err != nil {
		t.Fatalf("ListWaitingRunsByJobIDs() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListWaitingRunsByJobIDs() len = %d, want 1", len(runs))
	}

	// Empty job IDs.
	nilRuns, err := q.ListWaitingRunsByJobIDs(ctx, []string{}, 100)
	if err != nil {
		t.Fatalf("ListWaitingRunsByJobIDs(empty) error = %v", err)
	}
	if nilRuns != nil {
		t.Fatalf("ListWaitingRunsByJobIDs(empty) = %v, want nil", nilRuns)
	}
}

func TestAreJobDependenciesSatisfied_NoDeps(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-deps-satisfied-no-deps")
	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	satisfied, err := q.AreJobDependenciesSatisfied(ctx, run)
	if err != nil {
		t.Fatalf("AreJobDependenciesSatisfied() error = %v", err)
	}
	if !satisfied {
		t.Fatal("AreJobDependenciesSatisfied() = false, want true (no deps)")
	}
}
