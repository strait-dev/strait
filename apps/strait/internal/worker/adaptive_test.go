package worker

import "testing"

func TestAdaptiveConcurrency_TieredScaleUp_DeepQueue(t *testing.T) {
	t.Parallel()

	// Queue > 1000: doubles concurrency (factor = 1.0).
	a := NewAdaptiveConcurrency(5, 200, 50)
	next := a.Observe(1500, 0.85)
	if next != 100 {
		t.Fatalf("Observe(1500, 0.85) = %d, want 100 (double)", next)
	}
}

func TestAdaptiveConcurrency_TieredScaleUp_ModerateQueue(t *testing.T) {
	t.Parallel()

	// Queue > 100: 50% increase.
	a := NewAdaptiveConcurrency(5, 200, 40)
	next := a.Observe(500, 0.85)
	if next != 60 {
		t.Fatalf("Observe(500, 0.85) = %d, want 60 (50%% increase)", next)
	}
}

func TestAdaptiveConcurrency_TieredScaleUp_MildQueue(t *testing.T) {
	t.Parallel()

	// Queue > 2*current but <= 100: 25% increase.
	a := NewAdaptiveConcurrency(5, 200, 8)
	next := a.Observe(17, 0.81)
	if next != 10 {
		t.Fatalf("Observe(17, 0.81) = %d, want 10 (25%% increase)", next)
	}
}

func TestAdaptiveConcurrency_ScaleUp_UtilizationThreshold(t *testing.T) {
	t.Parallel()

	// At 0.70 utilization (new threshold): should scale up.
	a := NewAdaptiveConcurrency(5, 200, 20)
	next := a.Observe(500, 0.71)
	if next != 30 {
		t.Fatalf("Observe(500, 0.71) = %d, want 30", next)
	}

	// At 0.69 utilization: should NOT scale up.
	b := NewAdaptiveConcurrency(5, 200, 20)
	next = b.Observe(500, 0.69)
	if next != 20 {
		t.Fatalf("Observe(500, 0.69) = %d, want 20 (no change)", next)
	}
}

func TestAdaptiveConcurrency_FasterScaleDown(t *testing.T) {
	t.Parallel()

	// 33% decrease after 2 idle checks (was 25%).
	a := NewAdaptiveConcurrency(5, 40, 30)
	first := a.Observe(0, 0.10)
	if first != 30 {
		t.Fatalf("first Observe() = %d, want 30 (no change)", first)
	}

	second := a.Observe(0, 0.10)
	// 30 * 0.33 = 9.9 -> ceil = 10; 30 - 10 = 20
	if second != 20 {
		t.Fatalf("second Observe() = %d, want 20 (33%% decrease)", second)
	}
}

func TestAdaptiveConcurrency_ScaleDownRespectsMin(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(5, 12, 6)
	_ = a.Observe(0, 0.10)
	next := a.Observe(0, 0.10)
	// 6 * 0.33 = 1.98 -> ceil = 2; 6 - 2 = 4 -> clamped to min 5
	if next != 5 {
		t.Fatalf("Observe() at lower bound = %d, want 5 (min)", next)
	}
}

func TestAdaptiveConcurrency_ScaleUpRespectsMax(t *testing.T) {
	t.Parallel()

	// Deep queue on a system near max: doubling would exceed max.
	a := NewAdaptiveConcurrency(5, 80, 60)
	next := a.Observe(2000, 0.90)
	if next != 80 {
		t.Fatalf("Observe() with deep queue = %d, want 80 (max)", next)
	}
}

func TestAdaptiveConcurrency_NoFlapping(t *testing.T) {
	t.Parallel()

	// Alternating between queued and idle should not oscillate rapidly.
	a := NewAdaptiveConcurrency(5, 100, 20)

	// One idle check: no change.
	a.Observe(0, 0.10)
	// Spike resets idle counter and scales up.
	after := a.Observe(500, 0.85)
	if after != 30 {
		t.Fatalf("Observe(500, 0.85) = %d, want 30 (50%% increase)", after)
	}
	// One idle check: no scale-down because only 1 idle check since spike.
	next := a.Observe(0, 0.10)
	if next != 30 {
		t.Fatalf("first idle after spike = %d, want 30 (no change)", next)
	}
	// Second idle: NOW scale down.
	next = a.Observe(0, 0.10)
	if next >= 30 {
		t.Fatalf("second idle = %d, want < 30 (should scale down)", next)
	}
}

func TestAdaptiveConcurrency_RespectsBounds(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(5, 12, 12)
	next := a.Observe(2000, 1.0)
	if next != 12 {
		t.Fatalf("Observe() at upper bound = %d, want %d", next, 12)
	}

	b := NewAdaptiveConcurrency(5, 12, 5)
	_ = b.Observe(0, 0.10)
	next = b.Observe(0, 0.10)
	if next != 5 {
		t.Fatalf("Observe() at lower bound = %d, want %d", next, 5)
	}
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
			if a.CurrentLimit() != tt.wantCurrent {
				t.Errorf("CurrentLimit() = %d, want %d", a.CurrentLimit(), tt.wantCurrent)
			}
		})
	}
}

func TestAdaptiveConcurrency_QueueNotDeepEnough(t *testing.T) {
	t.Parallel()

	// Queue depth <= 2*current: no scale-up even with high utilization.
	a := NewAdaptiveConcurrency(5, 100, 20)
	next := a.Observe(39, 0.90) // 39 <= 20*2 = 40
	if next != 20 {
		t.Fatalf("Observe(39, 0.90) = %d, want 20 (no change)", next)
	}
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
	// 20 * 0.33 = 6.6 -> ceil = 7; 20 - 7 = 13
	if next != 13 {
		t.Fatalf("Observe() after reset = %d, want 13", next)
	}
}
