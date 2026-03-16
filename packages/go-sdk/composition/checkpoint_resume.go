package composition

import (
	"github.com/strait-dev/go-sdk/authoring"
)

// CheckpointResumeOptions configures checkpoint resume behavior.
type CheckpointResumeOptions struct {
	InitialState       map[string]any
	CheckpointInterval int // default 1
}

// WithCheckpointResume wraps a function with checkpoint/resume state management.
func WithCheckpointResume[T any](
	ctx authoring.RunContext,
	lastCheckpoint map[string]any,
	fn func(state map[string]any, updateState func(map[string]any)) (T, error),
	opts CheckpointResumeOptions,
) (T, error) {
	currentState := opts.InitialState
	if lastCheckpoint != nil {
		currentState = lastCheckpoint
	}

	interval := opts.CheckpointInterval
	if interval <= 0 {
		interval = 1
	}
	stepCount := 0

	updateState := func(newState map[string]any) {
		currentState = newState
		stepCount++
		if stepCount%interval == 0 && ctx.Checkpoint != nil {
			_ = ctx.Checkpoint(currentState)
		}
	}

	result, err := fn(currentState, updateState)
	if err != nil {
		var zero T
		return zero, err
	}

	// Final checkpoint
	if ctx.Checkpoint != nil {
		_ = ctx.Checkpoint(currentState)
	}

	return result, nil
}
