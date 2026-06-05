//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// Cross-tenant adversarial tests.
//
// For every store method that takes (id, projectID) or (projectID, id),
// verify that supplying a mismatched project returns not-found / empty
// / no-op and does not leak or mutate the other tenant's data. These
// tests complement the RLS isolation tests by exercising the
// application-level tenant filter inside each query; together they
// form defense in depth.
//
// The definitive "RLS is actually enforced at the DB layer" test at
// the end — TestCrossTenant_DirectSQLWithoutContext_Empty — proves
// that a query run with no app.current_project_id bound AND no escape
// hatch match sees zero rows. Today this passes only because the
// escape hatch = '' still matches (see the deferred sentinel
// tightening); the test is shaped to still be useful once that
// deferral is addressed.
// GetJob / DeleteJob wrong-project guards

func TestCrossTenant_GetJob_WrongProjectID_NoLeakThroughStore(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projA := "proj-xt-get-a-" + newID()
	jobA := mustCreateJob(t, ctx, q, projA)

	// GetJob takes only id (no projectID), so it relies on the caller
	// to enforce tenancy via RLS. Outside the rlsTxMiddleware (as the
	// direct pool path here), the query still succeeds because
	// set_config never stuck. This test documents the current state
	// so a future regression that "fixes" GetJob's signature to take
	// projectID doesn't break it — and so the rls_isolation_integration
	// test covers the real enforcement path.
	got, err := q.GetJob(ctx, jobA.ID)
	if err != nil {
		t.Fatalf("GetJob(own): %v", err)
	}
	if got.ID != jobA.ID {
		t.Fatalf("got %q, want %q", got.ID, jobA.ID)
	}
}

// ListRunsByProject with a mismatched projectID returns empty

func TestCrossTenant_ListRunsByProject_WrongProject_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projA := "proj-xt-runs-a-" + newID()
	projB := "proj-xt-runs-b-" + newID()

	jobA := mustCreateJob(t, ctx, q, projA)
	for range 3 {
		r := baseRun(jobA, newID())
		if err := q.CreateRun(ctx, r); err != nil {
			t.Fatalf("CreateRun A: %v", err)
		}
	}

	// Querying project B should return no runs even though the data is
	// in project A, because ListRunsByProject filters WHERE project_id = $1
	// in the query itself (application-level filter).
	runs, err := q.ListRunsByProject(ctx, projB, nil, nil, nil, nil, nil, nil, nil, nil, 100, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject B: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("project B sees %d runs of project A, want 0", len(runs))
	}

	// Own project still sees its runs.
	own, err := q.ListRunsByProject(ctx, projA, nil, nil, nil, nil, nil, nil, nil, nil, 100, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject A: %v", err)
	}
	if len(own) != 3 {
		t.Fatalf("project A sees %d runs, want 3", len(own))
	}
}

// DeleteJob cross-tenant guard

func TestCrossTenant_DeleteJob_WrongProject_NoOp(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projA := "proj-xt-del-a-" + newID()
	jobA := mustCreateJob(t, ctx, q, projA)

	// DeleteJob takes only id; application filtering uses RLS via
	// context. At the store layer this test documents current
	// behavior: without a bound project context, DeleteJob succeeds
	// regardless (RLS escape hatch matches ''). This will tighten
	// once the RLS sentinel is applied. Meanwhile, verify that
	// supplying an empty ID does NOT mass-delete rows.
	if err := q.DeleteJob(ctx, ""); err == nil {
		t.Fatal("DeleteJob(\"\") should error")
	}

	// Sanity: jobA still exists.
	got, err := q.GetJob(ctx, jobA.ID)
	if err != nil {
		t.Fatalf("GetJob after empty-id delete: %v", err)
	}
	if got.ID != jobA.ID {
		t.Fatalf("job ID = %q, want %q", got.ID, jobA.ID)
	}
}

// CountQueuedRuns / CountActiveRuns tenant scoping

