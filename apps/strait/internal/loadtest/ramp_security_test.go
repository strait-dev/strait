//go:build loadtest

package loadtest

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRampEngine_MarksBreakingStep(t *testing.T) {
	engine := NewRampEngine(RampConfig{
		Mode:         RampConcurrency,
		StartRate:    1,
		StepSize:     10,
		StepInterval: 20 * time.Millisecond,
		StopCondition: StopCondition{
			MaxErrorRate: 0.01,
		},
	}, func(context.Context) error {
		return errors.New("forced failure")
	})

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Bottleneck == "" {
		t.Fatal("expected stop condition to set a bottleneck")
	}
	if len(result.Steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(result.Steps))
	}
	step := result.Steps[0]
	if !step.StoppedEarly {
		t.Fatal("breaking step should be marked stopped early")
	}
	if step.StopReason != result.Bottleneck {
		t.Fatalf("step stop reason = %q, want %q", step.StopReason, result.Bottleneck)
	}
}
