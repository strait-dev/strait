package store

import (
	"testing"
	"time"
)

func TestEvaluatePoolRecommendations(t *testing.T) {
	t.Parallel()

	state := &poolTuningState{lastWaitCount: -1}

	for range poolAdviceSampleConsistencyMin - 1 {
		recs := evaluatePoolRecommendations(poolSnapshot{Acquired: 25, Idle: 5, Total: 25, WaitCount: 0}, 25, 5, state)
		if len(recs) != 0 {
			t.Fatalf("unexpected recommendations before consistency threshold: %v", recs)
		}
	}

	recs := evaluatePoolRecommendations(poolSnapshot{Acquired: 25, Idle: 0, Total: 25, WaitCount: 2}, 25, 5, state)
	if len(recs) != 2 {
		t.Fatalf("recommendation count = %d, want 2 (%v)", len(recs), recs)
	}

	wants := map[string]bool{
		"Consider increasing DB_MAX_CONNS":                        false,
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

func TestEvaluatePoolRecommendations_DoesNotTreatIdleWaitCountAsPressure(t *testing.T) {
	t.Parallel()

	state := &poolTuningState{lastWaitCount: 100}
	recs := evaluatePoolRecommendations(poolSnapshot{
		Acquired:  2,
		Idle:      48,
		Total:     50,
		WaitCount: 101,
	}, 50, 10, state)

	for _, rec := range recs {
		if rec == "Connection pool under pressure, check query performance" {
			t.Fatalf("unexpected pressure recommendation with idle pool: %v", recs)
		}
	}
}

func TestEvaluatePoolRecommendations_AcquireLatencyEvidenceIsPressure(t *testing.T) {
	t.Parallel()

	state := &poolTuningState{lastWaitCount: 100, lastWaitDuration: time.Second}
	recs := evaluatePoolRecommendations(poolSnapshot{
		Acquired:     10,
		Idle:         0,
		Total:        50,
		WaitCount:    101,
		WaitDuration: time.Second + poolAdviceWaitDurationMin,
	}, 50, 10, state)

	if len(recs) != 1 || recs[0] != "Connection pool under pressure, check query performance" {
		t.Fatalf("recommendations = %v, want pool pressure from acquire latency", recs)
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

func BenchmarkEvaluatePoolRecommendationsIdleWaitCount(b *testing.B) {
	state := &poolTuningState{lastWaitCount: 100}
	snapshot := poolSnapshot{
		Acquired:  2,
		Idle:      48,
		Total:     50,
		WaitCount: 101,
	}

	b.ReportAllocs()
	for b.Loop() {
		recs := evaluatePoolRecommendations(snapshot, 50, 10, state)
		for _, rec := range recs {
			if rec == "Connection pool under pressure, check query performance" {
				b.Fatal("idle wait count classified as pressure")
			}
		}
	}
}
