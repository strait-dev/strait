//go:build integration

package store_test

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
)

// wfSingletonInputs builds the (run, stepRuns) pair the engine would hand to
// CreateWorkflowRunSingletonBootstrap for a single-root workflow. run.SingletonKey
// is set so the parked/queued lookups (CountSingletonWaiters, release) resolve.
func wfSingletonInputs(t *testing.T, wf *domain.Workflow, step *domain.WorkflowStep, key string) (*domain.WorkflowRun, []domain.WorkflowStepRun) {
	t.Helper()
	run := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: &wf.ProjectID})
	run.SingletonKey = key
	run.WorkflowVersion = wf.Version
	sr := domain.WorkflowStepRun{
		ID:             uuid.Must(uuid.NewV7()).String(),
		WorkflowRunID:  run.ID,
		WorkflowStepID: step.ID,
		StepRef:        step.StepRef,
		Status:         domain.StepPending,
	}
	return run, []domain.WorkflowStepRun{sr}
}

func wfHolderRow(t *testing.T, ctx context.Context, q *store.Queries, wf *domain.Workflow, key string) *domain.SingletonLock {
	t.Helper()
	row, err := q.GetSingletonHolder(ctx, wf.ProjectID, domain.SingletonKindWorkflow, wf.ID, key)
	if err != nil {
		t.Fatalf("GetSingletonHolder() error = %v", err)
	}
	return row
}

// TestCreateWorkflowRunSingletonBootstrap_Dispatched: an uncontended key is
// claimed, the run is bootstrapped to running, its step runs exist, and the lock
// points at the run with a NULL lease (workflow holders never lease).
func TestCreateWorkflowRunSingletonBootstrap_Dispatched(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	const key = "acct-1"

	run, srs := wfSingletonInputs(t, wf, step, key)
	outcome, holder, created, err := q.CreateWorkflowRunSingletonBootstrap(ctx, run, srs, time.Now(), key, domain.SingletonOnConflictDrop, nil, false)
	if err != nil {
		t.Fatalf("CreateWorkflowRunSingletonBootstrap() error = %v", err)
	}
	if outcome != domain.SingletonOutcomeDispatched {
		t.Fatalf("outcome = %q, want dispatched", outcome)
	}
	if holder != "" {
		t.Fatalf("holder = %q, want empty on acquire", holder)
	}
	if !created {
		t.Fatal("expected runCreated = true")
	}

	got, err := q.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if got.Status != domain.WfStatusRunning {
		t.Fatalf("run status = %q, want running", got.Status)
	}

	row := wfHolderRow(t, ctx, q, wf, key)
	if row.HolderRunID != run.ID {
		t.Fatalf("lock holder = %q, want %q", row.HolderRunID, run.ID)
	}
	if row.LeaseUntil != nil {
		t.Fatalf("workflow holder lease = %v, want NULL", row.LeaseUntil)
	}

	srList, err := q.ListStepRunsByWorkflowRun(ctx, run.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListStepRunsByWorkflowRun() error = %v", err)
	}
	if len(srList) != 1 {
		t.Fatalf("step runs = %d, want 1", len(srList))
	}
}

// TestCreateWorkflowRunSingletonBootstrap_Drop: a contended key with the drop
// policy creates no run and reports the holder it lost to.
func TestCreateWorkflowRunSingletonBootstrap_Drop(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	const key = "k-drop"

	holderRun, srs := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, holderRun, srs, time.Now(), key, domain.SingletonOnConflictDrop, nil, false); err != nil {
		t.Fatalf("bootstrap holder error = %v", err)
	}

	newcomer, srs2 := wfSingletonInputs(t, wf, step, key)
	outcome, holder, created, err := q.CreateWorkflowRunSingletonBootstrap(ctx, newcomer, srs2, time.Now(), key, domain.SingletonOnConflictDrop, nil, false)
	if err != nil {
		t.Fatalf("bootstrap newcomer error = %v", err)
	}
	if outcome != domain.SingletonOutcomeDropped {
		t.Fatalf("outcome = %q, want dropped", outcome)
	}
	if holder != holderRun.ID {
		t.Fatalf("holder = %q, want %q", holder, holderRun.ID)
	}
	if created {
		t.Fatal("expected runCreated = false on drop")
	}
	if _, err := q.GetWorkflowRun(ctx, newcomer.ID); err != store.ErrWorkflowRunNotFound {
		t.Fatalf("dropped run should not exist, got err = %v", err)
	}
}

