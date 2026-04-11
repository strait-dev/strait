package agents

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
)

// resolveCanaryTargetMockStore is a minimal agentStore that implements
// only the two methods resolveCanaryTarget actually calls. Other
// methods are inherited from the embedded nil interface and would
// panic if invoked — the tests below don't hit them.
type resolveCanaryTargetMockStore struct {
	agentStore
	getCanaryWithTargetFn func(ctx context.Context, agentID string) (*domain.AgentCanaryDeployment, *domain.AgentDeployment, error)
	getDeploymentByIDFn   func(ctx context.Context, id string) (*domain.AgentDeployment, error)
	getDeploymentByIDCalls int
}

func (m *resolveCanaryTargetMockStore) GetActiveAgentCanaryWithTarget(ctx context.Context, agentID string) (*domain.AgentCanaryDeployment, *domain.AgentDeployment, error) {
	return m.getCanaryWithTargetFn(ctx, agentID)
}

func (m *resolveCanaryTargetMockStore) GetAgentDeploymentByID(ctx context.Context, id string) (*domain.AgentDeployment, error) {
	m.getDeploymentByIDCalls++
	return m.getDeploymentByIDFn(ctx, id)
}

// TestResolveCanaryTarget_SourceFallback_TriggersExtraLookup exercises
// the rare path in resolveCanaryTarget (service.go:930-939) where the
// canary router picks the source deployment instead of the target.
// The function should fall back to GetAgentDeploymentByID(source) to
// load the full source row, then return it after the env-match check
// passes. This branch was the only one in the Phase E1.3 canary
// collapse that had no coverage.
func TestResolveCanaryTarget_SourceFallback_TriggersExtraLookup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	primary := &domain.AgentDeployment{
		ID:            "primary-dep",
		AgentID:       "agent-1",
		EnvironmentID: "env-prod",
	}
	canary := &domain.AgentCanaryDeployment{
		AgentID:            "agent-1",
		SourceDeploymentID: "source-dep",
		TargetDeploymentID: "target-dep",
		TrafficPct:         50,
		Status:             AgentCanaryStatusActive,
	}
	target := &domain.AgentDeployment{
		ID:            "target-dep",
		AgentID:       "agent-1",
		EnvironmentID: "env-prod",
	}
	source := &domain.AgentDeployment{
		ID:            "source-dep",
		AgentID:       "agent-1",
		EnvironmentID: "env-prod",
	}

	mock := &resolveCanaryTargetMockStore{
		getCanaryWithTargetFn: func(context.Context, string) (*domain.AgentCanaryDeployment, *domain.AgentDeployment, error) {
			return canary, target, nil
		},
		getDeploymentByIDFn: func(_ context.Context, id string) (*domain.AgentDeployment, error) {
			if id != source.ID {
				return nil, errors.New("unexpected deployment lookup id: " + id)
			}
			return source, nil
		},
	}

	// A randFn that always returns 0.99 forces Route() to pick the
	// source deployment (random value >= threshold 0.50 at traffic=50).
	router := &AgentCanaryRouter{randFn: func() float64 { return 0.99 }}

	svc := &localService{
		store:        mock,
		canaryRouter: router,
	}

	got := svc.resolveCanaryTarget(ctx, "agent-1", primary)
	if got == nil {
		t.Fatal("resolveCanaryTarget() = nil, want source deployment (fallback path)")
	}
	if got.ID != source.ID {
		t.Fatalf("resolveCanaryTarget().ID = %q, want %q (source)", got.ID, source.ID)
	}
	// The fallback branch must invoke GetAgentDeploymentByID exactly
	// once with the source ID. Zero invocations would mean Postgres
	// accidentally returned the target; two would mean a regression
	// that double-fetches.
	if mock.getDeploymentByIDCalls != 1 {
		t.Fatalf("GetAgentDeploymentByID called %d times, want 1", mock.getDeploymentByIDCalls)
	}
}

// TestResolveCanaryTarget_SourceFallback_MissingSourceReturnsNil
// covers the sub-branch where the fallback GetAgentDeploymentByID
// returns an error. resolveCanaryTarget must return nil (and the
// caller sticks with the primary deployment).
func TestResolveCanaryTarget_SourceFallback_MissingSourceReturnsNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	primary := &domain.AgentDeployment{ID: "primary", AgentID: "agent-1", EnvironmentID: "env-prod"}
	canary := &domain.AgentCanaryDeployment{
		AgentID: "agent-1", SourceDeploymentID: "source", TargetDeploymentID: "target",
		TrafficPct: 50, Status: AgentCanaryStatusActive,
	}
	target := &domain.AgentDeployment{ID: "target", AgentID: "agent-1", EnvironmentID: "env-prod"}

	mock := &resolveCanaryTargetMockStore{
		getCanaryWithTargetFn: func(context.Context, string) (*domain.AgentCanaryDeployment, *domain.AgentDeployment, error) {
			return canary, target, nil
		},
		getDeploymentByIDFn: func(context.Context, string) (*domain.AgentDeployment, error) {
			return nil, errors.New("source deployment vanished")
		},
	}
	router := &AgentCanaryRouter{randFn: func() float64 { return 0.99 }} // picks source

	svc := &localService{store: mock, canaryRouter: router}

	if got := svc.resolveCanaryTarget(ctx, "agent-1", primary); got != nil {
		t.Fatalf("resolveCanaryTarget() = %+v, want nil on fallback lookup failure", got)
	}
}

// TestResolveCanaryTarget_TargetHappyPath_NoExtraLookup is the
// complementary check: when the router picks the target, the
// pre-joined target from GetActiveAgentCanaryWithTarget is used
// directly and there is no second roundtrip. This pins the Phase
// E1.3 N+1 collapse from the opposite side of the fallback branch.
func TestResolveCanaryTarget_TargetHappyPath_NoExtraLookup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	primary := &domain.AgentDeployment{ID: "primary", AgentID: "agent-1", EnvironmentID: "env-prod"}
	canary := &domain.AgentCanaryDeployment{
		AgentID: "agent-1", SourceDeploymentID: "source", TargetDeploymentID: "target",
		TrafficPct: 50, Status: AgentCanaryStatusActive,
	}
	target := &domain.AgentDeployment{ID: "target", AgentID: "agent-1", EnvironmentID: "env-prod"}

	mock := &resolveCanaryTargetMockStore{
		getCanaryWithTargetFn: func(context.Context, string) (*domain.AgentCanaryDeployment, *domain.AgentDeployment, error) {
			return canary, target, nil
		},
		getDeploymentByIDFn: func(context.Context, string) (*domain.AgentDeployment, error) {
			t.Fatal("unexpected GetAgentDeploymentByID call on target-picked path")
			return nil, nil
		},
	}
	router := &AgentCanaryRouter{randFn: func() float64 { return 0.1 }} // picks target

	svc := &localService{store: mock, canaryRouter: router}
	got := svc.resolveCanaryTarget(ctx, "agent-1", primary)
	if got == nil || got.ID != target.ID {
		t.Fatalf("resolveCanaryTarget() = %+v, want target", got)
	}
	if mock.getDeploymentByIDCalls != 0 {
		t.Fatalf("GetAgentDeploymentByID called %d times on target path, want 0", mock.getDeploymentByIDCalls)
	}
}
