package worker

import (
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestRunTimingHelpers(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startedAt := createdAt.Add(250 * time.Millisecond)
	run := &domain.JobRun{
		CreatedAt: createdAt,
		StartedAt: &startedAt,
	}
	require.Equal(t,
		400*time.
			Millisecond, runQueueWaitUntil(run, createdAt.Add(400*time.Millisecond)))
	require.Equal(t,
		250*time.
			Millisecond, runStartedQueueWait(run))
	require.Equal(t,
		175*time.
			Millisecond, runDequeueDurationUntil(run, createdAt.Add(425*time.Millisecond)))

}

func TestRunTimingHelpersHandleMissingTimes(t *testing.T) {
	t.Parallel()
	require.EqualValues(t, 0, runQueueWaitUntil(nil, time.Now()))
	require.EqualValues(t, 0, runStartedQueueWait(&domain.JobRun{}))
	require.EqualValues(t, 0, runDequeueDurationUntil(&domain.
		JobRun{}, time.Now()))

	startedAt := time.Now()
	run := &domain.JobRun{StartedAt: &startedAt}
	require.EqualValues(t, 0, runQueueWaitUntil(run, time.Now()))
	require.EqualValues(t, 0, runDequeueDurationUntil(run, time.
		Time{}))

}

func TestRunTimingHelpersClampClockSkew(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startedAt := createdAt.Add(-time.Second)
	run := &domain.JobRun{
		CreatedAt: createdAt,
		StartedAt: &startedAt,
	}
	require.EqualValues(t, 0, runQueueWaitUntil(run, createdAt.
		Add(-time.Millisecond)))
	require.EqualValues(t, 0, runStartedQueueWait(run))
	require.EqualValues(t, 0, runDequeueDurationUntil(run, startedAt.
		Add(-time.Millisecond)))

}
