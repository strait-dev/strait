package store

import "testing"

func TestEvaluatePoolRecommendations(t *testing.T) {
	t.Parallel()

	state := &poolTuningState{lastWaitCount: -1}

	for range poolAdviceSampleConsistencyMin - 1 {
		recs := evaluatePoolRecommendations(poolSnapshot{Acquired: 25, Idle: 5, Total: 25, WaitCount: 0}, 25, 5, state)
		if len(recs) != 0 {
			t.Fatalf("unexpected recommendations before consistency threshold: %v", recs)
		}
	}

	recs := evaluatePoolRecommendations(poolSnapshot{Acquired: 25, Idle: 5, Total: 25, WaitCount: 2}, 25, 5, state)
	if len(recs) != 3 {
		t.Fatalf("recommendation count = %d, want 3 (%v)", len(recs), recs)
	}

	wants := map[string]bool{
		"Consider increasing DB_MAX_CONNS":                        false,
		"Consider decreasing DB_MIN_CONNS":                        false,
		"Connection pool under pressure, check query performance": false,
	}
	for _, rec := range recs {
		if _, ok := wants[rec]; ok {
			wants[rec] = true
		}
	}

	for rec, seen := range wants {
		if !seen {
			t.Fatalf("missing recommendation %q in %v", rec, recs)
		}
	}
}

func TestEvaluatePoolRecommendations_ResetsStreaks(t *testing.T) {
	t.Parallel()

	state := &poolTuningState{lastWaitCount: -1}

	_ = evaluatePoolRecommendations(poolSnapshot{Acquired: 25, Idle: 5, Total: 25, WaitCount: 0}, 25, 5, state)
	_ = evaluatePoolRecommendations(poolSnapshot{Acquired: 10, Idle: 1, Total: 25, WaitCount: 0}, 25, 5, state)

	if state.acquiredAtMaxStreak != 0 {
		t.Fatalf("acquired streak = %d, want 0", state.acquiredAtMaxStreak)
	}
	if state.idleAtMaxStreak != 0 {
		t.Fatalf("idle streak = %d, want 0", state.idleAtMaxStreak)
	}
}
