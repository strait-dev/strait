package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/queue"
)

type fakeBPSampler struct {
	calls   int32
	samples []queue.TokenSample
	err     error
}

func (f *fakeBPSampler) SampleAvailableTokens(_ context.Context, _ int) ([]queue.TokenSample, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.samples, f.err
}

func TestBackpressureSampler_Disabled(t *testing.T) {
	t.Parallel()
	if s := NewBackpressureSampler(nil, nil, 0, 0); s != nil {
		t.Fatalf("expected nil sampler when disabled")
	}
	if s := NewBackpressureSampler(&fakeBPSampler{}, nil, 10*time.Second, 0); s != nil {
		t.Fatalf("expected nil sampler when metrics missing")
	}
}

func TestBackpressureSampler_TickCallsSampler(t *testing.T) {
	t.Parallel()
	// queue.Metrics is a process-wide sync.Once singleton; the fast path
	// is race-safe and does not need ResetMetricsForTest here (calling it
	// from t.Parallel subtests would race with other tests in the package
	// that read the singleton).
	m, err := queue.Metrics()
	if err != nil {
		t.Fatalf("queue metrics: %v", err)
	}
	fake := &fakeBPSampler{samples: []queue.TokenSample{
		{ProjectID: "proj-a", Tokens: 42},
		{ProjectID: "proj-b", Tokens: 7},
	}}
	s := NewBackpressureSampler(fake, m, 15*time.Second, 100)
	if s == nil {
		t.Fatal("expected sampler")
	}
	s.Tick(context.Background())
	s.Tick(context.Background())
	if got := atomic.LoadInt32(&fake.calls); got != 2 {
		t.Fatalf("expected 2 sample calls, got %d", got)
	}
}

func TestBackpressureSampler_TickSwallowsError(t *testing.T) {
	t.Parallel()
	m, err := queue.Metrics()
	if err != nil {
		t.Fatalf("queue metrics: %v", err)
	}
	fake := &fakeBPSampler{err: errors.New("boom")}
	s := NewBackpressureSampler(fake, m, time.Second, 10)
	s.Tick(context.Background()) // must not panic
	if atomic.LoadInt32(&fake.calls) != 1 {
		t.Fatal("sampler should have been invoked once")
	}
}

func TestBackpressureSampler_RunHonoursContext(t *testing.T) {
	t.Parallel()
	m, err := queue.Metrics()
	if err != nil {
		t.Fatalf("queue metrics: %v", err)
	}
	fake := &fakeBPSampler{}
	s := NewBackpressureSampler(fake, m, 5*time.Millisecond, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	time.Sleep(25 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit after cancel")
	}
	if atomic.LoadInt32(&fake.calls) == 0 {
		t.Fatal("expected at least one tick before cancel")
	}
}
