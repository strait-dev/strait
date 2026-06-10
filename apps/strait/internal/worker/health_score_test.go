package worker

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

type mockHealthScoreStore struct {
	mu          sync.Mutex
	scores      map[string]*domain.EndpointHealthScore
	recordCalls int
}

func newMockHealthScoreStore() *mockHealthScoreStore {
	return &mockHealthScoreStore{scores: make(map[string]*domain.EndpointHealthScore)}
}

func (m *mockHealthScoreStore) GetEndpointHealthScore(_ context.Context, endpointURL string) (*domain.EndpointHealthScore, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.scores[endpointURL]
	if !ok {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}

func (m *mockHealthScoreStore) UpsertEndpointHealthScore(_ context.Context, score *domain.EndpointHealthScore) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *score
	m.scores[score.EndpointURL] = &cp
	return nil
}

func (m *mockHealthScoreStore) AtomicRecordHealthResult(
	_ context.Context,
	endpointURL string,
	successVal, timeoutVal, latencyVal, alpha float64,
	wSuccess, wTimeout, wLatency float64,
	lastLatencyMs float64,
) (*domain.EndpointHealthScore, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCalls++

	existing := m.scores[endpointURL]
	prevSuccess := 1.0
	prevTimeout := 0.0
	prevLatency := 1.0
	var totalReqs int64
	if existing != nil {
		prevSuccess = existing.SuccessRate
		prevTimeout = existing.TimeoutRate
		prevLatency = existing.LatencyScore
		totalReqs = existing.TotalRequests
	}

	newSuccess := alpha*successVal + (1-alpha)*prevSuccess
	newTimeout := alpha*timeoutVal + (1-alpha)*prevTimeout
	newLatency := alpha*latencyVal + (1-alpha)*prevLatency
	composite := (wSuccess*newSuccess + wTimeout*(1-newTimeout) + wLatency*newLatency) * 100.0
	if composite < 0 {
		composite = 0
	}
	if composite > 100 {
		composite = 100
	}

	score := &domain.EndpointHealthScore{
		EndpointURL:   endpointURL,
		HealthScore:   composite,
		SuccessRate:   newSuccess,
		TimeoutRate:   newTimeout,
		LatencyScore:  newLatency,
		TotalRequests: totalReqs + 1,
		LastLatencyMs: lastLatencyMs,
	}
	cp := *score
	m.scores[endpointURL] = &cp
	return score, nil
}

func (m *mockHealthScoreStore) recordCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recordCalls
}

func TestEWMA(t *testing.T) {
	t.Parallel()
	tests := []struct {
		prev     float64
		current  float64
		alpha    float64
		expected float64
	}{
		{prev: 1.0, current: 1.0, alpha: 0.1, expected: 1.0},
		{prev: 1.0, current: 0.0, alpha: 0.1, expected: 0.9},
		{prev: 0.0, current: 1.0, alpha: 0.1, expected: 0.1},
		{prev: 0.5, current: 0.5, alpha: 0.1, expected: 0.5},
		{prev: 0.0, current: 0.0, alpha: 0.1, expected: 0.0},
		{prev: 1.0, current: 0.0, alpha: 0.5, expected: 0.5},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("prev=%.1f_cur=%.1f_alpha=%.1f", tt.prev, tt.current, tt.alpha), func(t *testing.T) {
			t.Parallel()
			result := ewma(tt.prev, tt.current, tt.alpha)
			assert.LessOrEqual(t, math.
				Abs(
					result-
						tt.expected,
				), 1e-9)
		})
	}
}

func TestHealthScorer_NewEndpoint_StartsHealthy(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	score, allowed, err := hs.CheckHealth(ctx, "https://example.com/api")
	require.NoError(
		t, err)
	assert.True(t, allowed)
	assert.InDelta(t, 100.0,
		score.HealthScore, 1e-9,
	)
}

func TestHealthScorer_SuccessSequence_StaysHealthy(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	for range 20 {
		_, err := hs.RecordResult(ctx, DispatchResult{
			EndpointURL:  "https://example.com/api",
			Success:      true,
			LatencyMs:    50,
			JobTimeoutMs: 5000,
		})
		require.NoError(
			t, err)
	}

	score, allowed, err := hs.CheckHealth(ctx, "https://example.com/api")
	require.NoError(
		t, err)
	assert.True(t, allowed)
	assert.GreaterOrEqual(t,
		score.
			HealthScore,
		90.0)
}

func TestHealthScorer_FailureSequence_BecomesUnhealthy(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	// Send many failures to drive score below the unhealthy threshold.
	for range 100 {
		_, err := hs.RecordResult(ctx, DispatchResult{
			EndpointURL:  "https://failing.com/api",
			Success:      false,
			LatencyMs:    0,
			JobTimeoutMs: 5000,
		})
		require.NoError(
			t, err)
	}

	score, allowed, err := hs.CheckHealth(ctx, "https://failing.com/api")
	require.NoError(
		t, err)
	assert.False(t,
		allowed,
	)
	assert.Less(t,
		score.HealthScore, healthScoreUnhealthy,
	)
}

