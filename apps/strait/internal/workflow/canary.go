package workflow

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"
)

// CanaryDeployment represents an active canary deployment between workflow versions.
type CanaryDeployment struct {
	ID                string             `json:"id"`
	WorkflowID        string             `json:"workflow_id"`
	ProjectID         string             `json:"project_id"`
	SourceVersion     int                `json:"source_version"`
	TargetVersion     int                `json:"target_version"`
	TrafficPct        int                `json:"traffic_pct"`
	Status            CanaryStatus       `json:"status"`
	AutoPromoteConfig *AutoPromoteConfig `json:"auto_promote_config,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	CompletedAt       *time.Time         `json:"completed_at,omitempty"`
}

// CanaryStatus represents the current state of a canary deployment.
type CanaryStatus string

const (
	CanaryActive      CanaryStatus = "active"
	CanaryPromoting   CanaryStatus = "promoting"
	CanaryRollingBack CanaryStatus = "rolling_back"
	CanaryCompleted   CanaryStatus = "completed"
	CanaryRolledBack  CanaryStatus = "rolled_back"
)

// AutoPromoteConfig defines automatic promotion thresholds and schedule.
type AutoPromoteConfig struct {
	Enabled              bool          `json:"enabled"`
	Steps                []int         `json:"steps"`                  // e.g., [10, 25, 50, 100]
	Interval             time.Duration `json:"interval"`               // time between steps
	FailureRateThreshold float64       `json:"failure_rate_threshold"` // max failure rate (percent)
	LatencyP99Threshold  time.Duration `json:"latency_p99_threshold"`  // max P99 latency
}

// CanaryRouter determines which workflow version a new run should use
// based on active canary deployment configuration.
type CanaryRouter struct {
	randFn func() float64 // injectable for testing
}

// NewCanaryRouter creates a new CanaryRouter with standard randomness.
func NewCanaryRouter() *CanaryRouter {
	return &CanaryRouter{
		randFn: rand.Float64,
	}
}

// newCanaryRouterWithRandFn creates a CanaryRouter with a custom random function (for testing).
func newCanaryRouterWithRandFn(fn func() float64) *CanaryRouter {
	return &CanaryRouter{randFn: fn}
}

// ResolveVersion determines the workflow version to use for a new run.
// Returns the target version if the random roll falls within the traffic
// percentage, otherwise returns the source version.
func (r *CanaryRouter) ResolveVersion(canary *CanaryDeployment) int {
	if canary == nil || canary.Status != CanaryActive {
		return 0 // no active canary, use latest
	}

	if canary.TrafficPct <= 0 {
		return canary.SourceVersion
	}
	if canary.TrafficPct >= 100 {
		return canary.TargetVersion
	}

	threshold := float64(canary.TrafficPct) / 100.0
	if r.randFn() < threshold {
		return canary.TargetVersion
	}
	return canary.SourceVersion
}

// CanaryHealthCheck represents the health metrics for a canary comparison.
type CanaryHealthCheck struct {
	SourceFailureRate float64       `json:"source_failure_rate"`
	TargetFailureRate float64       `json:"target_failure_rate"`
	SourceLatencyP99  time.Duration `json:"source_latency_p99"`
	TargetLatencyP99  time.Duration `json:"target_latency_p99"`
	SourceRunCount    int           `json:"source_run_count"`
	TargetRunCount    int           `json:"target_run_count"`
}

// EvaluateCanaryHealth determines if a canary should be promoted, rolled back, or held.
type CanaryDecision string

const (
	CanaryDecisionPromote  CanaryDecision = "promote"
	CanaryDecisionRollback CanaryDecision = "rollback"
	CanaryDecisionHold     CanaryDecision = "hold"
)

// EvaluateHealth evaluates canary health metrics against thresholds.
func EvaluateHealth(health CanaryHealthCheck, config *AutoPromoteConfig) CanaryDecision {
	if config == nil || !config.Enabled {
		return CanaryDecisionHold
	}

	// Insufficient data to evaluate.
	minRuns := 5
	if health.TargetRunCount < minRuns {
		return CanaryDecisionHold
	}

	// Check failure rate threshold.
	if config.FailureRateThreshold > 0 && health.TargetFailureRate > config.FailureRateThreshold {
		return CanaryDecisionRollback
	}

	// Check latency threshold.
	if config.LatencyP99Threshold > 0 && health.TargetLatencyP99 > config.LatencyP99Threshold {
		return CanaryDecisionRollback
	}

	// All checks passed, promote.
	return CanaryDecisionPromote
}

// ValidateCanaryRequest validates a canary deployment creation request.
func ValidateCanaryRequest(workflowID string, sourceVersion, targetVersion, trafficPct int) error {
	if workflowID == "" {
		return fmt.Errorf("workflow_id is required")
	}
	if sourceVersion == targetVersion {
		return fmt.Errorf("source and target versions must be different")
	}
	if trafficPct < 0 || trafficPct > 100 {
		return fmt.Errorf("traffic_pct must be between 0 and 100, got %d", trafficPct)
	}
	return nil
}

// NextPromoteStep returns the next traffic percentage in the auto-promote sequence.
// Returns -1 if the current traffic is already at or past the final step.
func NextPromoteStep(config *AutoPromoteConfig, currentPct int) int {
	if config == nil || !config.Enabled || len(config.Steps) == 0 {
		return -1
	}

	for _, step := range config.Steps {
		if step > currentPct {
			return step
		}
	}
	return -1 // already at or past final step
}

// MarshalAutoPromoteConfig serializes auto-promote config to JSON.
func MarshalAutoPromoteConfig(config *AutoPromoteConfig) json.RawMessage {
	if config == nil {
		return nil
	}
	data, _ := json.Marshal(config)
	return data
}
