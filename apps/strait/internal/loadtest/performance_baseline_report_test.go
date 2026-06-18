package loadtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewTransactionMetric(t *testing.T) {
	metric := NewTransactionMetric("trigger", 10, 20, 250)
	require.InDelta(t, 2,
		metric.TransactionsPerOp, 1e-9,
	)
	require.InDelta(t, 25,
		metric.StatementsPerOp, 1e-9,
	)

	zero := NewTransactionMetric("empty", 0, 20, 250)
	require.False(t, zero.
		TransactionsPerOp !=
		0 || zero.StatementsPerOp != 0)
}

func TestNewRuntimeMetric(t *testing.T) {
	metric := NewRuntimeMetric("trigger", 10, 50, 4096, 20, 10, 5)
	require.InDelta(t, 5,
		metric.AllocsPerOp, 1e-9,
	)
	require.InDelta(t, 409.6,
		metric.
			BytesPerOp, 1e-9,
	)
	require.InDelta(t, 2,
		metric.SpansPerOp, 1e-9,
	)
	require.InDelta(t, 1,
		metric.RedisOpsPerOp, 1e-9,
	)
	require.InDelta(t, 0.5,
		metric.
			LogLinesPerOp, 1e-9,
	)

	zero := NewRuntimeMetric("empty", 0, 50, 4096, 20, 10, 5)
	require.False(t, zero.
		AllocsPerOp !=
		0 ||
		zero.BytesPerOp != 0 || zero.SpansPerOp != 0,
	)
}

func TestPerformanceBaselineReportMarkdown(t *testing.T) {
	report := PerformanceBaselineReport{
		Name:     "phase-1",
		Duration: time.Second,
		Scenarios: []ScenarioMetric{{
			Name:      "core-api",
			RPS:       150,
			ErrorRate: 0.01,
			Latency: LatencySummary{
				Count: 100,
				P50:   10 * time.Millisecond,
				P95:   100 * time.Millisecond,
				P99:   250 * time.Millisecond,
			},
		}},
		SQL: []SQLStatementMetric{{
			Name:       "trigger advisory lock",
			QueryMatch: "pg_advisory_xact_lock",
			Calls:      50,
			TotalTime:  5 * time.Second,
			MeanTime:   100 * time.Millisecond,
			P95Time:    500 * time.Millisecond,
			WALBytes:   1024,
		}},
		Runtime: []RuntimeMetric{
			NewRuntimeMetric("trigger", 10, 50, 4096, 20, 10, 5),
		},
		Complexity: []ComplexityLedgerEntry{{
			Area:     "trigger admission",
			Current:  ComplexityJobHistory,
			Target:   ComplexityConstant,
			Evidence: "CountRunsForJobSince scans job history",
		}},
	}

	md := report.Markdown()
	for _, want := range []string{
		"# phase-1",
		"core-api",
		"trigger advisory lock",
		"Runtime",
		"Redis ops/op",
		"Complexity Ledger",
		"O(job_history)",
	} {
		require.Contains(t, md, want)
	}
}

func TestDefaultPerformanceComplexityLedger(t *testing.T) {
	ledger := DefaultPerformanceComplexityLedger()
	require.GreaterOrEqual(t, len(ledger),

		17,
	)

	byArea := make(map[string]ComplexityLedgerEntry, len(ledger))
	for _, entry := range ledger {
		require.NotEmpty(t,
			entry.
				Area)
		require.NotEmpty(t,
			entry.
				Evidence,
		)
		require.NotEmpty(t,
			entry.
				ImprovementReason,
		)

		byArea[entry.Area] = entry
	}

	tests := []struct {
		area    string
		current ComplexityClass
		target  ComplexityClass
	}{
		{area: "trigger admission", current: ComplexityJobHistory, target: ComplexityConstant},
		{area: "enqueue idempotency", current: ComplexityProjectActive, target: ComplexityConstant},
		{area: "job health stats", current: ComplexityJobHistory, target: ComplexityConstant},
		{area: "workflow progression", current: ComplexityWorkflowSteps, target: ComplexityBatch},
		{area: "endpoint circuit check", current: ComplexityConstant, target: ComplexityConstant},
		{area: "health percentiles", current: ComplexityJobHistory, target: ComplexityConstant},
		{area: "rate limit checks", current: ComplexityRequest, target: ComplexityConstant},
	}
	for _, tt := range tests {
		t.Run(tt.area, func(t *testing.T) {
			entry, ok := byArea[tt.area]
			require.True(t, ok)
			require.Equal(t, tt.
				current,
				entry.Current,
			)
			require.Equal(t, tt.
				target, entry.
				Target,
			)
		})
	}
}

