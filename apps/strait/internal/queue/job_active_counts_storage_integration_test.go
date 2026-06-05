//go:build integration

package queue_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration_JobActiveCountsStorage_AppliesParams reads pg_class.reloptions
// and asserts the four storage params set by migration 000258 are present.
// Guards against regression of the hot-counter table tuning.
func TestMigration_JobActiveCountsStorage_AppliesParams(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	var opts []string
	err := testDB.Pool.QueryRow(ctx, `
		SELECT reloptions
		FROM pg_class
		WHERE relname = 'job_active_counts'
	`).Scan(&opts)
	require.NoError(t, err)

	want := map[string]string{
		"fillfactor":                      "70",
		"autovacuum_vacuum_scale_factor":  "0.01",
		"autovacuum_vacuum_cost_limit":    "1000",
		"autovacuum_analyze_scale_factor": "0.02",
	}

	got := make(map[string]string, len(opts))
	for _, kv := range opts {
		for i := range len(kv) {
			if kv[i] == '=' {
				got[kv[:i]] = kv[i+1:]
				break
			}
		}
	}

	for k, v := range want {
		if gv, ok := got[k]; !ok {
			assert.Failf(t, "test failure",

				"missing reloption %s; full reloptions=%v", k, opts)
		} else if gv != v {
			assert.Failf(t, "test failure",

				"reloption %s = %s, want %s", k, gv, v)
		}
	}
}
