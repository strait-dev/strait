package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvaluatePoolRecommendations(t *testing.T) {
	t.Parallel()

	state := &poolTuningState{lastWaitCount: -1}

	for range poolAdviceSampleConsistencyMin - 1 {
		recs := evaluatePoolRecommendations(poolSnapshot{Acquired: 25, Idle: 5, Total: 25, WaitCount: 0}, 25, 5, state)
		require.Len(t,
			recs, 0)

	}

	recs := evaluatePoolRecommendations(poolSnapshot{Acquired: 25, Idle: 0, Total: 25, WaitCount: 2}, 25, 5, state)
	require.Len(t,
		recs, 2)

	wants := map[string]bool{
		"Consider increasing DB_MAX_CONNS":                        false,
		"Connection pool under pressure, check query performance": false,
	}
	for _, rec := range recs {
		if _, ok := wants[rec]; ok {
			wants[rec] = true
		}
	}

	for _, seen := range wants {
		require.True(t,
			seen)

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
		require.NotEqual(t, "Connection pool under pressure, check query performance",

			rec)

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
	require.False(t,
		len(recs) !=
			1 || recs[0] != "Connection pool under pressure, check query performance",
	)

}

func TestEvaluatePoolRecommendations_ResetsStreaks(t *testing.T) {
	t.Parallel()

	state := &poolTuningState{lastWaitCount: -1}

	_ = evaluatePoolRecommendations(poolSnapshot{Acquired: 25, Idle: 5, Total: 25, WaitCount: 0}, 25, 5, state)
	_ = evaluatePoolRecommendations(poolSnapshot{Acquired: 10, Idle: 1, Total: 25, WaitCount: 0}, 25, 5, state)
	require.EqualValues(t, 0, state.
		acquiredAtMaxStreak)
	require.EqualValues(t, 0, state.
		idleAtMaxStreak)

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
