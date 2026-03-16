package composition

import (
	"testing"

	"github.com/strait-dev/go-sdk/authoring"
)

func TestWithCheckpointResume_InitialState(t *testing.T) {
	ctx, record := authoring.CreateTestContext("test-cp-resume")

	result, err := WithCheckpointResume(ctx, nil, func(state map[string]any, updateState func(map[string]any)) (string, error) {
		if state["step"] != "init" {
			t.Errorf("expected initial state step=init, got %v", state["step"])
		}
		updateState(map[string]any{"step": "done"})
		return "completed", nil
	}, CheckpointResumeOptions{
		InitialState: map[string]any{"step": "init"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "completed" {
		t.Errorf("expected 'completed', got %q", result)
	}
	// Should have 2 checkpoints: one from updateState, one final
	if len(record.Checkpoints) != 2 {
		t.Errorf("expected 2 checkpoints, got %d", len(record.Checkpoints))
	}
}

func TestWithCheckpointResume_LastCheckpointOverridesInitial(t *testing.T) {
	ctx, _ := authoring.CreateTestContext("test-cp-override")

	_, err := WithCheckpointResume(ctx, map[string]any{"step": "resumed"}, func(state map[string]any, updateState func(map[string]any)) (string, error) {
		if state["step"] != "resumed" {
			t.Errorf("expected resumed state, got %v", state["step"])
		}
		return "ok", nil
	}, CheckpointResumeOptions{
		InitialState: map[string]any{"step": "init"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWithCheckpointResume_DefaultInterval(t *testing.T) {
	ctx, record := authoring.CreateTestContext("test-cp-interval")

	_, err := WithCheckpointResume(ctx, nil, func(state map[string]any, updateState func(map[string]any)) (string, error) {
		updateState(map[string]any{"step": 1})
		updateState(map[string]any{"step": 2})
		updateState(map[string]any{"step": 3})
		return "done", nil
	}, CheckpointResumeOptions{
		InitialState: map[string]any{"step": 0},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 3 from updateState (interval=1) + 1 final = 4
	if len(record.Checkpoints) != 4 {
		t.Errorf("expected 4 checkpoints, got %d", len(record.Checkpoints))
	}
}

func TestWithCheckpointResume_CustomInterval(t *testing.T) {
	ctx, record := authoring.CreateTestContext("test-cp-custom-interval")

	_, err := WithCheckpointResume(ctx, nil, func(state map[string]any, updateState func(map[string]any)) (string, error) {
		updateState(map[string]any{"step": 1}) // step 1, no checkpoint (1%3 != 0)
		updateState(map[string]any{"step": 2}) // step 2, no checkpoint (2%3 != 0)
		updateState(map[string]any{"step": 3}) // step 3, checkpoint (3%3 == 0)
		updateState(map[string]any{"step": 4}) // step 4, no checkpoint (4%3 != 0)
		return "done", nil
	}, CheckpointResumeOptions{
		InitialState:       map[string]any{"step": 0},
		CheckpointInterval: 3,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1 from updateState at step 3 + 1 final = 2
	if len(record.Checkpoints) != 2 {
		t.Errorf("expected 2 checkpoints, got %d", len(record.Checkpoints))
	}
}

func TestWithCheckpointResume_ErrorPropagation(t *testing.T) {
	ctx, _ := authoring.CreateTestContext("test-cp-error")

	_, err := WithCheckpointResume(ctx, nil, func(state map[string]any, updateState func(map[string]any)) (string, error) {
		return "", errTest
	}, CheckpointResumeOptions{
		InitialState: map[string]any{"step": 0},
	})

	if err == nil {
		t.Fatal("expected error")
	}
	if err != errTest {
		t.Errorf("expected errTest, got %v", err)
	}
}

var errTest = &testError{msg: "test error"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestWithCheckpointResume_NilLastCheckpoint(t *testing.T) {
	ctx, _ := authoring.CreateTestContext("test-cp-nil")

	_, err := WithCheckpointResume(ctx, nil, func(state map[string]any, updateState func(map[string]any)) (string, error) {
		if state["init"] != true {
			t.Error("expected initial state")
		}
		return "ok", nil
	}, CheckpointResumeOptions{
		InitialState: map[string]any{"init": true},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWithCheckpointResume_FinalCheckpointAlways(t *testing.T) {
	ctx, record := authoring.CreateTestContext("test-cp-final")

	_, err := WithCheckpointResume(ctx, nil, func(state map[string]any, updateState func(map[string]any)) (string, error) {
		// No updateState calls, but final checkpoint should still fire
		return "ok", nil
	}, CheckpointResumeOptions{
		InitialState: map[string]any{"step": 0},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(record.Checkpoints) != 1 {
		t.Errorf("expected 1 final checkpoint, got %d", len(record.Checkpoints))
	}
}

func TestWithCheckpointResume_StateUpdatedCorrectly(t *testing.T) {
	ctx, record := authoring.CreateTestContext("test-cp-state-update")

	_, err := WithCheckpointResume(ctx, nil, func(state map[string]any, updateState func(map[string]any)) (string, error) {
		updateState(map[string]any{"count": 1})
		updateState(map[string]any{"count": 2})
		return "ok", nil
	}, CheckpointResumeOptions{
		InitialState: map[string]any{"count": 0},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Last checkpoint should have count=2
	lastCP := record.Checkpoints[len(record.Checkpoints)-1]
	if lastCP["count"] != 2 {
		t.Errorf("expected final state count=2, got %v", lastCP["count"])
	}
}

func TestWithCheckpointResume_ZeroInterval(t *testing.T) {
	ctx, record := authoring.CreateTestContext("test-cp-zero-interval")

	_, err := WithCheckpointResume(ctx, nil, func(state map[string]any, updateState func(map[string]any)) (string, error) {
		updateState(map[string]any{"step": 1})
		updateState(map[string]any{"step": 2})
		return "ok", nil
	}, CheckpointResumeOptions{
		InitialState:       map[string]any{"step": 0},
		CheckpointInterval: 0, // should default to 1
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2 from updateState (interval=1) + 1 final = 3
	if len(record.Checkpoints) != 3 {
		t.Errorf("expected 3 checkpoints, got %d", len(record.Checkpoints))
	}
}