// TestCreateWorkflowRunSingletonBootstrap_QueueParks: the queue policy parks the
// newcomer as 'queued' with its step runs created but not started, leaving the
// original holder's lock untouched.
func TestCreateWorkflowRunSingletonBootstrap_QueueParks(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	const key = "k-queue"

	holderRun, srs := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, holderRun, srs, time.Now(), key, domain.SingletonOnConflictQueue, nil, false); err != nil {
		t.Fatalf("bootstrap holder error = %v", err)
	}

	waiter, srs2 := wfSingletonInputs(t, wf, step, key)
	outcome, holder, created, err := q.CreateWorkflowRunSingletonBootstrap(ctx, waiter, srs2, time.Now(), key, domain.SingletonOnConflictQueue, nil, false)
	if err != nil {
		t.Fatalf("bootstrap waiter error = %v", err)
	}
	if outcome != domain.SingletonOutcomeQueuedBehind {
		t.Fatalf("outcome = %q, want queued_behind", outcome)
	}
	if holder != holderRun.ID {
		t.Fatalf("holder = %q, want %q", holder, holderRun.ID)
	}
	if !created {
		t.Fatal("expected runCreated = true on queue")
	}

	got, err := q.GetWorkflowRun(ctx, waiter.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun(waiter) error = %v", err)
	}
	if got.Status != domain.WfStatusQueued {
		t.Fatalf("waiter status = %q, want queued", got.Status)
	}
	n, err := q.CountSingletonWaiters(ctx, domain.SingletonKindWorkflow, wf.ID, key)
	if err != nil {
		t.Fatalf("CountSingletonWaiters() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("waiters = %d, want 1", n)
	}
	// Lock is still the original holder.
	if row := wfHolderRow(t, ctx, q, wf, key); row.HolderRunID != holderRun.ID {
		t.Fatalf("lock holder = %q, want unchanged %q", row.HolderRunID, holderRun.ID)
	}
}

// TestCreateWorkflowRunSingletonBootstrap_QueueOverflow: at the depth cap, an
// extra queued arrival is dropped (0 billable runs) rather than parked.
func TestCreateWorkflowRunSingletonBootstrap_QueueOverflow(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	const key = "k-cap"
	cap1 := 1

	holderRun, srs := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, holderRun, srs, time.Now(), key, domain.SingletonOnConflictQueue, &cap1, false); err != nil {
		t.Fatalf("bootstrap holder error = %v", err)
	}

	w1, s1 := wfSingletonInputs(t, wf, step, key)
	if outcome, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, w1, s1, time.Now(), key, domain.SingletonOnConflictQueue, &cap1, false); err != nil || outcome != domain.SingletonOutcomeQueuedBehind {
		t.Fatalf("first waiter outcome = %q err = %v, want queued_behind", outcome, err)
	}

	w2, s2 := wfSingletonInputs(t, wf, step, key)
	outcome, _, created, err := q.CreateWorkflowRunSingletonBootstrap(ctx, w2, s2, time.Now(), key, domain.SingletonOnConflictQueue, &cap1, false)
	if err != nil {
		t.Fatalf("second waiter error = %v", err)
	}
	if outcome != domain.SingletonOutcomeDropped {
		t.Fatalf("over-cap outcome = %q, want dropped", outcome)
	}
	if created {
		t.Fatal("over-cap run should not be created")
	}
	if _, err := q.GetWorkflowRun(ctx, w2.ID); err != store.ErrWorkflowRunNotFound {
		t.Fatalf("over-cap run should not exist, got err = %v", err)
	}
}

