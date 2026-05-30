//go:build integration

package store_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
)

// mkSingletonRun creates a job run carrying singleton_key in the requested
// status. Phase 3 release/promote/reaper tests need waiters parked in 'waiting'
// and holders in assorted states, so the helper keeps the setup terse.
func mkSingletonRun(t *testing.T, ctx context.Context, q *store.Queries, job *domain.Job, status domain.RunStatus, key string) *domain.JobRun {
	t.Helper()
	st := status
	run := testutil.BuildRun(job, &testutil.RunOpts{Status: &st})
	run.SingletonKey = key
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun(%s) error = %v", status, err)
	}
	return run
}

func acquireFor(t *testing.T, ctx context.Context, q *store.Queries, job *domain.Job, key, holderRunID string, lease *time.Time) {
	t.Helper()
	acquired, _, err := q.AcquireSingletonLock(ctx, domain.SingletonLock{
		ProjectID:   job.ProjectID,
		Kind:        domain.SingletonKindJob,
		OwnerID:     job.ID,
		LockKey:     key,
		HolderRunID: holderRunID,
		LeaseUntil:  lease,
	})
	if err != nil {
		t.Fatalf("AcquireSingletonLock() error = %v", err)
	}
	if !acquired {
		t.Fatalf("expected to acquire lock for holder %s", holderRunID)
	}
}

// TestReleaseSingletonJobLockAndPromote_PromotesOldestWaiter is the core
// release/promote contract: the lock moves to the oldest parked waiter, which is
// transitioned waiting -> queued, while later waiters keep waiting.
func TestReleaseSingletonJobLockAndPromote_PromotesOldestWaiter(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	const key = "acct-1"

	holder := mkSingletonRun(t, ctx, q, job, domain.StatusExecuting, key)
	lease := time.Now().Add(time.Minute)
	acquireFor(t, ctx, q, job, key, holder.ID, &lease)

	first := mkSingletonRun(t, ctx, q, job, domain.StatusWaiting, key)
	// Force a strictly later created_at so FIFO ordering is unambiguous.
	time.Sleep(5 * time.Millisecond)
	second := mkSingletonRun(t, ctx, q, job, domain.StatusWaiting, key)

	released, promotedRunID, err := q.ReleaseSingletonJobLockAndPromote(ctx, holder.ID)
	if err != nil {
		t.Fatalf("ReleaseSingletonJobLockAndPromote() error = %v", err)
	}
	if !released {
		t.Fatal("expected released = true")
	}
	if promotedRunID != first.ID {
		t.Fatalf("promoted %q, want oldest waiter %q", promotedRunID, first.ID)
	}

	holderRow, err := q.GetSingletonHolder(ctx, projectID, domain.SingletonKindJob, job.ID, key)
	if err != nil {
		t.Fatalf("GetSingletonHolder() error = %v", err)
	}
	if holderRow.HolderRunID != first.ID {
		t.Fatalf("lock holder = %q, want promoted waiter %q", holderRow.HolderRunID, first.ID)
	}
	// The promoted waiter is queued, not yet executing: it must carry a NULL
	// lease, stamped only by its first heartbeat once it runs. A non-NULL lease
	// here would let the reaper reclaim the key from a queued holder before it
	// starts (the double-execution bug this guards against).
	if holderRow.LeaseUntil != nil {
		t.Errorf("expected promoted job holder to carry a NULL lease, got %v", holderRow.LeaseUntil)
	}

	if st, _ := q.GetRunStatus(ctx, first.ID); st != domain.StatusQueued {
		t.Errorf("promoted waiter status = %q, want queued", st)
	}
	if st, _ := q.GetRunStatus(ctx, second.ID); st != domain.StatusWaiting {
		t.Errorf("later waiter status = %q, want still waiting", st)
	}
}

// mkSingletonWaiterPriority parks a 'waiting' run with an explicit priority so
// the priority-ordered promotion contract can be asserted.
func mkSingletonWaiterPriority(t *testing.T, ctx context.Context, q *store.Queries, job *domain.Job, key string, priority int) *domain.JobRun {
	t.Helper()
	st := domain.StatusWaiting
	run := testutil.BuildRun(job, &testutil.RunOpts{Status: &st})
	run.SingletonKey = key
	run.Priority = priority
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun(waiter priority %d) error = %v", priority, err)
	}
	return run
}

