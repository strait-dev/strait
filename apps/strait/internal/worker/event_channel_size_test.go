package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestResolveEventChannelSize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want int
	}{
		{0, defaultEventChannelSize},
		{-5, defaultEventChannelSize},
		{1, minEventChannelSize},
		{minEventChannelSize - 1, minEventChannelSize},
		{minEventChannelSize, minEventChannelSize},
		{2048, 2048},
	}
	for _, tc := range cases {
		require.Equal(t,
			tc.want, resolveEventChannelSize(tc.in))

	}
}

func TestNewExecutor_AppliesEventChannelSize(t *testing.T) {
	t.Parallel()
	e := NewExecutor(ExecutorConfig{EventChannelSize: 2048})
	require.EqualValues(t, 2048, cap(e.
		eventCh))
	require.EqualValues(t, 2048, e.eventChannelSize)

}

func TestNewExecutor_DefaultEventChannelSize(t *testing.T) {
	t.Parallel()
	e := NewExecutor(ExecutorConfig{})
	require.Equal(t,
		defaultEventChannelSize,

		cap(e.
			eventCh))

}

// TestEmit_SaturationDropsAndNoDeadlock fills the channel, then emits enough
// to trigger drops. It asserts that emit never blocks (no deadlock) and that
// the warning throttle does not panic or leak state.
func TestEmit_SaturationDropsAndNoDeadlock(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	exec := &Executor{
		eventCh:            make(chan runEventEnvelope, 4),
		eventChannelSize:   4,
		logger:             slog.Default(),
		subscribers:        []RunEventSubscriber{func(_ context.Context, _ RunLifecycleEvent) {}},
		saturationLastWarn: make(map[eventChannelKind]time.Time),
	}
	run := &domain.JobRun{ID: "r1"}

	done := make(chan struct{})
	concWG.Go(func() {
		for range 128 {
			exec.emit(context.Background(), RunLifecycleEvent{Type: EventCompleted, Run: run})
		}
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "emit deadlocked under saturation")
	}
}

func TestShouldLogSaturation_Throttles(t *testing.T) {
	t.Parallel()
	exec := &Executor{saturationLastWarn: make(map[eventChannelKind]time.Time)}
	require.True(t,
		exec.shouldLogSaturation(eventChannelKind(EventCompleted)),
	)
	require.False(t,
		exec.shouldLogSaturation(eventChannelKind(EventCompleted)),
	)
	require.True(t,
		exec.shouldLogSaturation(eventChannelKind("other")))

	// Simulate expiry.
	exec.saturationLastWarn[eventChannelKind(EventCompleted)] = time.Now().Add(-2 * eventChannelWarnInterval)
	require.True(t,
		exec.shouldLogSaturation(eventChannelKind(EventCompleted)),
	)

}

func TestResolveInstanceID_StableAndNonEmpty(t *testing.T) {
	t.Parallel()

	exec := &Executor{}

	first := exec.resolveInstanceID()
	second := exec.resolveInstanceID()
	require.NotEqual(t, "", first)
	require.Equal(t,
		first, second,
	)

}
