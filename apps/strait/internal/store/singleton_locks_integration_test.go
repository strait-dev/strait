//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
)

// buildSingletonJob constructs a job carrying a full singleton configuration so
// the round-trip and version-snapshot assertions have something to verify.
func buildSingletonJob(projectID string) *domain.Job {
	n := uuid.Must(uuid.NewV7()).String()
	depth := 5
	return &domain.Job{
		ID:                     uuid.Must(uuid.NewV7()).String(),
		ProjectID:              projectID,
		Name:                   "singleton-job-" + n,
		Slug:                   "singleton-job-" + n,
		PayloadSchema:          json.RawMessage(`{"type":"object"}`),
		EndpointURL:            "https://example.com/webhook",
		MaxAttempts:            3,
		TimeoutSecs:            300,
		Enabled:                true,
		SingletonKeyExpr:       json.RawMessage(`{"template":"${account.id}"}`),
		SingletonOnConflict:    domain.SingletonOnConflictQueue,
		SingletonMaxQueueDepth: &depth,
	}
}

func TestSingleton_JobConfigRoundTrip(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	job := buildSingletonJob(projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.SingletonOnConflict != domain.SingletonOnConflictQueue {
		t.Errorf("SingletonOnConflict = %q, want queue", got.SingletonOnConflict)
	}
	if got.SingletonMaxQueueDepth == nil || *got.SingletonMaxQueueDepth != 5 {
		t.Errorf("SingletonMaxQueueDepth = %v, want 5", got.SingletonMaxQueueDepth)
	}
	expr, err := domain.ParseSingletonKeyExpr(got.SingletonKeyExpr)
	if err != nil {
		t.Fatalf("ParseSingletonKeyExpr(roundtrip) error = %v", err)
	}
	if expr.Template != "${account.id}" {
		t.Errorf("template = %q, want ${account.id}", expr.Template)
	}

	// Update bumps the version and snapshots the prior definition into
	// job_versions. The snapshot must carry the singleton columns.
	job.Description = "bump version"
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}
	snapshot, err := q.GetJobAtVersion(ctx, job.ID, 1)
	if err != nil {
		t.Fatalf("GetJobAtVersion(1) error = %v", err)
	}
	if snapshot.SingletonOnConflict != domain.SingletonOnConflictQueue {
		t.Errorf("snapshot SingletonOnConflict = %q, want queue", snapshot.SingletonOnConflict)
	}
	if snapshot.SingletonMaxQueueDepth == nil || *snapshot.SingletonMaxQueueDepth != 5 {
		t.Errorf("snapshot SingletonMaxQueueDepth = %v, want 5", snapshot.SingletonMaxQueueDepth)
	}
}

func TestSingleton_WorkflowConfigRoundTrip(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	depth := 3
	wf := testutil.BuildWorkflow(nil)
	wf.SingletonKeyExpr = json.RawMessage(`{"template":"${tenant}"}`)
	wf.SingletonOnConflict = domain.SingletonOnConflictReplace
	wf.SingletonMaxQueueDepth = &depth
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	got, err := q.GetWorkflow(ctx, wf.ID)
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if got.SingletonOnConflict != domain.SingletonOnConflictReplace {
		t.Errorf("SingletonOnConflict = %q, want replace", got.SingletonOnConflict)
	}
	if got.SingletonMaxQueueDepth == nil || *got.SingletonMaxQueueDepth != 3 {
		t.Errorf("SingletonMaxQueueDepth = %v, want 3", got.SingletonMaxQueueDepth)
	}
	expr, err := domain.ParseSingletonKeyExpr(got.SingletonKeyExpr)
	if err != nil {
		t.Fatalf("ParseSingletonKeyExpr(roundtrip) error = %v", err)
	}
	if expr.Template != "${tenant}" {
		t.Errorf("template = %q, want ${tenant}", expr.Template)
	}
}

