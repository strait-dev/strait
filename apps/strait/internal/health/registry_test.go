package health

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
)

func TestRegistry_AllUp(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("db", func(ctx context.Context) error { return nil }))
	r.Register(NewChecker("redis", func(ctx context.Context) error { return nil }))

	result := r.CheckAll(context.Background())
	if result.Status != StatusUp {
		t.Fatalf("status = %q, want %q", result.Status, StatusUp)
	}
	if len(result.Components) != 2 {
		t.Fatalf("components = %d, want 2", len(result.Components))
	}
	for _, c := range result.Components {
		if c.Status != StatusUp {
			t.Errorf("component %q status = %q, want %q", c.Name, c.Status, StatusUp)
		}
	}
}

func TestRegistry_OneDown(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("db", func(ctx context.Context) error { return nil }))
	r.Register(NewChecker("redis", func(ctx context.Context) error { return errors.New("connection refused") }))

	result := r.CheckAll(context.Background())
	if result.Status != StatusDown {
		t.Fatalf("status = %q, want %q", result.Status, StatusDown)
	}

	var downCount int
	for _, c := range result.Components {
		if c.Status == StatusDown {
			downCount++
			if c.Error == "" {
				t.Errorf("component %q has no error message", c.Name)
			}
		}
	}
	if downCount != 1 {
		t.Fatalf("down components = %d, want 1", downCount)
	}
}

func TestRegistry_Empty(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	result := r.CheckAll(context.Background())
	if result.Status != StatusUp {
		t.Fatalf("empty registry status = %q, want %q", result.Status, StatusUp)
	}
	if len(result.Components) != 0 {
		t.Fatalf("components = %d, want 0", len(result.Components))
	}
}

func TestRegistry_LatencyTracking(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("slow", func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}))

	result := r.CheckAll(context.Background())
	if result.Components[0].Latency < 10*time.Millisecond {
		t.Fatalf("latency = %v, want >= 10ms", result.Components[0].Latency)
	}
	if result.Components[0].LatencyMs < 10 {
		t.Fatalf("latency_ms = %d, want >= 10", result.Components[0].LatencyMs)
	}
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
	if result.Status != StatusDown {
		t.Fatalf("status = %q, want %q", result.Status, StatusDown)
	}
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
	if len(result.Components) != 10 {
		t.Fatalf("components = %d, want 10", len(result.Components))
	}
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

	if result.Status != StatusUp {
		t.Fatalf("status = %q, want %q", result.Status, StatusUp)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("elapsed = %v, want < 200ms (checks should run in parallel, sequential would be 100ms+)", elapsed)
	}
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
	if result.Status != StatusUp {
		t.Fatalf("status = %q, want %q", result.Status, StatusUp)
	}
	if got := calls.Load(); got != 20 {
		t.Errorf("checker invocations = %d, want 20", got)
	}
}
