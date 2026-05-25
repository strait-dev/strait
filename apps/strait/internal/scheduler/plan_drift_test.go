package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"strait/internal/store"
)

type fakePlanStore struct {
	baselines   map[string]store.PlanBaselineRow
	nextExplain []byte
	explainErr  error
	upsertErr   error
	getErr      error
	upsertCalls int
}

func newFakePlanStore() *fakePlanStore {
	return &fakePlanStore{baselines: map[string]store.PlanBaselineRow{}}
}

func (f *fakePlanStore) Explain(_ context.Context, _ string) ([]byte, error) {
	return f.nextExplain, f.explainErr
}
func (f *fakePlanStore) GetPlanBaseline(_ context.Context, name string) (store.PlanBaselineRow, bool, error) {
	if f.getErr != nil {
		return store.PlanBaselineRow{}, false, f.getErr
	}
	b, ok := f.baselines[name]
	return b, ok, nil
}
func (f *fakePlanStore) UpsertPlanBaseline(_ context.Context, b store.PlanBaselineRow) error {
	f.upsertCalls++
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.baselines[b.QueryName] = b
	return nil
}

var indexScanPlan = []byte(`[{"Plan":{"Node Type":"Index Scan","Total Cost":12.5}}]`)
var seqScanPlan = []byte(`[{"Plan":{"Node Type":"Seq Scan","Total Cost":500.0}}]`)
var bitmapHeapPlan = []byte(`[{"Plan":{"Node Type":"Bitmap Heap Scan","Total Cost":42.0}}]`)
var samePlanHigherCost = []byte(`[{"Plan":{"Node Type":"Index Scan","Total Cost":50.0}}]`)
var samePlanSimilarCost = []byte(`[{"Plan":{"Node Type":"Index Scan","Total Cost":13.5}}]`)

func TestParsePlanTopNode_IndexScan(t *testing.T) {
	node, cost, err := ParsePlanTopNode(indexScanPlan)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if node != "Index Scan" || cost != 12.5 {
		t.Errorf("got (%q, %v)", node, cost)
	}
}

func TestParsePlanTopNode_SeqScan(t *testing.T) {
	node, cost, err := ParsePlanTopNode(seqScanPlan)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if node != "Seq Scan" || cost != 500.0 {
		t.Errorf("got (%q, %v)", node, cost)
	}
}

func TestParsePlanTopNode_BitmapHeap(t *testing.T) {
	node, _, err := ParsePlanTopNode(bitmapHeapPlan)
	if err != nil || node != "Bitmap Heap Scan" {
		t.Errorf("got %q %v", node, err)
	}
}

func TestParsePlanTopNode_Malformed(t *testing.T) {
	if _, _, err := ParsePlanTopNode([]byte("garbage")); err == nil {
		t.Error("expected error on garbage")
	}
	if _, _, err := ParsePlanTopNode([]byte("[]")); err == nil {
		t.Error("expected error on empty")
	}
}

func TestPlanDriftMonitor_FirstRunCapturesBaseline(t *testing.T) {
	s := newFakePlanStore()
	s.nextExplain = indexScanPlan
	m := NewPlanDriftMonitor(s, PlanDriftMonitorConfig{
		Queries: []WatchedQuery{{Name: "q1", SQL: "SELECT 1"}},
	})
	if err := m.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if m.DriftCount() != 0 {
		t.Errorf("drift count = %d, want 0 on first run", m.DriftCount())
	}
	if _, ok := s.baselines["q1"]; !ok {
		t.Error("baseline not captured")
	}
}

func TestPlanDriftMonitor_NodeTypeChangeDetected(t *testing.T) {
	s := newFakePlanStore()
	m := NewPlanDriftMonitor(s, PlanDriftMonitorConfig{
		Queries: []WatchedQuery{{Name: "q1", SQL: "SELECT 1"}},
	})
	// First run captures Index Scan.
	s.nextExplain = indexScanPlan
	_ = m.runOnce(context.Background())

	// Second run returns Seq Scan.
	s.nextExplain = seqScanPlan
	_ = m.runOnce(context.Background())

	if m.DriftCount() != 1 {
		t.Errorf("drift count = %d, want 1", m.DriftCount())
	}
	// Baseline should be updated after drift.
	b := s.baselines["q1"]
	if b.TopNodeType != "Seq Scan" {
		t.Errorf("baseline not updated: %q", b.TopNodeType)
	}
}

