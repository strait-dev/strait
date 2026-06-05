package billing

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

// stubPromAPI implements the narrow promAPI surface so tests do not need
// a real Prometheus server.
type stubPromAPI struct {
	value   model.Value
	warns   promv1.Warnings
	err     error
	queryFn func(ctx context.Context, query string, ts time.Time, opts ...promv1.Option) (model.Value, promv1.Warnings, error)
}

func (s stubPromAPI) Query(ctx context.Context, query string, ts time.Time, opts ...promv1.Option) (model.Value, promv1.Warnings, error) {
	if s.queryFn != nil {
		return s.queryFn(ctx, query, ts, opts...)
	}
	return s.value, s.warns, s.err
}

func newTestUptimeSource(t *testing.T, api promAPI) *PrometheusUptimeSource {
	t.Helper()
	return &PrometheusUptimeSource{
		api:          api,
		query:        "avg_over_time(up{job=\"strait\"}[30d]) * 100",
		queryTimeout: defaultPrometheusUptimeQueryTimeout,
		logger:       slog.New(slog.DiscardHandler),
	}
}

func scalar(v float64) *model.Scalar {
	return &model.Scalar{Value: model.SampleValue(v), Timestamp: model.Time(0)}
}

// TestPrometheusUptimeSource_ScalarValue verifies the happy path: a
// scalar reading in range returns verbatim.
func TestPrometheusUptimeSource_ScalarValue(t *testing.T) {
	t.Parallel()

	src := newTestUptimeSource(t, stubPromAPI{value: scalar(99.5)})

	got, err := src.MonthlyUptimePct(context.Background(), "org-1", time.Time{}, time.Now())
	require.NoError(t,
		err)
	require.InDelta(t, 99.5, got, 1e-9)
}

// TestPrometheusUptimeSource_VectorValue covers the Vector shape, which
// is what PromQL returns when the expression carries labels.
func TestPrometheusUptimeSource_VectorValue(t *testing.T) {
	t.Parallel()

	vec := model.Vector{
		{Value: model.SampleValue(97.25), Timestamp: model.Time(0), Metric: model.Metric{}},
	}
	src := newTestUptimeSource(t, stubPromAPI{value: vec})

	got, err := src.MonthlyUptimePct(context.Background(), "org-1", time.Time{}, time.Now())
	require.NoError(t,
		err)
	require.InDelta(t, 97.25, got, 1e-9)
}

func TestDeepSecPrometheusUptimeSource_VectorAveragesAllSeries(t *testing.T) {
	t.Parallel()

	vec := model.Vector{
		{Value: model.SampleValue(80), Timestamp: model.Time(0), Metric: model.Metric{"region": "iad1"}},
		{Value: model.SampleValue(100), Timestamp: model.Time(0), Metric: model.Metric{"region": "fra1"}},
	}
	src := newTestUptimeSource(t, stubPromAPI{value: vec})

	got, err := src.MonthlyUptimePct(context.Background(), "org-1", time.Time{}, time.Now())
	require.NoError(t,
		err)
	require.InDelta(t, 90, got, 1e-9)
}

// TestPrometheusUptimeSource_NegativeReadingCoercesToFull guards the
// conservative path: a negative value (almost certainly broken
// telemetry) MUST NOT slip into the bottom band and silently issue a
// 50% credit. Coerce to 100.
func TestPrometheusUptimeSource_NegativeReadingCoercesToFull(t *testing.T) {
	t.Parallel()

	src := newTestUptimeSource(t, stubPromAPI{value: scalar(-3.2)})

	got, err := src.MonthlyUptimePct(context.Background(), "org-1", time.Time{}, time.Now())
	require.NoError(t,
		err)
	require.InDelta(t, 100, got, 1e-9)
}

// TestPrometheusUptimeSource_OverHundredClamps protects the upper edge:
// PromQL math (or a misconfigured query that double-multiplies by 100)
// can produce > 100. The calculator only cares about [0, 100].
func TestPrometheusUptimeSource_OverHundredClamps(t *testing.T) {
	t.Parallel()

	src := newTestUptimeSource(t, stubPromAPI{value: scalar(200)})

	got, err := src.MonthlyUptimePct(context.Background(), "org-1", time.Time{}, time.Now())
	require.NoError(t,
		err)
	require.InDelta(t, 100, got, 1e-9)
}

// TestPrometheusUptimeSource_APIErrorWrapped guards the caller's
// failure path: when Prometheus is unreachable the SLA calculator must
// see an error so it logs + skips this tick instead of issuing a credit
// from a missing reading.
func TestPrometheusUptimeSource_APIErrorWrapped(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("prom blew up")
	src := newTestUptimeSource(t, stubPromAPI{err: sentinel})

	_, err := src.MonthlyUptimePct(context.Background(), "org-1", time.Time{}, time.Now())
	require.ErrorIs(t, err, sentinel)
}

func TestPrometheusUptimeSource_QueryReceivesBoundedDeadline(t *testing.T) {
	t.Parallel()

	var sawDeadline atomic.Bool
	const timeout = 25 * time.Millisecond
	src := newTestUptimeSource(t, stubPromAPI{
		queryFn: func(ctx context.Context, _ string, _ time.Time, _ ...promv1.Option) (model.Value, promv1.Warnings, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)

			remaining := time.Until(deadline)
			require.False(t,
				remaining <=
					0 || remaining >
					timeout,
			)

			sawDeadline.Store(true)
			return scalar(99.9), nil, nil
		},
	})
	src.queryTimeout = timeout

	got, err := src.MonthlyUptimePct(context.Background(), "org-1", time.Time{}, time.Now())
	require.NoError(t,
		err)
	require.InDelta(t, 99.9, got, 1e-9)
	require.True(t, sawDeadline.
		Load())
}

// TestPrometheusUptimeSource_EmptyQueryRejected guards the constructor:
// an empty query would resolve to NaN or 0 and silently auto-issue a
// max credit on every tick.
func TestPrometheusUptimeSource_EmptyQueryRejected(t *testing.T) {
	t.Parallel()

	if _, err := NewPrometheusUptimeSource("http://prom:9090", "", nil); err == nil {
		require.Fail(t,

			"expected error for empty query, got nil")
	}
}

// TestPrometheusUptimeSource_EmptyURLRejected guards the constructor's
// URL check — the caller (cmd/strait) relies on this to fall back to
// the static source rather than instantiating a broken Prom client.
func TestPrometheusUptimeSource_EmptyURLRejected(t *testing.T) {
	t.Parallel()

	if _, err := NewPrometheusUptimeSource("", "avg_over_time(up[30d])*100", nil); err == nil {
		require.Fail(t,

			"expected error for empty URL, got nil")
	}
}

// TestPrometheusUptimeSource_UnexpectedTypeError covers the
// matrix/string fallthrough: an operator who configures a range query
// gets a clear error instead of a silent 0.
func TestPrometheusUptimeSource_UnexpectedTypeError(t *testing.T) {
	t.Parallel()

	src := newTestUptimeSource(t, stubPromAPI{value: model.Matrix{}})

	_, err := src.MonthlyUptimePct(context.Background(), "org-1", time.Time{}, time.Now())
	require.Error(t,
		err)
}
