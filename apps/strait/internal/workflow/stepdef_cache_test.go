package workflow

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestStepDefCache_DisabledAlwaysMisses(t *testing.T) {
	t.Parallel()
	c := newStepDefCache(0)
	c.set("k", []domain.WorkflowStep{{StepRef: "a"}})
	if _, ok := c.get("k"); ok {
		t.Fatal("disabled cache returned a hit")
	}
}

func TestStepDefCache_RoundTrip(t *testing.T) {
	t.Parallel()
	c := newStepDefCache(stepDefCacheTTL)
	want := []domain.WorkflowStep{{StepRef: "build"}, {StepRef: "deploy"}}
	c.set("k", want)
	got, ok := c.get("k")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 2 || got[0].StepRef != "build" || got[1].StepRef != "deploy" {
		t.Fatalf("unexpected cached value: %+v", got)
	}
}

func TestStepDefCacheKey_SnapshotAndVersionNamespacesDoNotCollide(t *testing.T) {
	t.Parallel()
	snapKey := stepDefCacheKey(&domain.WorkflowRun{WorkflowSnapshotID: "s1"})
	verKey := stepDefCacheKey(&domain.WorkflowRun{WorkflowID: "s1", WorkflowVersion: 1})
	if snapKey == verKey {
		t.Fatalf("snapshot and version keys collided: %q", snapKey)
	}
}

func TestLoadStepDefinitions_MemoizesSnapshotLoad(t *testing.T) {
	t.Parallel()

	def := domain.WorkflowSnapshotDefinition{
		Workflow: domain.WorkflowSnapshotMeta{ID: "wf-1"},
		Steps:    []domain.WorkflowStep{{StepRef: "build", StepType: domain.WorkflowStepTypeJob}},
	}
	defJSON, _ := json.Marshal(def)

	var snapshotLoads atomic.Int64
	ms := &snapshotMockCallbackStore{
		mockCallbackStore: &mockCallbackStore{},
		getWorkflowSnapshotFn: func(_ context.Context, id string) (*domain.WorkflowSnapshot, error) {
			snapshotLoads.Add(1)
			return &domain.WorkflowSnapshot{ID: id, WorkflowID: "wf-1", Definition: defJSON}, nil
		},
	}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, WorkflowSnapshotID: "snap-1"}

	for range 3 {
		steps, err := cb.loadStepDefinitions(context.Background(), wfRun)
		if err != nil {
			t.Fatalf("loadStepDefinitions() error = %v", err)
		}
		if len(steps) != 1 || steps[0].StepRef != "build" {
			t.Fatalf("unexpected steps: %+v", steps)
		}
	}

	if n := snapshotLoads.Load(); n != 1 {
		t.Fatalf("snapshot loaded %d times, want 1 (cache miss only on first call)", n)
	}
}

func TestLoadStepDefinitions_MemoizesVersionFallback(t *testing.T) {
	t.Parallel()

	var versionLoads atomic.Int64
	ms := &mockCallbackStore{
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			versionLoads.Add(1)
			return []domain.WorkflowStep{{StepRef: "test", StepType: domain.WorkflowStepTypeJob}}, nil
		},
	}

	cb := newTestCallback(ms)
	wfRun := &domain.WorkflowRun{ID: "wr-2", WorkflowID: "wf-1", WorkflowVersion: 2}

	for range 3 {
		if _, err := cb.loadStepDefinitions(context.Background(), wfRun); err != nil {
			t.Fatalf("loadStepDefinitions() error = %v", err)
		}
	}

	if n := versionLoads.Load(); n != 1 {
		t.Fatalf("version steps loaded %d times, want 1", n)
	}
}

func TestLoadSchedulingState_DerivesSetsFromSingleRead(t *testing.T) {
	t.Parallel()

	all := []domain.WorkflowStepRun{
		{ID: "r1", StepRef: "running", Status: domain.StepRunning, DepsCompleted: 1, DepsRequired: 1},
		{ID: "r2", StepRef: "ready", Status: domain.StepPending, DepsCompleted: 2, DepsRequired: 2},
		{ID: "r3", StepRef: "waiting-ready", Status: domain.StepWaiting, DepsCompleted: 1, DepsRequired: 1},
		{ID: "r4", StepRef: "blocked", Status: domain.StepPending, DepsCompleted: 0, DepsRequired: 1},
		{ID: "r5", StepRef: "done", Status: domain.StepCompleted, DepsCompleted: 1, DepsRequired: 1},
	}

	var reads atomic.Int64
	ms := &mockCallbackStore{
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			reads.Add(1)
			return all, nil
		},
	}

	cb := newTestCallback(ms)
	statuses, running, runnable, err := cb.loadSchedulingState(context.Background(), "wr-1")
	if err != nil {
		t.Fatalf("loadSchedulingState() error = %v", err)
	}
	if n := reads.Load(); n != 1 {
		t.Fatalf("step runs read %d times, want 1", n)
	}
	if len(statuses) != 5 {
		t.Fatalf("status map size = %d, want 5", len(statuses))
	}
	if len(running) != 1 || running[0].StepRef != "running" {
		t.Fatalf("running set = %+v, want [running]", running)
	}
	if len(runnable) != 2 {
		t.Fatalf("runnable set size = %d, want 2 (ready, waiting-ready)", len(runnable))
	}
	gotRunnable := map[string]bool{}
	for _, sr := range runnable {
		gotRunnable[sr.StepRef] = true
	}
	if !gotRunnable["ready"] || !gotRunnable["waiting-ready"] {
		t.Fatalf("runnable refs = %v, want {ready, waiting-ready}", gotRunnable)
	}
}
