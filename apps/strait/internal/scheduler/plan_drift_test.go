package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t,
		err)
	assert.False(t, node !=
		"Index Scan" ||
		cost !=
			12.5,
	)

}

func TestParsePlanTopNode_SeqScan(t *testing.T) {
	node, cost, err := ParsePlanTopNode(seqScanPlan)
	require.NoError(t,
		err)
	assert.False(t, node !=
		"Seq Scan" ||
		cost !=
			500.0,
	)

}

func TestParsePlanTopNode_BitmapHeap(t *testing.T) {
	node, _, err := ParsePlanTopNode(bitmapHeapPlan)
	assert.False(t, err !=
		nil ||
		node != "Bitmap Heap Scan",
	)

}

func TestParsePlanTopNode_Malformed(t *testing.T) {
	if _, _, err := ParsePlanTopNode([]byte("garbage")); err == nil {
		assert.Fail(t,

			"expected error on garbage")
	}
	if _, _, err := ParsePlanTopNode([]byte("[]")); err == nil {
		assert.Fail(t,

			"expected error on empty")
	}
}

func TestPlanDriftMonitor_FirstRunCapturesBaseline(t *testing.T) {
	s := newFakePlanStore()
	s.nextExplain = indexScanPlan
	m := NewPlanDriftMonitor(s, PlanDriftMonitorConfig{
		Queries: []WatchedQuery{{Name: "q1", SQL: "SELECT 1"}},
	})
	require.NoError(t,
		m.runOnce(
			context.Background(),
		))
	assert.EqualValues(t, 0,
		m.DriftCount())

	if _, ok := s.baselines["q1"]; !ok {
		assert.Fail(t,

			"baseline not captured")
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
	assert.EqualValues(t, 1,
		m.DriftCount())

	// Baseline should be updated after drift.
	b := s.baselines["q1"]
	assert.Equal(t, "Seq Scan",
		b.
			TopNodeType,
	)

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
	assert.EqualValues(t, 1,
		m.DriftCount())

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
	assert.EqualValues(t, 0,
		m.DriftCount())

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
	assert.EqualValues(t, 0,
		m.DriftCount())

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
	assert.EqualValues(t, 1,
		m.Iterations())

}

func TestPlanDriftMonitor_LockNotAcquired(t *testing.T) {
	s := newFakePlanStore()
	s.nextExplain = indexScanPlan
	locker := &fakeLocker{acquireOK: false}
	m := NewPlanDriftMonitor(s, PlanDriftMonitorConfig{
		Queries: []WatchedQuery{{Name: "q1", SQL: "SELECT 1"}},
	}).WithAdvisoryLocker(locker)
	_ = m.runOnce(context.Background())
	assert.EqualValues(t, 0,
		s.upsertCalls,
	)

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
		require.Fail(t, "did not exit")
	}
}