func TestAcquireSingletonLock_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	lock := domain.SingletonLock{
		ProjectID:   "proj-1",
		Kind:        domain.SingletonKindJob,
		OwnerID:     "job-1",
		LockKey:     "acct-42",
		HolderRunID: "run-1",
	}
	acquired, held, err := q.AcquireSingletonLock(ctx, lock)
	if err != nil {
		t.Fatalf("AcquireSingletonLock() error = %v", err)
	}
	if !acquired {
		t.Fatal("expected first acquire to succeed")
	}
	if held.AcquiredAt.IsZero() {
		t.Error("expected AcquiredAt to be populated on acquire")
	}

	holder, err := q.GetSingletonHolder(ctx, "proj-1", domain.SingletonKindJob, "job-1", "acct-42")
	if err != nil {
		t.Fatalf("GetSingletonHolder() error = %v", err)
	}
	if holder.HolderRunID != "run-1" {
		t.Errorf("holder run = %q, want run-1", holder.HolderRunID)
	}
}

func TestAcquireSingletonLock_ConflictReturnsHolder(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	base := domain.SingletonLock{
		ProjectID:   "proj-1",
		Kind:        domain.SingletonKindJob,
		OwnerID:     "job-1",
		LockKey:     "acct-42",
		HolderRunID: "run-1",
	}
	if _, _, err := q.AcquireSingletonLock(ctx, base); err != nil {
		t.Fatalf("first AcquireSingletonLock() error = %v", err)
	}

	contender := base
	contender.HolderRunID = "run-2"
	acquired, holder, err := q.AcquireSingletonLock(ctx, contender)
	if err != nil {
		t.Fatalf("second AcquireSingletonLock() error = %v", err)
	}
	if acquired {
		t.Fatal("expected second acquire to lose the race")
	}
	if holder == nil || holder.HolderRunID != "run-1" {
		t.Fatalf("expected existing holder run-1, got %+v", holder)
	}
}

func TestGetSingletonHolder_NotFound(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	_, err := q.GetSingletonHolder(ctx, "proj-1", domain.SingletonKindJob, "job-x", "missing")
	if !errors.Is(err, store.ErrSingletonLockNotFound) {
		t.Fatalf("expected ErrSingletonLockNotFound, got %v", err)
	}
}

func TestReleaseSingletonLock_Idempotent(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	lock := domain.SingletonLock{
		ProjectID:   "proj-1",
		Kind:        domain.SingletonKindJob,
		OwnerID:     "job-1",
		LockKey:     "k",
		HolderRunID: "run-1",
	}
	if _, _, err := q.AcquireSingletonLock(ctx, lock); err != nil {
		t.Fatalf("AcquireSingletonLock() error = %v", err)
	}

	released, err := q.ReleaseSingletonLock(ctx, "run-1")
	if err != nil {
		t.Fatalf("ReleaseSingletonLock() error = %v", err)
	}
	if !released {
		t.Fatal("expected first release to report a deletion")
	}

	released, err = q.ReleaseSingletonLock(ctx, "run-1")
	if err != nil {
		t.Fatalf("second ReleaseSingletonLock() error = %v", err)
	}
	if released {
		t.Fatal("expected second release to be a no-op")
	}

	if _, err := q.GetSingletonHolder(ctx, "proj-1", domain.SingletonKindJob, "job-1", "k"); !errors.Is(err, store.ErrSingletonLockNotFound) {
		t.Fatalf("expected lock gone after release, got %v", err)
	}
}

func TestListSingletonLocks_OrderedByOwner(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	for i, key := range []string{"a", "b", "c"} {
		lock := domain.SingletonLock{
			ProjectID:   "proj-1",
			Kind:        domain.SingletonKindJob,
			OwnerID:     "job-1",
			LockKey:     key,
			HolderRunID: fmt.Sprintf("run-%d", i),
		}
		if _, _, err := q.AcquireSingletonLock(ctx, lock); err != nil {
			t.Fatalf("AcquireSingletonLock(%s) error = %v", key, err)
		}
	}
	// A lock for a different owner must not leak into the listing.
	other := domain.SingletonLock{
		ProjectID:   "proj-1",
		Kind:        domain.SingletonKindJob,
		OwnerID:     "job-2",
		LockKey:     "z",
		HolderRunID: "run-other",
	}
	if _, _, err := q.AcquireSingletonLock(ctx, other); err != nil {
		t.Fatalf("AcquireSingletonLock(other) error = %v", err)
	}

	locks, err := q.ListSingletonLocks(ctx, "proj-1", domain.SingletonKindJob, "job-1")
	if err != nil {
		t.Fatalf("ListSingletonLocks() error = %v", err)
	}
	if len(locks) != 3 {
		t.Fatalf("expected 3 locks, got %d", len(locks))
	}
	for _, l := range locks {
		if l.OwnerID != "job-1" {
			t.Errorf("unexpected owner %q in listing", l.OwnerID)
		}
	}
}

