//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

// buildSuccessor constructs a successor workflow run for continue-as-new with
// the lineage fields set, mirroring what the engine assembles.
func buildSuccessor(workflowID, projectID, predecessorID string, depth int, payload json.RawMessage) *domain.WorkflowRun {
	return &domain.WorkflowRun{
		ID:                         uuid.Must(uuid.NewV7()).String(),
		WorkflowID:                 workflowID,
		ProjectID:                  projectID,
		Status:                     domain.WfStatusPending,
		TriggeredBy:                domain.TriggerManual,
		Payload:                    payload,
		ContinuedFromWorkflowRunID: predecessorID,
		LineageDepth:               depth,
	}
}

func TestContinueWorkflowRunBootstrap_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-happy"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: new(job.ID)})

	// Predecessor running with an in-flight job + step run.
	running := domain.WfStatusRunning
	pred := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    &running,
	})

	jobRunStatus := domain.StatusExecuting
	jobRun := testutil.MustCreateRun(t, ctx, q, job, &testutil.RunOpts{Status: &jobRunStatus})

	stepRunStatus := domain.StepRunning
	predStepRun := testutil.BuildWorkflowStepRun(pred.ID, step.ID, &testutil.WorkflowStepRunOpts{
		Status:   &stepRunStatus,
		JobRunID: new(jobRun.ID),
	})
	if err := q.CreateWorkflowStepRun(ctx, predStepRun); err != nil {
		t.Fatalf("create predecessor step run: %v", err)
	}

	// Successor with carry-over payload and a fresh pending step run.
	carryOver := json.RawMessage(`{"cursor":42}`)
	successor := buildSuccessor(wf.ID, projectID, pred.ID, pred.LineageDepth+1, carryOver)
	successorStep := testutil.BuildWorkflowStepRun(successor.ID, step.ID, &testutil.WorkflowStepRunOpts{StepRef: new("root")})

	now := time.Now().UTC()
	if err := q.ContinueWorkflowRunBootstrap(ctx, pred.ID, running, successor, []domain.WorkflowStepRun{*successorStep}, now); err != nil {
		t.Fatalf("ContinueWorkflowRunBootstrap() error = %v", err)
	}

	// Successor is running, links back, and carries the incremented depth.
	gotSuccessor, err := q.GetWorkflowRun(ctx, successor.ID)
	if err != nil {
		t.Fatalf("get successor: %v", err)
	}
	if gotSuccessor.Status != domain.WfStatusRunning {
		t.Errorf("successor status = %s, want running", gotSuccessor.Status)
	}
	if gotSuccessor.ContinuedFromWorkflowRunID != pred.ID {
		t.Errorf("successor continued_from = %q, want %q", gotSuccessor.ContinuedFromWorkflowRunID, pred.ID)
	}
	if gotSuccessor.LineageDepth != 1 {
		t.Errorf("successor lineage_depth = %d, want 1", gotSuccessor.LineageDepth)
	}
	if !jsonEqual(gotSuccessor.Payload, carryOver) {
		t.Errorf("successor payload = %s, want %s", gotSuccessor.Payload, carryOver)
	}

	// Predecessor is continued, finished, and links forward.
	gotPred, err := q.GetWorkflowRun(ctx, pred.ID)
	if err != nil {
		t.Fatalf("get predecessor: %v", err)
	}
	if gotPred.Status != domain.WfStatusContinued {
		t.Errorf("predecessor status = %s, want continued", gotPred.Status)
	}
	if gotPred.FinishedAt == nil {
		t.Error("predecessor finished_at not set")
	}
	if gotPred.ContinuedToWorkflowRunID != successor.ID {
		t.Errorf("predecessor continued_to = %q, want %q", gotPred.ContinuedToWorkflowRunID, successor.ID)
	}

	// Predecessor in-flight work is torn down.
	predSteps, err := q.ListStepRunsByWorkflowRun(ctx, pred.ID, 100, nil)
	if err != nil {
		t.Fatalf("list predecessor step runs: %v", err)
	}
	if len(predSteps) != 1 || predSteps[0].Status != domain.StepCanceled {
		t.Errorf("predecessor step run not canceled: %+v", predSteps)
	}
	gotJobRun, err := q.GetRun(ctx, jobRun.ID)
	if err != nil {
		t.Fatalf("get job run: %v", err)
	}
	if gotJobRun.Status != domain.RunStatus("canceled") {
		t.Errorf("predecessor job run status = %s, want canceled", gotJobRun.Status)
	}

	// Successor starts with a fresh, flat step history.
	successorSteps, err := q.ListStepRunsByWorkflowRun(ctx, successor.ID, 100, nil)
	if err != nil {
		t.Fatalf("list successor step runs: %v", err)
	}
	if len(successorSteps) != 1 {
		t.Errorf("successor step runs = %d, want 1", len(successorSteps))
	}
}

