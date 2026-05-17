package billing

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// stubPromAPI implements the narrow promAPI surface so tests do not need
// a real Prometheus server.
type stubPromAPI struct {
	value model.Value
	warns promv1.Warnings
	err   error
}

func (s stubPromAPI) Query(_ context.Context, _ string, _ time.Time, _ ...promv1.Option) (model.Value, promv1.Warnings, error) {
	return s.value, s.warns, s.err
}

func newTestUptimeSource(t *testing.T, api promAPI) *PrometheusUptimeSource {
	t.Helper()
	return &PrometheusUptimeSource{
		api:    api,
		query:  "avg_over_time(up{job=\"strait\"}[30d]) * 100",
		logger: slog.New(slog.DiscardHandler),
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
	if err != nil {
		t.Fatalf("MonthlyUptimePct: %v", err)
	}
	if got != 99.5 {
		t.Fatalf("got %v, want 99.5", got)
	}
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
	if err != nil {
		t.Fatalf("MonthlyUptimePct: %v", err)
	}
	if got != 97.25 {
		t.Fatalf("got %v, want 97.25", got)
	}
}

// TestPrometheusUptimeSource_NegativeReadingCoercesToFull guards the
// conservative path: a negative value (almost certainly broken
// telemetry) MUST NOT slip into the bottom band and silently issue a
// 50% credit. Coerce to 100.
func TestPrometheusUptimeSource_NegativeReadingCoercesToFull(t *testing.T) {
	t.Parallel()

	src := newTestUptimeSource(t, stubPromAPI{value: scalar(-3.2)})

	got, err := src.MonthlyUptimePct(context.Background(), "org-1", time.Time{}, time.Now())
	if err != nil {
		t.Fatalf("MonthlyUptimePct: %v", err)
	}
	if got != 100 {
		t.Fatalf("got %v, want 100 (negative reading should coerce)", got)
	}
}

// TestPrometheusUptimeSource_OverHundredClamps protects the upper edge:
// PromQL math (or a misconfigured query that double-multiplies by 100)
// can produce > 100. The calculator only cares about [0, 100].
func TestPrometheusUptimeSource_OverHundredClamps(t *testing.T) {
	t.Parallel()

	src := newTestUptimeSource(t, stubPromAPI{value: scalar(200)})

	got, err := src.MonthlyUptimePct(context.Background(), "org-1", time.Time{}, time.Now())
	if err != nil {
		t.Fatalf("MonthlyUptimePct: %v", err)
	}
	if got != 100 {
		t.Fatalf("got %v, want 100 (over-100 reading should clamp)", got)
	}
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
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
}

// TestPrometheusUptimeSource_EmptyQueryRejected guards the constructor:
// an empty query would resolve to NaN or 0 and silently auto-issue a
// max credit on every tick.
func TestPrometheusUptimeSource_EmptyQueryRejected(t *testing.T) {
	t.Parallel()

	if _, err := NewPrometheusUptimeSource("http://prom:9090", "", nil); err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
}

// TestPrometheusUptimeSource_EmptyURLRejected guards the constructor's
// URL check — the caller (cmd/strait) relies on this to fall back to
// the static source rather than instantiating a broken Prom client.
func TestPrometheusUptimeSource_EmptyURLRejected(t *testing.T) {
	t.Parallel()

	if _, err := NewPrometheusUptimeSource("", "avg_over_time(up[30d])*100", nil); err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
}

// TestPrometheusUptimeSource_UnexpectedTypeError covers the
// matrix/string fallthrough: an operator who configures a range query
// gets a clear error instead of a silent 0.
func TestPrometheusUptimeSource_UnexpectedTypeError(t *testing.T) {
	t.Parallel()

	src := newTestUptimeSource(t, stubPromAPI{value: model.Matrix{}})

	_, err := src.MonthlyUptimePct(context.Background(), "org-1", time.Time{}, time.Now())
	if err == nil {
		t.Fatal("expected error for unsupported result type, got nil")
	}
}