func TestPlanDriftMonitor_CostChangeBeyondTolerance(t *testing.T) {
	s := newFakePlanStore()
	m := NewPlanDriftMonitor(s, PlanDriftMonitorConfig{
		Queries: []WatchedQuery{{Name: "q1", SQL: "SELECT 1"}},
	})
	s.nextExplain = indexScanPlan
	_ = m.runOnce(context.Background())

	s.nextExplain = samePlanHigherCost // 12.5 -> 50 = 300% delta
	_ = m.runOnce(context.Background())

	if m.DriftCount() != 1 {
		t.Errorf("drift count = %d, want 1 (cost jump)", m.DriftCount())
	}
}

func TestPlanDriftMonitor_CostChangeWithinTolerance(t *testing.T) {
	s := newFakePlanStore()
	m := NewPlanDriftMonitor(s, PlanDriftMonitorConfig{
		Queries: []WatchedQuery{{Name: "q1", SQL: "SELECT 1"}},
	})
	s.nextExplain = indexScanPlan
	_ = m.runOnce(context.Background())

	s.nextExplain = samePlanSimilarCost // 12.5 -> 13.5 = 8% delta
	_ = m.runOnce(context.Background())

	if m.DriftCount() != 0 {
		t.Errorf("drift count = %d, want 0 (within tolerance)", m.DriftCount())
	}
}

func TestPlanDriftMonitor_SamePlanNoDrift(t *testing.T) {
	s := newFakePlanStore()
	m := NewPlanDriftMonitor(s, PlanDriftMonitorConfig{
		Queries: []WatchedQuery{{Name: "q1", SQL: "SELECT 1"}},
	})
	s.nextExplain = indexScanPlan
	_ = m.runOnce(context.Background())
	_ = m.runOnce(context.Background())
	_ = m.runOnce(context.Background())
	if m.DriftCount() != 0 {
		t.Errorf("drift = %d, want 0 (stable plan)", m.DriftCount())
	}
}

func TestPlanDriftMonitor_ExplainErrorContinues(t *testing.T) {
	s := newFakePlanStore()
	s.explainErr = errors.New("boom")
	m := NewPlanDriftMonitor(s, PlanDriftMonitorConfig{
		Queries: []WatchedQuery{
			{Name: "q1", SQL: "SELECT 1"},
			{Name: "q2", SQL: "SELECT 2"},
		},
	})
	// Should not panic and should iterate over both queries.
	_ = m.runOnce(context.Background())
	if m.Iterations() != 1 {
		t.Errorf("iterations = %d, want 1", m.Iterations())
	}
}

func TestPlanDriftMonitor_LockNotAcquired(t *testing.T) {
	s := newFakePlanStore()
	s.nextExplain = indexScanPlan
	locker := &fakeLocker{acquireOK: false}
	m := NewPlanDriftMonitor(s, PlanDriftMonitorConfig{
		Queries: []WatchedQuery{{Name: "q1", SQL: "SELECT 1"}},
	}).WithAdvisoryLocker(locker)
	_ = m.runOnce(context.Background())
	if s.upsertCalls != 0 {
		t.Errorf("should not upsert without lock")
	}
}

func TestPlanDriftMonitor_RunExitsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	s := newFakePlanStore()
	s.nextExplain = indexScanPlan
	m := NewPlanDriftMonitor(s, PlanDriftMonitorConfig{
		Interval: 5 * time.Millisecond,
		Queries:  []WatchedQuery{{Name: "q1", SQL: "SELECT 1"}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		m.Run(ctx)
		close(done)
	})
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("did not exit")
	}
}