// TestCreateWorkflowRunSingletonBootstrap_Replace: the replace policy cancels the
// running holder (cascading to its step runs) and any already-parked waiter, then
// parks the newcomer as the sole successor.
func TestCreateWorkflowRunSingletonBootstrap_Replace(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	const key = "k-replace"

	holderRun, srs := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, holderRun, srs, time.Now(), key, domain.SingletonOnConflictReplace, nil, false); err != nil {
		t.Fatalf("bootstrap holder error = %v", err)
	}
	// Park a stale waiter that replace must discard.
	staleWaiter, sw := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, staleWaiter, sw, time.Now(), key, domain.SingletonOnConflictQueue, nil, false); err != nil {
		t.Fatalf("park stale waiter error = %v", err)
	}

	newcomer, ns := wfSingletonInputs(t, wf, step, key)
	outcome, holder, created, err := q.CreateWorkflowRunSingletonBootstrap(ctx, newcomer, ns, time.Now(), key, domain.SingletonOnConflictReplace, nil, false)
	if err != nil {
		t.Fatalf("bootstrap newcomer error = %v", err)
	}
	if outcome != domain.SingletonOutcomeReplaced {
		t.Fatalf("outcome = %q, want replaced", outcome)
	}
	if holder != holderRun.ID {
		t.Fatalf("holder = %q, want canceled holder %q", holder, holderRun.ID)
	}
	if !created {
		t.Fatal("expected runCreated = true on replace")
	}

	// Old holder canceled with its step runs cascaded.
	gotHolder, err := q.GetWorkflowRun(ctx, holderRun.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun(holder) error = %v", err)
	}
	if gotHolder.Status != domain.WfStatusCanceled {
		t.Fatalf("holder status = %q, want canceled", gotHolder.Status)
	}
	holderSteps, err := q.ListStepRunsByWorkflowRun(ctx, holderRun.ID, 100, nil)
	if err != nil {
		t.Fatalf("list holder step runs error = %v", err)
	}
	for _, sr := range holderSteps {
		if sr.Status != domain.StepCanceled {
			t.Fatalf("holder step %q status = %q, want canceled", sr.StepRef, sr.Status)
		}
	}

	// Stale waiter canceled.
	gotStale, err := q.GetWorkflowRun(ctx, staleWaiter.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun(staleWaiter) error = %v", err)
	}
	if gotStale.Status != domain.WfStatusCanceled {
		t.Fatalf("stale waiter status = %q, want canceled", gotStale.Status)
	}

	// Newcomer parked as queued.
	gotNew, err := q.GetWorkflowRun(ctx, newcomer.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun(newcomer) error = %v", err)
	}
	if gotNew.Status != domain.WfStatusQueued {
		t.Fatalf("newcomer status = %q, want queued", gotNew.Status)
	}
}

// TestReleaseSingletonWorkflowLockAndPromote_PromotesOldestQueued: releasing the
// holder promotes the oldest queued waiter (queued -> running) and re-points the
// lock; later waiters stay queued.
func TestReleaseSingletonWorkflowLockAndPromote_PromotesOldestQueued(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	const key = "k-promote"

	holderRun, srs := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, holderRun, srs, time.Now(), key, domain.SingletonOnConflictQueue, nil, false); err != nil {
		t.Fatalf("bootstrap holder error = %v", err)
	}
	first, fs := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, first, fs, time.Now(), key, domain.SingletonOnConflictQueue, nil, false); err != nil {
		t.Fatalf("park first waiter error = %v", err)
	}
	time.Sleep(5 * time.Millisecond) // strictly later created_at
	second, ss := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, second, ss, time.Now(), key, domain.SingletonOnConflictQueue, nil, false); err != nil {
		t.Fatalf("park second waiter error = %v", err)
	}

	released, promotedRunID, err := q.ReleaseSingletonWorkflowLockAndPromote(ctx, holderRun.ID)
	if err != nil {
		t.Fatalf("ReleaseSingletonWorkflowLockAndPromote() error = %v", err)
	}
	if !released {
		t.Fatal("expected released = true")
	}
	if promotedRunID != first.ID {
		t.Fatalf("promoted %q, want oldest %q", promotedRunID, first.ID)
	}

	if row := wfHolderRow(t, ctx, q, wf, key); row.HolderRunID != first.ID {
		t.Fatalf("lock holder = %q, want promoted %q", row.HolderRunID, first.ID)
	}
	if got, _ := q.GetWorkflowRun(ctx, first.ID); got.Status != domain.WfStatusRunning {
		t.Fatalf("promoted waiter status = %q, want running", got.Status)
	}
	if got, _ := q.GetWorkflowRun(ctx, second.ID); got.Status != domain.WfStatusQueued {
		t.Fatalf("later waiter status = %q, want still queued", got.Status)
	}
}

