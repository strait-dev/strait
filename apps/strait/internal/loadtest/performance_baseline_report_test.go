package loadtest

import (
	"strings"
	"testing"
	"time"
)

func TestNewTransactionMetric(t *testing.T) {
	metric := NewTransactionMetric("trigger", 10, 20, 250)

	if metric.TransactionsPerOp != 2 {
		t.Fatalf("TransactionsPerOp = %f, want 2", metric.TransactionsPerOp)
	}
	if metric.StatementsPerOp != 25 {
		t.Fatalf("StatementsPerOp = %f, want 25", metric.StatementsPerOp)
	}

	zero := NewTransactionMetric("empty", 0, 20, 250)
	if zero.TransactionsPerOp != 0 || zero.StatementsPerOp != 0 {
		t.Fatalf("zero operations metric = %+v, want zero ratios", zero)
	}
}

func TestNewRuntimeMetric(t *testing.T) {
	metric := NewRuntimeMetric("trigger", 10, 50, 4096, 20, 10, 5)

	if metric.AllocsPerOp != 5 {
		t.Fatalf("AllocsPerOp = %f, want 5", metric.AllocsPerOp)
	}
	if metric.BytesPerOp != 409.6 {
		t.Fatalf("BytesPerOp = %f, want 409.6", metric.BytesPerOp)
	}
	if metric.SpansPerOp != 2 {
		t.Fatalf("SpansPerOp = %f, want 2", metric.SpansPerOp)
	}
	if metric.RedisOpsPerOp != 1 {
		t.Fatalf("RedisOpsPerOp = %f, want 1", metric.RedisOpsPerOp)
	}
	if metric.LogLinesPerOp != 0.5 {
		t.Fatalf("LogLinesPerOp = %f, want 0.5", metric.LogLinesPerOp)
	}

	zero := NewRuntimeMetric("empty", 0, 50, 4096, 20, 10, 5)
	if zero.AllocsPerOp != 0 || zero.BytesPerOp != 0 || zero.SpansPerOp != 0 {
		t.Fatalf("zero operations metric = %+v, want zero ratios", zero)
	}
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
		if !strings.Contains(md, want) {
			t.Fatalf("Markdown missing %q:\n%s", want, md)
		}
	}
}

func TestDefaultPerformanceComplexityLedger(t *testing.T) {
	ledger := DefaultPerformanceComplexityLedger()
	if len(ledger) < 17 {
		t.Fatalf("ledger len = %d, want at least 17 hot paths", len(ledger))
	}

	byArea := make(map[string]ComplexityLedgerEntry, len(ledger))
	for _, entry := range ledger {
		if entry.Area == "" {
			t.Fatal("ledger entry has empty area")
		}
		if entry.Evidence == "" {
			t.Fatalf("%s evidence is empty", entry.Area)
		}
		if entry.ImprovementReason == "" {
			t.Fatalf("%s improvement reason is empty", entry.Area)
		}
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
		{area: "endpoint circuit check", current: ComplexityStatement, target: ComplexityConstant},
		{area: "health percentiles", current: ComplexityJobHistory, target: ComplexityConstant},
		{area: "rate limit checks", current: ComplexityRequest, target: ComplexityConstant},
	}
	for _, tt := range tests {
		t.Run(tt.area, func(t *testing.T) {
			entry, ok := byArea[tt.area]
			if !ok {
				t.Fatalf("missing ledger entry %q", tt.area)
			}
			if entry.Current != tt.current {
				t.Fatalf("%s current = %s, want %s", tt.area, entry.Current, tt.current)
			}
			if entry.Target != tt.target {
				t.Fatalf("%s target = %s, want %s", tt.area, entry.Target, tt.target)
			}
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
	if len(comparison.ScenarioDeltas) != 1 {
		t.Fatalf("ScenarioDeltas len = %d, want 1", len(comparison.ScenarioDeltas))
	}
	if comparison.ScenarioDeltas[0].RPSDelta != 30 {
		t.Fatalf("RPSDelta = %f, want 30", comparison.ScenarioDeltas[0].RPSDelta)
	}
	if comparison.ScenarioDeltas[0].P95Delta != -640*time.Millisecond {
		t.Fatalf("P95Delta = %s, want -640ms", comparison.ScenarioDeltas[0].P95Delta)
	}
	if comparison.SQLDeltas[0].CallsDelta != -100 {
		t.Fatalf("SQL calls delta = %d, want -100", comparison.SQLDeltas[0].CallsDelta)
	}
	if comparison.WaitDeltas[0].CountDelta != -19 {
		t.Fatalf("Wait count delta = %d, want -19", comparison.WaitDeltas[0].CountDelta)
	}
	if comparison.TransactionDeltas[0].StatementsPerOpDelta != -16 {
		t.Fatalf("StatementsPerOpDelta = %f, want -16", comparison.TransactionDeltas[0].StatementsPerOpDelta)
	}
	if comparison.RuntimeDeltas[0].AllocsPerOpDelta != -4 {
		t.Fatalf("AllocsPerOpDelta = %f, want -4", comparison.RuntimeDeltas[0].AllocsPerOpDelta)
	}
	if comparison.RuntimeDeltas[0].RedisOpsPerOpDelta != -1 {
		t.Fatalf("RedisOpsPerOpDelta = %f, want -1", comparison.RuntimeDeltas[0].RedisOpsPerOpDelta)
	}
	if len(comparison.ComplexityRegressions) != 0 {
		t.Fatalf("ComplexityRegressions = %+v, want none", comparison.ComplexityRegressions)
	}
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

	if len(comparison.ComplexityRegressions) != 1 {
		t.Fatalf("ComplexityRegressions len = %d, want 1", len(comparison.ComplexityRegressions))
	}
	if comparison.ComplexityRegressions[0].Area != "health stats" {
		t.Fatalf("regression area = %q, want health stats", comparison.ComplexityRegressions[0].Area)
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
