package workflow

import (
	"testing"
	"time"
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
	if ratio < 0.07 || ratio > 0.13 {
		t.Errorf("10%% canary: target ratio = %.3f, want ~0.10", ratio)
	}
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
	if ratio < 0.45 || ratio > 0.55 {
		t.Errorf("50%% canary: target ratio = %.3f, want ~0.50", ratio)
	}
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
		if router.ResolveVersion(canary) != 2 {
			t.Fatal("100% canary should always return target version")
		}
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
		if router.ResolveVersion(canary) != 1 {
			t.Fatal("0% canary should always return source version")
		}
	}
}

func TestCanary_NoActiveCanary(t *testing.T) {
	t.Parallel()
	router := NewCanaryRouter()
	if v := router.ResolveVersion(nil); v != 0 {
		t.Errorf("nil canary should return 0, got %d", v)
	}
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
	if v := router.ResolveVersion(canary); v != 0 {
		t.Errorf("completed canary should return 0, got %d", v)
	}
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
	// 0.3 < 0.5 threshold => target version.
	if v := router.ResolveVersion(canary); v != 2 {
		t.Errorf("expected target version 2, got %d", v)
	}

	// Always returns 0.7.
	router = newCanaryRouterWithRandFn(func() float64 { return 0.7 })
	// 0.7 >= 0.5 threshold => source version.
	if v := router.ResolveVersion(canary); v != 1 {
		t.Errorf("expected source version 1, got %d", v)
	}
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
	if decision != CanaryDecisionPromote {
		t.Errorf("decision = %s, want promote", decision)
	}
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
	if decision != CanaryDecisionRollback {
		t.Errorf("decision = %s, want rollback", decision)
	}
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
	if decision != CanaryDecisionRollback {
		t.Errorf("decision = %s, want rollback", decision)
	}
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
	if decision != CanaryDecisionHold {
		t.Errorf("decision = %s, want hold (insufficient data)", decision)
	}
}

func TestCanaryMonitor_DisabledConfig(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{TargetRunCount: 100}
	config := &AutoPromoteConfig{Enabled: false}

	decision := EvaluateHealth(health, config)
	if decision != CanaryDecisionHold {
		t.Errorf("decision = %s, want hold (disabled)", decision)
	}
}

func TestCanaryMonitor_NilConfig(t *testing.T) {
	t.Parallel()
	health := CanaryHealthCheck{TargetRunCount: 100}
	decision := EvaluateHealth(health, nil)
	if decision != CanaryDecisionHold {
		t.Errorf("decision = %s, want hold (nil config)", decision)
	}
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
		if got != tt.want {
			t.Errorf("NextPromoteStep(current=%d) = %d, want %d", tt.current, got, tt.want)
		}
	}
}

func TestCanaryMonitor_NilPromoteConfig(t *testing.T) {
	t.Parallel()
	got := NextPromoteStep(nil, 10)
	if got != -1 {
		t.Errorf("expected -1 for nil config, got %d", got)
	}
}

// Validation tests.

func TestValidateCanary_Valid(t *testing.T) {
	t.Parallel()
	err := ValidateCanaryRequest("wf-1", 1, 2, 10)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateCanary_SameVersions(t *testing.T) {
	t.Parallel()
	err := ValidateCanaryRequest("wf-1", 1, 1, 10)
	if err == nil {
		t.Error("expected error for same versions")
	}
}

func TestValidateCanary_NegativeTrafficPct(t *testing.T) {
	t.Parallel()
	err := ValidateCanaryRequest("wf-1", 1, 2, -1)
	if err == nil {
		t.Error("expected error for negative traffic pct")
	}
}

func TestValidateCanary_TrafficPctOver100(t *testing.T) {
	t.Parallel()
	err := ValidateCanaryRequest("wf-1", 1, 2, 150)
	if err == nil {
		t.Error("expected error for traffic pct > 100")
	}
}

func TestValidateCanary_EmptyWorkflowID(t *testing.T) {
	t.Parallel()
	err := ValidateCanaryRequest("", 1, 2, 10)
	if err == nil {
		t.Error("expected error for empty workflow ID")
	}
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
		if v != 1 && v != 2 {
			t.Errorf("unexpected version %d for pct=%d", v, pct)
		}
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
	// Non-active canary should return 0.
	if v := router.ResolveVersion(canary); v != 0 {
		t.Errorf("promoting canary should return 0, got %d", v)
	}
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
	if decision != CanaryDecisionHold {
		t.Errorf("decision = %s, want hold for zero runs", decision)
	}
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
		if got != tt.expect {
			t.Errorf("pct=%d rand=0.5: got version %d, want %d", tt.pct, got, tt.expect)
		}
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
	if data == nil {
		t.Fatal("expected non-nil data")
	}

	nilData := MarshalAutoPromoteConfig(nil)
	if nilData != nil {
		t.Error("expected nil for nil config")
	}
}
