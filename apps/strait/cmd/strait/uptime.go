package main

import (
	"log/slog"

	"strait/internal/billing"
	"strait/internal/config"
)

// newUptimeSource picks an UptimeSource for the SLA calculator. With
// PROMETHEUS_QUERY_URL unset (community / dev / pre-prom deployments)
// the calculator pins to a 100% StaticUptimeSource so no breach ever
// fires. A configured-but-broken Prometheus endpoint falls back the
// same way after logging a warning — silently auto-issuing credits on
// telemetry that never returns would be far worse than missing a real
// breach for one tick.
func newUptimeSource(cfg *config.Config, logger *slog.Logger) billing.UptimeSource {
	if cfg.PrometheusQueryURL == "" {
		logger.Info("static_uptime_source_in_use",
			"reason", "PROMETHEUS_QUERY_URL unset",
		)
		return billing.NewStaticUptimeSource(100.0)
	}
	src, err := billing.NewPrometheusUptimeSource(cfg.PrometheusQueryURL, cfg.PrometheusUptimeQuery, logger)
	if err != nil {
		logger.Warn("prometheus uptime source unavailable; falling back to static",
			"error", err,
		)
		return billing.NewStaticUptimeSource(100.0)
	}
	logger.Info("prometheus uptime source enabled",
		"url", cfg.PrometheusQueryURL,
	)
	return src
}
