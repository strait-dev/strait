package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/config"
)

// newUptimeSource has three observable branches: no URL → static 100%,
// constructor error → static 100% with warning, success → Prometheus
// source. The table walks all three and asserts the returned implementation
// behaves the way the SLACalculator depends on (static must report 100%
// so no breach ever fires on misconfigured deployments).
func TestNewUptimeSource_Branches(t *testing.T) {
	t.Parallel()

	silentLogger := slog.New(slog.DiscardHandler)

	tests := []struct {
		name           string
		cfg            *config.Config
		wantStaticPct  float64
		wantPrometheus bool
	}{
		{
			name:          "empty url falls back to static 100%",
			cfg:           &config.Config{},
			wantStaticPct: 100.0,
		},
		{
			name: "broken prometheus client falls back to static 100%",
			cfg: &config.Config{
				PrometheusQueryURL:    "://not-a-url",
				PrometheusUptimeQuery: "up",
			},
			wantStaticPct: 100.0,
		},
		{
			name: "valid prometheus config returns the prometheus source",
			cfg: &config.Config{
				PrometheusQueryURL:    "http://prometheus.local:9090",
				PrometheusUptimeQuery: `avg_over_time(up{job="strait"}[30d]) * 100`,
			},
			wantPrometheus: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := newUptimeSource(tc.cfg, silentLogger)
			if got == nil {
				t.Fatal("newUptimeSource returned nil")
			}

			switch src := got.(type) {
			case *billing.PrometheusUptimeSource:
				if !tc.wantPrometheus {
					t.Fatalf("got *PrometheusUptimeSource, want fallback to static")
				}
			case *billing.StaticUptimeSource:
				if tc.wantPrometheus {
					t.Fatalf("got *StaticUptimeSource, want *PrometheusUptimeSource")
				}
				pct, err := src.MonthlyUptimePct(context.Background(), "any-org", time.Now(), time.Now())
				if err != nil {
					t.Fatalf("static source MonthlyUptimePct: %v", err)
				}
				if pct != tc.wantStaticPct {
					t.Errorf("static source pct = %v, want %v", pct, tc.wantStaticPct)
				}
			default:
				t.Fatalf("unexpected source type %T", got)
			}
		})
	}
}
