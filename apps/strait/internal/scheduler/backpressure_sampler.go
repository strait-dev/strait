package scheduler

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"strait/internal/queue"
)

// maxBackpressureSampleN is a hard cap on the number of token-balance samples
// recorded per tick. Samples are recorded without project_id labels; the cap
// bounds work per cycle while keeping metrics cardinality independent of
// tenant/project creation.
const maxBackpressureSampleN = 1000

// BackpressureTokenSampler is the minimal surface of *queue.Backpressure
// consumed by the BackpressureSampler scheduler component. Extracted to
// an interface so tests can inject a fake without spinning up Postgres.
type BackpressureTokenSampler interface {
	SampleAvailableTokens(ctx context.Context, sampleN int) ([]queue.TokenSample, error)
}

// BackpressureSampler periodically reads per-project backpressure token
// balances and records them on strait.queue.backpressure_tokens_available.
// Without this loop the gauge has no producer and dashboards render
// empty, even though the token-bucket state is being maintained
// correctly in project_rate_limits.
type BackpressureSampler struct {
	sampler  BackpressureTokenSampler
	metrics  *queue.QueueMetrics
	interval time.Duration
	sampleN  int
	logger   *slog.Logger
}

// NewBackpressureSampler builds a sampler. When interval <= 0 the
// component returns nil; callers should treat that as "disabled" and
// skip registration. A nil sampler or metrics struct is also treated as
// disabled so wiring is tolerant of partial configuration in tests.
func NewBackpressureSampler(sampler BackpressureTokenSampler, metrics *queue.QueueMetrics, interval time.Duration, sampleN int) *BackpressureSampler {
	if sampler == nil {
		return nil
	}
	if metrics == nil {
		return nil
	}
	if interval <= 0 {
		return nil
	}
	if sampleN <= 0 {
		sampleN = 100
	}
	if sampleN > maxBackpressureSampleN {
		sampleN = maxBackpressureSampleN
	}
	return &BackpressureSampler{
		sampler:  sampler,
		metrics:  metrics,
		interval: interval,
		sampleN:  sampleN,
		logger:   slog.Default(),
	}
}

// Run blocks until ctx is cancelled, sampling once per interval.
func (s *BackpressureSampler) Run(ctx context.Context) {
	if s == nil {
		return
	}
	loop := NewMaintenanceLoop("backpressure_sampler", s.interval, s.logger, func(loopCtx context.Context) {
		s.sampleOnce(loopCtx)
	})
	loop.Run(ctx)
}

// sampleOnce is exported-for-test via the exported Tick helper.
func (s *BackpressureSampler) sampleOnce(ctx context.Context) {
	samples, err := s.sampler.SampleAvailableTokens(ctx, s.sampleN)
	if err != nil {
		s.logger.Warn("backpressure sampler: failed to sample tokens", "error", err)
		return
	}
	if s.metrics.BackpressureTokensAvailable == nil {
		return
	}
	// Defensive second cap: if the sampler returned more rows than
	// sampleN (shouldn't happen, but the interface doesn't enforce it),
	// keep only the N projects with the lowest available tokens. Those
	// are the ones operators actually want to see on a backpressure
	// dashboard, and bounding the slice here keeps label cardinality on
	// strait.queue.backpressure_tokens_available predictable.
	if len(samples) > s.sampleN {
		sort.Slice(samples, func(i, j int) bool {
			return samples[i].Tokens < samples[j].Tokens
		})
		samples = samples[:s.sampleN]
	}
	for _, sample := range samples {
		s.metrics.BackpressureTokensAvailable.Record(ctx, sample.Tokens, metric.WithAttributes(backpressureMetricAttributes()...))
	}
}

func backpressureMetricAttributes() []attribute.KeyValue {
	return nil
}

// Tick runs a single sampling pass. Exposed for deterministic unit
// tests that drive the loop manually instead of waiting on a real
// ticker.
func (s *BackpressureSampler) Tick(ctx context.Context) {
	if s == nil {
		return
	}
	s.sampleOnce(ctx)
}
