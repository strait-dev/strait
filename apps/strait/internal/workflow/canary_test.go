package workflow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Version routing tests.

func TestCanary_VersionRouting_10Percent(t *testing.T) {
	t.Parallel()
	canary := &CanaryDeployment{
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    10,
		Status:        CanaryActive,
	}

	targetCount := 0
	n := 10000
	router := NewCanaryRouter()
	for range n {
		if router.ResolveVersion(canary) == 2 {
			targetCount++
		}
	}

	ratio := float64(targetCount) / float64(n)
	assert.False(t, ratio <
		0.07 ||
		ratio > 0.13)

}

func TestCanary_VersionRouting_50Percent(t *testing.T) {
	t.Parallel()
	canary := &CanaryDeployment{
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    50,
		Status:        CanaryActive,
	}

	targetCount := 0
	n := 10000
	router := NewCanaryRouter()
	for range n {
		if router.ResolveVersion(canary) == 2 {
			targetCount++
		}
	}

	ratio := float64(targetCount) / float64(n)
	assert.False(t, ratio <
		0.45 ||
		ratio > 0.55)

}

func TestCanary_VersionRouting_100Percent(t *testing.T) {
	t.Parallel()
	canary := &CanaryDeployment{
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    100,
		Status:        CanaryActive,
	}

	router := NewCanaryRouter()
	for range 100 {
		require.EqualValues(t, 2,
			router.
				ResolveVersion(canary))

	}
}

func TestCanary_VersionRouting_0Percent(t *testing.T) {
	t.Parallel()
	canary := &CanaryDeployment{
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    0,
		Status:        CanaryActive,
	}

	router := NewCanaryRouter()
	for range 100 {
		require.EqualValues(t, 1,
			router.
				ResolveVersion(canary))

	}
}

func TestCanary_NoActiveCanary(t *testing.T) {
	t.Parallel()
	router := NewCanaryRouter()
	assert.EqualValues(t, 0,
		router.
			ResolveVersion(nil))

}

func TestCanary_InactiveStatus(t *testing.T) {
	t.Parallel()
	canary := &CanaryDeployment{
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    50,
		Status:        CanaryCompleted,
	}

	router := NewCanaryRouter()
	assert.EqualValues(t, 0,
		router.
			ResolveVersion(canary))

}

func TestCanary_DeterministicRouting(t *testing.T) {
	t.Parallel()
	canary := &CanaryDeployment{
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    50,
		Status:        CanaryActive,
	}

	// Inject deterministic random: always returns 0.3.
	router := newCanaryRouterWithRandFn(func() float64 { return 0.3 })
	assert.EqualValues(t, 2,
		router.
			ResolveVersion(canary))

	// 0.3 < 0.5 threshold => target version.

	// Always returns 0.7.
	router = newCanaryRouterWithRandFn(func() float64 { return 0.7 })
	assert.EqualValues(t, 1,
		router.
			ResolveVersion(canary))

	// 0.7 >= 0.5 threshold => source version.

}

// Health monitor tests.

func TestCanaryMonitor_AutoPromote_HealthyTarget(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{
		TargetFailureRate: 1.0,
		TargetLatencyP99:  100 * time.Millisecond,
		TargetRunCount:    50,
	}
	config := &AutoPromoteConfig{
		Enabled:              true,
		FailureRateThreshold: 5.0,
		LatencyP99Threshold:  10 * time.Second,
	}

	decision := EvaluateHealth(health, config)
	assert.Equal(t, CanaryDecisionPromote,

		decision)

}

func TestCanaryMonitor_AutoRollback_HighFailureRate(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{
		TargetFailureRate: 15.0,
		TargetRunCount:    50,
	}
	config := &AutoPromoteConfig{
		Enabled:              true,
		FailureRateThreshold: 5.0,
	}

	decision := EvaluateHealth(health, config)
	assert.Equal(t, CanaryDecisionRollback,

		decision)

}

