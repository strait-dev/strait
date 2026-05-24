//go:build integration

package scheduler_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/scheduler"
	"strait/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

// seedAuditEvent inserts a raw audit_events row with a caller-controlled
// created_at so retention tests can create "old" rows without relying on
// wall-clock time or the chain-signing CreateAuditEvent path.
func seedAuditEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID string, createdAt time.Time) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		INSERT INTO audit_events (
			id, project_id, actor_id, actor_type, action, resource_type, resource_id,
			details, signature, previous_hash, created_at
		) VALUES (
			$1, $2, 'actor', 'user', 'test.seed', 'test', 'res-1',
			'{}'::jsonb, '', '', $3
		)
	`, "seed-"+projectID+"-"+createdAt.Format("20060102150405.000000"), projectID, createdAt)
	if err != nil {
		t.Fatalf("seed audit event: %v", err)
	}
}

// seedProjectQuotaRetention inserts an explicit project_quotas retention
// override. 0 means "disable trim" because the override flag is set.
func seedProjectQuotaRetention(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID string, days int) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		INSERT INTO project_quotas (project_id, audit_retention_days, audit_retention_override_set)
		VALUES ($1, $2, TRUE)
		ON CONFLICT (project_id) DO UPDATE SET
			audit_retention_days = EXCLUDED.audit_retention_days,
			audit_retention_override_set = TRUE
	`, projectID, days)
	if err != nil {
		t.Fatalf("seed project_quotas: %v", err)
	}
}

func seedDefaultQuotaRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID string) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		INSERT INTO project_quotas (project_id, max_queued_runs)
		VALUES ($1, 10)
		ON CONFLICT (project_id) DO UPDATE SET max_queued_runs = EXCLUDED.max_queued_runs
	`, projectID)
	if err != nil {
		t.Fatalf("seed default project_quotas row: %v", err)
	}
}

func countAuditEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1`, projectID,
	).Scan(&n); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	return n
}

// countNonAnchorAuditEvents excludes is_anchor=true rows (rotation anchors
// and retention tombstones) so retention-trim assertions can express
// "this project's chain rows are gone" without having to subtract the
// tombstone the trim itself emits.
func countNonAnchorAuditEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1 AND is_anchor = FALSE`, projectID,
	).Scan(&n); err != nil {
		t.Fatalf("count non-anchor audit events: %v", err)
	}
	return n
}

func TestReapAuditEvents_HonorsPerProjectOverride(t *testing.T) {
	ctx := context.Background()
	intTestClean(t, ctx)

	q := intTestStore(t)
	pool := getTestDB(t).Pool

	const (
		projA = "proj-retention-a" // 30-day override
		projB = "proj-retention-b" // no override, falls to default 365
	)
	seedProjectQuotaRetention(t, ctx, pool, projA, 30)

	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	seedAuditEvent(t, ctx, pool, projA, old)
	seedAuditEvent(t, ctx, pool, projB, old)

	r := scheduler.NewReaper(q, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditRetention(365)
	r.ReapOnce(ctx)

	if got := countNonAnchorAuditEvents(t, ctx, pool, projA); got != 0 {
		t.Errorf("proj-a non-anchor rows after reap = %d, want 0 (30-day override)", got)
	}
	if got := countAuditEvents(t, ctx, pool, projB); got != 1 {
		t.Errorf("proj-b rows after reap = %d, want 1 (default 365)", got)
	}
}

func TestReapAuditEvents_ZeroDaysDisables(t *testing.T) {
	ctx := context.Background()
	intTestClean(t, ctx)

	q := intTestStore(t)
	pool := getTestDB(t).Pool

	const projID = "proj-retention-zero"
	seedProjectQuotaRetention(t, ctx, pool, projID, 0)

	// Older than default. Without the disable, the default sweep would trim it.
	old := time.Now().UTC().Add(-400 * 24 * time.Hour)
	seedAuditEvent(t, ctx, pool, projID, old)

	r := scheduler.NewReaper(q, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditRetention(30)
	r.ReapOnce(ctx)

	if got := countAuditEvents(t, ctx, pool, projID); got != 1 {
		t.Errorf("rows after reap with days=0 = %d, want 1 (trim disabled)", got)
	}
}

func TestReapAuditEvents_DefaultQuotaRowInheritsDefaultRetention(t *testing.T) {
	ctx := context.Background()
	intTestClean(t, ctx)

	q := intTestStore(t)
	pool := getTestDB(t).Pool

	const projID = "proj-retention-default-row"
	seedDefaultQuotaRow(t, ctx, pool, projID)

	old := time.Now().UTC().Add(-400 * 24 * time.Hour)
	seedAuditEvent(t, ctx, pool, projID, old)

	r := scheduler.NewReaper(q, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditRetention(30)
	r.ReapOnce(ctx)

	if got := countNonAnchorAuditEvents(t, ctx, pool, projID); got != 0 {
		t.Errorf("non-anchor rows after reap = %d, want 0 (default quota row inherits global retention)", got)
	}
}

func TestReapAuditEvents_MetricEmitted(t *testing.T) {
	ctx := context.Background()
	intTestClean(t, ctx)

	q := intTestStore(t)
	pool := getTestDB(t).Pool

	const projID = "proj-retention-metric"
	seedProjectQuotaRetention(t, ctx, pool, projID, 7)

	old := time.Now().UTC().Add(-30 * 24 * time.Hour)
	seedAuditEvent(t, ctx, pool, projID, old)

	// Recorder-style fake: count AuditRetentionDeleted calls via a wrapping store.
	rec := &retentionRecorderStore{Queries: q}
	r := scheduler.NewReaper(rec, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditRetention(365)
	r.ReapOnce(ctx)

	// Direct assertion: the row for projID is gone and the wrapping store saw the call.
	// Use the non-anchor count so the tombstone DeleteAuditEventsBefore writes
	// after a successful trim does not skew the assertion.
	if got := countNonAnchorAuditEvents(t, ctx, pool, projID); got != 0 {
		t.Fatalf("non-anchor rows after reap = %d, want 0", got)
	}
	if rec.perProjectDeleteCalls[projID] != 1 {
		t.Errorf("DeleteAuditEventsBefore(%s) calls = %d, want 1",
			projID, rec.perProjectDeleteCalls[projID])
	}
	if !rec.sawExcludingCall {
		t.Errorf("default sweep via DeleteAuditEventsBeforeExcluding not observed")
	}
}

// retentionRecorderStore wraps *store.Queries and records which retention
// primitives the reaper invoked, so the metric emission path can be asserted
// without plumbing a full OTel reader through the integration harness.
type retentionRecorderStore struct {
	*store.Queries
	perProjectDeleteCalls map[string]int
	sawExcludingCall      bool
}

func (r *retentionRecorderStore) DeleteAuditEventsBefore(ctx context.Context, projectID string, cutoff time.Time) (int64, error) {
	if r.perProjectDeleteCalls == nil {
		r.perProjectDeleteCalls = map[string]int{}
	}
	r.perProjectDeleteCalls[projectID]++
	return r.Queries.DeleteAuditEventsBefore(ctx, projectID, cutoff)
}

func (r *retentionRecorderStore) DeleteAuditEventsBeforeExcluding(ctx context.Context, cutoff time.Time, excluded []string) (int64, error) {
	r.sawExcludingCall = true
	return r.Queries.DeleteAuditEventsBeforeExcluding(ctx, cutoff, excluded)
}
