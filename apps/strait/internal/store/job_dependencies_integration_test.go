//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
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

// mustCreateTerminalDepRun inserts a run for depJob in the given terminal status,
// optionally tagged with an idempotency key, to back dependency-satisfaction tests.
func mustCreateTerminalDepRun(t *testing.T, ctx context.Context, q *store.Queries, depJob *domain.Job, status domain.RunStatus, idempotencyKey string) {
	t.Helper()
	run := baseRun(depJob, newID())
	run.Status = status
	run.IdempotencyKey = idempotencyKey
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun(dep %s) error = %v", status, err)
	}
}

func mustAddDependency(t *testing.T, ctx context.Context, q *store.Queries, job, depJob *domain.Job, condition string) {
	t.Helper()
	dep := &domain.JobDependency{JobID: job.ID, DependsOnJobID: depJob.ID, Condition: condition}
	if err := q.CreateJobDependency(ctx, dep); err != nil {
		t.Fatalf("CreateJobDependency() error = %v", err)
	}
}

func TestAreJobDependenciesSatisfied_Conditions(t *testing.T) {
	tests := []struct {
		name       string
		condition  string
		depStatus  domain.RunStatus
		wantOK     bool
		noTerminal bool // create no terminal run for the dependency
	}{
		{name: "completed satisfied", condition: "completed", depStatus: domain.StatusCompleted, wantOK: true},
		{name: "completed but failed", condition: "completed", depStatus: domain.StatusFailed, wantOK: false},
		{name: "failed satisfied", condition: "failed", depStatus: domain.StatusFailed, wantOK: true},
		{name: "failed but completed", condition: "failed", depStatus: domain.StatusCompleted, wantOK: false},
		{name: "any with canceled", condition: "any", depStatus: domain.StatusCanceled, wantOK: true},
		{name: "no terminal run", condition: "completed", noTerminal: true, wantOK: false},
	}

	ctx := context.Background()
	q := mustStore(t)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mustClean(t, ctx)
			projectID := "project-deps-cond"
			job := mustCreateJob(t, ctx, q, projectID)
			depJob := mustCreateJob(t, ctx, q, projectID)
			mustAddDependency(t, ctx, q, job, depJob, tc.condition)

			if !tc.noTerminal {
				mustCreateTerminalDepRun(t, ctx, q, depJob, tc.depStatus, "")
			}

			run := baseRun(job, newID())
			if err := q.CreateRun(ctx, run); err != nil {
				t.Fatalf("CreateRun() error = %v", err)
			}

			got, err := q.AreJobDependenciesSatisfied(ctx, run)
			if err != nil {
				t.Fatalf("AreJobDependenciesSatisfied() error = %v", err)
			}
			if got != tc.wantOK {
				t.Fatalf("AreJobDependenciesSatisfied() = %v, want %v", got, tc.wantOK)
			}
		})
	}
}

func TestAreJobDependenciesSatisfied_MultipleDeps(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-deps-multi"
	job := mustCreateJob(t, ctx, q, projectID)
	depA := mustCreateJob(t, ctx, q, projectID)
	depB := mustCreateJob(t, ctx, q, projectID)
	mustAddDependency(t, ctx, q, job, depA, "completed")
	mustAddDependency(t, ctx, q, job, depB, "completed")

	// Only depA has completed; depB has no terminal run yet.
	mustCreateTerminalDepRun(t, ctx, q, depA, domain.StatusCompleted, "")

	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	got, err := q.AreJobDependenciesSatisfied(ctx, run)
	if err != nil {
		t.Fatalf("AreJobDependenciesSatisfied() error = %v", err)
	}
	if got {
		t.Fatal("AreJobDependenciesSatisfied() = true, want false (depB unsatisfied)")
	}

	// Now satisfy depB.
	mustCreateTerminalDepRun(t, ctx, q, depB, domain.StatusCompleted, "")
	got, err = q.AreJobDependenciesSatisfied(ctx, run)
	if err != nil {
		t.Fatalf("AreJobDependenciesSatisfied() error = %v", err)
	}
	if !got {
		t.Fatal("AreJobDependenciesSatisfied() = false, want true (both satisfied)")
	}
}

func TestAreJobDependenciesSatisfied_IdempotencyScoped(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-deps-idem"
	job := mustCreateJob(t, ctx, q, projectID)
	depJob := mustCreateJob(t, ctx, q, projectID)
	mustAddDependency(t, ctx, q, job, depJob, "completed")

	// Two terminal dependency runs under different keys: K1 completed, K2 failed.
	mustCreateTerminalDepRun(t, ctx, q, depJob, domain.StatusCompleted, "K1")
	mustCreateTerminalDepRun(t, ctx, q, depJob, domain.StatusFailed, "K2")

	// A run scoped to K1 sees only the completed run -> satisfied.
	runK1 := baseRun(job, newID())
	runK1.IdempotencyKey = "K1"
	if err := q.CreateRun(ctx, runK1); err != nil {
		t.Fatalf("CreateRun(K1) error = %v", err)
	}
	got, err := q.AreJobDependenciesSatisfied(ctx, runK1)
	if err != nil {
		t.Fatalf("AreJobDependenciesSatisfied(K1) error = %v", err)
	}
	if !got {
		t.Fatal("AreJobDependenciesSatisfied(K1) = false, want true")
	}

	// A run scoped to K2 sees only the failed run -> not satisfied for "completed".
	runK2 := baseRun(job, newID())
	runK2.IdempotencyKey = "K2"
	if err := q.CreateRun(ctx, runK2); err != nil {
		t.Fatalf("CreateRun(K2) error = %v", err)
	}
	got, err = q.AreJobDependenciesSatisfied(ctx, runK2)
	if err != nil {
		t.Fatalf("AreJobDependenciesSatisfied(K2) error = %v", err)
	}
	if got {
		t.Fatal("AreJobDependenciesSatisfied(K2) = true, want false")
	}
}

func TestAreJobDependenciesSatisfied_UnknownCondition(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-deps-unknown"
	job := mustCreateJob(t, ctx, q, projectID)
	depJob := mustCreateJob(t, ctx, q, projectID)
	mustAddDependency(t, ctx, q, job, depJob, "bogus")
	mustCreateTerminalDepRun(t, ctx, q, depJob, domain.StatusCompleted, "")

	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if _, err := q.AreJobDependenciesSatisfied(ctx, run); err == nil {
		t.Fatal("AreJobDependenciesSatisfied() expected error for unknown condition, got nil")
	}
}