func TestContinueWorkflowRunBootstrap_TerminalPredecessorRejected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-terminal"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})

	completed := domain.WfStatusCompleted
	pred := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    &completed,
	})

	successor := buildSuccessor(wf.ID, projectID, pred.ID, 1, nil)

	// fromStatus completed is not a legal source for continued.
	err := q.ContinueWorkflowRunBootstrap(ctx, pred.ID, completed, successor, nil, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error continuing from terminal predecessor")
	}

	// No successor should have been created.
	if _, getErr := q.GetWorkflowRun(ctx, successor.ID); !errors.Is(getErr, store.ErrWorkflowRunNotFound) {
		t.Errorf("expected successor not found, got %v", getErr)
	}
}

func TestContinueWorkflowRunBootstrap_StatusConflictRejected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-conflict"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})

	// Predecessor is actually paused, but the caller claims it is running.
	paused := domain.WfStatusPaused
	pred := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    &paused,
	})

	successor := buildSuccessor(wf.ID, projectID, pred.ID, 1, nil)

	err := q.ContinueWorkflowRunBootstrap(ctx, pred.ID, domain.WfStatusRunning, successor, nil, time.Now().UTC())
	if !errors.Is(err, store.ErrWorkflowRunContinueConflict) {
		t.Fatalf("expected ErrWorkflowRunContinueConflict, got %v", err)
	}

	// Predecessor untouched; no successor created.
	gotPred, err := q.GetWorkflowRun(ctx, pred.ID)
	if err != nil {
		t.Fatalf("get predecessor: %v", err)
	}
	if gotPred.Status != domain.WfStatusPaused {
		t.Errorf("predecessor status = %s, want paused (untouched)", gotPred.Status)
	}
	if gotPred.ContinuedToWorkflowRunID != "" {
		t.Errorf("predecessor continued_to = %q, want empty", gotPred.ContinuedToWorkflowRunID)
	}
	if _, getErr := q.GetWorkflowRun(ctx, successor.ID); !errors.Is(getErr, store.ErrWorkflowRunNotFound) {
		t.Errorf("expected successor not found, got %v", getErr)
	}
}

// TestContinueWorkflowRunBootstrap_ConflictLeavesNoOrphanStepRuns asserts the
// guard-first ordering: when the predecessor is no longer in fromStatus, the
// status guard matches no row and the bootstrap aborts before inserting the
// successor or any of its step runs, so neither is left behind.
func TestContinueWorkflowRunBootstrap_ConflictLeavesNoOrphanStepRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-orphan"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})

	// Predecessor is paused, but the caller claims it is running, so the guard
	// will not match.
	paused := domain.WfStatusPaused
	pred := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    &paused,
	})

	// The successor step run is never reached (the guard rejects first), so its
	// step ID need not exist.
	successor := buildSuccessor(wf.ID, projectID, pred.ID, 1, nil)
	successorStep := testutil.BuildWorkflowStepRun(successor.ID, uuid.Must(uuid.NewV7()).String(), &testutil.WorkflowStepRunOpts{StepRef: new("root")})

	err := q.ContinueWorkflowRunBootstrap(ctx, pred.ID, domain.WfStatusRunning, successor, []domain.WorkflowStepRun{*successorStep}, time.Now().UTC())
	if !errors.Is(err, store.ErrWorkflowRunContinueConflict) {
		t.Fatalf("expected ErrWorkflowRunContinueConflict, got %v", err)
	}

	// No successor row and no orphan step runs for it.
	if _, getErr := q.GetWorkflowRun(ctx, successor.ID); !errors.Is(getErr, store.ErrWorkflowRunNotFound) {
		t.Errorf("expected successor not found, got %v", getErr)
	}
	orphanSteps, err := q.ListStepRunsByWorkflowRun(ctx, successor.ID, 100, nil)
	if err != nil {
		t.Fatalf("list successor step runs: %v", err)
	}
	if len(orphanSteps) != 0 {
		t.Errorf("expected no orphan step runs for rejected successor, got %d", len(orphanSteps))
	}
}

