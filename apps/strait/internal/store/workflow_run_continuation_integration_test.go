//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
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
		chain, err := q.GetWorkflowRunChain(ctx, from)
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

	chain, err := q.GetWorkflowRunChain(ctx, run.ID)
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

	if _, err := q.GetWorkflowRunChain(ctx, newID()); !errors.Is(err, store.ErrWorkflowRunNotFound) {
		t.Fatalf("expected ErrWorkflowRunNotFound, got %v", err)
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