func TestCanaryMonitor_AutoRollback_HighLatency(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{
		TargetLatencyP99: 30 * time.Second,
		TargetRunCount:   50,
	}
	config := &AutoPromoteConfig{
		Enabled:             true,
		LatencyP99Threshold: 10 * time.Second,
	}

	decision := EvaluateHealth(health, config)
	assert.Equal(t, CanaryDecisionRollback,

		decision)

}

func TestCanaryMonitor_InsufficientData(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{
		TargetFailureRate: 50.0, // terrible, but not enough data
		TargetRunCount:    2,
	}
	config := &AutoPromoteConfig{
		Enabled:              true,
		FailureRateThreshold: 5.0,
	}

	decision := EvaluateHealth(health, config)
	assert.Equal(t, CanaryDecisionHold,

		decision)

}

func TestCanaryMonitor_DisabledConfig(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{TargetRunCount: 100}
	config := &AutoPromoteConfig{Enabled: false}

	decision := EvaluateHealth(health, config)
	assert.Equal(t, CanaryDecisionHold,

		decision)

}

func TestCanaryMonitor_NilConfig(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{TargetRunCount: 100}
	decision := EvaluateHealth(health, nil)
	assert.Equal(t, CanaryDecisionHold,

		decision)

}

// NextPromoteStep tests.

func TestCanaryMonitor_PromoteSteps(t *testing.T) {
	t.Parallel()
	config := &AutoPromoteConfig{
		Enabled: true,
		Steps:   []int{10, 25, 50, 100},
	}

	tests := []struct {
		current int
		want    int
	}{
		{0, 10},
		{10, 25},
		{25, 50},
		{50, 100},
		{100, -1},
	}

	for _, tt := range tests {
		got := NextPromoteStep(config, tt.current)
		assert.Equal(t, tt.
			want, got,
		)

	}
}

func TestCanaryMonitor_NilPromoteConfig(t *testing.T) {
	t.Parallel()
	got := NextPromoteStep(nil, 10)
	assert.EqualValues(t, -1,
		got)

}

// Validation tests.

func TestValidateCanary_Valid(t *testing.T) {
	t.Parallel()
	err := ValidateCanaryRequest("wf-1", 1, 2, 10)
	assert.NoError(t,
		err)

}

func TestValidateCanary_SameVersions(t *testing.T) {
	t.Parallel()
	err := ValidateCanaryRequest("wf-1", 1, 1, 10)
	assert.Error(t, err)

}

func TestValidateCanary_NegativeTrafficPct(t *testing.T) {
	t.Parallel()
	err := ValidateCanaryRequest("wf-1", 1, 2, -1)
	assert.Error(t, err)

}

func TestValidateCanary_TrafficPctOver100(t *testing.T) {
	t.Parallel()
	err := ValidateCanaryRequest("wf-1", 1, 2, 150)
	assert.Error(t, err)

}

func TestValidateCanary_EmptyWorkflowID(t *testing.T) {
	t.Parallel()
	err := ValidateCanaryRequest("", 1, 2, 10)
	assert.Error(t, err)

}

// Fuzz tests.

func FuzzCanary_TrafficPercentage(f *testing.F) {
	f.Add(0)
	f.Add(10)
	f.Add(50)
	f.Add(100)
	f.Add(-1)
	f.Add(150)

	f.Fuzz(func(t *testing.T, pct int) {
		canary := &CanaryDeployment{
			SourceVersion: 1,
			TargetVersion: 2,
			TrafficPct:    pct,
			Status:        CanaryActive,
		}
		router := NewCanaryRouter()
		// Must never panic.
		v := router.ResolveVersion(canary)
		assert.False(t, v !=
			1 &&
			v !=
				2)

	})
}

func FuzzCanary_AutoPromoteConfig(f *testing.F) {
	f.Add(5.0, int64(10*time.Second), 10)
	f.Add(0.0, int64(0), 0)
	f.Add(100.0, int64(time.Hour), 100)

	f.Fuzz(func(t *testing.T, failureThreshold float64, latencyNanos int64, runCount int) {
		health := CanaryHealthCheck{
			TargetFailureRate: failureThreshold,
			TargetLatencyP99:  time.Duration(latencyNanos),
			TargetRunCount:    runCount,
		}
		config := &AutoPromoteConfig{
			Enabled:              true,
			FailureRateThreshold: failureThreshold,
			LatencyP99Threshold:  time.Duration(latencyNanos),
		}
		// Must never panic.
		_ = EvaluateHealth(health, config)
	})
}