func TestContinueWorkflowRunBootstrap_CrashMidContinueRollsBack(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-crash"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})

	running := domain.WfStatusRunning
	pred := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    &running,
	})

	successor := buildSuccessor(wf.ID, projectID, pred.ID, 1, nil)
	// A step run pointing at a non-existent workflow step violates the FK and
	// fails mid-transaction, simulating a crash during continuation.
	badStep := testutil.BuildWorkflowStepRun(successor.ID, uuid.Must(uuid.NewV7()).String(), &testutil.WorkflowStepRunOpts{StepRef: new("bad")})

	err := q.ContinueWorkflowRunBootstrap(ctx, pred.ID, running, successor, []domain.WorkflowStepRun{*badStep}, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error from failing successor step-run insert")
	}

	// Whole transaction rolled back: predecessor still running, no successor.
	gotPred, err := q.GetWorkflowRun(ctx, pred.ID)
	if err != nil {
		t.Fatalf("get predecessor: %v", err)
	}
	if gotPred.Status != domain.WfStatusRunning {
		t.Errorf("predecessor status = %s, want running (rolled back)", gotPred.Status)
	}
	if gotPred.ContinuedToWorkflowRunID != "" {
		t.Errorf("predecessor continued_to = %q, want empty (rolled back)", gotPred.ContinuedToWorkflowRunID)
	}
	if _, getErr := q.GetWorkflowRun(ctx, successor.ID); !errors.Is(getErr, store.ErrWorkflowRunNotFound) {
		t.Errorf("expected successor not found after rollback, got %v", getErr)
	}
}

func TestContinueWorkflowRunBootstrap_ConcurrentSingleWinner(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-race"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})

	running := domain.WfStatusRunning
	pred := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    &running,
	})

	const racers = 2
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]error, racers)
	successorIDs := make([]string, racers)

	for i := range racers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			successor := buildSuccessor(wf.ID, projectID, pred.ID, 1, nil)
			successorIDs[idx] = successor.ID
			<-start
			results[idx] = q.ContinueWorkflowRunBootstrap(ctx, pred.ID, running, successor, nil, time.Now().UTC())
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	for i, err := range results {
		if err == nil {
			winners++
			continue
		}
		if !errors.Is(err, store.ErrWorkflowRunContinueConflict) {
			t.Errorf("racer %d: unexpected error %v", i, err)
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", winners)
	}

	// Exactly one successor row exists and the predecessor points at it.
	gotPred, err := q.GetWorkflowRun(ctx, pred.ID)
	if err != nil {
		t.Fatalf("get predecessor: %v", err)
	}
	if gotPred.Status != domain.WfStatusContinued {
		t.Errorf("predecessor status = %s, want continued", gotPred.Status)
	}
	created := 0
	for _, id := range successorIDs {
		if _, err := q.GetWorkflowRun(ctx, id); err == nil {
			created++
			if id != gotPred.ContinuedToWorkflowRunID {
				t.Errorf("surviving successor %s is not the one linked by predecessor %s", id, gotPred.ContinuedToWorkflowRunID)
			}
		} else if !errors.Is(err, store.ErrWorkflowRunNotFound) {
			t.Errorf("unexpected error reading successor %s: %v", id, err)
		}
	}
	if created != 1 {
		t.Fatalf("expected exactly 1 successor row, got %d", created)
	}
}

