package worker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// Store error injection tests.

type failingHealthScoreStore struct {
	getErr    error
	upsertErr error
}

func (f *failingHealthScoreStore) GetEndpointHealthScore(_ context.Context, _ string) (*domain.EndpointHealthScore, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return nil, nil
}

func (f *failingHealthScoreStore) UpsertEndpointHealthScore(_ context.Context, _ *domain.EndpointHealthScore) error {
	return f.upsertErr
}

func (f *failingHealthScoreStore) AtomicRecordHealthResult(
	_ context.Context,
	_ string,
	_, _, _, _ float64,
	_, _, _ float64,
	_ float64,
) (*domain.EndpointHealthScore, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	return &domain.EndpointHealthScore{}, nil
}

func TestHealthScorer_RecordResult_GetError(t *testing.T) {
	t.Parallel()
	store := &failingHealthScoreStore{getErr: errors.New("db connection lost")}
	hs := NewHealthScorer(store)

	_, err := hs.RecordResult(context.Background(), DispatchResult{
		EndpointURL: "https://err.com/api",
		Success:     true,
	})
	require.Error(t,
		err)
	assert.True(t, errors.Is(err,
		store.
			getErr,
	))

}

func TestHealthScorer_RecordResult_UpsertError(t *testing.T) {
	t.Parallel()
	store := &failingHealthScoreStore{upsertErr: errors.New("disk full")}
	hs := NewHealthScorer(store)

	_, err := hs.RecordResult(context.Background(), DispatchResult{
		EndpointURL: "https://err.com/api",
		Success:     true,
	})
	require.Error(t,
		err)

}

func TestHealthScorer_CheckHealth_StoreError(t *testing.T) {
	t.Parallel()
	store := &failingHealthScoreStore{getErr: errors.New("timeout")}
	hs := NewHealthScorer(store)

	_, _, err := hs.CheckHealth(context.Background(), "https://err.com/api")
	require.Error(t,
		err)

}

// Boundary score value tests.

func TestHealthScorer_ExactBoundaryScores(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		healthScore float64
		wantAllowed bool
	}{
		{"score exactly 0", 0, false},
		{"score exactly 29.99", 29.99, false},
		{"score exactly 30", 30, true}, // 30 is degraded, still allowed
		{"score exactly 60", 60, true}, // 60 is degraded boundary
		{"score exactly 60.01", 60.01, true},
		{"score exactly 100", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockHealthScoreStore()
			store.scores["https://boundary.com"] = &domain.EndpointHealthScore{
				EndpointURL: "https://boundary.com",
				HealthScore: tt.healthScore,
			}
			hs := NewHealthScorer(store)

			_, allowed, err := hs.CheckHealth(context.Background(), "https://boundary.com")
			require.NoError(
				t, err)
			assert.Equal(t,
				tt.wantAllowed,
				allowed,
			)

		})
	}
}

// Multiple endpoint isolation test.

func TestHealthScorer_MultipleEndpoints_Isolated(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	// Fail endpoint A, succeed endpoint B.
	for range 100 {
		_, _ = hs.RecordResult(ctx, DispatchResult{
			EndpointURL: "https://a.com/api", Success: false, JobTimeoutMs: 1000,
		})
		_, _ = hs.RecordResult(ctx, DispatchResult{
			EndpointURL: "https://b.com/api", Success: true, LatencyMs: 10, JobTimeoutMs: 1000,
		})
	}

	scoreA, allowedA, _ := hs.CheckHealth(ctx, "https://a.com/api")
	scoreB, allowedB, _ := hs.CheckHealth(ctx, "https://b.com/api")
	assert.False(t,
		allowedA)
	assert.True(t, allowedB)
	assert.False(t,
		scoreA.HealthScore >=
			scoreB.
				HealthScore,
	)

}

// EWMA convergence mathematical verification.

