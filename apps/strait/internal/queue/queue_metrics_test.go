package queue

import (
	"context"
	"testing"
)

// Unit tests for queue metrics. The singleton uses a nop meter when
// no OTEL SDK is registered, which is the state during plain `go test`.

func TestMetrics_SingletonIsShared(t *testing.T) {
	a, err := Metrics()
	if err != nil {
		t.Fatalf("Metrics(): %v", err)
	}
	b, err := Metrics()
	if err != nil {
		t.Fatalf("Metrics(): %v", err)
	}
	if a != b {
		t.Error("Metrics() should return the same instance on repeat calls")
	}
}

func TestRecordPartitionStats_NilSafe(t *testing.T) {
	var m *QueueMetrics
	// Must not panic.
	m.RecordPartitionStats(context.Background(), "job_runs_p2026_04", PartitionStats{LiveTuples: 10})
}

func TestRecordPartitionStats_HotRatioNoDivByZero(t *testing.T) {
	m, err := Metrics()
	if err != nil {
		t.Fatalf("Metrics(): %v", err)
	}
	// TotalUpdates=0 must not panic, must not record a ratio.
	m.RecordPartitionStats(context.Background(), "job_runs_p_empty", PartitionStats{
		LiveTuples:   100,
		TotalUpdates: 0,
		HotUpdates:   0,
	})
}

func TestRecordPartitionStats_BasicRatio(t *testing.T) {
	m, err := Metrics()
	if err != nil {
		t.Fatalf("Metrics(): %v", err)
	}
	m.RecordPartitionStats(context.Background(), "job_runs_p2026_04", PartitionStats{
		LiveTuples:     500,
		DeadTuples:     50,
		TotalUpdates:   200,
		HotUpdates:     160,
		DeadTupleRatio: 0.0909,
	})
}

func TestResetMetricsForTest_AllowsReinit(t *testing.T) {
	m1, err := Metrics()
	if err != nil {
		t.Fatalf("Metrics(): %v", err)
	}
	ResetMetricsForTest()
	m2, err := Metrics()
	if err != nil {
		t.Fatalf("Metrics(): %v", err)
	}
	if m1 == m2 {
		t.Error("ResetMetricsForTest should yield a fresh instance")
	}
}

// FuzzPartitionLabelCardinality ensures arbitrary partition label values
// never cause the recorder to panic. Cardinality is bounded by the caller
// (who passes the Postgres relname) so we only assert safety here.
func FuzzPartitionLabelCardinality(f *testing.F) {
	f.Add("job_runs")
	f.Add("job_runs_p2026_04")
	f.Add("")
	f.Add("!!drop!!")
	f.Fuzz(func(t *testing.T, label string) {
		if len(label) > 256 {
			return
		}
		m, err := Metrics()
		if err != nil {
			t.Fatalf("Metrics(): %v", err)
		}
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("RecordPartitionStats panicked on %q: %v", label, r)
			}
		}()
		m.RecordPartitionStats(context.Background(), label, PartitionStats{
			LiveTuples:   10,
			DeadTuples:   1,
			TotalUpdates: 5,
			HotUpdates:   4,
		})
	})
}