func TestGetWorkflowRunChain_OrderedSeries(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-chain"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})

	running := domain.WfStatusRunning
	root := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    &running,
	})

	// Build a three-run chain: root -> mid -> latest.
	mid := buildSuccessor(wf.ID, projectID, root.ID, 1, nil)
	if err := q.ContinueWorkflowRunBootstrap(ctx, root.ID, running, mid, nil, time.Now().UTC()); err != nil {
		t.Fatalf("continue root->mid: %v", err)
	}
	latest := buildSuccessor(wf.ID, projectID, mid.ID, 2, nil)
	if err := q.ContinueWorkflowRunBootstrap(ctx, mid.ID, running, latest, nil, time.Now().UTC()); err != nil {
		t.Fatalf("continue mid->latest: %v", err)
	}

	wantOrder := []string{root.ID, mid.ID, latest.ID}

	// The chain resolves identically from any member.
	for _, from := range []string{root.ID, mid.ID, latest.ID} {
		chain, err := q.GetWorkflowRunChain(ctx, from, projectID, 100, "")
		if err != nil {
			t.Fatalf("GetWorkflowRunChain(%s): %v", from, err)
		}
		if len(chain) != 3 {
			t.Fatalf("chain from %s len = %d, want 3", from, len(chain))
		}
		for i, run := range chain {
			if run.ID != wantOrder[i] {
				t.Errorf("chain from %s position %d = %s, want %s", from, i, run.ID, wantOrder[i])
			}
			if run.LineageDepth != i {
				t.Errorf("chain from %s position %d lineage_depth = %d, want %d", from, i, run.LineageDepth, i)
			}
		}
	}
}

func TestGetWorkflowRunChain_SingleRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-chain-single"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})
	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})

	chain, err := q.GetWorkflowRunChain(ctx, run.ID, projectID, 100, "")
	if err != nil {
		t.Fatalf("GetWorkflowRunChain: %v", err)
	}
	if len(chain) != 1 || chain[0].ID != run.ID {
		t.Fatalf("expected single-element chain with %s, got %+v", run.ID, chain)
	}
}

func TestGetWorkflowRunChain_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if _, err := q.GetWorkflowRunChain(ctx, newID(), "project-missing", 100, ""); !errors.Is(err, store.ErrWorkflowRunNotFound) {
		t.Fatalf("expected ErrWorkflowRunNotFound, got %v", err)
	}
}

// buildChain creates a workflow plus a continue-as-new chain of the given length
// (root + length successors) and returns the ordered run ids root..latest.
func buildChain(t *testing.T, ctx context.Context, q *store.Queries, projectID string, length int) []string {
	t.Helper()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})
	running := domain.WfStatusRunning
	root := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    &running,
	})
	ids := []string{root.ID}
	predID := root.ID
	for i := 1; i <= length; i++ {
		succ := buildSuccessor(wf.ID, projectID, predID, i, nil)
		if err := q.ContinueWorkflowRunBootstrap(ctx, predID, running, succ, nil, time.Now().UTC()); err != nil {
			t.Fatalf("continue depth %d: %v", i, err)
		}
		ids = append(ids, succ.ID)
		predID = succ.ID
	}
	return ids
}

// TestGetWorkflowRunChain_Paginates walks a chain across several bounded pages
// using the returned next-cursor and asserts the concatenation reproduces the
// full ordered series exactly once, with the final page ending the walk.
func TestGetWorkflowRunChain_Paginates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-chain-page"
	want := buildChain(t, ctx, q, projectID, 6) // root + 6 successors = 7 runs

	const pageSize = 3
	var got []string
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		// Mirror the handler: over-fetch by one to detect a further page.
		entries, err := q.GetWorkflowRunChain(ctx, want[len(want)-1], projectID, pageSize+1, cursor)
		if err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		if len(entries) == 0 {
			break
		}
		more := len(entries) > pageSize
		if more {
			entries = entries[:pageSize]
		}
		for _, e := range entries {
			got = append(got, e.ID)
		}
		if !more {
			break
		}
		cursor = entries[len(entries)-1].ID
	}

	if len(got) != len(want) {
		t.Fatalf("paged chain len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("page-walk position %d = %s, want %s", i, got[i], want[i])
		}
	}
}