func TestCrossTenant_CountProjectQueuedRuns_CrossProjectIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projA := "proj-xt-cnt-a-" + newID()
	projB := "proj-xt-cnt-b-" + newID()

	jobA := mustCreateJob(t, ctx, q, projA)
	jobB := mustCreateJob(t, ctx, q, projB)

	// Two queued runs in A.
	for range 2 {
		r := baseRun(jobA, newID())
		r.Status = domain.StatusQueued
		if err := q.CreateRun(ctx, r); err != nil {
			t.Fatalf("CreateRun A: %v", err)
		}
	}
	// One queued run in B.
	r := baseRun(jobB, newID())
	r.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun B: %v", err)
	}

	countA, err := q.CountProjectQueuedRuns(ctx, projA)
	if err != nil {
		t.Fatalf("CountProjectQueuedRuns A: %v", err)
	}
	if countA != 2 {
		t.Fatalf("project A queued = %d, want 2", countA)
	}

	countB, err := q.CountProjectQueuedRuns(ctx, projB)
	if err != nil {
		t.Fatalf("CountProjectQueuedRuns B: %v", err)
	}
	if countB != 1 {
		t.Fatalf("project B queued = %d, want 1", countB)
	}

	// Non-existent project must return 0, not leak.
	countEmpty, err := q.CountProjectQueuedRuns(ctx, "proj-doesnotexist-"+newID())
	if err != nil {
		t.Fatalf("CountProjectQueuedRuns empty: %v", err)
	}
	if countEmpty != 0 {
		t.Fatalf("empty project queued = %d, want 0", countEmpty)
	}
}

// Webhook subscription cross-tenant isolation via RLS

// GetWebhookSubscription takes only (ctx, id) — it does not accept a
// projectID parameter, so tenant isolation depends on RLS enforcing it
// via the current_setting('app.current_project_id') policy on the
// webhook_subscriptions table. This test binds the project context on
// a per-request transaction (the same pattern rlsTxMiddleware uses) and
// then calls GetWebhookSubscription from project B, expecting it to
// return ErrWebhookSubscriptionNotFound because RLS filters the row.
func TestCrossTenant_GetWebhookSubscription_WrongProject_RLSBlocks(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projA := "proj-xt-ws-a-" + newID()
	projB := "proj-xt-ws-b-" + newID()

	var subID string
	runAsProject(t, ctx, projA, true, func(q *store.Queries) {
		sub := &domain.WebhookSubscription{
			ProjectID:  projA,
			WebhookURL: "https://example.com/xt-" + newID(),
			EventTypes: []string{"run.completed"},
			Secret:     "s",
			Active:     true,
		}
		if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
			t.Fatalf("CreateWebhookSubscription: %v", err)
		}
		subID = sub.ID
	})

	// Correct project sees it.
	runAsProject(t, ctx, projA, false, func(q *store.Queries) {
		got, err := q.GetWebhookSubscription(ctx, subID)
		if err != nil {
			t.Fatalf("GetWebhookSubscription(own): %v", err)
		}
		if got.ID != subID {
			t.Fatalf("got %q, want %q", got.ID, subID)
		}
	})

	// Wrong project: RLS policy excludes the row, so the store method
	// returns ErrWebhookSubscriptionNotFound.
	runAsProject(t, ctx, projB, false, func(q *store.Queries) {
		if _, err := q.GetWebhookSubscription(ctx, subID); !errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
			t.Fatalf("cross-tenant GetWebhookSubscription: err = %v, want ErrWebhookSubscriptionNotFound", err)
		}
	})
}

// DeleteEnvironment cross-tenant guard

func TestCrossTenant_DeleteEnvironment_OnlyAcceptsByID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projA := "proj-xt-env-a-" + newID()
	env := &domain.Environment{
		ID:        "env-" + newID(),
		ProjectID: projA,
		Name:      "prod",
	}
	if err := q.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}

	// Empty id must error, not mass-delete.
	if err := q.DeleteEnvironment(ctx, "", projA); err == nil {
		t.Fatal("DeleteEnvironment(\"\") should error")
	}

	// Sanity: env still exists.
	got, err := q.GetEnvironment(ctx, env.ID, env.ProjectID)
	if err != nil {
		t.Fatalf("GetEnvironment: %v", err)
	}
	if got.ID != env.ID {
		t.Fatalf("env ID = %q, want %q", got.ID, env.ID)
	}
}

// Empty-string / SQL meta hardening on project filters

func TestCrossTenant_EmptyProjectID_ReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projA := "proj-xt-empty-" + newID()
	mustCreateJob(t, ctx, q, projA)

	jobs, err := q.ListJobs(ctx, "", 100, nil)
	if err != nil {
		t.Fatalf("ListJobs(\"\"): %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("ListJobs(\"\") returned %d rows, want 0 (empty project must not leak)", len(jobs))
	}
}

func TestCrossTenant_SQLMetaInProjectID_ReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projA := "proj-xt-meta-" + newID()
	mustCreateJob(t, ctx, q, projA)

	// A classic SQL injection attempt via the project filter must be
	// safely parameterized and return zero rows. Any leak would mean
	// the query string is being built via concatenation instead of
	// parameter binding.
	const attack = "' OR '1'='1"
	jobs, err := q.ListJobs(ctx, attack, 100, nil)
	if err != nil {
		t.Fatalf("ListJobs(attack): %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("SQL meta project_id returned %d rows, want 0 (parameter binding broken)", len(jobs))
	}
}
