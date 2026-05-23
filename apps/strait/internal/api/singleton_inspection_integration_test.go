//go:build integration

package api

import (
	"context"
	"net/http"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

// listJobSingletons drives the job singleton inspection handler and returns the
// paginated body.
func listJobSingletons(t *testing.T, ctx context.Context, srv *Server, jobID, limit, cursor string) PaginatedResponse {
	t.Helper()
	out, err := srv.handleListJobSingletons(ctx, &ListJobSingletonsInput{JobID: jobID, Limit: limit, Cursor: cursor})
	if err != nil {
		t.Fatalf("handleListJobSingletons(%s) error = %v", jobID, err)
	}
	return out.Body
}

// findHolderView returns the view for a lock key, failing the test if absent.
func findHolderView(t *testing.T, views []SingletonHolderView, lockKey string) SingletonHolderView {
	t.Helper()
	for _, v := range views {
		if v.LockKey == lockKey {
			return v
		}
	}
	t.Fatalf("no holder view for lock_key %q in %+v", lockKey, views)
	return SingletonHolderView{}
}

func TestIntegration_ListJobSingletons_HoldersAndWaiters(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newSingletonTriggerServer(t, ctx, db)
	job := mustCreateSingletonJob(t, ctx, st, projectID, "${id}", string(domain.SingletonOnConflictQueue), nil)

	// acct-1: holder plus two parked waiters; acct-2: lone holder, no waiters.
	holder1 := triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)
	_ = triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)
	_ = triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)
	holder2 := triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-2"}`)

	body := listJobSingletons(t, reqCtx, srv, job.ID, "", "")
	views, ok := body.Data.([]SingletonHolderView)
	if !ok {
		t.Fatalf("body.Data type = %T, want []SingletonHolderView", body.Data)
	}
	if len(views) != 2 {
		t.Fatalf("holder count = %d, want 2", len(views))
	}
	if body.HasMore {
		t.Fatalf("has_more = true, want false for a full unpaginated listing")
	}

	v1 := findHolderView(t, views, "acct-1")
	if v1.HolderRunID != holder1["id"].(string) {
		t.Fatalf("acct-1 holder = %s, want %s", v1.HolderRunID, holder1["id"])
	}
	if v1.Waiters != 2 {
		t.Fatalf("acct-1 waiters = %d, want 2", v1.Waiters)
	}
	if v1.AcquiredAt.IsZero() {
		t.Fatalf("acct-1 acquired_at is zero")
	}
	if v1.LeaseUntil == nil {
		t.Fatalf("acct-1 lease_until is nil, want a job-run lease window")
	}

	v2 := findHolderView(t, views, "acct-2")
	if v2.HolderRunID != holder2["id"].(string) {
		t.Fatalf("acct-2 holder = %s, want %s", v2.HolderRunID, holder2["id"])
	}
	if v2.Waiters != 0 {
		t.Fatalf("acct-2 waiters = %d, want 0", v2.Waiters)
	}
}