// TestReleaseSingletonWorkflowLockAndPromote_PromotesByPriority verifies workflow
// waiters are promoted highest-priority first, with created_at breaking ties.
func TestReleaseSingletonWorkflowLockAndPromote_PromotesByPriority(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	const key = "k-prio"

	parkWaiter := func(priority int) *domain.WorkflowRun {
		t.Helper()
		run, srs := wfSingletonInputs(t, wf, step, key)
		run.Priority = priority
		if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, run, srs, time.Now(), key, domain.SingletonOnConflictQueue, nil, false); err != nil {
			t.Fatalf("bootstrap (priority %d) error = %v", priority, err)
		}
		return run
	}

	holder := parkWaiter(0) // first acquires the key
	low := parkWaiter(1)
	time.Sleep(5 * time.Millisecond)
	high := parkWaiter(5)
	time.Sleep(5 * time.Millisecond)
	mid := parkWaiter(3)

	// Highest priority (5) promoted first, then mid (3), then low (1).
	for _, want := range []*domain.WorkflowRun{high, mid, low} {
		var releasedHolder string
		switch want {
		case high:
			releasedHolder = holder.ID
		case mid:
			releasedHolder = high.ID
		case low:
			releasedHolder = mid.ID
		}
		_, promoted, err := q.ReleaseSingletonWorkflowLockAndPromote(ctx, releasedHolder)
		if err != nil {
			t.Fatalf("release/promote error = %v", err)
		}
		if promoted != want.ID {
			t.Fatalf("promoted %q, want %q (priority %d)", promoted, want.ID, want.Priority)
		}
	}
}

// TestCreateWorkflowRunSingletonBootstrap_Preemption verifies that under the
// queue policy with preemption enabled, a strictly higher-priority newcomer
// cancels the holder and parks; a lower/equal-priority newcomer queues normally.
func TestCreateWorkflowRunSingletonBootstrap_Preemption(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)

	run := func(t *testing.T, holderPriority, newcomerPriority int, preempt bool) (domain.SingletonOutcome, *domain.WorkflowRun, *domain.WorkflowRun) {
		t.Helper()
		stClean(t, ctx)
		projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
		wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
		stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
		step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
		const key = "k-preempt"

		holder, hs := wfSingletonInputs(t, wf, step, key)
		holder.Priority = holderPriority
		if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, holder, hs, time.Now(), key, domain.SingletonOnConflictQueue, nil, false); err != nil {
			t.Fatalf("bootstrap holder error = %v", err)
		}
		newcomer, ns := wfSingletonInputs(t, wf, step, key)
		newcomer.Priority = newcomerPriority
		outcome, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, newcomer, ns, time.Now(), key, domain.SingletonOnConflictQueue, nil, preempt)
		if err != nil {
			t.Fatalf("bootstrap newcomer error = %v", err)
		}
		return outcome, holder, newcomer
	}

	t.Run("higher priority preempts the holder", func(t *testing.T) {
		outcome, holder, _ := run(t, 1, 5, true)
		if outcome != domain.SingletonOutcomeReplaced {
			t.Fatalf("outcome = %q, want replaced (preempted)", outcome)
		}
		if got, _ := q.GetWorkflowRun(ctx, holder.ID); got.Status != domain.WfStatusCanceled {
			t.Fatalf("holder status = %q, want canceled", got.Status)
		}
	})

	t.Run("equal priority does not preempt", func(t *testing.T) {
		outcome, _, _ := run(t, 5, 5, true)
		if outcome != domain.SingletonOutcomeQueuedBehind {
			t.Fatalf("outcome = %q, want queued_behind", outcome)
		}
	})
}

// TestReleaseSingletonWorkflowLockAndPromote_NoWaiterFreesKey: releasing a holder
// with no parked waiters frees the key entirely.
func TestReleaseSingletonWorkflowLockAndPromote_NoWaiterFreesKey(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	const key = "k-lonely"

	holderRun, srs := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, holderRun, srs, time.Now(), key, domain.SingletonOnConflictQueue, nil, false); err != nil {
		t.Fatalf("bootstrap holder error = %v", err)
	}

	released, promotedRunID, err := q.ReleaseSingletonWorkflowLockAndPromote(ctx, holderRun.ID)
	if err != nil {
		t.Fatalf("ReleaseSingletonWorkflowLockAndPromote() error = %v", err)
	}
	if !released {
		t.Fatal("expected released = true")
	}
	if promotedRunID != "" {
		t.Fatalf("expected no promotion, got %q", promotedRunID)
	}
	if _, err := q.GetSingletonHolder(ctx, projectID, domain.SingletonKindWorkflow, wf.ID, key); err != store.ErrSingletonLockNotFound {
		t.Fatalf("expected key freed, got %v", err)
	}
}

