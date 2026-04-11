package config

import (
	"errors"
	"fmt"
	"time"
)

// Round 2 Phase 2: config invariants.
//
// Load() previously accepted any value from env parsing and let downstream
// code handle garbage. A `Validate()` call at the end of Load catches
// whole classes of misconfiguration at startup instead of at runtime:
//
//   - Zero and negative durations where positive is required
//   - Cross-field relationships (poll interval < heartbeat interval)
//   - Counts with impossible bounds
//
// The validator is written as a list of independent checks so failures
// accumulate and surface in one batch, which is much less frustrating
// than fix-one-then-try-again loops on a cold start.

// Validate returns nil if the config is internally consistent and
// compatible with the queue reliability contract. On failure it returns
// an errors.Join wrapping every individual invariant violation.
//
//nolint:gocyclo,cyclop // flat list of independent checks is clearer than grouping
func (c *Config) Validate() error {
	var errs []error

	// Positive-required durations.
	requirePositive := map[string]time.Duration{
		"HEARTBEAT_INTERVAL":       c.HeartbeatInterval,
		"REAPER_INTERVAL":          c.ReaperInterval,
		"STALE_THRESHOLD":          c.StaleThreshold,
		"POLLER_INTERVAL":          c.PollerInterval,
		"REQUEST_TIMEOUT":          c.RequestTimeout,
		"WORKER_DRAIN_TIMEOUT":     c.WorkerDrainTimeout,
		"DB_STATEMENT_TIMEOUT":     c.DBStatementTimeout,
		"DB_MAX_CONN_LIFETIME":     c.DBMaxConnLifetime,
		"DB_MAX_CONN_IDLE_TIME":    c.DBMaxConnIdleTime,
		"DB_HEALTH_CHECK_PERIOD":   c.DBHealthCheckPeriod,
		"DB_IDLE_IN_TRANSACTION_TIMEOUT": c.DBIdleInTransactionTimeout,
		"DB_LOCK_TIMEOUT":          c.DBLockTimeout,
		"DB_LONG_TXN_ALERT_THRESHOLD": c.DBLongTxnAlertThreshold,
		"DB_WATCHDOG_INTERVAL":     c.DBWatchdogInterval,
	}
	for name, d := range requirePositive {
		if d <= 0 {
			errs = append(errs, fmt.Errorf("%s must be > 0, got %v", name, d))
		}
	}

	// Sanity upper bounds. Durations that are absurdly large are almost
	// always a typo (24h1ms vs 24h, etc.).
	const maxReasonable = 7 * 24 * time.Hour
	reasonable := map[string]time.Duration{
		"HEARTBEAT_INTERVAL":   c.HeartbeatInterval,
		"POLLER_INTERVAL":      c.PollerInterval,
		"DB_STATEMENT_TIMEOUT": c.DBStatementTimeout,
		"DB_WATCHDOG_INTERVAL": c.DBWatchdogInterval,
	}
	for name, d := range reasonable {
		if d > maxReasonable {
			errs = append(errs, fmt.Errorf("%s %v exceeds reasonable max %v", name, d, maxReasonable))
		}
	}

	// Cross-field invariants.
	if c.PollerInterval >= c.HeartbeatInterval {
		errs = append(errs, fmt.Errorf("POLLER_INTERVAL (%v) must be < HEARTBEAT_INTERVAL (%v)", c.PollerInterval, c.HeartbeatInterval))
	}
	if c.StaleThreshold < c.HeartbeatInterval*3 {
		errs = append(errs, fmt.Errorf("STALE_THRESHOLD (%v) must be >= 3 * HEARTBEAT_INTERVAL (%v)", c.StaleThreshold, c.HeartbeatInterval*3))
	}
	if c.DBLockTimeout > c.DBStatementTimeout {
		errs = append(errs, fmt.Errorf("DB_LOCK_TIMEOUT (%v) must be <= DB_STATEMENT_TIMEOUT (%v)", c.DBLockTimeout, c.DBStatementTimeout))
	}
	if c.DBMinConns > c.DBMaxConns {
		errs = append(errs, fmt.Errorf("DB_MIN_CONNS (%d) must be <= DB_MAX_CONNS (%d)", c.DBMinConns, c.DBMaxConns))
	}
	if c.WorkerConcurrency <= 0 {
		errs = append(errs, fmt.Errorf("WORKER_CONCURRENCY must be > 0, got %d", c.WorkerConcurrency))
	}
	if c.DBMaxConns < 1 {
		errs = append(errs, fmt.Errorf("DB_MAX_CONNS must be >= 1, got %d", c.DBMaxConns))
	}

	// DLQ invariants.
	if c.DLQMaxPerJob > 0 && c.DLQMaxPerProject > 0 && c.DLQMaxPerJob > c.DLQMaxPerProject {
		errs = append(errs, fmt.Errorf("DLQ_MAX_PER_JOB (%d) must be <= DLQ_MAX_PER_PROJECT (%d)", c.DLQMaxPerJob, c.DLQMaxPerProject))
	}
	if c.DLQOverflowPolicy != "" && c.DLQOverflowPolicy != "drop_oldest" && c.DLQOverflowPolicy != "reject" {
		errs = append(errs, fmt.Errorf("DLQ_OVERFLOW_POLICY must be drop_oldest or reject, got %q", c.DLQOverflowPolicy))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