func TestHealthScorer_DegradedEndpoint(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	// Alternate success and failure (50% failure rate).
	for i := range 200 {
		_, err := hs.RecordResult(ctx, DispatchResult{
			EndpointURL:  "https://flaky.com/api",
			Success:      i%2 == 0,
			LatencyMs:    100,
			JobTimeoutMs: 5000,
		})
		require.NoError(
			t, err)
	}

	score, allowed, err := hs.CheckHealth(ctx, "https://flaky.com/api")
	require.NoError(
		t, err)
	assert.True(t, allowed)
	assert.False(t,
		score.HealthScore <
			healthScoreUnhealthy ||
			score.HealthScore > healthScoreDegraded+
				10)

	// With 50% success rate, the composite score should be in the degraded range.
}

func TestHealthScorer_Recovery(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	// Drive to unhealthy with many failures.
	for range 300 {
		_, _ = hs.RecordResult(ctx, DispatchResult{
			EndpointURL:  "https://recovering.com/api",
			Success:      false,
			JobTimeoutMs: 5000,
		})
	}

	_, allowed, _ := hs.CheckHealth(ctx, "https://recovering.com/api")
	require.False(t,
		allowed,
	)

	// Recover with consistent successes.
	for range 200 {
		_, _ = hs.RecordResult(ctx, DispatchResult{
			EndpointURL:  "https://recovering.com/api",
			Success:      true,
			LatencyMs:    50,
			JobTimeoutMs: 5000,
		})
	}

	score, allowed, err := hs.CheckHealth(ctx, "https://recovering.com/api")
	require.NoError(
		t, err)
	assert.True(t, allowed)
	assert.GreaterOrEqual(t,
		score.
			HealthScore,
		healthScoreDegraded,
	)
}

func TestHealthScorer_TimeoutAffectsScore(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	// Record many timeouts.
	for range 100 {
		_, _ = hs.RecordResult(ctx, DispatchResult{
			EndpointURL:  "https://timeout.com/api",
			Success:      false,
			TimedOut:     true,
			LatencyMs:    5000,
			JobTimeoutMs: 5000,
		})
	}

	_, allowed, err := hs.CheckHealth(ctx, "https://timeout.com/api")
	require.NoError(
		t, err)
	assert.False(t,
		allowed,
	)
}

func TestHealthScorer_HighLatencyReducesScore(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	// Record successes but with very high latency (near timeout).
	for range 100 {
		_, _ = hs.RecordResult(ctx, DispatchResult{
			EndpointURL:  "https://slow.com/api",
			Success:      true,
			LatencyMs:    4500,
			JobTimeoutMs: 5000,
		})
	}

	score, _, err := hs.CheckHealth(ctx, "https://slow.com/api")
	require.NoError(
		t, err)
	assert.Less(t,
		score.HealthScore, 100.0,
	)

	// Success rate is 1.0 but latency score should be low (4500/5000 = 0.9, so latency_score = 0.1).
	// Composite = 0.5*1.0 + 0.3*1.0 + 0.2*0.1 = 0.82 * 100 = 82.
	// With EWMA decay from initial 1.0, should still be lower than a fast endpoint.
}

func TestHealthScorer_TotalRequestsIncrement(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	for range 10 {
		_, _ = hs.RecordResult(ctx, DispatchResult{
			EndpointURL:  "https://counter.com/api",
			Success:      true,
			LatencyMs:    100,
			JobTimeoutMs: 5000,
		})
	}

	score, _, err := hs.CheckHealth(ctx, "https://counter.com/api")
	require.NoError(
		t, err)
	assert.EqualValues(t, 10, score.
		TotalRequests,
	)
}

func TestHealthScorer_SamplesRepeatedSuccesses(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store, WithHealthSuccessSampleInterval(time.Hour))
	ctx := context.Background()
	result := DispatchResult{
		EndpointURL:  "https://sampled.example.com/api",
		Success:      true,
		LatencyMs:    50,
		JobTimeoutMs: 5000,
	}

	first, err := hs.RecordResult(ctx, result)
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := hs.RecordResult(ctx, result)
	require.NoError(t, err)
	require.Nil(t, second)
	require.Equal(t, 1, store.recordCallCount())
}

func TestHealthScorer_FailuresBypassSuccessSamplingAndResetGate(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store, WithHealthSuccessSampleInterval(time.Hour))
	ctx := context.Background()
	success := DispatchResult{
		EndpointURL:  "https://reset.example.com/api",
		Success:      true,
		LatencyMs:    50,
		JobTimeoutMs: 5000,
	}

	_, err := hs.RecordResult(ctx, success)
	require.NoError(t, err)
	skipped, err := hs.RecordResult(ctx, success)
	require.NoError(t, err)
	require.Nil(t, skipped)

	_, err = hs.RecordResult(ctx, DispatchResult{
		EndpointURL:  success.EndpointURL,
		Success:      false,
		LatencyMs:    5000,
		JobTimeoutMs: 5000,
	})
	require.NoError(t, err)

	afterFailure, err := hs.RecordResult(ctx, success)
	require.NoError(t, err)
	require.NotNil(t, afterFailure)
	require.Equal(t, 3, store.recordCallCount())
}

