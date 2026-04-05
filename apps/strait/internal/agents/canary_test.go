package agents

import (
	"testing"

	"strait/internal/domain"
)

func TestAgentCanaryRouterRouteZeroTraffic(t *testing.T) {
	t.Parallel()
	router := &AgentCanaryRouter{randFn: func() float64 { return 0.5 }}
	canary := &domain.AgentCanaryDeployment{
		Status:             AgentCanaryStatusActive,
		SourceDeploymentID: "source",
		TargetDeploymentID: "target",
		TrafficPct:         0,
	}
	if got := router.Route(canary); got != "source" {
		t.Fatalf("Route() = %q, want source", got)
	}
}

func TestAgentCanaryRouterRouteFullTraffic(t *testing.T) {
	t.Parallel()
	router := &AgentCanaryRouter{randFn: func() float64 { return 0.99 }}
	canary := &domain.AgentCanaryDeployment{
		Status:             AgentCanaryStatusActive,
		SourceDeploymentID: "source",
		TargetDeploymentID: "target",
		TrafficPct:         100,
	}
	if got := router.Route(canary); got != "target" {
		t.Fatalf("Route() = %q, want target", got)
	}
}

func TestAgentCanaryRouterRouteSplitTraffic(t *testing.T) {
	t.Parallel()

	// When random value is below threshold, route to target.
	router := &AgentCanaryRouter{randFn: func() float64 { return 0.1 }}
	canary := &domain.AgentCanaryDeployment{
		Status:             AgentCanaryStatusActive,
		SourceDeploymentID: "source",
		TargetDeploymentID: "target",
		TrafficPct:         50,
	}
	if got := router.Route(canary); got != "target" {
		t.Fatalf("Route(rand=0.1, pct=50) = %q, want target", got)
	}

	// When random value is above threshold, route to source.
	router = &AgentCanaryRouter{randFn: func() float64 { return 0.8 }}
	if got := router.Route(canary); got != "source" {
		t.Fatalf("Route(rand=0.8, pct=50) = %q, want source", got)
	}
}

func TestAgentCanaryRouterRouteNilCanary(t *testing.T) {
	t.Parallel()
	router := NewAgentCanaryRouter()
	if got := router.Route(nil); got != "" {
		t.Fatalf("Route(nil) = %q, want empty", got)
	}
}

func TestAgentCanaryRouterRouteInactiveCanary(t *testing.T) {
	t.Parallel()
	router := NewAgentCanaryRouter()
	canary := &domain.AgentCanaryDeployment{
		Status:             AgentCanaryStatusCompleted,
		SourceDeploymentID: "source",
		TargetDeploymentID: "target",
		TrafficPct:         50,
	}
	if got := router.Route(canary); got != "" {
		t.Fatalf("Route(completed) = %q, want empty", got)
	}
}
