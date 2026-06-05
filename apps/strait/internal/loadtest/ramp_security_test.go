//go:build loadtest

package loadtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	require.NoError(t,

		err)
	require.NotEqual(t,

		"", result.
			Bottleneck,
	)
	require.Len(t, result.
		Steps,
		1)

	step := result.Steps[0]
	require.True(t, step.
		StoppedEarly,
	)
	require.Equal(t, result.
		Bottleneck,
		step.
			StopReason,
	)

}
