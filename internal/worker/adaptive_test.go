package worker

import "testing"

func TestAdaptiveConcurrency_ScalesUpWhenBackloggedAndBusy(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(5, 20, 8)
	next := a.Observe(17, 0.81)
	if next != 10 {
		t.Fatalf("Observe() = %d, want %d", next, 10)
	}
}

func TestAdaptiveConcurrency_ScalesDownAfterTwoIdleChecks(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(5, 40, 20)
	first := a.Observe(0, 0.19)
	if first != 20 {
		t.Fatalf("first Observe() = %d, want %d", first, 20)
	}

	second := a.Observe(0, 0.19)
	if second != 15 {
		t.Fatalf("second Observe() = %d, want %d", second, 15)
	}
}

func TestAdaptiveConcurrency_RespectsBounds(t *testing.T) {
	t.Parallel()

	a := NewAdaptiveConcurrency(5, 12, 12)
	next := a.Observe(100, 1.0)
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
