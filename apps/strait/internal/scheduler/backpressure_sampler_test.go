package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/queue"

	"github.com/sourcegraph/conc"
)

type fakeBPSampler struct {
	calls   atomic.Int32
	samples []queue.TokenSample
	err     error
}

func (f *fakeBPSampler) SampleAvailableTokens(_ context.Context, _ int) ([]queue.TokenSample, error) {
	f.calls.Add(1)
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
	if got := fake.calls.Load(); got != 2 {
		t.Fatalf("expected 2 sample calls, got %d", got)
	}
}

func TestBackpressureSamplerMetricAttributes_DoNotIncludeProjectID(t *testing.T) {
	t.Parallel()
	for _, attr := range backpressureMetricAttributes() {
		if string(attr.Key) == "project_id" {
			t.Fatalf("backpressure sampler must not emit raw project_id metric label")
		}
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
	if fake.calls.Load() != 1 {
		t.Fatal("sampler should have been invoked once")
	}
}

func TestBackpressureSampler_RunHonoursContext(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	m, err := queue.Metrics()
	if err != nil {
		t.Fatalf("queue metrics: %v", err)
	}
	fake := &fakeBPSampler{}
	s := NewBackpressureSampler(fake, m, 5*time.Millisecond, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		s.Run(ctx)
		close(done)
	})
	deadline := time.Now().Add(time.Second)
	for fake.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if fake.calls.Load() == 0 {
		t.Fatal("expected at least one tick before cancel")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit after cancel")
	}
}