// TestReleaseSingletonJobLockAndPromote_PromotesByPriority verifies waiters are
// promoted highest-priority first, with created_at breaking ties, rather than
// strict FIFO. The oldest waiter (lowest priority here) is promoted last.
func TestReleaseSingletonJobLockAndPromote_PromotesByPriority(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	const key = "acct-prio"

	holder := mkSingletonRun(t, ctx, q, job, domain.StatusExecuting, key)
	lease := time.Now().Add(time.Minute)
	acquireFor(t, ctx, q, job, key, holder.ID, &lease)

	// Parked oldest-to-newest, priorities out of order: low(1), high(5), mid(3).
	low := mkSingletonWaiterPriority(t, ctx, q, job, key, 1)
	time.Sleep(5 * time.Millisecond)
	high := mkSingletonWaiterPriority(t, ctx, q, job, key, 5)
	time.Sleep(5 * time.Millisecond)
	mid := mkSingletonWaiterPriority(t, ctx, q, job, key, 3)

	// First release promotes the highest priority (5), not the oldest.
	_, promoted, err := q.ReleaseSingletonJobLockAndPromote(ctx, holder.ID)
	if err != nil {
		t.Fatalf("release/promote error = %v", err)
	}
	if promoted != high.ID {
		t.Fatalf("first promotion = %q, want highest priority %q", promoted, high.ID)
	}

	// Then the mid priority (3) over the low priority (1).
	_, promoted, err = q.ReleaseSingletonJobLockAndPromote(ctx, high.ID)
	if err != nil {
		t.Fatalf("release/promote error = %v", err)
	}
	if promoted != mid.ID {
		t.Fatalf("second promotion = %q, want mid priority %q", promoted, mid.ID)
	}

	// Finally the lowest priority (1), even though it was the oldest waiter.
	_, promoted, err = q.ReleaseSingletonJobLockAndPromote(ctx, mid.ID)
	if err != nil {
		t.Fatalf("release/promote error = %v", err)
	}
	if promoted != low.ID {
		t.Fatalf("third promotion = %q, want lowest priority %q", promoted, low.ID)
	}
}

// TestReleaseSingletonJobLockAndPromote_NoWaiterFreesKey: releasing a holder
// with no parked waiters frees the key entirely.
func TestReleaseSingletonJobLockAndPromote_NoWaiterFreesKey(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	const key = "lonely"

	holder := mkSingletonRun(t, ctx, q, job, domain.StatusExecuting, key)
	lease := time.Now().Add(time.Minute)
	acquireFor(t, ctx, q, job, key, holder.ID, &lease)

	released, promotedRunID, err := q.ReleaseSingletonJobLockAndPromote(ctx, holder.ID)
	if err != nil {
		t.Fatalf("ReleaseSingletonJobLockAndPromote() error = %v", err)
	}
	if !released {
		t.Fatal("expected released = true")
	}
	if promotedRunID != "" {
		t.Fatalf("expected no promotion, got %q", promotedRunID)
	}
	if _, err := q.GetSingletonHolder(ctx, projectID, domain.SingletonKindJob, job.ID, key); err != store.ErrSingletonLockNotFound {
		t.Fatalf("expected key freed, got %v", err)
	}
}

// TestReleaseSingletonJobLockAndPromote_Idempotent: a second release for the same
// holder is a no-op (the lock row is already gone).
func TestReleaseSingletonJobLockAndPromote_Idempotent(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	const key = "once"

	holder := mkSingletonRun(t, ctx, q, job, domain.StatusExecuting, key)
	acquireFor(t, ctx, q, job, key, holder.ID, nil)

	if released, _, err := q.ReleaseSingletonJobLockAndPromote(ctx, holder.ID); err != nil || !released {
		t.Fatalf("first release: released=%v err=%v", released, err)
	}
	released, promotedRunID, err := q.ReleaseSingletonJobLockAndPromote(ctx, holder.ID)
	if err != nil {
		t.Fatalf("second release error = %v", err)
	}
	if released {
		t.Fatal("expected second release to be a no-op")
	}
	if promotedRunID != "" {
		t.Fatalf("expected no promotion on no-op release, got %q", promotedRunID)
	}
}