func TestCountSingletonWaiters(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-" + uuid.Must(uuid.NewV7()).String()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})

	waiting := domain.StatusWaiting
	for range 3 {
		run := testutil.MustCreateRun(t, ctx, q, job, &testutil.RunOpts{Status: &waiting})
		if _, err := testDB.Pool.Exec(ctx,
			`UPDATE job_runs SET singleton_key = $1 WHERE id = $2`, "acct-7", run.ID); err != nil {
			t.Fatalf("set singleton_key: %v", err)
		}
	}
	// A run on a different key must not be counted.
	other := testutil.MustCreateRun(t, ctx, q, job, &testutil.RunOpts{Status: &waiting})
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET singleton_key = $1 WHERE id = $2`, "acct-other", other.ID); err != nil {
		t.Fatalf("set singleton_key: %v", err)
	}

	n, err := q.CountSingletonWaiters(ctx, domain.SingletonKindJob, job.ID, "acct-7")
	if err != nil {
		t.Fatalf("CountSingletonWaiters(job) error = %v", err)
	}
	if n != 3 {
		t.Fatalf("waiter count = %d, want 3", n)
	}

	// Workflow branch with no parked runs returns 0.
	wfN, err := q.CountSingletonWaiters(ctx, domain.SingletonKindWorkflow, "wf-1", "acct-7")
	if err != nil {
		t.Fatalf("CountSingletonWaiters(workflow) error = %v", err)
	}
	if wfN != 0 {
		t.Fatalf("workflow waiter count = %d, want 0", wfN)
	}

	if _, err := q.CountSingletonWaiters(ctx, domain.SingletonKind("bogus"), "x", "y"); err == nil {
		t.Fatal("expected error for invalid kind")
	}
}

// TestAcquireSingletonLock_ConcurrentExactlyOne is the adversarial guarantee:
// under N concurrent acquires of the same (project, kind, owner, key) exactly
// one wins and the rest observe the single holder.
func TestAcquireSingletonLock_ConcurrentExactlyOne(t *testing.T) {
	ctx := context.Background()
	stClean(t, ctx)

	const n = 16
	var wg conc.WaitGroup
	type result struct {
		acquired bool
		holder   string
	}
	results := make(chan result, n)

	for i := range n {
		holderRun := fmt.Sprintf("run-%d", i)
		wg.Go(func() {
			q := store.New(testDB.Pool)
			lock := domain.SingletonLock{
				ProjectID:   "proj-c",
				Kind:        domain.SingletonKindJob,
				OwnerID:     "job-c",
				LockKey:     "hot-key",
				HolderRunID: holderRun,
			}
			acquired, held, err := q.AcquireSingletonLock(ctx, lock)
			if err != nil {
				results <- result{}
				return
			}
			results <- result{acquired: acquired, holder: held.HolderRunID}
		})
	}
	wg.Wait()
	close(results)

	winners := 0
	var winningHolder string
	holders := map[string]struct{}{}
	for r := range results {
		holders[r.holder] = struct{}{}
		if r.acquired {
			winners++
			winningHolder = r.holder
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", winners)
	}
	// Every loser must report the same single holder as the winner.
	if len(holders) != 1 {
		t.Fatalf("expected all callers to observe one holder, saw %d distinct: %v", len(holders), holders)
	}
	if _, ok := holders[winningHolder]; !ok {
		t.Fatalf("winning holder %q not the observed holder", winningHolder)
	}

	now := time.Now()
	final, err := store.New(testDB.Pool).GetSingletonHolder(ctx, "proj-c", domain.SingletonKindJob, "job-c", "hot-key")
	if err != nil {
		t.Fatalf("GetSingletonHolder() error = %v", err)
	}
	if final.HolderRunID != winningHolder {
		t.Fatalf("final holder %q != winner %q", final.HolderRunID, winningHolder)
	}
	if final.AcquiredAt.After(now) {
		t.Errorf("acquired_at %v is in the future", final.AcquiredAt)
	}
}