func TestIntegration_ListJobSingletons_Pagination(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newSingletonTriggerServer(t, ctx, db)
	job := mustCreateSingletonJob(t, ctx, st, projectID, "${id}", string(domain.SingletonOnConflictQueue), nil)

	// Three distinct keys -> three holders, ordered by acquisition time.
	_ = triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"k1"}`)
	_ = triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"k2"}`)
	_ = triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"k3"}`)

	first := listJobSingletons(t, reqCtx, srv, job.ID, "1", "")
	firstViews := first.Data.([]SingletonHolderView)
	if len(firstViews) != 1 {
		t.Fatalf("page 1 size = %d, want 1", len(firstViews))
	}
	if !first.HasMore || first.NextCursor == nil {
		t.Fatalf("page 1 has_more=%v next_cursor=%v, want true and a cursor", first.HasMore, first.NextCursor)
	}
	if firstViews[0].LockKey != "k1" {
		t.Fatalf("page 1 key = %s, want k1 (oldest first)", firstViews[0].LockKey)
	}

	second := listJobSingletons(t, reqCtx, srv, job.ID, "1", *first.NextCursor)
	secondViews := second.Data.([]SingletonHolderView)
	if len(secondViews) != 1 {
		t.Fatalf("page 2 size = %d, want 1", len(secondViews))
	}
	if secondViews[0].LockKey != "k2" {
		t.Fatalf("page 2 key = %s, want k2", secondViews[0].LockKey)
	}
	if !second.HasMore {
		t.Fatalf("page 2 has_more = false, want true (k3 remains)")
	}
}

func TestIntegration_ListJobSingletons_NotFoundAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newSingletonTriggerServer(t, ctx, db)
	job := mustCreateSingletonJob(t, ctx, st, projectID, "${id}", string(domain.SingletonOnConflictQueue), nil)

	// Unknown job -> 404.
	if _, err := srv.handleListJobSingletons(reqCtx, &ListJobSingletonsInput{JobID: uuid.Must(uuid.NewV7()).String()}); !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("unknown job error = %v, want 404", err)
	}

	// Same job, different project context -> 404 (no cross-tenant leak).
	otherCtx := context.WithValue(ctx, ctxProjectIDKey, "project-"+uuid.Must(uuid.NewV7()).String())
	if _, err := srv.handleListJobSingletons(otherCtx, &ListJobSingletonsInput{JobID: job.ID}); !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("cross-tenant error = %v, want 404", err)
	}
}

// seedWorkflowSingleton creates a workflow, a running holder run that owns the
// lock, and parkN queued waiter runs sharing the resolved key.
func seedWorkflowSingleton(t *testing.T, ctx context.Context, st store.Store, db *testutil.TestDB, projectID, lockKey string, parkN int) *domain.Workflow {
	t.Helper()
	wf := testutil.MustCreateWorkflow(t, ctx, st, &testutil.WorkflowOpts{ProjectID: &projectID})

	running := domain.WfStatusRunning
	holder := testutil.MustCreateWorkflowRun(t, ctx, st, wf.ID, &testutil.WorkflowRunOpts{ProjectID: &projectID, Status: &running})
	if _, err := db.Pool.Exec(ctx, `UPDATE workflow_runs SET singleton_key = $2 WHERE id = $1`, holder.ID, lockKey); err != nil {
		t.Fatalf("stamp holder singleton_key: %v", err)
	}

	q := store.New(db.Pool)
	acquired, _, err := q.AcquireSingletonLock(ctx, domain.SingletonLock{
		ProjectID:   projectID,
		Kind:        domain.SingletonKindWorkflow,
		OwnerID:     wf.ID,
		LockKey:     lockKey,
		HolderRunID: holder.ID,
	})
	if err != nil {
		t.Fatalf("AcquireSingletonLock() error = %v", err)
	}
	if !acquired {
		t.Fatalf("AcquireSingletonLock() did not acquire a fresh key")
	}

	for range parkN {
		queued := domain.WfStatusQueued
		waiter := testutil.MustCreateWorkflowRun(t, ctx, st, wf.ID, &testutil.WorkflowRunOpts{ProjectID: &projectID, Status: &queued})
		if _, err := db.Pool.Exec(ctx, `UPDATE workflow_runs SET singleton_key = $2 WHERE id = $1`, waiter.ID, lockKey); err != nil {
			t.Fatalf("stamp waiter singleton_key: %v", err)
		}
	}
	return wf
}

func TestIntegration_ListWorkflowSingletons_HoldersAndWaiters(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newSingletonTriggerServer(t, ctx, db)
	wf := seedWorkflowSingleton(t, ctx, st, db, projectID, "tenant-7", 2)

	out, err := srv.handleListWorkflowSingletons(reqCtx, &ListWorkflowSingletonsInput{WorkflowID: wf.ID})
	if err != nil {
		t.Fatalf("handleListWorkflowSingletons() error = %v", err)
	}
	views, ok := out.Body.Data.([]SingletonHolderView)
	if !ok {
		t.Fatalf("body.Data type = %T, want []SingletonHolderView", out.Body.Data)
	}
	if len(views) != 1 {
		t.Fatalf("holder count = %d, want 1", len(views))
	}
	v := findHolderView(t, views, "tenant-7")
	if v.Waiters != 2 {
		t.Fatalf("waiters = %d, want 2", v.Waiters)
	}
	// Workflow-run holders carry no lease (reclaimed on terminal/missing only).
	if v.LeaseUntil != nil {
		t.Fatalf("workflow holder lease_until = %v, want nil", v.LeaseUntil)
	}
	if v.AcquiredAt.IsZero() {
		t.Fatalf("acquired_at is zero")
	}
}

func TestIntegration_ListWorkflowSingletons_NotFoundAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newSingletonTriggerServer(t, ctx, db)
	wf := seedWorkflowSingleton(t, ctx, st, db, projectID, "tenant-7", 1)

	if _, err := srv.handleListWorkflowSingletons(reqCtx, &ListWorkflowSingletonsInput{WorkflowID: uuid.Must(uuid.NewV7()).String()}); !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("unknown workflow error = %v, want 404", err)
	}

	otherCtx := context.WithValue(ctx, ctxProjectIDKey, "project-"+uuid.Must(uuid.NewV7()).String())
	if _, err := srv.handleListWorkflowSingletons(otherCtx, &ListWorkflowSingletonsInput{WorkflowID: wf.ID}); !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("cross-tenant error = %v, want 404", err)
	}
}
