package queue

import (
	"context"
	"reflect"
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

func TestQueueMetrics_AllFieldsInitialized(t *testing.T) {
	ResetMetricsForTest()
	defer ResetMetricsForTest()

	m, err := Metrics()
	if err != nil {
		t.Fatalf("Metrics() error: %v", err)
	}

	rv := reflect.ValueOf(m).Elem()
	rt := rv.Type()
	for i := range rt.NumField() {
		f := rv.Field(i)
		if f.IsNil() {
			t.Errorf("QueueMetrics.%s is nil after init", rt.Field(i).Name)
		}
	}
}

func TestQueueMetrics_NoBannedFieldNames(t *testing.T) {
	banned := map[string]struct{}{
		"ProjectID": {},
		"JobID":     {},
		"RunID":     {},
		"UserID":    {},
		"OrgID":     {},
	}

	for _, f := range reflect.VisibleFields(reflect.TypeFor[QueueMetrics]()) {
		if _, ok := banned[f.Name]; ok {
			t.Errorf("QueueMetrics should not have field %q (high-cardinality label risk)", f.Name)
		}
	}
}

func TestRecordPartitionStats_AllGaugesExercised(t *testing.T) {
	m, err := Metrics()
	if err != nil {
		t.Fatalf("Metrics(): %v", err)
	}
	m.RecordPartitionStats(context.Background(), "job_runs_p2026_04", PartitionStats{
		LiveTuples:     1000,
		DeadTuples:     100,
		TotalUpdates:   500,
		HotUpdates:     400,
		DeadTupleRatio: 0.0909,
	})
}

func TestRecordPartitionStats_ZeroLiveTuples(t *testing.T) {
	m, err := Metrics()
	if err != nil {
		t.Fatalf("Metrics(): %v", err)
	}
	m.RecordPartitionStats(context.Background(), "job_runs_empty", PartitionStats{
		LiveTuples:     0,
		DeadTuples:     0,
		TotalUpdates:   0,
		HotUpdates:     0,
		DeadTupleRatio: 0,
	})
}

func TestRecordPartitionStats_MaxValues(t *testing.T) {
	m, err := Metrics()
	if err != nil {
		t.Fatalf("Metrics(): %v", err)
	}
	m.RecordPartitionStats(context.Background(), "job_runs_big", PartitionStats{
		LiveTuples:     1<<62 - 1,
		DeadTuples:     1<<62 - 1,
		TotalUpdates:   1<<62 - 1,
		HotUpdates:     1<<62 - 1,
		DeadTupleRatio: 0.999,
	})
}

func TestPartitionMetricLabel_BoundsCardinality(t *testing.T) {
	tests := map[string]string{
		"job_runs":              "job_runs",
		"job_runs_p2026_04":     "job_runs_partition",
		"job_runs_p9999_12":     "job_runs_partition",
		"":                      "unknown",
		"customer_123_job_runs": "other",
		"job_runs_p2026_13":     "other",
		"job_runs_p2026_04_x":   "other",
	}
	for partition, want := range tests {
		if got := partitionMetricLabel(partition); got != want {
			t.Errorf("partitionMetricLabel(%q) = %q, want %q", partition, got, want)
		}
	}
}

func TestPartitionMetricLabel_FuzzSeedsCollapseUnboundedNames(t *testing.T) {
	for _, partition := range []string{
		"job_runs_p2026_04",
		"job_runs_p2027_05",
		"job_runs_p2028_06",
		"tenant_a_jobs",
		"tenant_b_jobs",
		"tenant_c_jobs",
	} {
		got := partitionMetricLabel(partition)
		if got != "job_runs_partition" && got != "other" {
			t.Errorf("partitionMetricLabel(%q) = %q, want bounded label", partition, got)
		}
	}
}

// FuzzPartitionLabelCardinality ensures arbitrary partition label values
// never cause the recorder to panic and always collapses to the bounded label set.
func FuzzPartitionLabelCardinality(f *testing.F) {
	f.Add("job_runs")
	f.Add("job_runs_p2026_04")
	f.Add("")
	f.Add("!!drop!!")
	f.Fuzz(func(t *testing.T, label string) {
		if len(label) > 256 {
			return
		}
		got := partitionMetricLabel(label)
		allowed := map[string]bool{
			"job_runs":           true,
			"job_runs_partition": true,
			"unknown":            true,
			"other":              true,
		}
		if !allowed[got] {
			t.Fatalf("partitionMetricLabel(%q) = %q, want bounded label", label, got)
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
