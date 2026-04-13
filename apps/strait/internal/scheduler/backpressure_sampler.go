package scheduler

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"strait/internal/queue"
)

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
	if sampler == nil || metrics == nil || interval <= 0 {
		return nil
	}
	if sampleN <= 0 {
		sampleN = 100
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
	for _, sample := range samples {
		s.metrics.BackpressureTokensAvailable.Record(ctx, sample.Tokens,
			metric.WithAttributes(attribute.String("project_id", sample.ProjectID)))
	}
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