// TestReleaseSingletonWorkflowLockAndPromote_Idempotent: a second release for the
// same holder is a no-op.
func TestReleaseSingletonWorkflowLockAndPromote_Idempotent(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	const key = "k-once"

	holderRun, srs := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, holderRun, srs, time.Now(), key, domain.SingletonOnConflictQueue, nil, false); err != nil {
		t.Fatalf("bootstrap holder error = %v", err)
	}

	if released, _, err := q.ReleaseSingletonWorkflowLockAndPromote(ctx, holderRun.ID); err != nil || !released {
		t.Fatalf("first release: released=%v err=%v", released, err)
	}
	released, promotedRunID, err := q.ReleaseSingletonWorkflowLockAndPromote(ctx, holderRun.ID)
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

// TestReleaseSingletonWorkflowLockAndPromote_ConcurrentSinglePromote is the
// adversarial guarantee that the terminal fast-path and the reaper firing for the
// same holder cannot double-release or double-promote: exactly one caller wins.
func TestReleaseSingletonWorkflowLockAndPromote_ConcurrentSinglePromote(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	const key = "k-hot"

	holderRun, srs := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, holderRun, srs, time.Now(), key, domain.SingletonOnConflictQueue, nil, false); err != nil {
		t.Fatalf("bootstrap holder error = %v", err)
	}
	waiter, ws := wfSingletonInputs(t, wf, step, key)
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, waiter, ws, time.Now(), key, domain.SingletonOnConflictQueue, nil, false); err != nil {
		t.Fatalf("park waiter error = %v", err)
	}

	const n = 12
	var releases, promotes int64
	var wg conc.WaitGroup
	for range n {
		wg.Go(func() {
			cq := store.New(testDB.Pool)
			released, promotedRunID, err := cq.ReleaseSingletonWorkflowLockAndPromote(ctx, holderRun.ID)
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
	if row := wfHolderRow(t, ctx, q, wf, key); row.HolderRunID != waiter.ID {
		t.Fatalf("final holder = %q, want %q", row.HolderRunID, waiter.ID)
	}
	if got, _ := q.GetWorkflowRun(ctx, waiter.ID); got.Status != domain.WfStatusRunning {
		t.Fatalf("waiter status = %q, want running", got.Status)
	}
}

// TestListReapableSingletonWorkflowHolders: terminal and missing holders are
// reapable; a running holder is never reclaimed (no lease for workflow holders,
// so durable-wait runs are safe).
func TestListReapableSingletonWorkflowHolders(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})

	// reapable: holder reached a terminal state.
	terminalRun, ts := wfSingletonInputs(t, wf, step, "k-terminal")
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, terminalRun, ts, time.Now(), "k-terminal", domain.SingletonOnConflictQueue, nil, false); err != nil {
		t.Fatalf("bootstrap terminal holder error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, terminalRun.ID, domain.WfStatusRunning, domain.WfStatusCompleted, map[string]any{"finished_at": time.Now()}); err != nil {
		t.Fatalf("mark terminal holder completed error = %v", err)
	}

	// reapable: lock points at a run row that no longer exists.
	missingID := uuid.Must(uuid.NewV7()).String()
	if _, _, err := q.AcquireSingletonLock(ctx, domain.SingletonLock{
		ProjectID:   projectID,
		Kind:        domain.SingletonKindWorkflow,
		OwnerID:     wf.ID,
		LockKey:     "k-missing",
		HolderRunID: missingID,
		LeaseUntil:  nil,
	}); err != nil {
		t.Fatalf("AcquireSingletonLock(missing) error = %v", err)
	}

	// not reapable: a running holder (e.g. a long durable-wait workflow).
	runningRun, rs := wfSingletonInputs(t, wf, step, "k-running")
	if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, runningRun, rs, time.Now(), "k-running", domain.SingletonOnConflictQueue, nil, false); err != nil {
		t.Fatalf("bootstrap running holder error = %v", err)
	}

	holders, err := q.ListReapableSingletonWorkflowHolders(ctx, 0)
	if err != nil {
		t.Fatalf("ListReapableSingletonWorkflowHolders() error = %v", err)
	}
	got := map[string]struct{}{}
	for _, h := range holders {
		got[h] = struct{}{}
	}
	for _, want := range []string{terminalRun.ID, missingID} {
		if _, ok := got[want]; !ok {
			t.Errorf("expected holder %q to be reapable", want)
		}
	}
	if _, ok := got[runningRun.ID]; ok {
		t.Errorf("running holder %q should not be reapable", runningRun.ID)
	}
}

