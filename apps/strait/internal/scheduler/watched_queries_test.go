package scheduler

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestDefaultWatchedQueries_PinQueuedStatusClauses(t *testing.T) {
	t.Parallel()

	queriesByName := make(map[string]WatchedQuery)
	for _, watched := range DefaultWatchedQueries() {
		queriesByName[watched.Name] = watched
	}

	queuedStatusClause := "s.status = '" + string(domain.StatusQueued) + "'"
	for _, name := range []string{"DequeueN", "PgQueClaimCandidates"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			watched, ok := queriesByName[name]
			require.True(t, ok)
			require.Contains(t, watched.SQL, queuedStatusClause)
		})
	}
}
