package health

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_AllUp(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("db", func(ctx context.Context) error { return nil }))
	r.Register(NewChecker("redis", func(ctx context.Context) error { return nil }))

	result := r.CheckAll(context.Background())
	require.Equal(t, StatusUp, result.Status)
	require.Len(t, result.Components, 2)
	for _, c := range result.Components {
		assert.Equal(t, StatusUp, c.Status, "component %q", c.Name)
	}
}

func TestRegistry_OneDown(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("db", func(ctx context.Context) error { return nil }))
	r.Register(NewChecker("redis", func(ctx context.Context) error { return errors.New("connection refused") }))

	result := r.CheckAll(context.Background())
	require.Equal(t, StatusDown, result.Status)

	var downCount int
	for _, c := range result.Components {
		if c.Status == StatusDown {
			downCount++
			assert.NotEmpty(t, c.Error, "component %q", c.Name)
		}
	}
	require.Equal(t, 1, downCount)
}

func TestRegistry_Empty(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	result := r.CheckAll(context.Background())
	require.Equal(t, StatusUp, result.Status)
	require.Empty(t, result.Components)
}

func TestRegistry_LatencyTracking(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("slow", func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}))

	result := r.CheckAll(context.Background())
	require.Len(t, result.Components, 1)
	require.GreaterOrEqual(t, result.Components[0].Latency, 10*time.Millisecond)
	require.GreaterOrEqual(t, result.Components[0].LatencyMs, int64(10))
}

func TestRegistry_ContextCanceled(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("blocking", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := r.CheckAll(ctx)
	require.Equal(t, StatusDown, result.Status)
}

func TestRegistry_ConcurrentRegister(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	var wg conc.WaitGroup
	for range 10 {
		wg.Go(func() {
			r.Register(NewChecker("test", func(ctx context.Context) error { return nil }))
		})
	}
	wg.Wait()

	result := r.CheckAll(context.Background())
	require.Len(t, result.Components, 10)
}

func TestRegistry_ChecksRunInParallel(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	r.Register(NewChecker("slow1", func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}))
	r.Register(NewChecker("slow2", func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}))

	start := time.Now()
	result := r.CheckAll(context.Background())
	elapsed := time.Since(start)

	require.Equal(t, StatusUp, result.Status)
	require.LessOrEqual(t, elapsed, 200*time.Millisecond)
}

func TestRegistry_AllCheckersExecuted(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	var calls atomic.Int32
	for range 20 {
		r.Register(NewChecker("counter", func(_ context.Context) error {
			calls.Add(1)
			return nil
		}))
	}
	result := r.CheckAll(context.Background())
	require.Equal(t, StatusUp, result.Status)
	assert.Equal(t, int32(20), calls.Load())
}
