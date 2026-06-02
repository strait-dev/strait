package loadtest

import (
	"strings"
	"testing"
	"time"
)

func TestSummarizeLatencies(t *testing.T) {
	summary := SummarizeLatencies([]time.Duration{
		10 * time.Millisecond,
		1 * time.Millisecond,
		100 * time.Millisecond,
		20 * time.Millisecond,
		50 * time.Millisecond,
	})

	if summary.Count != 5 {
		t.Fatalf("Count = %d, want 5", summary.Count)
	}
	if summary.Min != time.Millisecond {
		t.Fatalf("Min = %s, want 1ms", summary.Min)
	}
	if summary.P50 != 20*time.Millisecond {
		t.Fatalf("P50 = %s, want 20ms", summary.P50)
	}
	if summary.Max != 100*time.Millisecond {
		t.Fatalf("Max = %s, want 100ms", summary.Max)
	}
}

func TestRelationBloatSampleRatios(t *testing.T) {
	sample := RelationBloatSample{
		LiveTuples:   80,
		DeadTuples:   20,
		TotalUpdates: 10,
		HOTUpdates:   7,
	}

	if got := sample.DeadTupleRatio(); got != 0.2 {
		t.Fatalf("DeadTupleRatio = %f, want 0.2", got)
	}
	if got := sample.HOTUpdateRatio(); got != 0.7 {
		t.Fatalf("HOTUpdateRatio = %f, want 0.7", got)
	}
}

func TestQueueBenchmarkReportMarkdown(t *testing.T) {
	report := QueueBenchmarkReport{
		Name:     "baseline",
		Engine:   "legacy",
		Duration: time.Second,
		Counters: QueueBenchmarkCounters{
			Enqueued:        10,
			Dequeued:        9,
			DuplicateClaims: 0,
			NotifyCount:     10,
		},
		DequeueLatency: LatencySummary{Count: 2, P50: time.Millisecond},
		Relations: []RelationBloatSample{{
			Name:         "job_runs",
			LiveTuples:   10,
			DeadTuples:   1,
			HOTUpdates:   1,
			TotalUpdates: 2,
		}},
		Plans: []SQLPlanSample{{
			Name:  "dequeue",
			Lines: []string{"Seq Scan on job_runs"},
		}},
	}

	md := report.Markdown()
	for _, want := range []string{"# baseline", "Engine: `legacy`", "Duplicate claims", "`job_runs`", "SQL Plans", "Seq Scan"} {
		if !strings.Contains(md, want) {
			t.Fatalf("Markdown missing %q:\n%s", want, md)
		}
	}
}

func TestCompareQueueBenchmarkReports(t *testing.T) {
	baseline := QueueBenchmarkReport{
		Name:     "legacy",
		Engine:   "legacy",
		Duration: 2 * time.Second,
		Counters: QueueBenchmarkCounters{
			Dequeued:            100,
			Completed:           100,
			NotifyCount:         10,
			WALBytes:            1000,
			LogicalSlotWALBytes: 900,
		},
		Relations: []RelationBloatSample{{Name: "job_runs", LiveTuples: 100, DeadTuples: 20, TotalIndexSize: 1000, TotalTableSize: 2000}},
		DequeueLatency: LatencySummary{
			P99: 10 * time.Millisecond,
		},
	}
	candidate := QueueBenchmarkReport{
		Name:     "batchlog",
		Engine:   "batchlog",
		Duration: 4 * time.Second,
		Counters: QueueBenchmarkCounters{
			Dequeued:            100,
			Completed:           100,
			NotifyCount:         5,
			WALBytes:            1200,
			LogicalSlotWALBytes: 700,
		},
		Relations: []RelationBloatSample{{Name: "job_runs", LiveTuples: 100, DeadTuples: 5, TotalIndexSize: 700, TotalTableSize: 1500}},
		DequeueLatency: LatencySummary{
			P99: 25 * time.Millisecond,
		},
	}

	comparison := CompareQueueBenchmarkReports("comparison", baseline, candidate)
	if comparison.BaselineEngine != "legacy" || comparison.CandidateEngine != "batchlog" {
		t.Fatalf("engines = %s/%s, want legacy/batchlog", comparison.BaselineEngine, comparison.CandidateEngine)
	}
	if comparison.CounterDelta.NotifyCount != -5 {
		t.Fatalf("NotifyCount delta = %d, want -5", comparison.CounterDelta.NotifyCount)
	}
	if comparison.CounterDelta.LogicalSlotWALBytes != -200 {
		t.Fatalf("LogicalSlotWALBytes delta = %d, want -200", comparison.CounterDelta.LogicalSlotWALBytes)
	}
	if comparison.P99LatencyDelta != 15*time.Millisecond {
		t.Fatalf("P99LatencyDelta = %s, want 15ms", comparison.P99LatencyDelta)
	}
	if comparison.ThroughputDelta != -25 {
		t.Fatalf("ThroughputDelta = %f, want -25", comparison.ThroughputDelta)
	}
	if len(comparison.RelationDeltas) != 1 {
		t.Fatalf("RelationDeltas len = %d, want 1", len(comparison.RelationDeltas))
	}
	delta := comparison.RelationDeltas[0]
	if delta.DeadTuplesDelta != -15 {
		t.Fatalf("DeadTuplesDelta = %d, want -15", delta.DeadTuplesDelta)
	}
	if delta.DeadTuplesPerKCompleted != -150 {
		t.Fatalf("DeadTuplesPerKCompleted = %f, want -150", delta.DeadTuplesPerKCompleted)
	}
	if len(comparison.ImprovementHints) == 0 {
		t.Fatal("expected improvement hints for slower candidate and higher WAL")
	}
}

func TestQueueBenchmarkComparisonMarkdown(t *testing.T) {
	comparison := CompareQueueBenchmarkReports("comparison", QueueBenchmarkReport{
		Engine:   "legacy",
		Duration: time.Second,
		Counters: QueueBenchmarkCounters{Dequeued: 10, Completed: 10},
		Relations: []RelationBloatSample{{
			Name: "job_runs",
		}},
	}, QueueBenchmarkReport{
		Engine:   "batchlog",
		Duration: 2 * time.Second,
		Counters: QueueBenchmarkCounters{Dequeued: 10, Completed: 10},
		Relations: []RelationBloatSample{{
			Name: "queue_entries",
		}},
		Plans: []SQLPlanSample{{
			Name:  "batchlog candidate selection",
			Lines: []string{"Nested Loop"},
		}},
	})

	md := comparison.Markdown()
	for _, want := range []string{"# comparison", "Baseline: `legacy`", "Candidate: `batchlog`", "Relation Deltas", "SQL Plans", "Nested Loop"} {
		if !strings.Contains(md, want) {
			t.Fatalf("Markdown missing %q:\n%s", want, md)
		}
	}
}