// TestCreateWorkflowRunSingletonBootstrap_ConcurrentAcquire is the core mutual
// exclusion guarantee: N concurrent bootstraps on one key yield exactly one
// dispatched run; the rest are dropped (0 extra runs created).
func TestCreateWorkflowRunSingletonBootstrap_ConcurrentAcquire(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
	const key = "k-race"

	const n = 16
	var dispatched, dropped int64
	var wg conc.WaitGroup
	for range n {
		wg.Go(func() {
			cq := store.New(testDB.Pool)
			run, srs := wfSingletonInputs(t, wf, step, key)
			outcome, _, _, err := cq.CreateWorkflowRunSingletonBootstrap(ctx, run, srs, time.Now(), key, domain.SingletonOnConflictDrop, nil, false)
			if err != nil {
				return
			}
			switch outcome {
			case domain.SingletonOutcomeDispatched:
				atomic.AddInt64(&dispatched, 1)
			case domain.SingletonOutcomeDropped:
				atomic.AddInt64(&dropped, 1)
			}
		})
	}
	wg.Wait()

	if dispatched != 1 {
		t.Fatalf("dispatched = %d, want exactly 1", dispatched)
	}
	if dropped != n-1 {
		t.Fatalf("dropped = %d, want %d", dropped, n-1)
	}
}

// TestCreateWorkflowRunSingletonBootstrap_ConcurrentQueueCap is the Phase B
// regression: with a held key and the queue policy capped at K, N concurrent
// arrivals must park exactly K waiters and drop the rest. Without serializing the
// cap check behind the holder row (FOR UPDATE), two arrivals could both read a
// stale under-cap count and both park, overflowing the depth. The cap is checked
// for several K so an off-by-one in the bound shows up.
func TestCreateWorkflowRunSingletonBootstrap_ConcurrentQueueCap(t *testing.T) {
	for _, cap := range []int{0, 1, 5} {
		t.Run("cap="+strconv.Itoa(cap), func(t *testing.T) {
			ctx := context.Background()
			q := stStore(t)
			stClean(t, ctx)

			projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
			wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})
			stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
			step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: &stepJob.ID})
			const key = "k-cap-race"

			// Seed the holder so every concurrent arrival contends for the queue.
			holderRun, hs := wfSingletonInputs(t, wf, step, key)
			if _, _, _, err := q.CreateWorkflowRunSingletonBootstrap(ctx, holderRun, hs, time.Now(), key, domain.SingletonOnConflictQueue, &cap, false); err != nil {
				t.Fatalf("bootstrap holder error = %v", err)
			}

			const n = 16
			capN := cap
			var queued, dropped int64
			var wg conc.WaitGroup
			for range n {
				wg.Go(func() {
					cq := store.New(testDB.Pool)
					run, srs := wfSingletonInputs(t, wf, step, key)
					outcome, _, _, err := cq.CreateWorkflowRunSingletonBootstrap(ctx, run, srs, time.Now(), key, domain.SingletonOnConflictQueue, &capN, false)
					if err != nil {
						return
					}
					switch outcome {
					case domain.SingletonOutcomeQueuedBehind:
						atomic.AddInt64(&queued, 1)
					case domain.SingletonOutcomeDropped:
						atomic.AddInt64(&dropped, 1)
					}
				})
			}
			wg.Wait()

			if int(queued) != cap {
				t.Fatalf("queued = %d, want exactly cap %d", queued, cap)
			}
			if int(dropped) != n-cap {
				t.Fatalf("dropped = %d, want %d", dropped, n-cap)
			}
			// The authoritative count from the DB must also respect the cap: never
			// more parked waiters than the bound allows.
			parked, err := q.CountSingletonWaiters(ctx, domain.SingletonKindWorkflow, wf.ID, key)
			if err != nil {
				t.Fatalf("CountSingletonWaiters() error = %v", err)
			}
			if parked != cap {
				t.Fatalf("parked waiters = %d, want cap %d", parked, cap)
			}
		})
	}
}
