package worker

import (
	"context"
	"fmt"
	"math"
	"time"

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
func (hs *HealthScorer) RecordResult(ctx context.Context, result DispatchResult) (*domain.EndpointHealthScore, error) {
	existing, err := hs.store.GetEndpointHealthScore(ctx, result.EndpointURL)
	if err != nil {
		return nil, fmt.Errorf("get endpoint health score: %w", err)
	}

	score := hs.calculateScore(existing, result)

	if err := hs.store.UpsertEndpointHealthScore(ctx, score); err != nil {
		return nil, fmt.Errorf("upsert endpoint health score: %w", err)
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
// endpoint based on its health score.
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

// calculateScore computes the new health score from the existing score and
// a new dispatch result using EWMA.
func (hs *HealthScorer) calculateScore(existing *domain.EndpointHealthScore, result DispatchResult) *domain.EndpointHealthScore {
	now := time.Now().UTC()

	if existing == nil {
		existing = &domain.EndpointHealthScore{
			EndpointURL:  result.EndpointURL,
			HealthScore:  100.0,
			SuccessRate:  1.0,
			TimeoutRate:  0.0,
			LatencyScore: 1.0,
			CreatedAt:    now,
		}
	}

	// Compute new success rate via EWMA.
	successVal := 0.0
	if result.Success {
		successVal = 1.0
	}
	newSuccessRate := ewma(existing.SuccessRate, successVal, ewmaAlpha)

	// Compute new timeout rate via EWMA. Any failure (not just timeouts)
	// contributes to the timeout rate since a hard failure is strictly
	// worse than a timeout.
	timeoutVal := 0.0
	if !result.Success {
		timeoutVal = 1.0
	}
	newTimeoutRate := ewma(existing.TimeoutRate, timeoutVal, ewmaAlpha)

	// Compute latency score. Failed requests use worst-case latency (0.0).
	var newLatencyScore float64
	if !result.Success {
		newLatencyScore = ewma(existing.LatencyScore, 0.0, ewmaAlpha)
	} else if result.JobTimeoutMs > 0 {
		rawLatency := 1.0 - math.Min(1.0, result.LatencyMs/result.JobTimeoutMs)
		newLatencyScore = ewma(existing.LatencyScore, rawLatency, ewmaAlpha)
	} else {
		newLatencyScore = existing.LatencyScore
	}

	// Composite health score (0-100).
	compositeScore := (weightSuccessRate*newSuccessRate +
		weightTimeoutRate*(1.0-newTimeoutRate) +
		weightLatency*newLatencyScore) * 100.0

	// Clamp to [0, 100].
	compositeScore = math.Max(0, math.Min(100, compositeScore))

	return &domain.EndpointHealthScore{
		EndpointURL:   result.EndpointURL,
		HealthScore:   compositeScore,
		SuccessRate:   newSuccessRate,
		TimeoutRate:   newTimeoutRate,
		LatencyScore:  newLatencyScore,
		TotalRequests: existing.TotalRequests + 1,
		LastLatencyMs: result.LatencyMs,
		UpdatedAt:     now,
		CreatedAt:     existing.CreatedAt,
	}
}

// ewma computes an Exponentially Weighted Moving Average.
func ewma(prev, current, alpha float64) float64 {
	return alpha*current + (1-alpha)*prev
}