func TestEWMA_ConvergenceAfterManyIterations(t *testing.T) {
	t.Parallel()
	// After many iterations of the same value, EWMA should converge to that value.
	value := 0.75
	current := 0.0
	for range 1000 {
		current = ewma(current, value, ewmaAlpha)
	}
	assert.LessOrEqual(t, math.
		Abs(current-
			value), 0.001,
	)

}

func TestEWMA_Symmetry(t *testing.T) {
	t.Parallel()
	// ewma(a, b, alpha) should equal alpha*b + (1-alpha)*a
	a, b, alpha := 0.3, 0.8, 0.1
	result := ewma(a, b, alpha)
	expected := alpha*b + (1-alpha)*a
	assert.LessOrEqual(t, math.
		Abs(result-
			expected,
		), 1e-15,
	)

}

// ThrottledConcurrency exhaustive boundary tests.

func TestThrottledConcurrency_NegativeMaxConcurrency(t *testing.T) {
	t.Parallel()
	// Negative max concurrency should pass through unchanged.
	got := ThrottledConcurrency(&domain.EndpointHealthScore{HealthScore: 50}, -5)
	assert.EqualValues(t, -5, got)

}

func TestThrottledConcurrency_MaxConcurrencyOne(t *testing.T) {
	t.Parallel()
	// With max=1 and degraded score, should still return at least 1.
	got := ThrottledConcurrency(&domain.EndpointHealthScore{HealthScore: 35}, 1)
	assert.EqualValues(t, 1, got)

}

func TestThrottledConcurrency_LargeMaxConcurrency(t *testing.T) {
	t.Parallel()
	score := &domain.EndpointHealthScore{HealthScore: 45}
	got := ThrottledConcurrency(score, 1000)
	assert.False(t,
		got <= 0 ||
			got >
				1000)
	assert.EqualValues(t, 625, got)

	// At 45, ratio = (45-30)/(60-30) = 0.5, factor = 0.25 + 0.5*0.75 = 0.625
	// throttled = ceil(1000 * 0.625) = 625

}

func TestThrottledConcurrency_ScoreExactlyAtDegradedBoundary(t *testing.T) {
	t.Parallel()
	// Score = 60.0 is the boundary between degraded and healthy.
	// healthScoreDegraded = 60.0, so score > 60 is healthy. score = 60 is degraded.
	got := ThrottledConcurrency(&domain.EndpointHealthScore{HealthScore: 60.0}, 10)
	assert.EqualValues(t, 10, got)

	// ratio = (60-30)/(60-30) = 1.0, factor = 0.25 + 1.0*0.75 = 1.0
	// throttled = ceil(10 * 1.0) = 10

}

// Score monotonicity tests.

func TestHealthScorer_ScoreMonotonicallyDecreases_OnFailures(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	prevScore := 100.0
	for range 50 {
		score, _ := hs.RecordResult(ctx, DispatchResult{
			EndpointURL: "https://decline.com/api", Success: false, JobTimeoutMs: 1000,
		})
		assert.LessOrEqual(t, score.
			HealthScore,

			prevScore+

				0.001)

		prevScore = score.HealthScore
	}
}

func TestHealthScorer_ScoreMonotonicallyIncreases_OnSuccesses(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	// First drive score down.
	for range 100 {
		_, _ = hs.RecordResult(ctx, DispatchResult{
			EndpointURL: "https://rise.com/api", Success: false, JobTimeoutMs: 1000,
		})
	}

	prevScore := 0.0
	for range 50 {
		score, _ := hs.RecordResult(ctx, DispatchResult{
			EndpointURL: "https://rise.com/api", Success: true, LatencyMs: 10, JobTimeoutMs: 1000,
		})
		assert.GreaterOrEqual(t, score.
			HealthScore,

			prevScore-
				0.001)

		prevScore = score.HealthScore
	}
}

// Latency edge cases.

