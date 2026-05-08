package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"strait/internal/store"
)

// Query plan drift detection.
//
// A single unexpected ANALYZE that skews table statistics can flip the
// dequeue planner from Index Scan to Seq Scan silently. Production
// latency tanks but nothing alarms because the SQL is unchanged. This
// monitor captures a daily baseline of (top node type, estimated cost)
// for a watched set of hot-path queries and emits a WARN log plus a
// gauge increment when the node type changes or cost drifts beyond a
// tolerance.

const planDriftAdvisoryLockID int64 = 0x53745064526674 // "StPdRft"

// costDriftTolerance is the relative cost delta below which a cost
// change alone is not reported (node-type changes always report).
const costDriftTolerance = 0.20

// WatchedQuery describes a query the drift monitor should baseline.
type WatchedQuery struct {
	Name string
	SQL  string
}

// PlanDriftStore is the minimal store surface the monitor uses.
// PlanBaseline is an alias for store.PlanBaselineRow so the scheduler
// and store agree on the shape without a second type definition.
type PlanDriftStore interface {
	Explain(ctx context.Context, sql string) ([]byte, error)
	GetPlanBaseline(ctx context.Context, name string) (store.PlanBaselineRow, bool, error)
	UpsertPlanBaseline(ctx context.Context, b store.PlanBaselineRow) error
}

// PlanBaseline is an alias for the store row type kept for readability.
type PlanBaseline = store.PlanBaselineRow

// PlanDriftMonitor runs EXPLAIN against watched queries and reports drift.
type PlanDriftMonitor struct {
	store          PlanDriftStore
	queries        []WatchedQuery
	advisoryLocker AdvisoryLocker
	interval       time.Duration
	logger         *slog.Logger
	iterations     atomic.Int64
	driftCount     atomic.Int64
}

// PlanDriftMonitorConfig configures the monitor.
type PlanDriftMonitorConfig struct {
	Queries  []WatchedQuery
	Interval time.Duration
	Logger   *slog.Logger
}

// NewPlanDriftMonitor builds the monitor with defaults.
func NewPlanDriftMonitor(s PlanDriftStore, cfg PlanDriftMonitorConfig) *PlanDriftMonitor {
	m := &PlanDriftMonitor{
		store:    s,
		queries:  cfg.Queries,
		interval: cfg.Interval,
		logger:   cfg.Logger,
	}
	if m.interval <= 0 {
		m.interval = 24 * time.Hour
	}
	if m.logger == nil {
		m.logger = slog.Default()
	}
	return m
}

// WithAdvisoryLocker enables single-leader execution.
func (m *PlanDriftMonitor) WithAdvisoryLocker(locker AdvisoryLocker) *PlanDriftMonitor {
	m.advisoryLocker = locker
	return m
}

func (m *PlanDriftMonitor) Iterations() int64 { return m.iterations.Load() }
func (m *PlanDriftMonitor) DriftCount() int64 { return m.driftCount.Load() }

// Run blocks until ctx is cancelled.
func (m *PlanDriftMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	runSchedulerCycleCheckIn(ctx, m.interval, func() {
		_ = m.runOnce(ctx)
	})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, m.interval, func() {
				_ = m.runOnce(ctx)
			})
		}
	}
}

// RunOnceForTest exposes a single pass to integration tests.
func (m *PlanDriftMonitor) RunOnceForTest(ctx context.Context) error {
	return m.runOnce(ctx)
}

func (m *PlanDriftMonitor) runOnce(ctx context.Context) error {
	defer func() {
		m.iterations.Add(1)
		if r := recover(); r != nil {
			m.logger.Warn("plan drift monitor panic recovered", "panic", r)
		}
	}()

	if m.advisoryLocker != nil {
		acquired, err := m.advisoryLocker.TryAdvisoryLock(ctx, planDriftAdvisoryLockID)
		if err != nil {
			return err
		}
		if !acquired {
			return nil
		}
		defer func() {
			_ = m.advisoryLocker.ReleaseAdvisoryLock(ctx, planDriftAdvisoryLockID)
		}()
	}

	for _, q := range m.queries {
		if err := m.checkOne(ctx, q); err != nil {
			m.logger.Warn("plan drift check failed", "query", q.Name, "error", err)
		}
	}
	return nil
}

func (m *PlanDriftMonitor) checkOne(ctx context.Context, q WatchedQuery) error {
	planBytes, err := m.store.Explain(ctx, q.SQL)
	if err != nil {
		return fmt.Errorf("explain %s: %w", q.Name, err)
	}
	topNode, cost, err := ParsePlanTopNode(planBytes)
	if err != nil {
		return fmt.Errorf("parse %s: %w", q.Name, err)
	}
	baseline, found, err := m.store.GetPlanBaseline(ctx, q.Name)
	if err != nil {
		return fmt.Errorf("get baseline %s: %w", q.Name, err)
	}
	current := PlanBaseline{
		QueryName:    q.Name,
		TopNodeType:  topNode,
		EstTotalCost: cost,
		PlanJSON:     planBytes,
	}
	if !found {
		// First run: store baseline without alerting.
		if err := m.store.UpsertPlanBaseline(ctx, current); err != nil {
			return fmt.Errorf("upsert baseline %s: %w", q.Name, err)
		}
		return nil
	}
	// Compare.
	if baseline.TopNodeType != topNode {
		m.driftCount.Add(1)
		m.logger.Warn("plan drift: node type changed",
			"query", q.Name,
			"before", baseline.TopNodeType,
			"after", topNode,
			"cost_before", baseline.EstTotalCost,
			"cost_after", cost,
		)
		if err := m.store.UpsertPlanBaseline(ctx, current); err != nil {
			return fmt.Errorf("upsert drifted baseline: %w", err)
		}
		return nil
	}
	// Node type unchanged; check cost tolerance.
	if baseline.EstTotalCost > 0 {
		delta := math.Abs(cost-baseline.EstTotalCost) / baseline.EstTotalCost
		if delta > costDriftTolerance {
			m.driftCount.Add(1)
			m.logger.Warn("plan drift: cost changed beyond tolerance",
				"query", q.Name,
				"node", topNode,
				"cost_before", baseline.EstTotalCost,
				"cost_after", cost,
				"delta_ratio", delta,
			)
			if err := m.store.UpsertPlanBaseline(ctx, current); err != nil {
				return fmt.Errorf("upsert cost-drifted baseline: %w", err)
			}
		}
	}
	return nil
}

// ParsePlanTopNode extracts the top-level node type and estimated total
// cost from EXPLAIN (FORMAT JSON) output. Exported so tests can call it
// with canned JSON.
func ParsePlanTopNode(jsonBytes []byte) (string, float64, error) {
	var plans []struct {
		Plan struct {
			NodeType  string  `json:"Node Type"`
			TotalCost float64 `json:"Total Cost"`
		} `json:"Plan"`
	}
	if err := json.Unmarshal(jsonBytes, &plans); err != nil {
		return "", 0, fmt.Errorf("unmarshal: %w", err)
	}
	if len(plans) == 0 {
		return "", 0, fmt.Errorf("empty plan")
	}
	return plans[0].Plan.NodeType, plans[0].Plan.TotalCost, nil
}

// Compile-time assertion that *store.Queries satisfies the interface.
var _ PlanDriftStore = (*store.Queries)(nil)
