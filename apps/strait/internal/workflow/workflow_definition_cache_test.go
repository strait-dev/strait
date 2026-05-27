package workflow

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestWorkflowDefinitionCache_EngineCachesAndClonesSteps(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	var calls atomic.Int32
	steps := []domain.WorkflowStep{{
		ID:                    "step-1",
		StepRef:               "first",
		DependsOn:             []string{"root"},
		Condition:             []byte(`{"if":true}`),
		Payload:               []byte(`{"payload":true}`),
		ApprovalApprovers:     []string{"user-1"},
		StageNotifications:    []byte(`{"notify":true}`),
		TimeoutSecsOverride:   30,
		RetryMaxAttempts:      2,
		RetryInitialDelaySecs: 1,
	}}
	engine := NewWorkflowEngine(&mockEngineStore{
		listStepsByWorkflowVerFn: func(context.Context, string, int) ([]domain.WorkflowStep, error) {
			calls.Add(1)
			return steps, nil
		},
	}, &mockEngineQueue{}, slog.Default()).WithDefinitionCaches(WorkflowDefinitionCacheConfig{
		Redis:      rdb,
		VersionTTL: time.Minute,
	})

	got, err := engine.listStepsByWorkflowVersion(t.Context(), "wf-1", 7)
	if err != nil {
		t.Fatalf("first listStepsByWorkflowVersion() error = %v", err)
	}
	got[0].DependsOn[0] = "poisoned"
	got[0].Condition[0] = '['
	got[0].Payload[0] = '['
	got[0].ApprovalApprovers[0] = "poisoned"
	got[0].StageNotifications[0] = '['

	got, err = engine.listStepsByWorkflowVersion(t.Context(), "wf-1", 7)
	if err != nil {
		t.Fatalf("second listStepsByWorkflowVersion() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("store calls = %d, want 1", calls.Load())
	}
	if got[0].DependsOn[0] != "root" || got[0].ApprovalApprovers[0] != "user-1" {
		t.Fatalf("cached steps were mutated: %+v", got[0])
	}
	byteFieldsWereCloned := string(got[0].Condition) == `{"if":true}` &&
		string(got[0].Payload) == `{"payload":true}` &&
		string(got[0].StageNotifications) == `{"notify":true}`
	if !byteFieldsWereCloned {
		t.Fatalf("cached byte fields were mutated: %+v", got[0])
	}
}

func TestWorkflowDefinitionCache_CallbackUsesSharedRedisL2(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	seed := NewWorkflowEngine(&mockEngineStore{
		listStepsByWorkflowVerFn: func(context.Context, string, int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "step-1", StepRef: "first", DependsOn: []string{"root"}}}, nil
		},
	}, &mockEngineQueue{}, slog.Default()).WithDefinitionCaches(WorkflowDefinitionCacheConfig{
		Redis:      rdb,
		VersionTTL: time.Minute,
	})
	if _, err := seed.listStepsByWorkflowVersion(t.Context(), "wf-shared", 3); err != nil {
		t.Fatalf("seed listStepsByWorkflowVersion() error = %v", err)
	}

	var callbackStoreCalls atomic.Int32
	cb := NewStepCallback(&mockCallbackStore{
		listStepsByWorkflowVerFn: func(context.Context, string, int) ([]domain.WorkflowStep, error) {
			callbackStoreCalls.Add(1)
			return nil, errors.New("store should not be called on L2 hit")
		},
	}, seed, slog.Default()).WithDefinitionCaches(WorkflowDefinitionCacheConfig{
		Redis:      rdb,
		VersionTTL: time.Minute,
	})

	got, err := cb.loadStepDefinitions(t.Context(), &domain.WorkflowRun{
		ID:              "run-1",
		WorkflowID:      "wf-shared",
		WorkflowVersion: 3,
	})
	if err != nil {
		t.Fatalf("loadStepDefinitions() error = %v", err)
	}
	if callbackStoreCalls.Load() != 0 {
		t.Fatalf("callback store calls = %d, want 0", callbackStoreCalls.Load())
	}
	if len(got) != 1 || got[0].StepRef != "first" || got[0].DependsOn[0] != "root" {
		t.Fatalf("loadStepDefinitions() = %+v, want seeded L2 steps", got)
	}
}
