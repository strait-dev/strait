package authoring

import (
	"context"
	"testing"
)

func TestCreateRunContext_DefaultFields(t *testing.T) {
	ctx, _ := CreateTestContext("run-1")

	if ctx.RunID != "run-1" {
		t.Errorf("expected RunID 'run-1', got %q", ctx.RunID)
	}
	if ctx.Attempt != 1 {
		t.Errorf("expected Attempt 1, got %d", ctx.Attempt)
	}
	if ctx.Logger == nil {
		t.Error("expected Logger to be non-nil")
	}
}

func TestCreateRunContext_WithAttempt(t *testing.T) {
	ctx, _ := CreateTestContext("run-2", WithAttempt(3))

	if ctx.Attempt != 3 {
		t.Errorf("expected Attempt 3, got %d", ctx.Attempt)
	}
}

func TestCreateRunContext_WithContext(t *testing.T) {
	bg := context.Background()
	child, cancel := context.WithCancel(bg)
	defer cancel()

	ctx, _ := CreateTestContext("run-3", WithContext(child))

	if ctx.Ctx != child {
		t.Error("expected Ctx to match provided context")
	}
}

func TestRunContext_Checkpoint(t *testing.T) {
	ctx, record := CreateTestContext("run-cp")

	err := ctx.Checkpoint(map[string]any{"step": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(record.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(record.Checkpoints))
	}
	if record.Checkpoints[0]["step"] != 1 {
		t.Error("expected checkpoint state with step=1")
	}
}

func TestRunContext_ReportProgress(t *testing.T) {
	t.Run("without message", func(t *testing.T) {
		ctx, record := CreateTestContext("run-prog")
		err := ctx.ReportProgress(0.5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(record.ProgressUpdates) != 1 {
			t.Fatalf("expected 1 progress update, got %d", len(record.ProgressUpdates))
		}
		if record.ProgressUpdates[0]["percent"] != 0.5 {
			t.Error("expected percent 0.5")
		}
		if _, ok := record.ProgressUpdates[0]["message"]; ok {
			t.Error("expected no message key")
		}
	})

	t.Run("with message", func(t *testing.T) {
		ctx, record := CreateTestContext("run-prog2")
		err := ctx.ReportProgress(0.75, "almost done")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record.ProgressUpdates[0]["message"] != "almost done" {
			t.Error("expected message 'almost done'")
		}
	})
}

func TestRunContext_Heartbeat(t *testing.T) {
	ctx, record := CreateTestContext("run-hb")

	_ = ctx.Heartbeat()
	_ = ctx.Heartbeat()

	if record.Heartbeats != 2 {
		t.Errorf("expected 2 heartbeats, got %d", record.Heartbeats)
	}
}

func TestRunContext_ReportUsage(t *testing.T) {
	ctx, record := CreateTestContext("run-usage")
	prompt := 100
	completion := 50
	total := 150
	cost := 500

	err := ctx.ReportUsage(UsageReport{
		Provider:         "openai",
		Model:            "gpt-4",
		PromptTokens:     &prompt,
		CompletionTokens: &completion,
		TotalTokens:      &total,
		CostMicrousd:     &cost,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(record.UsageReports) != 1 {
		t.Fatalf("expected 1 usage report, got %d", len(record.UsageReports))
	}
	report := record.UsageReports[0]
	if report["provider"] != "openai" {
		t.Error("expected provider openai")
	}
	if report["model"] != "gpt-4" {
		t.Error("expected model gpt-4")
	}
	if report["prompt_tokens"] != 100 {
		t.Error("expected prompt_tokens 100")
	}
	if report["cost_microusd"] != 500 {
		t.Error("expected cost_microusd 500")
	}
}

func TestRunContext_LogToolCall(t *testing.T) {
	ctx, record := CreateTestContext("run-tc")
	dur := 42

	err := ctx.LogToolCall(ToolCallReport{
		ToolName:   "search",
		Input:      map[string]any{"query": "test"},
		Output:     map[string]any{"results": 5},
		DurationMs: &dur,
		Status:     "success",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(record.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(record.ToolCalls))
	}
	tc := record.ToolCalls[0]
	if tc["tool_name"] != "search" {
		t.Error("expected tool_name search")
	}
	if tc["status"] != "success" {
		t.Error("expected status success")
	}
}

func TestRunContext_SaveOutput(t *testing.T) {
	t.Run("without schema", func(t *testing.T) {
		ctx, record := CreateTestContext("run-out")
		err := ctx.SaveOutput("result", map[string]any{"value": 42})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(record.Outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(record.Outputs))
		}
		if record.Outputs[0]["key"] != "result" {
			t.Error("expected key 'result'")
		}
	})

	t.Run("with schema", func(t *testing.T) {
		ctx, record := CreateTestContext("run-out2")
		schema := map[string]any{"type": "object"}
		err := ctx.SaveOutput("data", map[string]any{"v": 1}, schema)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record.Outputs[0]["schema"] == nil {
			t.Error("expected schema to be set")
		}
	})
}

func TestRunContext_State(t *testing.T) {
	ctx, record := CreateTestContext("run-state")

	t.Run("set and get", func(t *testing.T) {
		err := ctx.State.Set("foo", "bar")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record.StateStore["foo"] != "bar" {
			t.Error("expected state foo=bar")
		}

		result, err := ctx.State.Get("foo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("expected map result, got %T", result)
		}
		if m["value"] != "bar" {
			t.Error("expected value bar")
		}
	})

	t.Run("delete", func(t *testing.T) {
		_ = ctx.State.Set("to_delete", "val")
		err := ctx.State.Delete("to_delete")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := record.StateStore["to_delete"]; ok {
			t.Error("expected key to be deleted")
		}
	})

	t.Run("list", func(t *testing.T) {
		_ = ctx.State.Set("a", 1)
		_ = ctx.State.Set("b", 2)
		items, err := ctx.State.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) < 2 {
			t.Errorf("expected at least 2 items, got %d", len(items))
		}
	})
}

func TestRunContext_StreamChunk(t *testing.T) {
	t.Run("simple chunk", func(t *testing.T) {
		ctx, record := CreateTestContext("run-stream")
		err := ctx.StreamChunk("hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(record.StreamChunks) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(record.StreamChunks))
		}
		if record.StreamChunks[0]["chunk"] != "hello" {
			t.Error("expected chunk 'hello'")
		}
	})

	t.Run("with options", func(t *testing.T) {
		ctx, record := CreateTestContext("run-stream2")
		err := ctx.StreamChunk("world", StreamChunkOption{StreamID: "s1", Done: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record.StreamChunks[0]["stream_id"] != "s1" {
			t.Error("expected stream_id 's1'")
		}
		if record.StreamChunks[0]["done"] != true {
			t.Error("expected done=true")
		}
	})
}

func TestRunContext_WaitForEvent(t *testing.T) {
	ctx, record := CreateTestContext("run-event")

	result, err := ctx.WaitForEvent("payment.completed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["received"] != true {
		t.Error("expected received=true")
	}
	if len(record.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(record.Events))
	}
	if record.Events[0]["event_key"] != "payment.completed" {
		t.Error("expected event_key 'payment.completed'")
	}
}

func TestRunContext_WaitForEvent_WithOptions(t *testing.T) {
	ctx, record := CreateTestContext("run-event-opts")
	timeout := 30

	_, err := ctx.WaitForEvent("approval", WaitForEventOption{TimeoutSecs: &timeout, NotifyUrl: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.Events[0]["timeout_secs"] != 30 {
		t.Error("expected timeout_secs 30")
	}
	if record.Events[0]["notify_url"] != "https://example.com" {
		t.Error("expected notify_url")
	}
}

func TestRunContext_Spawn(t *testing.T) {
	ctx, record := CreateTestContext("run-spawn")
	priority := 5

	result, err := ctx.Spawn(SpawnOptions{
		JobSlug:   "child-job",
		ProjectID: "proj-1",
		Payload:   map[string]any{"input": "data"},
		Priority:  &priority,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["run_id"] != "spawned-123" {
		t.Error("expected run_id 'spawned-123'")
	}
	if len(record.Spawns) != 1 {
		t.Fatalf("expected 1 spawn, got %d", len(record.Spawns))
	}
	if record.Spawns[0]["job_slug"] != "child-job" {
		t.Error("expected job_slug 'child-job'")
	}
	if record.Spawns[0]["priority"] != 5 {
		t.Error("expected priority 5")
	}
}

func TestRunContext_Continue(t *testing.T) {
	t.Run("without payload", func(t *testing.T) {
		ctx, _ := CreateTestContext("run-cont")
		result, err := ctx.Continue()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result["continued"] != true {
			t.Error("expected continued=true")
		}
	})

	t.Run("with payload", func(t *testing.T) {
		ctx, record := CreateTestContext("run-cont2")
		_, err := ctx.Continue(map[string]any{"next": "step"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(record.Continuations) != 1 {
			t.Fatalf("expected 1 continuation, got %d", len(record.Continuations))
		}
	})
}

func TestRunContext_Annotate(t *testing.T) {
	ctx, record := CreateTestContext("run-ann")

	err := ctx.Annotate(map[string]string{"env": "prod", "team": "backend"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(record.Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(record.Annotations))
	}
	if record.Annotations[0]["env"] != "prod" {
		t.Error("expected annotation env=prod")
	}
}

func TestRunContext_Complete(t *testing.T) {
	t.Run("without result", func(t *testing.T) {
		ctx, record := CreateTestContext("run-comp")
		err := ctx.Complete()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !record.Completed {
			t.Error("expected Completed=true")
		}
	})

	t.Run("with result", func(t *testing.T) {
		ctx, record := CreateTestContext("run-comp2")
		err := ctx.Complete(map[string]any{"output": "done"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !record.Completed {
			t.Error("expected Completed=true")
		}
		if record.Result["output"] != "done" {
			t.Error("expected result output=done")
		}
	})
}

func TestRunContext_Fail(t *testing.T) {
	ctx, record := CreateTestContext("run-fail")

	err := ctx.Fail("something went wrong")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !record.Failed {
		t.Error("expected Failed=true")
	}
	if record.FailError != "something went wrong" {
		t.Errorf("expected FailError 'something went wrong', got %q", record.FailError)
	}
}
