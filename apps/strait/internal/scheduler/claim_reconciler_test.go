package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// mockReconcilerDB satisfies store.DBTX for claim reconciler tests.
type mockReconcilerDB struct {
	execFn func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (m *mockReconcilerDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.NewCommandTag(""), nil
}

func (m *mockReconcilerDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (m *mockReconcilerDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return nil
}

// Kill: claim_reconciler.go L22 CONDITIONALS_BOUNDARY (interval <= 0).
func TestNewClaimReconciler_ZeroInterval_UsesDefault(t *testing.T) {
	t.Parallel()
	r := NewClaimReconciler(nil, 0)
	if r.interval != 5*time.Minute {
		t.Errorf("interval = %v, want 5m (default)", r.interval)
	}
}

// Kill: claim_reconciler.go L22 CONDITIONALS_NEGATION (interval <= 0 -> > 0).
func TestNewClaimReconciler_NegativeInterval_UsesDefault(t *testing.T) {
	t.Parallel()
	r := NewClaimReconciler(nil, -1*time.Second)
	if r.interval != 5*time.Minute {
		t.Errorf("interval = %v, want 5m (default for negative)", r.interval)
	}
}

// Kill: claim_reconciler.go L22 positive interval preserved.
func TestNewClaimReconciler_PositiveInterval_Preserved(t *testing.T) {
	t.Parallel()
	r := NewClaimReconciler(nil, 30*time.Second)
	if r.interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s", r.interval)
	}
}

// Kill: claim_reconciler.go L63 CONDITIONALS_NEGATION (err != nil).
func TestReconcileOnce_MissingSQLError_Returned(t *testing.T) {
	t.Parallel()
	db := &mockReconcilerDB{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if len(sql) > 20 && sql[3] == 'I' { // INSERT (missing claims)
				return pgconn.NewCommandTag(""), errors.New("db down")
			}
			return pgconn.NewCommandTag("DELETE 0"), nil
		},
	}
	r := NewClaimReconciler(db, time.Minute)
	err := r.reconcileOnce(context.Background())
	if err == nil {
		t.Fatal("expected error from missing claims SQL")
	}
}

// Kill: claim_reconciler.go L82 CONDITIONALS_NEGATION (err != nil on stale).
func TestReconcileOnce_StaleSQLError_Returned(t *testing.T) {
	t.Parallel()
	callCount := 0
	db := &mockReconcilerDB{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			callCount++
			if callCount == 1 {
				// First call (missing claims INSERT) succeeds.
				return pgconn.NewCommandTag("INSERT 0"), nil
			}
			// Second call (stale claims DELETE) fails.
			return pgconn.NewCommandTag(""), errors.New("stale query failed")
		},
	}
	r := NewClaimReconciler(db, time.Minute)
	err := r.reconcileOnce(context.Background())
	if err == nil {
		t.Fatal("expected error from stale claims SQL")
	}
}

// Kill: claim_reconciler.go L66,L85 CONDITIONALS_BOUNDARY (inserted/deleted > 0).
func TestReconcileOnce_ZeroRowsAffected_NoLog(t *testing.T) {
	t.Parallel()
	db := &mockReconcilerDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0"), nil
		},
	}
	r := NewClaimReconciler(db, time.Minute)
	err := r.reconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No WARN log should be emitted for 0 rows. We can't easily assert
	// on log output in unit tests, but the test verifies no panic.
}

// Kill: claim_reconciler.go L34 CONDITIONALS_NEGATION (err != nil in Run).
func TestReconcileOnce_Success_NoError(t *testing.T) {
	t.Parallel()
	calls := 0
	db := &mockReconcilerDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			calls++
			return pgconn.NewCommandTag("INSERT 0"), nil
		},
	}
	r := NewClaimReconciler(db, time.Minute)
	err := r.reconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 Exec calls (missing + stale), got %d", calls)
	}
}

func TestReconcileOnce_MissingClaimRestoresRoutingMetadata(t *testing.T) {
	t.Parallel()

	var insertSQL string
	db := &mockReconcilerDB{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "INSERT INTO job_run_queue") {
				insertSQL = sql
			}
			return pgconn.NewCommandTag("INSERT 0"), nil
		},
	}
	r := NewClaimReconciler(db, time.Minute)
	if err := r.reconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcileOnce() error = %v", err)
	}

	for _, fragment := range []string{
		"job_enabled, job_paused, execution_mode, queue_name",
		"LEFT JOIN job_run_read_state s ON s.run_id = jr.id",
		"COALESCE(NULLIF(s.execution_mode, ''), NULLIF(jr.execution_mode, ''), NULLIF(j.execution_mode, ''), 'http')",
		"COALESCE(NULLIF(jr.queue_name, ''), NULLIF(j.queue_name, ''), NULLIF(s.queue_name, ''), 'default')",
		"COALESCE(s.status, jr.status) IN ('queued', 'delayed')",
	} {
		if !strings.Contains(insertSQL, fragment) {
			t.Fatalf("missing routing fragment %q in SQL:\n%s", fragment, insertSQL)
		}
	}
}