func TestComparePerformanceBaselineReports(t *testing.T) {
	baseline := PerformanceBaselineReport{
		Name: "baseline",
		Scenarios: []ScenarioMetric{{
			Name:      "core-api",
			RPS:       150,
			ErrorRate: 0.02,
			Latency: LatencySummary{
				Count: 100,
				P95:   700 * time.Millisecond,
				P99:   time.Second,
			},
		}},
		SQL: []SQLStatementMetric{{
			Name:      "trigger advisory lock",
			Calls:     100,
			TotalTime: 10 * time.Second,
			MeanTime:  100 * time.Millisecond,
			WALBytes:  10,
		}},
		Waits: []WaitMetric{{
			Name:  "pool.acquire",
			Count: 20,
			Total: 2 * time.Second,
			P95:   200 * time.Millisecond,
		}},
		Transactions: []TransactionMetric{
			NewTransactionMetric("trigger", 10, 60, 250),
		},
		Runtime: []RuntimeMetric{
			NewRuntimeMetric("trigger", 10, 50, 4096, 20, 10, 5),
		},
		Bloat: []RelationBloatSample{{
			Name:           "job_runs",
			LiveTuples:     100,
			DeadTuples:     20,
			TotalTableSize: 2000,
			TotalIndexSize: 1000,
		}},
		Complexity: []ComplexityLedgerEntry{{
			Area:    "trigger admission",
			Current: ComplexityJobHistory,
			Target:  ComplexityConstant,
		}},
	}
	candidate := PerformanceBaselineReport{
		Name: "candidate",
		Scenarios: []ScenarioMetric{{
			Name:      "core-api",
			RPS:       180,
			ErrorRate: 0,
			Latency: LatencySummary{
				Count: 100,
				P95:   60 * time.Millisecond,
				P99:   90 * time.Millisecond,
			},
		}},
		SQL: []SQLStatementMetric{{
			Name:      "trigger advisory lock",
			Calls:     0,
			TotalTime: 0,
			MeanTime:  0,
			WALBytes:  4,
		}},
		Waits: []WaitMetric{{
			Name:  "pool.acquire",
			Count: 1,
			Total: 5 * time.Millisecond,
			P95:   5 * time.Millisecond,
		}},
		Transactions: []TransactionMetric{
			NewTransactionMetric("trigger", 10, 10, 90),
		},
		Runtime: []RuntimeMetric{
			NewRuntimeMetric("trigger", 10, 10, 1024, 3, 0, 1),
		},
		Bloat: []RelationBloatSample{{
			Name:           "job_runs",
			LiveTuples:     100,
			DeadTuples:     5,
			TotalTableSize: 1500,
			TotalIndexSize: 800,
		}},
		Complexity: []ComplexityLedgerEntry{{
			Area:    "trigger admission",
			Current: ComplexityConstant,
			Target:  ComplexityConstant,
		}},
	}

	comparison := ComparePerformanceBaselineReports("delta", baseline, candidate)
	require.Len(t, comparison.
		ScenarioDeltas,

		1)
	require.InDelta(t, 30,
		comparison.
			ScenarioDeltas[0].RPSDelta, 1e-9)
	require.Equal(t, -640*time.
		Millisecond,

		comparison.ScenarioDeltas[0].P95Delta)
	require.InDelta(t, -100, comparison.
		SQLDeltas[0].CallsDelta, 1e-9)
	require.InDelta(t, -19, comparison.
		WaitDeltas[0].CountDelta, 1e-9)
	require.InDelta(t, -16, comparison.
		TransactionDeltas[0].StatementsPerOpDelta, 1e-9)
	require.InDelta(t, -4, comparison.
		RuntimeDeltas[0].AllocsPerOpDelta, 1e-9)
	require.InDelta(t, -1, comparison.
		RuntimeDeltas[0].RedisOpsPerOpDelta, 1e-9)
	require.Empty(t, comparison.
		ComplexityRegressions)
}