func TestHealthScorer_LatencyExceedsTimeout(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	score, err := hs.RecordResult(ctx, DispatchResult{
		EndpointURL:  "https://overlatency.com/api",
		Success:      true,
		LatencyMs:    10000, // 2x the timeout
		JobTimeoutMs: 5000,
	})
	require.NoError(
		t, err)
	assert.LessOrEqual(t, score.
		LatencyScore,

		0.91)

	// latency_score = 1 - min(1, 10000/5000) = 1 - 1 = 0
	// EWMA from 1.0: 0.1*0 + 0.9*1.0 = 0.9

}

func TestHealthScorer_ZeroLatency(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	score, err := hs.RecordResult(ctx, DispatchResult{
		EndpointURL:  "https://instant.com/api",
		Success:      true,
		LatencyMs:    0,
		JobTimeoutMs: 5000,
	})
	require.NoError(
		t, err)
	assert.EqualValues(t, 1.0, score.
		LatencyScore,
	)

	// latency_score = 1 - min(1, 0/5000) = 1 - 0 = 1.0
	// EWMA stays at 1.0

}

// Concurrent multi-endpoint stress test.

func TestHealthScorer_ConcurrentMultiEndpoint(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	var wg conc.WaitGroup
	endpoints := []string{"https://a.com", "https://b.com", "https://c.com", "https://d.com"}

	for _, ep := range endpoints {
		for i := range 25 {
			wg.Go(func() {
				_, _ = hs.RecordResult(ctx, DispatchResult{
					EndpointURL:  ep,
					Success:      i%2 == 0,
					LatencyMs:    float64(i * 5),
					JobTimeoutMs: 1000,
				})
			})
		}
	}
	wg.Wait()

	// All endpoints should have valid scores.
	for _, ep := range endpoints {
		score, _, err := hs.CheckHealth(ctx, ep)
		assert.NoError(t,
			err)
		assert.False(t,
			score.HealthScore <
				0 ||
				score.HealthScore >
					100)

	}
}

// DispatchResult field combinations.

func TestHealthScorer_AllDispatchResultCombinations(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	combos := []DispatchResult{
		{EndpointURL: "https://combo.com", Success: true, TimedOut: false, LatencyMs: 100, JobTimeoutMs: 5000},
		{EndpointURL: "https://combo.com", Success: false, TimedOut: false, LatencyMs: 0, JobTimeoutMs: 5000},
		{EndpointURL: "https://combo.com", Success: false, TimedOut: true, LatencyMs: 5000, JobTimeoutMs: 5000},
		{EndpointURL: "https://combo.com", Success: true, TimedOut: false, LatencyMs: 0, JobTimeoutMs: 0},
		{EndpointURL: "https://combo.com", Success: true, TimedOut: false, LatencyMs: 1000000, JobTimeoutMs: 1},
	}

	for _, c := range combos {
		score, err := hs.RecordResult(ctx, c)
		require.NoError(
			t, err)
		assert.False(t,
			score.HealthScore <
				0 ||
				score.HealthScore >
					100)
		assert.False(t,
			score.SuccessRate <
				0 ||
				score.SuccessRate >
					1)
		assert.False(t,
			score.TimeoutRate <
				0 ||
				score.TimeoutRate >
					1)
		assert.False(t,
			score.LatencyScore <
				0 ||
				score.LatencyScore >
					1)

	}
}

// HealthLevel exhaustive test.

func TestEndpointHealthScore_HealthLevel_Exhaustive(t *testing.T) {
	t.Parallel()
	for score := 0.0; score <= 100.0; score += 0.5 {
		t.Run(fmt.Sprintf("%.1f", score), func(t *testing.T) {
			t.Parallel()
			h := &domain.EndpointHealthScore{HealthScore: score}
			level := h.HealthLevel()
			switch {
			case score < 30:
				if level != "unhealthy" {
					assert.Failf(t, "test failure",

						"score %.1f: got %q, want unhealthy", score, level)
				}
			case score <= 60:
				if level != "degraded" {
					assert.Failf(t, "test failure",

						"score %.1f: got %q, want degraded", score, level)
				}
			default:
				if level != "healthy" {
					assert.Failf(t, "test failure",

						"score %.1f: got %q, want healthy", score, level)
				}
			}
		})
	}
}
