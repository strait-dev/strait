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
	"github.com/stretchr/testify/require"
)

func TestStartTrackedLoadtestTriggerBoundsAndDrains(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	var wg conc.WaitGroup
	slots := make(chan struct{}, 1)
	release := make(chan struct{})
	started := make(chan struct{}, 1)

	ok := startTrackedLoadtestTrigger(context.Background(), &wg, slots, func(context.Context) error {
		started <- struct{}{}
		<-release
		return nil
	}, nil, nil)
	require.True(t, ok)

	<-started

	secondReturned := make(chan struct{})
	concWG.Go(func() {
		_ = startTrackedLoadtestTrigger(context.Background(), &wg, slots, func(context.Context) error {
			started <- struct{}{}
			return nil
		}, nil, nil)
		close(secondReturned)
	})

	select {
	case <-secondReturned:
		require.Fail(t, "second trigger should wait for an in-flight slot")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	wg.Wait()

	select {
	case <-secondReturned:
	case <-time.After(time.Second):
		require.Fail(t, "second trigger did not start after the first drained")
	}
	wg.Wait()
}

func TestSleepWithContextReturnsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	require.False(t, sleepWithContext(ctx,
		time.Hour),
	)

	require.LessOrEqual(t, time.Since(start), 100*time.Millisecond)
}

func TestRecordLongRunOutcomeCountsOnlyCompletedTerminalStatus(t *testing.T) {
	t.Parallel()

	var completed, failed atomic.Int32
	recordLongRunOutcome("completed", nil, &completed, &failed)
	recordLongRunOutcome("failed", nil, &completed, &failed)
	recordLongRunOutcome("completed", errors.New("poll timeout"), &completed, &failed)
	require.EqualValues(t, 1,

		completed.
			Load())
	require.EqualValues(t, 2,

		failed.Load())

}

func TestRecordLongRunOutcomeDoesNotCountAcceptedSubmissionAsCompleted(t *testing.T) {
	t.Parallel()

	var completed, failed atomic.Int32
	recordLongRunOutcome("", nil, &completed, &failed)
	require.EqualValues(t, 0,

		completed.
			Load())
	require.EqualValues(t, 1,

		failed.Load())

}

func TestEnduranceLongRunsUseTriggerAndWait(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("endurance.go")
	require.NoError(t,

		err)

	source := string(data)
	require.True(t, strings.Contains(source,
		"h.TriggerAndWait(ctx, \"loadtest-project\", slowProcessJobID",
	))
	require.False(t, strings.Contains(source,
		"h.TriggerJob(ctx, \"loadtest-project\", slowProcessJobID",
	),
	)

}
