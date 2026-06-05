package loadtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSummarizeLatencies(t *testing.T) {
	summary := SummarizeLatencies([]time.Duration{
		10 * time.Millisecond,
		1 * time.Millisecond,
		100 * time.Millisecond,
		20 * time.Millisecond,
		50 * time.Millisecond,
	})
	require.InDelta(t, 5,
		summary.Count, 1e-9,
	)
	require.Equal(t, time.
		Millisecond,
		summary.
			Min)
	require.Equal(t, 20*
		time.Millisecond,

		summary.P50)
	require.Equal(t, 100*
		time.Millisecond,

		summary.Max)
}

func TestRelationBloatSampleRatios(t *testing.T) {
	sample := RelationBloatSample{
		LiveTuples:   80,
		DeadTuples:   20,
		TotalUpdates: 10,
		HOTUpdates:   7,
	}

	require.InDelta(t, 0.2, sample.DeadTupleRatio(), 1e-9)
	require.InDelta(t, 0.7, sample.HOTUpdateRatio(), 1e-9)
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
		require.Contains(t, md, want)
	}
}

func TestCompareQueueBenchmarkReports(t *testing.T) {
	baseline := QueueBenchmarkReport{
		Name:     "previous",
		Engine:   "previous",
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
		Name:     "pgque",
		Engine:   "pgque",
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
	require.False(t, comparison.
		BaselineEngine !=
		"previous" || comparison.CandidateEngine !=
		"pgque")
	require.InDelta(t, -5, comparison.
		CounterDelta.
		NotifyCount, 1e-9)
	require.InDelta(t, -200, comparison.
		CounterDelta.
		LogicalSlotWALBytes, 1e-9)
	require.Equal(t, 15*
		time.Millisecond,

		comparison.P99LatencyDelta)
	require.InDelta(t, -25, comparison.
		ThroughputDelta, 1e-9,
	)
	require.Len(t, comparison.
		RelationDeltas,

		1)

	delta := comparison.RelationDeltas[0]
	require.InDelta(t, -15, delta.DeadTuplesDelta, 1e-9)
	require.InDelta(t, -150, delta.
		DeadTuplesPerKCompleted, 1e-9,
	)
	require.NotEmpty(t,
		comparison.
			ImprovementHints,
	)
}

func TestQueueBenchmarkComparisonMarkdown(t *testing.T) {
	comparison := CompareQueueBenchmarkReports("comparison", QueueBenchmarkReport{
		Engine:   "previous",
		Duration: time.Second,
		Counters: QueueBenchmarkCounters{Dequeued: 10, Completed: 10},
		Relations: []RelationBloatSample{{
			Name: "job_runs",
		}},
	}, QueueBenchmarkReport{
		Engine:   "pgque",
		Duration: 2 * time.Second,
		Counters: QueueBenchmarkCounters{Dequeued: 10, Completed: 10},
		Relations: []RelationBloatSample{{
			Name: "queue_entries",
		}},
		Plans: []SQLPlanSample{{
			Name:  "pgque candidate selection",
			Lines: []string{"Nested Loop"},
		}},
	})

	md := comparison.Markdown()
	for _, want := range []string{"# comparison", "Baseline: `previous`", "Candidate: `pgque`", "Relation Deltas", "SQL Plans", "Nested Loop"} {
		require.Contains(t, md, want)
	}
}
