//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetEventTriggerStats_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	stats, err := q.GetEventTriggerStats(ctx, "project-event-trigger-stats-empty", "")
	require.NoError(t, err)
	require.EqualValues(t, 0, stats.
		TotalCount,
	)
	require.EqualValues(t, 0, stats.
		WaitingCount,
	)
	require.EqualValues(t, 0, stats.
		ReceivedCount,
	)
	require.EqualValues(t, 0, stats.
		AvgWaitDuration,
	)

}
