package authoring

import (
	"testing"
)

func TestDefineAgent_BasicFields(t *testing.T) {
	agent := DefineAgent(AgentOptions{
		Name:        "Test Agent",
		Slug:        "test-agent",
		EndpointURL: "https://example.com/agent",
		ProjectID:   "proj-1",
		Description: "A test agent",
	})

	if agent.Slug != "test-agent" {
		t.Errorf("expected slug 'test-agent', got %q", agent.Slug)
	}
	if agent.Kind != "job" {
		t.Errorf("expected kind 'job', got %q", agent.Kind)
	}
}

func TestDefineAgent_Tags(t *testing.T) {
	agent := DefineAgent(AgentOptions{
		Name:      "Tagged Agent",
		Slug:      "tagged-agent",
		ProjectID: "proj-1",
		Tags:      map[string]string{"team": "ml"},
	})

	body, err := agent.ToRegistrationBody("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tags, ok := body["tags"].(map[string]string)
	if !ok {
		t.Fatal("expected tags to be map[string]string")
	}
	if tags["strait.kind"] != "agent" {
		t.Error("expected strait.kind=agent tag")
	}
	if tags["team"] != "ml" {
		t.Error("expected team=ml tag")
	}
}

func TestDefineAgent_DefaultTimeoutAndRetry(t *testing.T) {
	agent := DefineAgent(AgentOptions{
		Name:      "Defaults Agent",
		Slug:      "defaults-agent",
		ProjectID: "proj-1",
	})

	body, err := agent.ToRegistrationBody("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if body["timeout_secs"] != 600 {
		t.Errorf("expected timeout_secs 600, got %v", body["timeout_secs"])
	}
	if body["max_attempts"] != 5 {
		t.Errorf("expected max_attempts 5, got %v", body["max_attempts"])
	}
	if body["retry_strategy"] != "exponential" {
		t.Errorf("expected retry_strategy 'exponential', got %v", body["retry_strategy"])
	}
}

func TestDefineAgent_CustomTimeoutAndRetry(t *testing.T) {
	timeout := 300
	attempts := 3
	agent := DefineAgent(AgentOptions{
		Name:          "Custom Agent",
		Slug:          "custom-agent",
		ProjectID:     "proj-1",
		TimeoutSecs:   &timeout,
		MaxAttempts:   &attempts,
		RetryStrategy: "fixed",
	})

	body, err := agent.ToRegistrationBody("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if body["timeout_secs"] != 300 {
		t.Errorf("expected timeout_secs 300, got %v", body["timeout_secs"])
	}
	if body["max_attempts"] != 3 {
		t.Errorf("expected max_attempts 3, got %v", body["max_attempts"])
	}
	if body["retry_strategy"] != "fixed" {
		t.Errorf("expected retry_strategy 'fixed', got %v", body["retry_strategy"])
	}
}

func TestDefineAgent_RunHandler(t *testing.T) {
	var capturedCtx *AgentRunContext
	agent := DefineAgent(AgentOptions{
		Name:      "Runner Agent",
		Slug:      "runner-agent",
		ProjectID: "proj-1",
		Run: func(payload any, ctx *AgentRunContext) (any, error) {
			capturedCtx = ctx
			return map[string]any{"done": true}, nil
		},
	})

	rctx, _ := CreateTestContext("run-agent-1")
	result, err := agent.Run("input", rctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map result")
	}
	if resultMap["done"] != true {
		t.Error("expected done=true")
	}
	if capturedCtx == nil {
		t.Fatal("expected AgentRunContext to be set")
	}
	if capturedCtx.RunID != "run-agent-1" {
		t.Errorf("expected RunID 'run-agent-1', got %q", capturedCtx.RunID)
	}
}

func TestDefineAgent_NilRunHandler(t *testing.T) {
	agent := DefineAgent(AgentOptions{
		Name:      "Empty Agent",
		Slug:      "empty-agent",
		ProjectID: "proj-1",
	})

	rctx, _ := CreateTestContext("run-nil")
	result, err := agent.Run("input", rctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for nil run handler")
	}
}

func TestAgentRunContext_Iteration(t *testing.T) {
	agent := DefineAgent(AgentOptions{
		Name:      "Iter Agent",
		Slug:      "iter-agent",
		ProjectID: "proj-1",
		Run: func(payload any, ctx *AgentRunContext) (any, error) {
			if ctx.Iteration() != 0 {
				t.Errorf("expected initial iteration 0, got %d", ctx.Iteration())
			}
			_ = ctx.Checkpoint(map[string]any{"step": 1})
			if ctx.Iteration() != 1 {
				t.Errorf("expected iteration 1, got %d", ctx.Iteration())
			}
			_ = ctx.Checkpoint(map[string]any{"step": 2})
			if ctx.Iteration() != 2 {
				t.Errorf("expected iteration 2, got %d", ctx.Iteration())
			}
			return nil, nil
		},
	})

	rctx, _ := CreateTestContext("run-iter")
	_, err := agent.Run("input", rctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentRunContext_CostTracking(t *testing.T) {
	maxCost := int64(10000)
	agent := DefineAgent(AgentOptions{
		Name:            "Cost Agent",
		Slug:            "cost-agent",
		ProjectID:       "proj-1",
		MaxCostMicrousd: &maxCost,
		Run: func(payload any, ctx *AgentRunContext) (any, error) {
			cost := 5000
			_ = ctx.ReportUsage(UsageReport{
				Provider:     "openai",
				Model:        "gpt-4",
				CostMicrousd: &cost,
			})
			if ctx.AccumulatedCostMicrousd() != 5000 {
				t.Errorf("expected accumulated cost 5000, got %d", ctx.AccumulatedCostMicrousd())
			}
			if ctx.IsBudgetExceeded() {
				t.Error("budget should not be exceeded yet")
			}

			cost2 := 6000
			_ = ctx.ReportUsage(UsageReport{
				Provider:     "openai",
				Model:        "gpt-4",
				CostMicrousd: &cost2,
			})
			if ctx.AccumulatedCostMicrousd() != 11000 {
				t.Errorf("expected accumulated cost 11000, got %d", ctx.AccumulatedCostMicrousd())
			}
			if !ctx.IsBudgetExceeded() {
				t.Error("budget should be exceeded")
			}
			return nil, nil
		},
	})

	rctx, _ := CreateTestContext("run-cost")
	_, err := agent.Run("input", rctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentRunContext_AutoCheckpointEnabled(t *testing.T) {
	agent := DefineAgent(AgentOptions{
		Name:      "AutoCP Agent",
		Slug:      "autocp-agent",
		ProjectID: "proj-1",
		Run: func(payload any, ctx *AgentRunContext) (any, error) {
			_ = ctx.Checkpoint(map[string]any{"step": 1})
			return nil, nil
		},
	})

	rctx, record := CreateTestContext("run-autocp")
	_, _ = agent.Run("input", rctx)

	if len(record.Checkpoints) != 1 {
		t.Errorf("expected 1 checkpoint (auto), got %d", len(record.Checkpoints))
	}
}

func TestAgentRunContext_AutoCheckpointDisabled(t *testing.T) {
	autoCP := false
	agent := DefineAgent(AgentOptions{
		Name:           "NoAutoCP Agent",
		Slug:           "noautocp-agent",
		ProjectID:      "proj-1",
		AutoCheckpoint: &autoCP,
		Run: func(payload any, ctx *AgentRunContext) (any, error) {
			_ = ctx.Checkpoint(map[string]any{"step": 1})
			if ctx.Iteration() != 1 {
				t.Errorf("expected iteration still incremented, got %d", ctx.Iteration())
			}
			return nil, nil
		},
	})

	rctx, record := CreateTestContext("run-noautocp")
	_, _ = agent.Run("input", rctx)

	if len(record.Checkpoints) != 0 {
		t.Errorf("expected 0 checkpoints (auto disabled), got %d", len(record.Checkpoints))
	}
}

func TestDefineAgent_OnStartCallback(t *testing.T) {
	onStartCalled := false
	agent := DefineAgent(AgentOptions{
		Name:      "OnStart Agent",
		Slug:      "onstart-agent",
		ProjectID: "proj-1",
		OnStart: func(payload any, ctx RunContext) error {
			onStartCalled = true
			return nil
		},
	})

	if agent.OnStart == nil {
		t.Error("expected OnStart to be set")
	}
	rctx, _ := CreateTestContext("run-onstart")
	_ = agent.OnStart("input", rctx)
	if !onStartCalled {
		t.Error("expected OnStart to be called")
	}
}

func TestDefineAgent_OnSuccessCallback(t *testing.T) {
	onSuccessCalled := false
	agent := DefineAgent(AgentOptions{
		Name:      "OnSuccess Agent",
		Slug:      "onsuccess-agent",
		ProjectID: "proj-1",
		OnSuccess: func(payload any, result any, ctx RunContext) error {
			onSuccessCalled = true
			return nil
		},
	})

	if agent.OnSuccess == nil {
		t.Error("expected OnSuccess to be set")
	}
	rctx, _ := CreateTestContext("run-onsuccess")
	_ = agent.OnSuccess("input", "result", rctx)
	if !onSuccessCalled {
		t.Error("expected OnSuccess to be called")
	}
}

func TestDefineAgent_OnFailureCallback(t *testing.T) {
	onFailureCalled := false
	agent := DefineAgent(AgentOptions{
		Name:      "OnFailure Agent",
		Slug:      "onfailure-agent",
		ProjectID: "proj-1",
		OnFailure: func(payload any, err error, ctx RunContext) error {
			onFailureCalled = true
			return nil
		},
	})

	if agent.OnFailure == nil {
		t.Error("expected OnFailure to be set")
	}
	rctx, _ := CreateTestContext("run-onfailure")
	_ = agent.OnFailure("input", nil, rctx)
	if !onFailureCalled {
		t.Error("expected OnFailure to be called")
	}
}