// TestReleaseSingletonJobLockAndPromote_ConcurrentSinglePromote is the
// adversarial guarantee that the executor fast-path and the reaper firing for the
// same holder cannot double-release or double-promote: exactly one caller wins.
func TestReleaseSingletonJobLockAndPromote_ConcurrentSinglePromote(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	const key = "hot"

	holder := mkSingletonRun(t, ctx, q, job, domain.StatusExecuting, key)
	lease := time.Now().Add(time.Minute)
	acquireFor(t, ctx, q, job, key, holder.ID, &lease)
	waiter := mkSingletonRun(t, ctx, q, job, domain.StatusWaiting, key)

	const n = 12
	var releases, promotes int64
	var wg conc.WaitGroup
	for range n {
		wg.Go(func() {
			cq := store.New(testDB.Pool)
			released, promotedRunID, err := cq.ReleaseSingletonJobLockAndPromote(ctx, holder.ID)
			if err != nil {
				return
			}
			if released {
				atomic.AddInt64(&releases, 1)
			}
			if promotedRunID == waiter.ID {
				atomic.AddInt64(&promotes, 1)
			}
		})
	}
	wg.Wait()

	if releases != 1 {
		t.Fatalf("expected exactly 1 release, got %d", releases)
	}
	if promotes != 1 {
		t.Fatalf("expected exactly 1 promotion, got %d", promotes)
	}
	holderRow, err := q.GetSingletonHolder(ctx, projectID, domain.SingletonKindJob, job.ID, key)
	if err != nil {
		t.Fatalf("GetSingletonHolder() error = %v", err)
	}
	if holderRow.HolderRunID != waiter.ID {
		t.Fatalf("final holder = %q, want %q", holderRow.HolderRunID, waiter.ID)
	}
	if st, _ := q.GetRunStatus(ctx, waiter.ID); st != domain.StatusQueued {
		t.Errorf("waiter status = %q, want queued", st)
	}
}

// TestListReapableSingletonJobHolders verifies the reaper's selection: terminal,
// missing, and crashed-with-expired-lease holders are reapable; healthy
// executing holders and not-yet-started (queued/waiting) holders are not.
func TestListReapableSingletonJobHolders(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	// reapable: holder run reached a terminal state.
	terminal := mkSingletonRun(t, ctx, q, job, domain.StatusCompleted, "k-terminal")
	acquireFor(t, ctx, q, job, "k-terminal", terminal.ID, &future)

	// reapable: lock points at a run row that no longer exists.
	missingID := uuid.Must(uuid.NewV7()).String()
	acquireFor(t, ctx, q, job, "k-missing", missingID, &future)

	// reapable: executing holder whose lease has expired (crash).
	crashed := mkSingletonRun(t, ctx, q, job, domain.StatusExecuting, "k-crashed")
	acquireFor(t, ctx, q, job, "k-crashed", crashed.ID, &past)

	// not reapable: executing holder with a live lease.
	healthy := mkSingletonRun(t, ctx, q, job, domain.StatusExecuting, "k-healthy")
	acquireFor(t, ctx, q, job, "k-healthy", healthy.ID, &future)

	// not reapable: queued holder that has not started, even past its lease.
	queued := mkSingletonRun(t, ctx, q, job, domain.StatusQueued, "k-queued")
	acquireFor(t, ctx, q, job, "k-queued", queued.ID, &past)

	// not reapable: executing holder that has not heartbeated yet (NULL lease).
	// This is the window between trigger-time acquire and the first heartbeat.
	// The reaper must never reclaim it on the strength of a NULL lease, or it
	// would promote a waiter while the live holder is about to run -> double
	// execution. The run-status stale checks (ListStaleRuns / ListStaleDequeued)
	// are the safety net here, not the lease.
	preHeartbeat := mkSingletonRun(t, ctx, q, job, domain.StatusExecuting, "k-preheartbeat")
	acquireFor(t, ctx, q, job, "k-preheartbeat", preHeartbeat.ID, nil)

	holders, err := q.ListReapableSingletonJobHolders(ctx, 0)
	if err != nil {
		t.Fatalf("ListReapableSingletonJobHolders() error = %v", err)
	}
	got := map[string]struct{}{}
	for _, h := range holders {
		got[h] = struct{}{}
	}

	for _, want := range []string{terminal.ID, missingID, crashed.ID} {
		if _, ok := got[want]; !ok {
			t.Errorf("expected holder %q to be reapable", want)
		}
	}
	for _, notWant := range []string{healthy.ID, queued.ID, preHeartbeat.ID} {
		if _, ok := got[notWant]; ok {
			t.Errorf("holder %q should not be reapable", notWant)
		}
	}
}