func TestComparePerformanceBaselineReports_ComplexityRegression(t *testing.T) {
	comparison := ComparePerformanceBaselineReports("regression", PerformanceBaselineReport{
		Complexity: []ComplexityLedgerEntry{{
			Area:   "health stats",
			Target: ComplexityConstant,
		}},
	}, PerformanceBaselineReport{
		Complexity: []ComplexityLedgerEntry{{
			Area:    "health stats",
			Current: ComplexityJobHistory,
			Target:  ComplexityConstant,
		}},
	})
	require.Len(t, comparison.
		ComplexityRegressions,

		1)
	require.Equal(t, "health stats",

		comparison.
			ComplexityRegressions[0].Area)
}

func TestPerformanceBaselineComparisonMarkdown(t *testing.T) {
	comparison := PerformanceBaselineComparison{
		Name:      "phase-delta",
		Baseline:  PerformanceBaselineReport{Name: "baseline"},
		Candidate: PerformanceBaselineReport{Name: "candidate"},
		ScenarioDeltas: []ScenarioDelta{{
			Name:           "core-api",
			RPSDelta:       30,
			ErrorRateDelta: -0.01,
			P95Delta:       -640 * time.Millisecond,
			P99Delta:       -910 * time.Millisecond,
		}},
		SQLDeltas: []SQLStatementDelta{{
			Name:           "trigger advisory lock",
			CallsDelta:     -100,
			TotalTimeDelta: -10 * time.Second,
			MeanTimeDelta:  -100 * time.Millisecond,
			WALBytesDelta:  -6,
		}},
		TransactionDeltas: []TransactionDelta{{
			Name:                   "trigger",
			TransactionsPerOpDelta: -5,
			StatementsPerOpDelta:   -16,
		}},
		RuntimeDeltas: []RuntimeDelta{{
			Name:               "trigger",
			AllocsPerOpDelta:   -4,
			BytesPerOpDelta:    -307.2,
			SpansPerOpDelta:    -1.7,
			RedisOpsPerOpDelta: -1,
			LogLinesPerOpDelta: -0.4,
		}},
		ComplexityRegressions: []ComplexityLedgerEntry{{
			Area:    "health stats",
			Current: ComplexityJobHistory,
			Target:  ComplexityConstant,
		}},
	}

	md := comparison.Markdown()
	for _, want := range []string{
		"# phase-delta",
		"Baseline: `baseline`",
		"Scenario Deltas",
		"| `core-api` | 30.00 | -1.000 | -640ms | -910ms |",
		"SQL Deltas",
		"| `trigger advisory lock` | -100 | -10s | -100ms | -6 |",
		"Transaction Deltas",
		"| `trigger` | -5.00 | -16.00 |",
		"Runtime Deltas",
		"| `trigger` | -4.00 | -307.20 | -1.70 | -1.00 | -0.40 |",
		"Complexity Regressions",
		"`health stats`: current `O(job_history)`, target `O(1)`",
	} {
		require.Contains(t, md, want)
	}
}

func BenchmarkPerformanceBaselineReportMarkdown(b *testing.B) {
	report := PerformanceBaselineReport{
		Name:     "benchmark",
		Duration: time.Minute,
		Scenarios: []ScenarioMetric{
			{Name: "core-api", RPS: 150, Latency: LatencySummary{Count: 1000, P50: 10 * time.Millisecond, P95: 50 * time.Millisecond, P99: 100 * time.Millisecond}},
			{Name: "workflow", RPS: 15, Latency: LatencySummary{Count: 1000, P50: 20 * time.Millisecond, P95: 70 * time.Millisecond, P99: 120 * time.Millisecond}},
		},
		SQL: []SQLStatementMetric{
			{Name: "trigger admission", Calls: 1000, TotalTime: time.Second, MeanTime: time.Millisecond, P95Time: 5 * time.Millisecond},
			{Name: "health stats", Calls: 2000, TotalTime: 4 * time.Second, MeanTime: 2 * time.Millisecond, P95Time: 10 * time.Millisecond},
		},
		Complexity: []ComplexityLedgerEntry{
			{Area: "trigger admission", Current: ComplexityJobHistory, Target: ComplexityConstant, Evidence: "history scan"},
			{Area: "executor job load", Current: ComplexityConstant, Target: ComplexityConstant, Evidence: "singleflight cache"},
		},
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = report.Markdown()
	}
}