// Adversarial tests.

func TestCanary_RollbackDuringPromotion(t *testing.T) {
	t.Parallel()
	// Simulates checking a canary in promoting state.
	canary := &CanaryDeployment{
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    50,
		Status:        CanaryPromoting,
	}

	router := NewCanaryRouter()
	assert.EqualValues(t, 0,
		router.
			ResolveVersion(canary))

	// Non-active canary should return 0.

}

func TestCanary_ZeroRunsForMetrics(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{
		TargetRunCount: 0,
	}
	config := &AutoPromoteConfig{
		Enabled:              true,
		FailureRateThreshold: 5.0,
	}

	decision := EvaluateHealth(health, config)
	assert.Equal(t, CanaryDecisionHold,

		decision)

}

func TestCanary_BoundaryTrafficValues(t *testing.T) {
	t.Parallel()
	// Exactly at boundary values.
	tests := []struct {
		pct    int
		expect int // expected version for deterministic rand=0.5
	}{
		{0, 1},
		{50, 1}, // 0.5 >= 0.5 => source
		{51, 2}, // 0.5 < 0.51 => target
		{100, 2},
	}

	for _, tt := range tests {
		canary := &CanaryDeployment{
			SourceVersion: 1,
			TargetVersion: 2,
			TrafficPct:    tt.pct,
			Status:        CanaryActive,
		}
		router := newCanaryRouterWithRandFn(func() float64 { return 0.5 })
		got := router.ResolveVersion(canary)
		assert.Equal(t, tt.
			expect,
			got,
		)

	}
}

func TestCanary_AllStatuses(t *testing.T) {
	t.Parallel()
	statuses := []CanaryStatus{
		CanaryActive,
		CanaryPromoting,
		CanaryRollingBack,
		CanaryCompleted,
		CanaryRolledBack,
	}

	router := NewCanaryRouter()
	for _, status := range statuses {
		canary := &CanaryDeployment{
			SourceVersion: 1,
			TargetVersion: 2,
			TrafficPct:    50,
			Status:        status,
		}
		// Must never panic.
		_ = router.ResolveVersion(canary)
	}
}

func TestMarshalAutoPromoteConfig(t *testing.T) {
	t.Parallel()
	config := &AutoPromoteConfig{
		Enabled:              true,
		Steps:                []int{10, 25, 50, 100},
		Interval:             15 * time.Minute,
		FailureRateThreshold: 5.0,
		LatencyP99Threshold:  10 * time.Second,
	}

	data := MarshalAutoPromoteConfig(config)
	require.NotNil(t,
		data)

	nilData := MarshalAutoPromoteConfig(nil)
	assert.Nil(t, nilData)

}

// 2A. ResolveVersion TrafficPct boundary tests.

func TestCanary_ResolveVersion_TrafficPctZero_RandNotCalled(t *testing.T) {
	t.Parallel()
	canary := &CanaryDeployment{
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    0,
		Status:        CanaryActive,
	}
	called := false
	router := newCanaryRouterWithRandFn(func() float64 {
		called = true
		return 0.0
	})
	v := router.ResolveVersion(canary)
	assert.EqualValues(t, 1,
		v)
	assert.False(t, called)

}

func TestCanary_ResolveVersion_TrafficPct100_RandNotCalled(t *testing.T) {
	t.Parallel()
	canary := &CanaryDeployment{
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    100,
		Status:        CanaryActive,
	}
	called := false
	router := newCanaryRouterWithRandFn(func() float64 {
		called = true
		return 0.99
	})
	v := router.ResolveVersion(canary)
	assert.EqualValues(t, 2,
		v)
	assert.False(t, called)

}

