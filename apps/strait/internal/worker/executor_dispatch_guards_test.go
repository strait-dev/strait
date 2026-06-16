package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestExecutorEndpointGuardCacheReusesAllowedDecision(t *testing.T) {
	t.Parallel()

	var circuitReads atomic.Int32
	var healthReads atomic.Int32
	store := &mockExecutorStore{}
	store.canDispatchFn = func(context.Context, string, time.Time) (bool, *time.Time, error) {
		circuitReads.Add(1)
		return true, nil, nil
	}
	store.getEndpointHealthScoreFn = func(_ context.Context, endpointURL string) (*domain.EndpointHealthScore, error) {
		healthReads.Add(1)
		return &domain.EndpointHealthScore{
			EndpointURL:  endpointURL,
			HealthScore:  100,
			SuccessRate:  1,
			LatencyScore: 1,
		}, nil
	}

	exec := NewExecutor(ExecutorConfig{
		Store:                 store,
		EndpointGuardCacheTTL: time.Minute,
	})
	job := testJob("https://example.com/dispatch", 3, 30)

	first := exec.prefetchDispatchGuards(context.Background(), job, executionPolicy{})
	second := exec.prefetchDispatchGuards(context.Background(), job, executionPolicy{})

	require.True(t, first.circuitAllowed)
	require.True(t, first.healthAllowed)
	require.True(t, second.circuitAllowed)
	require.True(t, second.healthAllowed)
	require.EqualValues(t, 1, circuitReads.Load())
	require.EqualValues(t, 1, healthReads.Load())
}

func TestExecutorEndpointGuardCacheFailureInvalidatesAllowedDecision(t *testing.T) {
	t.Parallel()

	var circuitReads atomic.Int32
	var healthReads atomic.Int32
	store := &mockExecutorStore{}
	store.canDispatchFn = func(context.Context, string, time.Time) (bool, *time.Time, error) {
		circuitReads.Add(1)
		return true, nil, nil
	}
	store.getEndpointHealthScoreFn = func(_ context.Context, endpointURL string) (*domain.EndpointHealthScore, error) {
		healthReads.Add(1)
		return &domain.EndpointHealthScore{
			EndpointURL:  endpointURL,
			HealthScore:  100,
			SuccessRate:  1,
			LatencyScore: 1,
		}, nil
	}

	exec := NewExecutor(ExecutorConfig{
		Store:                 store,
		EndpointGuardCacheTTL: time.Minute,
	})
	job := testJob("https://example.com/dispatch", 3, 30)

	first := exec.prefetchDispatchGuards(context.Background(), job, executionPolicy{})
	exec.recordFailedDispatchSignals(context.Background(), job, failedDispatchSignalFailure)
	second := exec.prefetchDispatchGuards(context.Background(), job, executionPolicy{})

	require.True(t, first.circuitAllowed)
	require.True(t, first.healthAllowed)
	require.True(t, second.circuitAllowed)
	require.True(t, second.healthAllowed)
	require.EqualValues(t, 2, circuitReads.Load())
	require.EqualValues(t, 2, healthReads.Load())
}