// TestListReapableSingletonJobHolders_BoundedAndOldestFirst verifies the Phase C
// batch bound: with more reapable holders than the limit, the query returns at
// most limit rows, oldest acquisition first, and the reaper drains the rest on a
// follow-up call. This keeps a large reclaim backlog from loading into memory in
// one cycle.
func TestListReapableSingletonJobHolders_BoundedAndOldestFirst(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})

	// Six terminal (hence reapable) holders, each on its own key, acquired in a
	// known order so the oldest-first guarantee is checkable.
	const total = 6
	ordered := make([]string, 0, total)
	for i := range total {
		key := "k-" + uuid.Must(uuid.NewV7()).String()
		run := mkSingletonRun(t, ctx, q, job, domain.StatusCompleted, key)
		acquireFor(t, ctx, q, job, key, run.ID, nil)
		ordered = append(ordered, run.ID)
		// Space out acquired_at so ORDER BY acquired_at ASC is unambiguous.
		if _, err := testDB.Pool.Exec(ctx,
			`UPDATE singleton_locks SET acquired_at = $2 WHERE holder_run_id = $1`,
			run.ID, time.Now().Add(time.Duration(i)*time.Second),
		); err != nil {
			t.Fatalf("stamp acquired_at: %v", err)
		}
	}

	// First bounded page: oldest `limit` holders only.
	const limit = 4
	page1, err := q.ListReapableSingletonJobHolders(ctx, limit)
	if err != nil {
		t.Fatalf("ListReapableSingletonJobHolders(limit) error = %v", err)
	}
	if len(page1) != limit {
		t.Fatalf("page1 size = %d, want %d", len(page1), limit)
	}
	for i := range limit {
		if page1[i] != ordered[i] {
			t.Fatalf("page1[%d] = %q, want oldest %q", i, page1[i], ordered[i])
		}
	}

	// Reap (release) the first page, then the next call surfaces the remainder.
	for _, holderRunID := range page1 {
		if _, _, rerr := q.ReleaseSingletonJobLockAndPromote(ctx, holderRunID); rerr != nil {
			t.Fatalf("release %q error = %v", holderRunID, rerr)
		}
	}
	page2, err := q.ListReapableSingletonJobHolders(ctx, limit)
	if err != nil {
		t.Fatalf("ListReapableSingletonJobHolders(page2) error = %v", err)
	}
	if len(page2) != total-limit {
		t.Fatalf("page2 size = %d, want %d", len(page2), total-limit)
	}
	if page2[0] != ordered[limit] {
		t.Fatalf("page2[0] = %q, want %q", page2[0], ordered[limit])
	}
}

