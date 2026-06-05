package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
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
		if got := resolveEventChannelSize(tc.in); got != tc.want {
			t.Fatalf("resolveEventChannelSize(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestNewExecutor_AppliesEventChannelSize(t *testing.T) {
	t.Parallel()
	e := NewExecutor(ExecutorConfig{EventChannelSize: 2048})
	if cap(e.eventCh) != 2048 {
		t.Fatalf("expected eventCh cap=2048, got %d", cap(e.eventCh))
	}
	if e.eventChannelSize != 2048 {
		t.Fatalf("expected eventChannelSize=2048, got %d", e.eventChannelSize)
	}
}

func TestNewExecutor_DefaultEventChannelSize(t *testing.T) {
	t.Parallel()
	e := NewExecutor(ExecutorConfig{})
	if cap(e.eventCh) != defaultEventChannelSize {
		t.Fatalf("expected default cap=%d, got %d", defaultEventChannelSize, cap(e.eventCh))
	}
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
		t.Fatal("emit deadlocked under saturation")
	}
}

func TestShouldLogSaturation_Throttles(t *testing.T) {
	t.Parallel()
	exec := &Executor{saturationLastWarn: make(map[eventChannelKind]time.Time)}
	if !exec.shouldLogSaturation(eventChannelKind(EventCompleted)) {
		t.Fatal("first call should log")
	}
	if exec.shouldLogSaturation(eventChannelKind(EventCompleted)) {
		t.Fatal("second call within window should be throttled")
	}
	if !exec.shouldLogSaturation(eventChannelKind("other")) {
		t.Fatal("different kind should log once")
	}
	// Simulate expiry.
	exec.saturationLastWarn[eventChannelKind(EventCompleted)] = time.Now().Add(-2 * eventChannelWarnInterval)
	if !exec.shouldLogSaturation(eventChannelKind(EventCompleted)) {
		t.Fatal("after interval, should log again")
	}
}

func TestResolveInstanceID_StableAndNonEmpty(t *testing.T) {
	t.Parallel()

	exec := &Executor{}

	first := exec.resolveInstanceID()
	second := exec.resolveInstanceID()

	if first == "" {
		t.Fatal("resolveInstanceID returned empty string")
	}
	if second != first {
		t.Fatalf("resolveInstanceID changed between calls: first=%q second=%q", first, second)
	}
}
