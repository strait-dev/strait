package authoring

import (
	"testing"
)

func TestCreateTestContext_ReturnsRunContext(t *testing.T) {
	ctx, record := CreateTestContext("test-run-1")

	if ctx.RunID != "test-run-1" {
		t.Errorf("expected RunID 'test-run-1', got %q", ctx.RunID)
	}
	if record == nil {
		t.Fatal("expected non-nil record")
	}
}

func TestCreateTestContext_DefaultAttempt(t *testing.T) {
	ctx, _ := CreateTestContext("test-run-2")
	if ctx.Attempt != 1 {
		t.Errorf("expected default Attempt 1, got %d", ctx.Attempt)
	}
}

func TestCreateTestContext_WithCustomAttempt(t *testing.T) {
	ctx, _ := CreateTestContext("test-run-3", WithAttempt(5))
	if ctx.Attempt != 5 {
		t.Errorf("expected Attempt 5, got %d", ctx.Attempt)
	}
}

func TestTestRunRecord_Checkpoints(t *testing.T) {
	ctx, record := CreateTestContext("test-cp")
	_ = ctx.Checkpoint(map[string]any{"phase": "init"})
	_ = ctx.Checkpoint(map[string]any{"phase": "done"})

	if len(record.Checkpoints) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(record.Checkpoints))
	}
	if record.Checkpoints[0]["phase"] != "init" {
		t.Error("expected first checkpoint phase=init")
	}
	if record.Checkpoints[1]["phase"] != "done" {
		t.Error("expected second checkpoint phase=done")
	}
}

func TestTestRunRecord_Heartbeats(t *testing.T) {
	ctx, record := CreateTestContext("test-hb")
	_ = ctx.Heartbeat()
	_ = ctx.Heartbeat()
	_ = ctx.Heartbeat()

	if record.Heartbeats != 3 {
		t.Errorf("expected 3 heartbeats, got %d", record.Heartbeats)
	}
}

func TestTestRunRecord_ProgressUpdates(t *testing.T) {
	ctx, record := CreateTestContext("test-prog")
	_ = ctx.ReportProgress(0.25, "quarter done")
	_ = ctx.ReportProgress(0.5)

	if len(record.ProgressUpdates) != 2 {
		t.Fatalf("expected 2 progress updates, got %d", len(record.ProgressUpdates))
	}
	if record.ProgressUpdates[0]["percent"] != 0.25 {
		t.Error("expected first progress percent=0.25")
	}
	if record.ProgressUpdates[0]["message"] != "quarter done" {
		t.Error("expected first progress message")
	}
}

func TestTestRunRecord_UsageReports(t *testing.T) {
	ctx, record := CreateTestContext("test-usage")
	cost := 1000
	_ = ctx.ReportUsage(UsageReport{Provider: "anthropic", Model: "claude-3", CostMicrousd: &cost})

	if len(record.UsageReports) != 1 {
		t.Fatalf("expected 1 usage report, got %d", len(record.UsageReports))
	}
	if record.UsageReports[0]["provider"] != "anthropic" {
		t.Error("expected provider=anthropic")
	}
}

func TestTestRunRecord_ToolCalls(t *testing.T) {
	ctx, record := CreateTestContext("test-tc")
	_ = ctx.LogToolCall(ToolCallReport{ToolName: "search", Status: "success"})

	if len(record.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(record.ToolCalls))
	}
	if record.ToolCalls[0]["tool_name"] != "search" {
		t.Error("expected tool_name=search")
	}
}

func TestTestRunRecord_Outputs(t *testing.T) {
	ctx, record := CreateTestContext("test-out")
	_ = ctx.SaveOutput("result", map[string]any{"value": 42})

	if len(record.Outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(record.Outputs))
	}
	if record.Outputs[0]["key"] != "result" {
		t.Error("expected key=result")
	}
}

func TestTestRunRecord_StateStore(t *testing.T) {
	ctx, record := CreateTestContext("test-state")
	_ = ctx.State.Set("key1", "value1")
	_ = ctx.State.Set("key2", "value2")

	if record.StateStore["key1"] != "value1" {
		t.Error("expected key1=value1")
	}
	if record.StateStore["key2"] != "value2" {
		t.Error("expected key2=value2")
	}

	_ = ctx.State.Delete("key1")
	if _, ok := record.StateStore["key1"]; ok {
		t.Error("expected key1 to be deleted")
	}
}

func TestTestRunRecord_StreamChunks(t *testing.T) {
	ctx, record := CreateTestContext("test-stream")
	_ = ctx.StreamChunk("chunk1")
	_ = ctx.StreamChunk("chunk2", StreamChunkOption{Done: true})

	if len(record.StreamChunks) != 2 {
		t.Fatalf("expected 2 stream chunks, got %d", len(record.StreamChunks))
	}
	if record.StreamChunks[0]["chunk"] != "chunk1" {
		t.Error("expected first chunk='chunk1'")
	}
	if record.StreamChunks[1]["done"] != true {
		t.Error("expected second chunk done=true")
	}
}

func TestTestRunRecord_Spawns(t *testing.T) {
	ctx, record := CreateTestContext("test-spawn")
	_, _ = ctx.Spawn(SpawnOptions{JobSlug: "child", ProjectID: "proj"})

	if len(record.Spawns) != 1 {
		t.Fatalf("expected 1 spawn, got %d", len(record.Spawns))
	}
	if record.Spawns[0]["job_slug"] != "child" {
		t.Error("expected job_slug=child")
	}
}

func TestTestRunRecord_Events(t *testing.T) {
	ctx, record := CreateTestContext("test-event")
	_, _ = ctx.WaitForEvent("my.event")

	if len(record.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(record.Events))
	}
	if record.Events[0]["event_key"] != "my.event" {
		t.Error("expected event_key=my.event")
	}
}

func TestTestRunRecord_Annotations(t *testing.T) {
	ctx, record := CreateTestContext("test-ann")
	_ = ctx.Annotate(map[string]string{"key": "value"})

	if len(record.Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(record.Annotations))
	}
	if record.Annotations[0]["key"] != "value" {
		t.Error("expected annotation key=value")
	}
}

func TestTestRunRecord_CompletedAndFailed(t *testing.T) {
	t.Run("complete", func(t *testing.T) {
		ctx, record := CreateTestContext("test-comp")
		_ = ctx.Complete(map[string]any{"status": "ok"})

		if !record.Completed {
			t.Error("expected Completed=true")
		}
		if record.Result["status"] != "ok" {
			t.Error("expected result status=ok")
		}
		if record.Failed {
			t.Error("expected Failed=false")
		}
	})

	t.Run("fail", func(t *testing.T) {
		ctx, record := CreateTestContext("test-fail")
		_ = ctx.Fail("oops")

		if !record.Failed {
			t.Error("expected Failed=true")
		}
		if record.FailError != "oops" {
			t.Errorf("expected FailError='oops', got %q", record.FailError)
		}
		if record.Completed {
			t.Error("expected Completed=false")
		}
	})
}

func TestTestRunRecord_Continuations(t *testing.T) {
	ctx, record := CreateTestContext("test-cont")
	_, _ = ctx.Continue(map[string]any{"next": true})

	if len(record.Continuations) != 1 {
		t.Fatalf("expected 1 continuation, got %d", len(record.Continuations))
	}
}
