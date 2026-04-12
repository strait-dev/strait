package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// R3 Phase 3: pg_partman fallback and self-heal.
//
// Migration 000066 sets up pg_partman with p_premake=4, meaning it
// creates partitions four months ahead of the current month. If the
// service is down for five or more months, the next enqueue fails with
// "no partition of relation 'job_runs' found for row" because the
// current month's partition never got created.
//
// This file adds a runtime check that ensures the current month and
// several months ahead exist, and falls back to raw CREATE TABLE when
// pg_partman is unavailable. cmd/strait calls EnsureJobRunsPartitions
// at startup (fatal on failure) and the scheduler re-runs it daily.

// EnsureJobRunsPartitions guarantees that partitions exist for the
// current month through `monthsAhead` months ahead. Uses pg_partman's
// create_parent helper when available; falls back to a raw CREATE TABLE
// PARTITION OF when not. Safe to call repeatedly.
func (q *Queries) EnsureJobRunsPartitions(ctx context.Context, monthsAhead int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.EnsureJobRunsPartitions")
	defer span.End()

	if monthsAhead < 1 {
		monthsAhead = 1
	}
	now := time.Now().UTC()
	// Ensure for the current month plus monthsAhead future months.
	for i := 0; i <= monthsAhead; i++ {
		target := addMonths(now, i)
		if err := q.ensureMonthPartition(ctx, target); err != nil {
			return fmt.Errorf("ensure partition for %s: %w", target.Format("2006-01"), err)
		}
	}
	return nil
}

// ensureMonthPartition makes sure the partition covering the given
// month exists. Tries pg_partman first; on failure falls back to raw
// CREATE TABLE.
func (q *Queries) ensureMonthPartition(ctx context.Context, month time.Time) error {
	start := startOfMonth(month)
	end := startOfMonth(addMonths(month, 1))
	name := fmt.Sprintf("job_runs_p%04d_%02d", start.Year(), int(start.Month()))

	exists, err := q.PartitionExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	// Try pg_partman first.
	if err := q.createPartitionViaPartman(ctx, month); err == nil {
		// Verify it actually created the partition; pg_partman will
		// quietly succeed even if the install is stale.
		exists, err := q.PartitionExists(ctx, name)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}

	// Fallback: raw CREATE TABLE PARTITION OF. Uses IF NOT EXISTS so
	// a concurrent ensurer cannot race us into a duplicate-name error.
	sql := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF job_runs FOR VALUES FROM ('%s') TO ('%s')`,
		quoteIdent(name),
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
	if _, err := q.db.Exec(ctx, sql); err != nil {
		return fmt.Errorf("fallback CREATE TABLE: %w", err)
	}
	return nil
}

// createPartitionViaPartman asks pg_partman to extend its premake window.
// The function is a no-op if pg_partman is not installed or is out of
// date; callers should verify via partitionExists afterward.
func (q *Queries) createPartitionViaPartman(ctx context.Context, month time.Time) error {
	// pg_partman.run_maintenance_proc creates premake partitions for all
	// managed parents. Safe to call repeatedly.
	const sql = `
DO $$
BEGIN
  PERFORM partman.run_maintenance_proc();
EXCEPTION WHEN OTHERS THEN
  -- pg_partman not installed or out of date; caller falls back.
  NULL;
END;
$$`
	_ = month
	if _, err := q.db.Exec(ctx, sql); err != nil {
		return fmt.Errorf("run_maintenance_proc: %w", err)
	}
	return nil
}

// PartitionExists returns true when the given partition relation is
// present in pg_class.
func (q *Queries) PartitionExists(ctx context.Context, name string) (bool, error) {
	var present bool
	err := q.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1 AND relkind = 'r')`, name,
	).Scan(&present)
	if err != nil {
		return false, fmt.Errorf("check partition %s: %w", name, err)
	}
	return present, nil
}

// ListJobRunsPartitions returns every partition of the job_runs parent
// in creation order. Used by the scheduler tuner (Phase 4) and by the
// observability dashboards.
func (q *Queries) ListJobRunsPartitions(ctx context.Context) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobRunsPartitions")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = 'job_runs'
		ORDER BY c.relname
	`)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("list partitions: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// addMonths returns t + n months, normalized to the first of the month.
func addMonths(t time.Time, n int) time.Time {
	year, month, _ := t.UTC().Date()
	month += time.Month(n)
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
}

// startOfMonth normalizes t to 00:00 UTC on the first of t's month.
func startOfMonth(t time.Time) time.Time {
	y, m, _ := t.UTC().Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

// quoteIdent wraps an identifier in double quotes for safe interpolation
// into CREATE TABLE statements. The identifier is assumed to come from
// code, not user input.
func quoteIdent(s string) string {
	return `"` + s + `"`
}

// ExecDDL runs a single DDL statement via the underlying pool. Used by
// the partition tuner (R3 Phase 4) which issues ALTER TABLE SET/RESET
// commands on individual partitions.
func (q *Queries) ExecDDL(ctx context.Context, sql string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ExecDDL")
	defer span.End()
	if _, err := q.db.Exec(ctx, sql); err != nil {
		return fmt.Errorf("exec ddl: %w", err)
	}
	return nil
}