// TestSingletonJobLeaseSetByFirstHeartbeat verifies the core deferred-lease
// behavior: a job lock acquired with a NULL lease (as the trigger path now does)
// has its lease stamped by the first heartbeat once the holder starts executing.
// Before the heartbeat the lease is NULL (reaper-safe via status checks); after,
// it is a concrete future window the reaper honors.
func TestSingletonJobLeaseSetByFirstHeartbeat(t *testing.T) {
	ctx := context.Background()
	stClean(t, ctx)

	q := store.New(testDB.Pool)
	q.SetSingletonLeaseTTL(time.Minute)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	const key = "fresh"

	// Acquire exactly as the trigger path does now: NULL lease.
	holder := mkSingletonRun(t, ctx, q, job, domain.StatusExecuting, key)
	acquireFor(t, ctx, q, job, key, holder.ID, nil)

	before, err := q.GetSingletonHolder(ctx, projectID, domain.SingletonKindJob, job.ID, key)
	if err != nil {
		t.Fatalf("GetSingletonHolder() before heartbeat error = %v", err)
	}
	if before.LeaseUntil != nil {
		t.Fatalf("fresh job holder lease = %v, want NULL until first heartbeat", before.LeaseUntil)
	}

	if err := q.BatchUpdateHeartbeat(ctx, []string{holder.ID}); err != nil {
		t.Fatalf("BatchUpdateHeartbeat() error = %v", err)
	}

	after, err := q.GetSingletonHolder(ctx, projectID, domain.SingletonKindJob, job.ID, key)
	if err != nil {
		t.Fatalf("GetSingletonHolder() after heartbeat error = %v", err)
	}
	if after.LeaseUntil == nil || !after.LeaseUntil.After(time.Now()) {
		t.Fatalf("expected first heartbeat to stamp a future lease, got %v", after.LeaseUntil)
	}
}

// TestSingletonLeaseExtensionViaHeartbeat verifies BatchUpdateHeartbeat bumps the
// lease_until of job singleton holders so long-running runs are never reclaimed.
func TestSingletonLeaseExtensionViaHeartbeat(t *testing.T) {
	ctx := context.Background()
	stClean(t, ctx)

	q := store.New(testDB.Pool)
	q.SetSingletonLeaseTTL(time.Minute)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	const key = "leased"

	holder := mkSingletonRun(t, ctx, q, job, domain.StatusExecuting, key)
	stale := time.Now().Add(-time.Hour)
	acquireFor(t, ctx, q, job, key, holder.ID, &stale)

	if err := q.BatchUpdateHeartbeat(ctx, []string{holder.ID}); err != nil {
		t.Fatalf("BatchUpdateHeartbeat() error = %v", err)
	}

	holderRow, err := q.GetSingletonHolder(ctx, projectID, domain.SingletonKindJob, job.ID, key)
	if err != nil {
		t.Fatalf("GetSingletonHolder() error = %v", err)
	}
	if holderRow.LeaseUntil == nil || !holderRow.LeaseUntil.After(time.Now()) {
		t.Fatalf("expected lease extended into the future, got %v", holderRow.LeaseUntil)
	}
}

// TestSingletonLeaseExtensionSkipsWorkflowHolders ensures the heartbeat lease
// bump never touches workflow holders (lease_until IS NULL), so durable-wait
// workflows are not reclaimed.
func TestSingletonLeaseExtensionSkipsWorkflowHolders(t *testing.T) {
	ctx := context.Background()
	stClean(t, ctx)

	q := store.New(testDB.Pool)
	q.SetSingletonLeaseTTL(time.Minute)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()

	// A workflow-kind holder with a NULL lease (durable-wait workflows never lease).
	wfHolderRun := uuid.Must(uuid.NewV7()).String()
	if _, _, err := q.AcquireSingletonLock(ctx, domain.SingletonLock{
		ProjectID:   projectID,
		Kind:        domain.SingletonKindWorkflow,
		OwnerID:     "wf-" + uuid.Must(uuid.NewV7()).String(),
		LockKey:     "wf-key",
		HolderRunID: wfHolderRun,
		LeaseUntil:  nil,
	}); err != nil {
		t.Fatalf("AcquireSingletonLock(workflow) error = %v", err)
	}

	if err := q.BatchUpdateHeartbeat(ctx, []string{wfHolderRun}); err != nil {
		t.Fatalf("BatchUpdateHeartbeat() error = %v", err)
	}

	var leaseUntil *time.Time
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT lease_until FROM singleton_locks WHERE holder_run_id = $1`, wfHolderRun,
	).Scan(&leaseUntil); err != nil {
		t.Fatalf("scan lease_until: %v", err)
	}
	if leaseUntil != nil {
		t.Fatalf("workflow holder lease should remain NULL, got %v", leaseUntil)
	}
}