func TestCanary_ResolveVersion_TrafficPctNegative(t *testing.T) {
	t.Parallel()
	canary := &CanaryDeployment{
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    -1,
		Status:        CanaryActive,
	}
	router := newCanaryRouterWithRandFn(func() float64 { return 0.0 })
	assert.EqualValues(t, 1,
		router.
			ResolveVersion(canary))

}

func TestCanary_ResolveVersion_TrafficPct101(t *testing.T) {
	t.Parallel()
	canary := &CanaryDeployment{
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    101,
		Status:        CanaryActive,
	}
	router := newCanaryRouterWithRandFn(func() float64 { return 0.99 })
	assert.EqualValues(t, 2,
		router.
			ResolveVersion(canary))

}

// 2B. EvaluateHealth TargetRunCount boundary.

func TestCanary_EvaluateHealth_ExactlyMinRuns(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{
		TargetFailureRate: 1.0,
		TargetLatencyP99:  100 * time.Millisecond,
		TargetRunCount:    5,
	}
	config := &AutoPromoteConfig{
		Enabled:              true,
		FailureRateThreshold: 5.0,
		LatencyP99Threshold:  10 * time.Second,
	}
	decision := EvaluateHealth(health, config)
	assert.Equal(t, CanaryDecisionPromote,

		decision)

}

func TestCanary_EvaluateHealth_OneBelowMinRuns(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{
		TargetFailureRate: 1.0,
		TargetRunCount:    4,
	}
	config := &AutoPromoteConfig{
		Enabled:              true,
		FailureRateThreshold: 5.0,
	}
	decision := EvaluateHealth(health, config)
	assert.Equal(t, CanaryDecisionHold,

		decision)

}

// 2C. EvaluateHealth threshold=0 means disabled.

func TestCanary_EvaluateHealth_ZeroFailureThreshold(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{
		TargetFailureRate: 50.0,
		TargetRunCount:    10,
	}
	config := &AutoPromoteConfig{
		Enabled:              true,
		FailureRateThreshold: 0,
	}
	decision := EvaluateHealth(health, config)
	assert.Equal(t, CanaryDecisionPromote,

		decision)

}

func TestCanary_EvaluateHealth_ZeroLatencyThreshold(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{
		TargetLatencyP99: 999 * time.Second,
		TargetRunCount:   10,
	}
	config := &AutoPromoteConfig{
		Enabled:             true,
		LatencyP99Threshold: 0,
	}
	decision := EvaluateHealth(health, config)
	assert.Equal(t, CanaryDecisionPromote,

		decision)

}

// 2D. ValidateCanaryRequest trafficPct boundary valid values.

func TestValidateCanary_TrafficPctZeroValid(t *testing.T) {
	t.Parallel()
	assert.NoError(t,
		ValidateCanaryRequest("wf-1", 1, 2, 0),
	)

}

func TestValidateCanary_TrafficPct100Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t,
		ValidateCanaryRequest("wf-1", 1, 2, 100))

}

// 2E. NextPromoteStep boundary: currentPct equals a step value.

func TestCanary_NextPromoteStep_CurrentEqualsStep(t *testing.T) {
	t.Parallel()
	config := &AutoPromoteConfig{
		Enabled: true,
		Steps:   []int{10, 25, 50, 100},
	}
	got := NextPromoteStep(config, 10)
	assert.EqualValues(t, 25,
		got)

	got = NextPromoteStep(config, 25)
	assert.EqualValues(t, 50,
		got)

}

func TestCanary_NextPromoteStep_EmptySteps(t *testing.T) {
	t.Parallel()
	config := &AutoPromoteConfig{Enabled: true, Steps: []int{}}
	got := NextPromoteStep(config, 0)
	assert.EqualValues(t, -1,
		got)

}

func TestCanary_NextPromoteStep_DisabledConfig(t *testing.T) {
	t.Parallel()
	config := &AutoPromoteConfig{Enabled: false, Steps: []int{10, 25}}
	got := NextPromoteStep(config, 0)
	assert.EqualValues(t, -1,
		got)

}
