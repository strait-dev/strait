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
	}

	md := report.Markdown()
	for _, want := range []string{"# baseline", "Engine: `legacy`", "Duplicate claims", "`job_runs`"} {
		if !strings.Contains(md, want) {
			t.Fatalf("Markdown missing %q:\n%s", want, md)
		}
	}
}
