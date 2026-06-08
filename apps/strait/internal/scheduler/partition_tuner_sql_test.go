package scheduler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPartitionSQLBuilders_RejectInvalidIdentifier is the regression guard for
// the swallowed-error path: an invalid partition identifier must surface an error
// rather than yield an empty SQL string (which produced a misleading "empty
// query" failure downstream).
func TestPartitionSQLBuilders_RejectInvalidIdentifier(t *testing.T) {
	t.Parallel()
	bad := "job_runs_2026; DROP TABLE users"
	for _, b := range []struct {
		name string
		fn   func(string) (string, error)
	}{
		{"hot", hotSettingsSQL},
		{"reset", resetSettingsSQL},
		{"fillfactor", fillfactorSQL},
	} {
		sql, err := b.fn(bad)
		require.Errorf(t, err, "%s should reject invalid identifier", b.name)
		require.Empty(t, sql)
	}

	// Valid identifier still produces SQL.
	good := "job_runs_2026_01"
	sql, err := hotSettingsSQL(good)
	require.NoError(t, err)
	require.True(t, strings.Contains(sql, good))
}
