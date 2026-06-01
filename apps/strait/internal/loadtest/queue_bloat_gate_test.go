package loadtest

import (
	"testing"
	"time"
)

func TestEvaluateQueueBloatGate_PassesBloatFirstCandidate(t *testing.T) {
	baseline := QueueBenchmarkReport{
		Engine: "batchlog",
		Counters: QueueBenchmarkCounters{
			Completed: 1000,
			WALBytes:  10_000,
		},
		DequeueLatency: LatencySummary{P99: 10 * time.Millisecond},
		Relations: []RelationBloatSample{{
			Name:           "queue_entries",
			LiveTuples:     1000,
			DeadTuples:     500,
			TotalTableSize: 1_000_000,
			TotalIndexSize: 500_000,
		}},
	}
	candidate := QueueBenchmarkReport{
		Engine: "pgque",
		Counters: QueueBenchmarkCounters{
			Completed: 1000,
			WALBytes:  8_000,
		},
		DequeueLatency: LatencySummary{P99: 50 * time.Millisecond},
		Relations: []RelationBloatSample{{
			Name:           "queue_entries",
			LiveTuples:     1000,
			DeadTuples:     20,
			TotalUpdates:   100,
			HOTUpdates:     80,
			TotalTableSize: 900_000,
			TotalIndexSize: 450_000,
		}},
	}

	result := EvaluateQueueBloatGate(CompareQueueBenchmarkReports("pgque", baseline, candidate), QueueBloatGate{
		MaxP99Latency:         100 * time.Millisecond,
		RequireWALImprovement: true,
		RelationGates: []RelationBloatGate{{
			Name:              "queue_entries",
			MaxDeadTupleDelta: 0,
			MaxDeadTupleRatio: 0.05,
			MinHOTUpdateRatio: 0.75,
		}},
	})

	if !result.Passed {
		t.Fatalf("gate failed: %v", result.Failures)
	}
}

func TestEvaluateQueueBloatGate_FailsWALAndBloatRegression(t *testing.T) {
	baseline := QueueBenchmarkReport{
		Engine:         "batchlog",
		Counters:       QueueBenchmarkCounters{Completed: 1000, WALBytes: 10_000},
		DequeueLatency: LatencySummary{P99: 10 * time.Millisecond},
		Relations:      []RelationBloatSample{{Name: "queue_entries", LiveTuples: 1000, DeadTuples: 20}},
	}
	candidate := QueueBenchmarkReport{
		Engine:         "pgque",
		Counters:       QueueBenchmarkCounters{Completed: 1000, DuplicateClaims: 1, WALBytes: 12_000},
		DequeueLatency: LatencySummary{P99: 150 * time.Millisecond},
		Relations:      []RelationBloatSample{{Name: "queue_entries", LiveTuples: 1000, DeadTuples: 500}},
	}

	result := EvaluateQueueBloatGate(CompareQueueBenchmarkReports("pgque", baseline, candidate), QueueBloatGate{
		MaxP99Latency:         100 * time.Millisecond,
		RequireWALImprovement: true,
		RelationGates: []RelationBloatGate{{
			Name:              "queue_entries",
			MaxDeadTupleDelta: 0,
			MaxDeadTupleRatio: 0.10,
		}},
	})

	if result.Passed {
		t.Fatal("gate passed, want failure")
	}
	if len(result.Failures) < 4 {
		t.Fatalf("failures = %v, want duplicate, latency, WAL, and bloat failures", result.Failures)
	}
}

func TestEvaluateQueueBloatGate_FailsLowHOTRatio(t *testing.T) {
	baseline := QueueBenchmarkReport{
		Engine:         "legacy",
		Counters:       QueueBenchmarkCounters{Completed: 1000, WALBytes: 10_000},
		DequeueLatency: LatencySummary{P99: 10 * time.Millisecond},
	}
	candidate := QueueBenchmarkReport{
		Engine:         "pgque",
		Counters:       QueueBenchmarkCounters{Completed: 1000, WALBytes: 8_000},
		DequeueLatency: LatencySummary{P99: 50 * time.Millisecond},
		Relations: []RelationBloatSample{{
			Name:         "job_run_state",
			LiveTuples:   1000,
			DeadTuples:   20,
			TotalUpdates: 100,
			HOTUpdates:   10,
		}},
	}

	result := EvaluateQueueBloatGate(CompareQueueBenchmarkReports("pgque", baseline, candidate), QueueBloatGate{
		MaxP99Latency:         100 * time.Millisecond,
		RequireWALImprovement: true,
		RelationGates: []RelationBloatGate{{
			Name:              "job_run_state",
			MaxDeadTupleRatio: 0.05,
			MinHOTUpdateRatio: 0.50,
		}},
	})

	if result.Passed {
		t.Fatal("gate passed, want low HOT ratio failure")
	}
	found := false
	for _, failure := range result.Failures {
		if failure == "job_run_state HOT update ratio = 0.1000, min 0.5000" {
			found = true
		}
	}
	if !found {
		t.Fatalf("failures = %v, want HOT ratio failure", result.Failures)
	}
}
