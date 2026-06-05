//go:build integration

package scheduler

import (
	"context"
	"testing"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestDefaultWatchedQueriesExplainAgainstCurrentSchema(t *testing.T) {
	ctx := context.Background()
	tdb := cleanSchedulerIntegrationDB(t, ctx)
	q := store.New(tdb.Pool)

	for _, watched := range DefaultWatchedQueries() {
		watched := watched
		t.Run(watched.Name, func(t *testing.T) {
			_, err := q.Explain(ctx, watched.SQL)
			require.NoError(t, err, "Explain(%s)", watched.Name)
		})
	}
}

func TestDefaultWatchedQueriesDoNotReferenceDroppedPriorityColumn(t *testing.T) {
	for _, watched := range DefaultWatchedQueries() {
		require.NotContains(t, watched.SQL, "promoted_priority", watched.Name)
	}
}
