//go:build integration

package config_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestApplyDBRuntimeParams_ActualSession verifies that the runtime params
// applied by ApplyDBRuntimeParams end up in effect on every session pgxpool
// opens. This is the Phase 1 behavioral contract: SHOW idle_in_transaction_session_timeout
// must reflect the config value, otherwise the watchdog is the only line of
// defense.
func TestApplyDBRuntimeParams_ActualSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tdb, err := testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup test db: %v", err)
	}
	defer tdb.Cleanup(ctx)

	cfg := &config.Config{
		DBStatementTimeout:         7 * time.Second,
		DBIdleInTransactionTimeout: 11 * time.Second,
		DBLockTimeout:              3 * time.Second,
	}

	poolCfg, err := pgxpool.ParseConfig(tdb.ConnStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if poolCfg.ConnConfig.RuntimeParams == nil {
		poolCfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ApplyDBRuntimeParams(poolCfg.ConnConfig.RuntimeParams, cfg)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	checks := []struct {
		setting string
		want    string
	}{
		{"statement_timeout", "7s"},
		{"idle_in_transaction_session_timeout", "11s"},
		{"lock_timeout", "3s"},
	}
	for _, c := range checks {
		var got string
		if err := pool.QueryRow(ctx, "SHOW "+c.setting).Scan(&got); err != nil {
			t.Fatalf("SHOW %s: %v", c.setting, err)
		}
		if got != c.want {
			t.Errorf("SHOW %s = %q, want %q", c.setting, got, c.want)
		}
	}
}

func TestApplyDBRuntimeParams_ZeroLeavesDefaults(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tdb, err := testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup test db: %v", err)
	}
	defer tdb.Cleanup(ctx)

	cfg := &config.Config{} // all zero
	poolCfg, err := pgxpool.ParseConfig(tdb.ConnStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if poolCfg.ConnConfig.RuntimeParams == nil {
		poolCfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ApplyDBRuntimeParams(poolCfg.ConnConfig.RuntimeParams, cfg)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	// With nothing set, the session should keep server defaults. We only
	// assert that the connection works and that statement_timeout is NOT
	// forced to a small value.
	var val string
	if err := pool.QueryRow(ctx, "SHOW statement_timeout").Scan(&val); err != nil {
		t.Fatalf("SHOW statement_timeout: %v", err)
	}
	// Server default is "0" which pg formats as "0".
	if val != "0" {
		t.Errorf("expected server default statement_timeout=0, got %q", val)
	}
}
