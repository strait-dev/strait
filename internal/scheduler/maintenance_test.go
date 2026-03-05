package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestMaintenanceLoop_Run(t *testing.T) {
	var ticks atomic.Int32
	loop := NewMaintenanceLoop("test-loop", 20*time.Millisecond, nil, func(context.Context) {
		ticks.Add(1)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Millisecond)
	defer cancel()

	loop.Run(ctx)

	if ticks.Load() < 2 {
		t.Fatalf("ticks = %d, want >= 2", ticks.Load())
	}
}

func TestMaintenanceLoop_DefaultInterval(t *testing.T) {
	loop := NewMaintenanceLoop("default-interval", 0, nil, nil)
	if loop.interval != time.Second {
		t.Fatalf("interval = %v, want %v", loop.interval, time.Second)
	}
}
