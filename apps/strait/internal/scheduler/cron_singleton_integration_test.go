//go:build integration

package scheduler

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// cronSingletonJob builds an enabled cron job in a fresh project and returns the
// real store/queue-backed scheduler that drives triggerJobLocked end to end.
func cronSingletonJob(t *testing.T, ctx context.Context, configure func(*domain.Job)) (*CronScheduler, *pgxpool.Pool, *domain.Job) {
	t.Helper()
	tdb := cleanSchedulerIntegrationDB(t, ctx)
	st := store.New(tdb.Pool)
	pq := queue.NewPostgresQueue(tdb.Pool)

	project := &domain.Project{
		ID:    "cron-singleton-" + uuid.Must(uuid.NewV7()).String(),
		OrgID: "org-" + uuid.Must(uuid.NewV7()).String(),
		Name:  "cron singleton",
	}
	if err := st.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := &domain.Job{
		ID:          uuid.Must(uuid.NewV7()).String(),
		ProjectID:   project.ID,
		Name:        "cron singleton job",
		Slug:        "cron-singleton-" + uuid.Must(uuid.NewV7()).String()[:8],
		Cron:        "* * * * *",
		EndpointURL: "https://example.com/cron",
		MaxAttempts: 3,
		TimeoutSecs: 60,
		Enabled:     true,
	}
	configure(job)
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	return NewCronScheduler(ctx, st, pq, nil), tdb.Pool, job
}

// countRunsByStatus returns how many runs the job has in the given status.
func countRunsByStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID string, status domain.RunStatus) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_runs WHERE job_id = $1 AND status = $2`, jobID, string(status),
	).Scan(&n); err != nil {
		t.Fatalf("count runs (%s): %v", status, err)
	}
	return n
}

func countSingletonLocks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID, lockKey string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM singleton_locks WHERE kind = 'job' AND owner_id = $1 AND lock_key = $2`, jobID, lockKey,
	).Scan(&n); err != nil {
		t.Fatalf("count singleton locks: %v", err)
	}
	return n
}

// TestCronScheduler_OverlapSkip_DropsOverlappingFire verifies the unified path:
// a skip-policy cron job becomes a constant-key drop singleton, so a second fire
// while the holder is still active is dropped (no second run) and the lock stays
// with the original holder.
func TestCronScheduler_OverlapSkip_DropsOverlappingFire(t *testing.T) {
	ctx := context.Background()
	cs, pool, job := cronSingletonJob(t, ctx, func(j *domain.Job) {
		j.CronOverlapPolicy = domain.OverlapPolicySkip
	})

	cs.triggerJobLocked(ctx, *job, "fire-1")
	cs.triggerJobLocked(ctx, *job, "fire-2")

	if got := countRunsByStatus(t, ctx, pool, job.ID, domain.StatusQueued); got != 1 {
		t.Fatalf("queued runs = %d, want 1 (second fire dropped)", got)
	}
	if got := countSingletonLocks(t, ctx, pool, job.ID, domain.CronSingletonKey); got != 1 {
		t.Fatalf("singleton locks for cron key = %d, want 1", got)
	}
}

// TestCronScheduler_OverlapCancelRunning_ReplacesHolder verifies cancel_running
// maps to replace: the active holder is canceled and the newcomer is parked to
// take the key.
func TestCronScheduler_OverlapCancelRunning_ReplacesHolder(t *testing.T) {
	ctx := context.Background()
	cs, pool, job := cronSingletonJob(t, ctx, func(j *domain.Job) {
		j.CronOverlapPolicy = domain.OverlapPolicyCancelRunning
	})

	cs.triggerJobLocked(ctx, *job, "fire-1")
	cs.triggerJobLocked(ctx, *job, "fire-2")

	if got := countRunsByStatus(t, ctx, pool, job.ID, domain.StatusCanceled); got != 1 {
		t.Fatalf("canceled runs = %d, want 1 (original holder replaced)", got)
	}
	if got := countRunsByStatus(t, ctx, pool, job.ID, domain.StatusWaiting); got != 1 {
		t.Fatalf("waiting runs = %d, want 1 (newcomer parked for promotion)", got)
	}
}

// TestCronScheduler_ExplicitSingleton_HonoredOnCron verifies that an explicit
// singleton config (queue policy) is enforced on a cron fire — closing the prior
// gap where cron bypassed singleton policy entirely.
func TestCronScheduler_ExplicitSingleton_HonoredOnCron(t *testing.T) {
	ctx := context.Background()
	cs, pool, job := cronSingletonJob(t, ctx, func(j *domain.Job) {
		j.SingletonOnConflict = domain.SingletonOnConflictQueue
		j.SingletonKeyExpr = json.RawMessage(`{"template":"tenant-const"}`)
	})

	cs.triggerJobLocked(ctx, *job, "fire-1")
	cs.triggerJobLocked(ctx, *job, "fire-2")

	if got := countRunsByStatus(t, ctx, pool, job.ID, domain.StatusQueued); got != 1 {
		t.Fatalf("queued runs = %d, want 1 (holder)", got)
	}
	if got := countRunsByStatus(t, ctx, pool, job.ID, domain.StatusWaiting); got != 1 {
		t.Fatalf("waiting runs = %d, want 1 (queued behind holder)", got)
	}
	if got := countSingletonLocks(t, ctx, pool, job.ID, "tenant-const"); got != 1 {
		t.Fatalf("singleton locks for resolved key = %d, want 1", got)
	}
}
