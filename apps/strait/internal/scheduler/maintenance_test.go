package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMaintenanceLoop_Run(t *testing.T) {
	t.Parallel()
	var ticks atomic.Int32
	loop := NewMaintenanceLoop("test-loop", 20*time.Millisecond, nil, func(context.Context) {
		ticks.Add(1)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Millisecond)
	defer cancel()

	loop.Run(ctx)
	require.GreaterOrEqual(t, ticks.Load(), int32(2))

}

func TestMaintenanceLoop_DefaultInterval(t *testing.T) {
	t.Parallel()
	loop := NewMaintenanceLoop("default-interval", 0, nil, nil)
	require.Equal(t, time.
		Second, loop.
		interval)

}
