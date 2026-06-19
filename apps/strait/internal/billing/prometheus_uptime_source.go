package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// promAPI is the narrow surface of promv1.API the uptime source needs.
// Defined as an interface so tests can stub Query without standing up a
// real Prometheus server.
type promAPI interface {
	Query(ctx context.Context, query string, ts time.Time, opts ...promv1.Option) (model.Value, promv1.Warnings, error)
}

func defaultPrometheusUptimeQueryTimeout() time.Duration {
	return 10 * time.Second
}

// PrometheusUptimeSource resolves monthly platform uptime by running an
// instant query against a Prometheus-compatible API at the end of the
// billing period. The query is operator-configurable; the default is
// service-level (`up{job="strait"}`) — when per-tenant uptime is needed
// the same interface accepts any PromQL that returns a percentage in
// [0, 100] for the period.
type PrometheusUptimeSource struct {
	api          promAPI
	query        string
	queryTimeout time.Duration
	logger       *slog.Logger
}

// NewPrometheusUptimeSource constructs the uptime source against the
// Prometheus query API at promURL. An empty query is rejected so the
// caller can fall back to a static source instead of silently returning
// 0% (which would auto-issue a 50% credit on every tick).
func NewPrometheusUptimeSource(promURL, query string, logger *slog.Logger) (*PrometheusUptimeSource, error) {
	if promURL == "" {
		return nil, errors.New("prometheus uptime source: empty PROMETHEUS_QUERY_URL")
	}
	if query == "" {
		return nil, errors.New("prometheus uptime source: empty PROMETHEUS_UPTIME_QUERY")
	}
	if logger == nil {
		logger = slog.Default()
	}
	client, err := api.NewClient(api.Config{Address: promURL})
	if err != nil {
		return nil, fmt.Errorf("prometheus uptime source: new client: %w", err)
	}
	return &PrometheusUptimeSource{
		api:          promv1.NewAPI(client),
		query:        query,
		queryTimeout: defaultPrometheusUptimeQueryTimeout(),
		logger:       logger,
	}, nil
}

// MonthlyUptimePct runs the configured query as an instant query at
// periodEnd. Out-of-range readings clamp to [0, 100]; a negative reading
// (almost always a clock-skew or broken-source signal) maps to 100% so
// we never auto-issue a credit on bad telemetry.
//
// The orgID parameter is intentionally ignored: Strait's `up{job="strait"}`
// metric is service-level, so every org observes the same uptime. The
// UptimeSource interface still carries orgID so a future per-tenant
// implementation (Loki / ClickHouse aggregation) can swap in without
// changing the calculator.
func (p *PrometheusUptimeSource) MonthlyUptimePct(ctx context.Context, _ string, _, periodEnd time.Time) (float64, error) {
	timeout := p.queryTimeout
	if timeout <= 0 {
		timeout = defaultPrometheusUptimeQueryTimeout()
	}
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	val, warnings, err := p.api.Query(queryCtx, p.query, periodEnd)
	if err != nil {
		return 0, fmt.Errorf("prometheus uptime query: %w", err)
	}
	for _, w := range warnings {
		p.logger.Warn("prometheus uptime query warning", "warning", w)
	}

	uptime, ok := extractUptime(val)
	if !ok {
		return 0, fmt.Errorf("prometheus uptime query: unexpected result type %s", val.Type())
	}
	if uptime < 0 {
		p.logger.Warn("prometheus uptime reading was negative; coercing to 100",
			"raw", uptime,
		)
		return 100, nil
	}
	if uptime > 100 {
		return 100, nil
	}
	return uptime, nil
}

// extractUptime unwraps numeric samples from a Prometheus instant-query
// result. Scalar and Vector are both valid shapes for
// `avg_over_time(...) * 100`; Matrix / String are not.
func extractUptime(v model.Value) (float64, bool) {
	switch x := v.(type) {
	case *model.Scalar:
		return float64(x.Value), true
	case model.Vector:
		if len(x) == 0 {
			return 0, false
		}
		var sum float64
		for _, sample := range x {
			sum += float64(sample.Value)
		}
		return sum / float64(len(x)), true
	default:
		return 0, false
	}
}
