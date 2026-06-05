package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdaptiveConcurrency_TieredScaleUp_DeepQueue(t *testing.T) {
	t.Parallel()

	// Queue > 1000: doubles concurrency (factor = 1.0).
	a := NewAdaptiveConcurrency(5, 200, 50)
	next := a.Observe(1500, 0.85)
	require.Equal(t, 100, next)
}

func TestAdaptiveConcurrency_TieredScaleUp_ModerateQueue(t *testing.T) {
	t.Parallel()

	// Queue > 100: 50% increase.
	a := NewAdaptiveConcurrency(5, 200, 40)
	next := a.Observe(500, 0.85)
	require.Equal(t, 60, next)
}

func TestAdaptiveConcurrency_TieredScaleUp_MildQueue(t *testing.T) {
	t.Parallel()

	// Queue > 2*current but <= 100: 25% increase.
	a := NewAdaptiveConcurrency(5, 200, 8)
	next := a.Observe(17, 0.81)
	require.Equal(t, 10, next)
}

func TestAdaptiveConcurrency_ScaleUp_UtilizationThreshold(t *testing.T) {
	t.Parallel()

	// At 0.70 utilization (new threshold): should scale up.
	a := NewAdaptiveConcurrency(5, 200, 20)
	next := a.Observe(500, 0.71)
	require.Equal(t, 30, next)

	// At 0.69 utilization: should NOT scale up.
	b := NewAdaptiveConcurrency(5, 200, 20)
	next = b.Observe(500, 0.69)
	require.Equal(t, 20, next)
}

func TestAdaptiveConcurrency_FasterScaleDown(t *testing.T) {
	t.Parallel()

	// 33% decrease after 2 idle checks (was 25%).
	a := NewAdaptiveConcurrency(5, 40, 30)
	first := a.Observe(0, 0.10)
	require.Equal(t, 30, first)

	second := a.Observe(0, 0.10)
	require.Equal(t, 20, second)

	// 30 * 0.33 = 9.9 -> ceil = 10; 30 - 10 = 20
}

func TestAdaptiveConcurrency_DoesNotShedAtColdIdle(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(5, 40, 30)
	for range 10 {
		next := a.Observe(0, 0)
		require.Equal(t, 30, next)
	}
}

func TestAdaptiveConcurrency_ScaleDownRespectsMin(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(5, 12, 6)
	_ = a.Observe(0, 0.10)
	next := a.Observe(0, 0.10)
	require.Equal(t, 5, next)

	// 6 * 0.33 = 1.98 -> ceil = 2; 6 - 2 = 4 -> clamped to min 5
}

func TestAdaptiveConcurrency_ScaleUpRespectsMax(t *testing.T) {
	t.Parallel()

	// Deep queue on a system near max: doubling would exceed max.
	a := NewAdaptiveConcurrency(5, 80, 60)
	next := a.Observe(2000, 0.90)
	require.Equal(t, 80, next)
}

func TestAdaptiveConcurrency_NoFlapping(t *testing.T) {
	t.Parallel()

	// Alternating between queued and idle should not oscillate rapidly.
	a := NewAdaptiveConcurrency(5, 100, 20)

	// One idle check: no change.
	a.Observe(0, 0.10)
	// Spike resets idle counter and scales up.
	after := a.Observe(500, 0.85)
	require.Equal(t, 30, after)

	// One idle check: no scale-down because only 1 idle check since spike.
	next := a.Observe(0, 0.10)
	require.Equal(t, 30, next)

	// Second idle: NOW scale down.
	next = a.Observe(0, 0.10)
	require.Less(
		t, next, 30)
}

func TestAdaptiveConcurrency_RespectsBounds(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(5, 12, 12)
	next := a.Observe(2000, 1.0)
	require.Equal(t, 12, next)

	b := NewAdaptiveConcurrency(5, 12, 5)
	_ = b.Observe(0, 0.10)
	next = b.Observe(0, 0.10)
	require.Equal(t, 5, next)
}

func TestAdaptiveConcurrency_Constructor_Clamps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		min, max, init       int
		wantMin, wantCurrent int
	}{
		{"min below 1", 0, 10, 5, 1, 5},
		{"max below min", 5, 3, 5, 5, 5},
		{"initial below min", 5, 20, 2, 5, 5},
		{"initial above max", 5, 20, 30, 5, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := NewAdaptiveConcurrency(tt.min, tt.max, tt.init)
			assert.Equal(
				t,
				tt.wantCurrent, a.CurrentLimit())
		})
	}
}

func TestAdaptiveConcurrency_QueueNotDeepEnough(t *testing.T) {
	t.Parallel()

	// Queue depth <= 2*current: no scale-up even with high utilization.
	a := NewAdaptiveConcurrency(5, 100, 20)
	next := a.Observe(39, 0.90)
	require.Equal(t, 20, next)

	// 39 <= 20*2 = 40
}

func TestAdaptiveConcurrency_IdleCheckResetOnActivity(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(5, 100, 20)

	// First idle check.
	a.Observe(0, 0.10)
	// Activity resets idle counter.
	a.Observe(10, 0.50)
	// First idle check again (counter was reset).
	a.Observe(0, 0.10)
	// Second idle check: now should scale down.
	next := a.Observe(0, 0.10)
	require.Equal(t, 13, next)

	// 20 * 0.33 = 6.6 -> ceil = 7; 20 - 7 = 13
}

func BenchmarkAdaptiveConcurrencyObserveIdle(b *testing.B) {
	a := NewAdaptiveConcurrency(5, 200, 50)

	b.ReportAllocs()
	for b.Loop() {
		if got := a.Observe(0, 0); got != 50 {
			b.Fatalf("Observe() = %d, want 50", got)
		}
	}
}
