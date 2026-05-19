//go:build loadtest

package loadtest

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
)

func TestStartTrackedLoadtestTriggerBoundsAndDrains(t *testing.T) {
	var wg conc.WaitGroup
	slots := make(chan struct{}, 1)
	release := make(chan struct{})
	started := make(chan struct{}, 1)

	ok := startTrackedLoadtestTrigger(context.Background(), &wg, slots, func(context.Context) error {
		started <- struct{}{}
		<-release
		return nil
	}, nil, nil)
	if !ok {
		t.Fatal("expected first trigger to start")
	}
	<-started

	secondReturned := make(chan struct{})
	go func() {
		_ = startTrackedLoadtestTrigger(context.Background(), &wg, slots, func(context.Context) error {
			started <- struct{}{}
			return nil
		}, nil, nil)
		close(secondReturned)
	}()

	select {
	case <-secondReturned:
		t.Fatal("second trigger should wait for an in-flight slot")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	wg.Wait()

	select {
	case <-secondReturned:
	case <-time.After(time.Second):
		t.Fatal("second trigger did not start after the first drained")
	}
	wg.Wait()
}

func TestSleepWithContextReturnsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	if sleepWithContext(ctx, time.Hour) {
		t.Fatal("sleepWithContext returned true for cancelled context")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("sleepWithContext took %s after cancellation", elapsed)
	}
}

func TestRecordLongRunOutcomeCountsOnlyCompletedTerminalStatus(t *testing.T) {
	t.Parallel()

	var completed, failed atomic.Int32
	recordLongRunOutcome("completed", nil, &completed, &failed)
	recordLongRunOutcome("failed", nil, &completed, &failed)
	recordLongRunOutcome("completed", errors.New("poll timeout"), &completed, &failed)

	if completed.Load() != 1 {
		t.Fatalf("completed = %d, want 1", completed.Load())
	}
	if failed.Load() != 2 {
		t.Fatalf("failed = %d, want 2", failed.Load())
	}
}

func TestEnduranceLongRunsUseTriggerAndWait(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("endurance.go")
	if err != nil {
		t.Fatalf("read endurance.go: %v", err)
	}
	source := string(data)
	if !strings.Contains(source, "h.TriggerAndWait(ctx, \"loadtest-project\", slowProcessJobID") {
		t.Fatal("long-run endurance jobs must wait for terminal status")
	}
	if strings.Contains(source, "h.TriggerJob(ctx, \"loadtest-project\", slowProcessJobID") {
		t.Fatal("long-run endurance jobs still count trigger acceptance as completion")
	}
}
