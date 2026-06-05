package worker

import (
	"context"
	"fmt"
	"math"

	"strait/internal/domain"
)

const (
	// EWMA smoothing factor: lower values weight history more heavily.
	ewmaAlpha = 0.1

	// Health score thresholds.
	healthScoreUnhealthy = 30.0
	healthScoreDegraded  = 60.0

	// Throttle factor: reduce concurrency to 25% for degraded endpoints.
	degradedConcurrencyFactor = 0.25

	// Score component weights.
	weightSuccessRate = 0.5
	weightTimeoutRate = 0.3
	weightLatency     = 0.2
)

// HealthScoreStore defines the storage interface for endpoint health scores.
type HealthScoreStore interface {
	GetEndpointHealthScore(ctx context.Context, endpointURL string) (*domain.EndpointHealthScore, error)
	UpsertEndpointHealthScore(ctx context.Context, score *domain.EndpointHealthScore) error
	AtomicRecordHealthResult(
		ctx context.Context,
		endpointURL string,
		successVal, timeoutVal, latencyVal, alpha float64,
		weightSuccess, weightTimeout, weightLatency float64,
		lastLatencyMs float64,
	) (*domain.EndpointHealthScore, error)
}

// DispatchResult captures the outcome of a dispatch for health score calculation.
type DispatchResult struct {
	EndpointURL  string
	Success      bool
	TimedOut     bool
	LatencyMs    float64
	JobTimeoutMs float64
}

// HealthScorer calculates and manages endpoint health scores using EWMA.
type HealthScorer struct {
	store HealthScoreStore
}

// NewHealthScorer creates a new HealthScorer.
func NewHealthScorer(store HealthScoreStore) *HealthScorer {
	return &HealthScorer{store: store}
}

// RecordResult updates the health score for an endpoint based on a dispatch result.
// It pre-computes the raw signal values in Go and delegates the EWMA computation
// to an atomic SQL statement that prevents lost updates under concurrent writes.
func (hs *HealthScorer) RecordResult(ctx context.Context, result DispatchResult) (*domain.EndpointHealthScore, error) {
	successVal := 0.0
	if result.Success {
		successVal = 1.0
	}

	timeoutVal := 0.0
	if !result.Success {
		timeoutVal = 1.0
	}

	var latencyVal float64
	if !result.Success {
		latencyVal = 0.0
	} else if result.JobTimeoutMs > 0 {
		latencyVal = 1.0 - math.Min(1.0, result.LatencyMs/result.JobTimeoutMs)
	} else {
		// No timeout configured: preserve existing latency score by using 1.0
		// (the default initial value). The EWMA will keep it stable.
		latencyVal = 1.0
	}

	score, err := hs.store.AtomicRecordHealthResult(
		ctx,
		result.EndpointURL,
		successVal, timeoutVal, latencyVal, ewmaAlpha,
		weightSuccessRate, weightTimeoutRate, weightLatency,
		result.LatencyMs,
	)
	if err != nil {
		return nil, fmt.Errorf("record health result: %w", err)
	}

	return score, nil
}

// CheckHealth retrieves the current health status for an endpoint.
// Returns the health score and whether the endpoint is allowed to receive traffic.
func (hs *HealthScorer) CheckHealth(ctx context.Context, endpointURL string) (*domain.EndpointHealthScore, bool, error) {
	score, err := hs.store.GetEndpointHealthScore(ctx, endpointURL)
	if err != nil {
		return nil, false, fmt.Errorf("get endpoint health score: %w", err)
	}

	// No score recorded yet: endpoint is healthy by default.
	if score == nil {
		return &domain.EndpointHealthScore{
			EndpointURL:  endpointURL,
			HealthScore:  100.0,
			SuccessRate:  1.0,
			LatencyScore: 1.0,
		}, true, nil
	}

	// Unhealthy endpoints are blocked.
	if score.HealthScore < healthScoreUnhealthy {
		return score, false, nil
	}

	return score, true, nil
}

// ThrottledConcurrency returns the effective max concurrency for a given
// endpoint based on its health score. A maxConcurrency of 0 means "no limit"
// and is returned unchanged.
func ThrottledConcurrency(score *domain.EndpointHealthScore, maxConcurrency int) int {
	if maxConcurrency <= 0 {
		return maxConcurrency
	}
	if score == nil || score.HealthScore > healthScoreDegraded {
		return maxConcurrency
	}

	if score.HealthScore < healthScoreUnhealthy {
		return 0
	}

	// Degraded: scale concurrency proportionally between 25% and 100%.
	ratio := (score.HealthScore - healthScoreUnhealthy) / (healthScoreDegraded - healthScoreUnhealthy)
	factor := degradedConcurrencyFactor + ratio*(1.0-degradedConcurrencyFactor)
	throttled := int(math.Ceil(float64(maxConcurrency) * factor))
	return max(throttled, 1)
}

// ewma computes an Exponentially Weighted Moving Average.
func ewma(prev, current, alpha float64) float64 {
	return alpha*current + (1-alpha)*prev
}