func TestHealthScorer_SamplingIsPerEndpoint(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store, WithHealthSuccessSampleInterval(time.Hour))
	ctx := context.Background()

	_, err := hs.RecordResult(ctx, DispatchResult{
		EndpointURL:  "https://first.example.com/api",
		Success:      true,
		LatencyMs:    50,
		JobTimeoutMs: 5000,
	})
	require.NoError(t, err)
	_, err = hs.RecordResult(ctx, DispatchResult{
		EndpointURL:  "https://second.example.com/api",
		Success:      true,
		LatencyMs:    50,
		JobTimeoutMs: 5000,
	})
	require.NoError(t, err)
	require.Equal(t, 2, store.recordCallCount())
}

func TestThrottledConcurrency(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		score          *domain.EndpointHealthScore
		maxConcurrency int
		wantMin        int
		wantMax        int
	}{
		{
			name:           "nil score returns full concurrency",
			score:          nil,
			maxConcurrency: 10,
			wantMin:        10,
			wantMax:        10,
		},
		{
			name:           "healthy endpoint returns full concurrency",
			score:          &domain.EndpointHealthScore{HealthScore: 80},
			maxConcurrency: 10,
			wantMin:        10,
			wantMax:        10,
		},
		{
			name:           "degraded endpoint at 30 returns throttled",
			score:          &domain.EndpointHealthScore{HealthScore: 30},
			maxConcurrency: 10,
			wantMin:        1,
			wantMax:        4,
		},
		{
			name:           "degraded endpoint at 45 returns moderate throttle",
			score:          &domain.EndpointHealthScore{HealthScore: 45},
			maxConcurrency: 10,
			wantMin:        4,
			wantMax:        8,
		},
		{
			name:           "unhealthy endpoint returns zero",
			score:          &domain.EndpointHealthScore{HealthScore: 10},
			maxConcurrency: 10,
			wantMin:        0,
			wantMax:        0,
		},
		{
			name:           "zero max concurrency passes through",
			score:          &domain.EndpointHealthScore{HealthScore: 50},
			maxConcurrency: 0,
			wantMin:        0,
			wantMax:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ThrottledConcurrency(tt.score, tt.maxConcurrency)
			assert.False(t,
				got < tt.
					wantMin ||
					got >
						tt.wantMax,
			)
		})
	}
}

func TestEndpointHealthScore_HealthLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		score    float64
		expected string
	}{
		{0, "unhealthy"},
		{10, "unhealthy"},
		{29.9, "unhealthy"},
		{30, "degraded"},
		{45, "degraded"},
		{60, "degraded"},
		{60.1, "healthy"},
		{80, "healthy"},
		{100, "healthy"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("score_%.1f", tt.score), func(t *testing.T) {
			t.Parallel()
			h := &domain.EndpointHealthScore{HealthScore: tt.score}
			assert.Equal(t,
				tt.expected,
				h.
					HealthLevel())
		})
	}
}

func TestHealthScorer_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	// Concurrent access should not panic or produce corrupt state.
	// Due to read-modify-write semantics, total_requests may be lower
	// than the number of goroutines (lost updates are expected without
	// external serialization).
	var wg conc.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			_, _ = hs.RecordResult(ctx, DispatchResult{
				EndpointURL:  "https://concurrent.com/api",
				Success:      i%3 != 0,
				LatencyMs:    float64(i * 10),
				JobTimeoutMs: 5000,
			})
		})
	}
	wg.Wait()

	score, _, err := hs.CheckHealth(ctx, "https://concurrent.com/api")
	require.NoError(
		t, err)
	assert.GreaterOrEqual(t,
		score.
			TotalRequests,
		int64(1))
	assert.False(t,
		score.HealthScore <
			0 ||
			score.HealthScore >
				100)
}

func TestHealthScorer_ZeroJobTimeout(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	// When job timeout is 0, latency score should stay at default.
	score, err := hs.RecordResult(ctx, DispatchResult{
		EndpointURL:  "https://notimeout.com/api",
		Success:      true,
		LatencyMs:    500,
		JobTimeoutMs: 0,
	})
	require.NoError(
		t, err)
	assert.InDelta(t, 1.0, score.
		LatencyScore, 1e-9,
	)
}

func TestHealthScorer_ScoreClampedToRange(t *testing.T) {
	t.Parallel()
	store := newMockHealthScoreStore()
	hs := NewHealthScorer(store)
	ctx := context.Background()

	// Record many results and verify score stays in [0, 100].
	for i := range 50 {
		score, err := hs.RecordResult(ctx, DispatchResult{
			EndpointURL:  "https://clamped.com/api",
			Success:      i%5 != 0,
			LatencyMs:    float64(i * 100),
			JobTimeoutMs: 5000,
		})
		require.NoError(
			t, err)
		require.False(t,
			score.
				HealthScore <
				0 ||
				score.HealthScore >
					100)
	}
}
