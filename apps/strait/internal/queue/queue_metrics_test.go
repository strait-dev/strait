package queue

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// Unit tests for queue metrics. The singleton uses a nop meter when
// no OTEL SDK is registered, which is the state during plain `go test`.

func TestMetrics_SingletonIsShared(t *testing.T) {
	a, err := Metrics()
	require.NoError(t, err)

	b, err := Metrics()
	require.NoError(t, err)
	assert.Equal(t,
		b, a)

}

func TestRecordPartitionStats_NilSafe(t *testing.T) {
	var m *QueueMetrics
	// Must not panic.
	m.RecordPartitionStats(context.Background(), "job_runs_p2026_04", PartitionStats{LiveTuples: 10})
}

func TestRecordPartitionStats_HotRatioNoDivByZero(t *testing.T) {
	m, err := Metrics()
	require.NoError(t, err)

	// TotalUpdates=0 must not panic, must not record a ratio.
	m.RecordPartitionStats(context.Background(), "job_runs_p_empty", PartitionStats{
		LiveTuples:   100,
		TotalUpdates: 0,
		HotUpdates:   0,
	})
}

func TestRecordPartitionStats_BasicRatio(t *testing.T) {
	m, err := Metrics()
	require.NoError(t, err)

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
	require.NoError(t, err)

	ResetMetricsForTest()
	m2, err := Metrics()
	require.NoError(t, err)
	assert.NotSame(t, m1, m2)

}

func TestQueueMetrics_AllFieldsInitialized(t *testing.T) {
	ResetMetricsForTest()
	defer ResetMetricsForTest()

	m, err := Metrics()
	require.NoError(t, err)

	rv := reflect.ValueOf(m).Elem()
	rt := rv.Type()
	for i := range rt.NumField() {
		f := rv.Field(i)
		assert.False(t,
			f.IsNil(),
		)

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
			assert.Failf(t, "test failure",

				"QueueMetrics should not have field %q (high-cardinality label risk)", f.Name)
		}
	}
}

func TestPgQueBackgroundErrorIncrementsBoundedCounter(t *testing.T) {
	reader := setupQueueMetricsReader(t)

	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	q.logBackgroundError(context.Background(), "ticker", "pgque ticker failed", errors.New("tick failed"))
	q.logBackgroundError(context.Background(), "route:tenant-a", "pgque custom failed", errors.New("custom failed"))
	require.EqualValues(t, 1, pgQueBackgroundErrorSum(t,
		reader,
		"ticker"))
	require.EqualValues(t, 1, pgQueBackgroundErrorSum(t,
		reader,
		"other"))

}

func setupQueueMetricsReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	oldProvider := otel.GetMeterProvider()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	ResetMetricsForTest()

	t.Cleanup(func() {
		ResetMetricsForTest()
		otel.SetMeterProvider(oldProvider)
		require.NoError(t, provider.
			Shutdown(context.Background()))

	})
	return reader
}

func pgQueBackgroundErrorSum(t *testing.T, reader *sdkmetric.ManualReader, operation string) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.
		Collect(context.
			Background(), &rm))

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "strait_queue_pgque_background_errors_total" {
				continue
			}
			data, ok := m.Data.(metricdata.Sum[int64])
			require.True(t,
				ok)

			var total int64
			for _, dp := range data.DataPoints {
				if queueMetricAttrEq(dp.Attributes, "operation", operation) {
					total += dp.Value
				}
			}
			return total
		}
	}
	return 0
}

func queueMetricAttrEq(set attribute.Set, key, want string) bool {
	got, ok := set.Value(attribute.Key(key))
	return ok && got.AsString() == want
}

func TestRecordPartitionStats_AllGaugesExercised(t *testing.T) {
	m, err := Metrics()
	require.NoError(t, err)

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
	require.NoError(t, err)

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
	require.NoError(t, err)

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
		assert.Equal(t,
			want, partitionMetricLabel(partition))

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
		assert.False(t,
			got != "job_runs_partition" &&
				got !=
					"other")

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
		require.True(t,
			allowed[got])

		m, err := Metrics()
		require.NoError(t, err)

		defer func() {
			require.Nil(t, recover())

		}()
		m.RecordPartitionStats(context.Background(), label, PartitionStats{
			LiveTuples:   10,
			DeadTuples:   1,
			TotalUpdates: 5,
			HotUpdates:   4,
		})
	})
}
