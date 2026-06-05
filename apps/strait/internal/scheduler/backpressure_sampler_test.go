package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/queue"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
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
	require.Nil(t, NewBackpressureSampler(nil, nil,
		0, 0))
	require.Nil(t, NewBackpressureSampler(&fakeBPSampler{},
		nil,
		10*time.Second, 0))

}

func TestBackpressureSampler_TickCallsSampler(t *testing.T) {
	t.Parallel()
	// queue.Metrics is a process-wide sync.Once singleton; the fast path
	// is race-safe and does not need ResetMetricsForTest here (calling it
	// from t.Parallel subtests would race with other tests in the package
	// that read the singleton).
	m, err := queue.Metrics()
	require.NoError(t,
		err)

	fake := &fakeBPSampler{samples: []queue.TokenSample{
		{ProjectID: "proj-a", Tokens: 42},
		{ProjectID: "proj-b", Tokens: 7},
	}}
	s := NewBackpressureSampler(fake, m, 15*time.Second, 100)
	require.NotNil(t, s)

	s.Tick(context.Background())
	s.Tick(context.Background())
	require.EqualValues(t, 2,
		fake.calls.
			Load())

}

func TestBackpressureSamplerMetricAttributes_DoNotIncludeProjectID(t *testing.T) {
	t.Parallel()
	for _, attr := range backpressureMetricAttributes() {
		require.NotEqual(t,
			"project_id",
			string(attr.
				Key))

	}
}

func TestBackpressureSampler_TickSwallowsError(t *testing.T) {
	t.Parallel()
	m, err := queue.Metrics()
	require.NoError(t,
		err)

	fake := &fakeBPSampler{err: errors.New("boom")}
	s := NewBackpressureSampler(fake, m, time.Second, 10)
	s.Tick(context.Background())
	require.EqualValues(t, 1,
		fake.calls.
			Load())

	// must not panic

}

func TestBackpressureSampler_RunHonoursContext(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	m, err := queue.Metrics()
	require.NoError(t,
		err)

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
	require.NotEqual(t,
		0, fake.calls.
			Load())

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "Run did not exit after cancel")
	}
}
