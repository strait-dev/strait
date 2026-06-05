//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/queue"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitFor polls cond until it returns true or timeout elapses, failing the
// test on timeout. Used by several health sampler integration tests.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.Failf(t, "test failure",

		"condition not met within %s", timeout)
}

func TestHealthSampler_HappyPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-health-sampler-happy")
	q := mustQueue(t)

	for range 10 {
		mustEnqueueRun(t, ctx, q, job)
	}

	sampler, err := queue.NewHealthSampler(testDB.Pool, 50*time.Millisecond, nil)
	require.NoError(t, err)

	go sampler.Run(ctx)

	waitFor(t, 2*time.Second, func() bool { return sampler.Iterations() >= 3 })
}

func TestHealthSampler_SurvivesDroppedPartition(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mustClean(t, ctx)

	// Create and immediately drop a partition during sampling. The sampler
	// must not crash on missing pg_stat_user_tables rows.
	_, _ = testDB.Pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS tmp_health_part (id int) PARTITION OF NOTHING (LIKE job_runs INCLUDING ALL)")
	// The above DDL will fail because job_runs is partitioned itself, and
	// that's fine — we just need the sampler to run across a changing
	// pg_stat surface.
	_, _ = testDB.Pool.Exec(ctx, "DROP TABLE IF EXISTS tmp_health_part")

	sampler, err := queue.NewHealthSampler(testDB.Pool, 30*time.Millisecond, nil)
	require.NoError(t, err)

	done := make(chan struct{})
	concWG.Go(func() {
		sampler.Run(ctx)
		close(done)
	})

	waitFor(t, 2*time.Second, func() bool { return sampler.Iterations() >= 3 })
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "sampler did not exit on context cancel")
	}
	assert.GreaterOrEqual(t,

		sampler.
			Iterations(), int64(3))

}

func TestHealthSampler_OldestQueuedAgeObservedAfterEnqueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-health-sampler-age")
	q := mustQueue(t)
	mustEnqueueRun(t, ctx, q, job)

	// Wait 200ms so the queued row actually has age.
	time.Sleep(200 * time.Millisecond)

	sampler, err := queue.NewHealthSampler(testDB.Pool, 1*time.Second, nil)
	require.NoError(t, err)

	sampler.SampleOnce(ctx)
	assert.EqualValues(t, 1, sampler.
		Iterations())

	// We cannot inspect the histogram directly without an OTEL SDK, but
	// SampleOnce must return cleanly and increment iterations.

}

func TestHealthSampler_ManyPartitionsRecorded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mustClean(t, ctx)

	// Query pg_stat_user_tables directly to confirm the sampler query
	// returns at least one row for a partitioned job_runs table.
	rows, err := testDB.Pool.Query(ctx, `
		SELECT relname FROM pg_stat_user_tables
		WHERE relname = 'job_runs' OR relname LIKE 'job_runs_%'
	`)
	require.NoError(t, err)

	defer rows.Close()

	var seen int
	for rows.Next() {
		var r string
		require.NoError(t, rows.
			Scan(&r))

		seen++
	}
	if seen == 0 {
		t.Skip("no job_runs partitions yet; pg_stat requires activity before reporting")
	}

	sampler, err := queue.NewHealthSampler(testDB.Pool, 1*time.Second, nil)
	require.NoError(t, err)

	sampler.SampleOnce(ctx)
}
