package agents

import (
	"math/rand/v2"

	"strait/internal/domain"
)

const (
	AgentCanaryStatusActive     = "active"
	AgentCanaryStatusCompleted  = "completed"
	AgentCanaryStatusRolledBack = "rolled_back"
)

// AgentCanaryRouter picks which deployment to use for a new agent run
// based on the canary traffic percentage.
type AgentCanaryRouter struct {
	randFn func() float64
}

// NewAgentCanaryRouter creates a router with standard randomness.
func NewAgentCanaryRouter() *AgentCanaryRouter {
	return &AgentCanaryRouter{randFn: rand.Float64}
}

// Route returns the deployment ID to use for this run. When a canary is
// active, runs are routed to the target deployment with probability
// equal to traffic_pct / 100.
func (r *AgentCanaryRouter) Route(canary *domain.AgentCanaryDeployment) string {
	if canary == nil || canary.Status != AgentCanaryStatusActive {
		return ""
	}
	if r.randFn == nil {
		return canary.SourceDeploymentID
	}
	if canary.TrafficPct <= 0 {
		return canary.SourceDeploymentID
	}
	if canary.TrafficPct >= 100 {
		return canary.TargetDeploymentID
	}
	threshold := float64(canary.TrafficPct) / 100.0
	if r.randFn() < threshold {
		return canary.TargetDeploymentID
	}
	return canary.SourceDeploymentID
}
