//go:build integration

package queue_test

import (
	"context"
	"strings"
	"testing"
)

// TestMigration_JobRunStateStorage_AppliesParams guards the hot mutable
// run-state table tuning used to keep claim, heartbeat, retry, and terminal
// updates from bloating the immutable job_runs ledger path.
func TestMigration_JobRunStateStorage_AppliesParams(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	var opts []string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT reloptions
		FROM pg_class
		WHERE relname = 'job_run_state'
	`).Scan(&opts); err != nil {
		t.Fatalf("query job_run_state reloptions: %v", err)
	}

	wantOptions := []string{
		"fillfactor=70",
		"autovacuum_vacuum_threshold=50",
		"autovacuum_vacuum_scale_factor=0.005",
		"autovacuum_vacuum_cost_delay=0",
		"autovacuum_vacuum_cost_limit=2000",
		"autovacuum_analyze_threshold=50",
		"autovacuum_analyze_scale_factor=0.005",
	}
	gotOptions := strings.Join(opts, ",")
	for _, want := range wantOptions {
		if !strings.Contains(gotOptions, want) {
			t.Fatalf("job_run_state reloptions missing %q; got %v", want, opts)
		}
	}
}