// TestGetWorkflowRunChain_TenantIsolation proves the cursor is project-scoped:
// a forged cursor pointing at another project's run must not leak that chain.
// The chain endpoint only project-matches the path run, so the store query is
// the boundary that prevents cross-tenant traversal via an attacker-chosen
// cursor.
func TestGetWorkflowRunChain_TenantIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	victim := buildChain(t, ctx, q, "project-chain-victim", 3)
	attacker := buildChain(t, ctx, q, "project-chain-attacker", 3)

	// Seeding the attacker's query with a cursor from the victim's chain must
	// surface nothing: the successor lookup is scoped to the attacker project.
	entries, err := q.GetWorkflowRunChain(ctx, attacker[0], "project-chain-attacker", 100, victim[0])
	if err != nil {
		t.Fatalf("GetWorkflowRunChain with cross-tenant cursor: %v", err)
	}
	for _, e := range entries {
		for _, v := range victim {
			if e.ID == v {
				t.Fatalf("cross-tenant leak: attacker query returned victim run %s", v)
			}
		}
	}
}

// TestGetWorkflowRunChain_BogusCursor proves a subsequent-page request whose
// cursor matches no successor in the project returns an empty page rather than
// an error: the walk has simply reached (or run past) the end of the chain.
func TestGetWorkflowRunChain_BogusCursor(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-chain-bogus"
	ids := buildChain(t, ctx, q, projectID, 2)

	entries, err := q.GetWorkflowRunChain(ctx, ids[0], projectID, 100, newID())
	if err != nil {
		t.Fatalf("GetWorkflowRunChain with bogus cursor: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("bogus-cursor page len = %d, want 0", len(entries))
	}
}

// setFinishedAt forces a workflow run's status and finished_at directly, so
// retention tests can age a run past the reaper cutoff regardless of the FSM.
func setFinishedAt(t *testing.T, ctx context.Context, id string, status domain.WorkflowRunStatus, finishedAt time.Time) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE workflow_runs SET status = $1, finished_at = $2 WHERE id = $3`,
		status, finishedAt, id,
	); err != nil {
		t.Fatalf("set finished_at for %s: %v", id, err)
	}
}

// TestDeleteWorkflowRun_NullsSuccessorPointerOnDelete proves the continued_to
// foreign key is ON DELETE SET NULL: reaping a successor must not raise an FK
// violation against the surviving predecessor's continued_to pointer. With the
// bare FK this DELETE aborts the whole batch and stalls retention (R1).
func TestDeleteWorkflowRun_NullsSuccessorPointerOnDelete(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-fk-succ"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})

	running := domain.WfStatusRunning
	pred := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID), Status: &running})
	succ := buildSuccessor(wf.ID, projectID, pred.ID, 1, nil)
	if err := q.ContinueWorkflowRunBootstrap(ctx, pred.ID, running, succ, nil, time.Now().UTC()); err != nil {
		t.Fatalf("continue pred->succ: %v", err)
	}

	// Age only the successor past the cutoff; the predecessor stays recent.
	setFinishedAt(t, ctx, succ.ID, domain.WfStatusCompleted, time.Now().UTC().Add(-72*time.Hour))

	deleted, err := q.DeleteWorkflowRunsFinishedBefore(ctx, time.Now().UTC().Add(-24*time.Hour), 100)
	if err != nil {
		t.Fatalf("DeleteWorkflowRunsFinishedBefore() error = %v (FK should be ON DELETE SET NULL)", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	if _, err := q.GetWorkflowRun(ctx, succ.ID); !errors.Is(err, store.ErrWorkflowRunNotFound) {
		t.Errorf("successor still present after delete: %v", err)
	}
	gotPred, err := q.GetWorkflowRun(ctx, pred.ID)
	if err != nil {
		t.Fatalf("get predecessor: %v", err)
	}
	if gotPred.ContinuedToWorkflowRunID != "" {
		t.Errorf("predecessor continued_to = %q, want empty after successor reaped", gotPred.ContinuedToWorkflowRunID)
	}
}

// TestDeleteWorkflowRun_NullsPredecessorPointerOnDelete proves the
// continued_from foreign key is ON DELETE SET NULL from the other direction:
// reaping a continued root must null the surviving successor's continued_from
// pointer rather than aborting. This also exercises R2 (continued runs are
// eligible for retention at all).
func TestDeleteWorkflowRun_NullsPredecessorPointerOnDelete(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-fk-pred"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})

	running := domain.WfStatusRunning
	root := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID), Status: &running})
	succ := buildSuccessor(wf.ID, projectID, root.ID, 1, nil)
	if err := q.ContinueWorkflowRunBootstrap(ctx, root.ID, running, succ, nil, time.Now().UTC()); err != nil {
		t.Fatalf("continue root->succ: %v", err)
	}

	// Age the continued root past the cutoff; the running successor survives.
	setFinishedAt(t, ctx, root.ID, domain.WfStatusContinued, time.Now().UTC().Add(-72*time.Hour))

	deleted, err := q.DeleteWorkflowRunsFinishedBefore(ctx, time.Now().UTC().Add(-24*time.Hour), 100)
	if err != nil {
		t.Fatalf("DeleteWorkflowRunsFinishedBefore() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1 (continued root reaped)", deleted)
	}

	if _, err := q.GetWorkflowRun(ctx, root.ID); !errors.Is(err, store.ErrWorkflowRunNotFound) {
		t.Errorf("continued root still present after delete: %v", err)
	}
	gotSucc, err := q.GetWorkflowRun(ctx, succ.ID)
	if err != nil {
		t.Fatalf("get successor: %v", err)
	}
	if gotSucc.ContinuedFromWorkflowRunID != "" {
		t.Errorf("successor continued_from = %q, want empty after root reaped", gotSucc.ContinuedFromWorkflowRunID)
	}
	chain, err := q.GetWorkflowRunChain(ctx, succ.ID, projectID, 100, "")
	if err != nil {
		t.Fatalf("GetWorkflowRunChain(succ): %v", err)
	}
	if len(chain) != 1 || chain[0].ID != succ.ID {
		t.Fatalf("chain after root reaped = %+v, want single element %s", chain, succ.ID)
	}
}

// TestDeleteWorkflowRun_MiddleOfChainTruncates reaps the middle run of a
// three-run chain and asserts both neighbors survive with their lineage
// pointers nulled and the chain split, with no FK violation.
func TestDeleteWorkflowRun_MiddleOfChainTruncates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-fk-mid"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})

	running := domain.WfStatusRunning
	root := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID), Status: &running})
	mid := buildSuccessor(wf.ID, projectID, root.ID, 1, nil)
	if err := q.ContinueWorkflowRunBootstrap(ctx, root.ID, running, mid, nil, time.Now().UTC()); err != nil {
		t.Fatalf("continue root->mid: %v", err)
	}
	latest := buildSuccessor(wf.ID, projectID, mid.ID, 2, nil)
	if err := q.ContinueWorkflowRunBootstrap(ctx, mid.ID, running, latest, nil, time.Now().UTC()); err != nil {
		t.Fatalf("continue mid->latest: %v", err)
	}

	// Age only the middle run past the cutoff.
	setFinishedAt(t, ctx, mid.ID, domain.WfStatusContinued, time.Now().UTC().Add(-72*time.Hour))

	deleted, err := q.DeleteWorkflowRunsFinishedBefore(ctx, time.Now().UTC().Add(-24*time.Hour), 100)
	if err != nil {
		t.Fatalf("DeleteWorkflowRunsFinishedBefore() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1 (only the middle run)", deleted)
	}

	if _, err := q.GetWorkflowRun(ctx, mid.ID); !errors.Is(err, store.ErrWorkflowRunNotFound) {
		t.Errorf("middle run still present after delete: %v", err)
	}

	gotRoot, err := q.GetWorkflowRun(ctx, root.ID)
	if err != nil {
		t.Fatalf("get root: %v", err)
	}
	if gotRoot.ContinuedToWorkflowRunID != "" {
		t.Errorf("root continued_to = %q, want empty after middle reaped", gotRoot.ContinuedToWorkflowRunID)
	}
	gotLatest, err := q.GetWorkflowRun(ctx, latest.ID)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if gotLatest.ContinuedFromWorkflowRunID != "" {
		t.Errorf("latest continued_from = %q, want empty after middle reaped", gotLatest.ContinuedFromWorkflowRunID)
	}

	rootChain, err := q.GetWorkflowRunChain(ctx, root.ID, projectID, 100, "")
	if err != nil {
		t.Fatalf("GetWorkflowRunChain(root): %v", err)
	}
	if len(rootChain) != 1 || rootChain[0].ID != root.ID {
		t.Errorf("root chain after split = %+v, want [%s]", rootChain, root.ID)
	}
	latestChain, err := q.GetWorkflowRunChain(ctx, latest.ID, projectID, 100, "")
	if err != nil {
		t.Fatalf("GetWorkflowRunChain(latest): %v", err)
	}
	if len(latestChain) != 1 || latestChain[0].ID != latest.ID {
		t.Errorf("latest chain after split = %+v, want [%s]", latestChain, latest.ID)
	}
}

// TestBulkCancelWorkflowRuns_SkipsContinued proves bulk cancel treats continued
// as terminal: a continued predecessor must not be flipped to canceled (which
// would bypass the FSM and corrupt lineage), while a live sibling is canceled.
func TestBulkCancelWorkflowRuns_SkipsContinued(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-continue-bulkcancel"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})

	running := domain.WfStatusRunning
	pred := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID), Status: &running})
	succ := buildSuccessor(wf.ID, projectID, pred.ID, 1, nil)
	if err := q.ContinueWorkflowRunBootstrap(ctx, pred.ID, running, succ, nil, time.Now().UTC()); err != nil {
		t.Fatalf("continue pred->succ: %v", err)
	}

	canceled, err := q.BulkCancelWorkflowRuns(ctx, projectID, []string{pred.ID, succ.ID}, time.Now().UTC())
	if err != nil {
		t.Fatalf("BulkCancelWorkflowRuns() error = %v", err)
	}
	if len(canceled) != 1 || canceled[0] != succ.ID {
		t.Fatalf("canceled = %v, want [%s] (continued predecessor must be skipped)", canceled, succ.ID)
	}

	gotPred, err := q.GetWorkflowRun(ctx, pred.ID)
	if err != nil {
		t.Fatalf("get predecessor: %v", err)
	}
	if gotPred.Status != domain.WfStatusContinued {
		t.Errorf("predecessor status = %s, want continued (untouched by bulk cancel)", gotPred.Status)
	}
	if gotPred.ContinuedToWorkflowRunID != succ.ID {
		t.Errorf("predecessor continued_to = %q, want %q (lineage intact)", gotPred.ContinuedToWorkflowRunID, succ.ID)
	}
}

// FuzzWorkflowRunLineageRoundTrip ensures arbitrary carry-over payloads and tag
// values survive a CreateWorkflowRun/GetWorkflowRun round-trip with the new
// lineage columns populated, without corruption or panic.
func FuzzWorkflowRunLineageRoundTrip(f *testing.F) {
	f.Add([]byte("hello"), "env", "prod", 0)
	f.Add([]byte(""), "", "", 5)
	f.Add([]byte("\x00\x01\x02"), "k", "v", 100000)

	ctx := context.Background()
	if testDB == nil || testDB.Pool == nil {
		f.Skip("testDB is not initialized")
	}
	q := store.New(testDB.Pool)
	if err := testDB.CleanTables(ctx); err != nil {
		f.Fatalf("CleanTables() error = %v", err)
	}

	projectID := "project-continue-fuzz"
	wf := testutil.MustCreateWorkflow(f, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})

	f.Fuzz(func(t *testing.T, raw []byte, tagKey, tagVal string, depth int) {
		if depth < 0 {
			depth = -depth
		}
		// Wrap arbitrary bytes as a JSON string so payload is always valid JSONB.
		payload, err := json.Marshal(string(raw))
		if err != nil {
			t.Skip()
		}
		run := &domain.WorkflowRun{
			ID:           uuid.Must(uuid.NewV7()).String(),
			WorkflowID:   wf.ID,
			ProjectID:    projectID,
			Status:       domain.WfStatusPending,
			TriggeredBy:  domain.TriggerManual,
			Payload:      json.RawMessage(payload),
			LineageDepth: depth,
		}
		// Tags map to a Postgres text/jsonb column, which only accepts valid
		// UTF-8; invalid byte sequences are outside the supported domain.
		hasTag := tagKey != "" && utf8.ValidString(tagKey) && utf8.ValidString(tagVal)
		if hasTag {
			run.Tags = map[string]string{tagKey: tagVal}
		}
		if err := q.CreateWorkflowRun(ctx, run); err != nil {
			// Postgres jsonb rejects some byte sequences (e.g. NUL); those
			// inputs are not storable and are not what this round-trip exercises.
			t.Skipf("payload not storable as jsonb: %v", err)
		}
		got, err := q.GetWorkflowRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetWorkflowRun: %v", err)
		}
		if !jsonEqual(got.Payload, payload) {
			t.Errorf("payload round-trip mismatch: got %s want %s", got.Payload, payload)
		}
		if got.LineageDepth != depth {
			t.Errorf("lineage_depth round-trip mismatch: got %d want %d", got.LineageDepth, depth)
		}
		if hasTag && got.Tags[tagKey] != tagVal {
			t.Errorf("tag round-trip mismatch for %q: got %q want %q", tagKey, got.Tags[tagKey], tagVal)
		}
	})
}

// BenchmarkGetWorkflowRunChain measures one bounded page of chain navigation
// against chains of increasing depth, isolating the two cost components.
//
//   - allocations stay flat across depth on both variants: the projection plus
//     LIMIT cap mean a page never materializes more than page-size light rows,
//     regardless of how long the chain is. This is the regression guard for the
//     original defect, which materialized every run in the chain (with the heavy
//     payload, tags, and trace_context columns) on every call. A regression back
//     to full-row materialization shows up as B/op and allocs/op scaling with
//     depth.
//   - first_page latency scales with depth because the first page (empty cursor)
//     discovers the chain root by walking continued_from up from the queried run
//     over the primary key; that walk is inherently O(chain length) without a
//     denormalized root pointer.
//   - next_page latency stays flat across depth: a cursored page seeds the
//     forward walk directly at the cursor's successor via
//     idx_workflow_runs_continued_from and reads at most page-size rows, so it is
//     O(limit). This is the common paging case once navigation has started.
func BenchmarkGetWorkflowRunChain(b *testing.B) {
	ctx := context.Background()
	if testDB == nil || testDB.Pool == nil {
		b.Fatal("testDB is not initialized")
	}
	q := store.New(testDB.Pool)
	if err := testDB.CleanTables(ctx); err != nil {
		b.Fatalf("CleanTables() error = %v", err)
	}

	const pageLimit = 50

	for _, depth := range []int{10, 100, 1000} {
		projectID := "project-chain-bench-" + strconv.Itoa(depth)
		wf := testutil.MustCreateWorkflow(b, ctx, q, &testutil.WorkflowOpts{ProjectID: &projectID})

		running := domain.WfStatusRunning
		root := testutil.MustCreateWorkflowRun(b, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
			ProjectID: &projectID,
			Status:    &running,
		})

		predID := root.ID
		for i := 1; i <= depth; i++ {
			succ := buildSuccessor(wf.ID, projectID, predID, i, nil)
			if err := q.ContinueWorkflowRunBootstrap(ctx, predID, running, succ, nil, time.Now().UTC()); err != nil {
				b.Fatalf("continue depth %d: %v", i, err)
			}
			predID = succ.ID
		}
		latestID := predID

		// First page from the deep end: includes O(depth) root discovery.
		b.Run("depth="+strconv.Itoa(depth)+"/first_page", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := q.GetWorkflowRunChain(ctx, latestID, projectID, pageLimit, ""); err != nil {
					b.Fatalf("GetWorkflowRunChain: %v", err)
				}
			}
		})

		// Subsequent page seeded at the root: O(limit) regardless of depth.
		b.Run("depth="+strconv.Itoa(depth)+"/next_page", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := q.GetWorkflowRunChain(ctx, latestID, projectID, pageLimit, root.ID); err != nil {
					b.Fatalf("GetWorkflowRunChain: %v", err)
				}
			}
		})
	}
}
