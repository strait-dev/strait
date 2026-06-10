package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Config invariants.
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
func (c *Config) Validate() error {
	var errs []error

	if c.DatabaseURL == "" {
		errs = append(errs, fmt.Errorf("DATABASE_URL is required"))
	}
	if c.RedisURL == "" && (c.RedisSentinelMaster == "" || len(c.RedisSentinelAddrs) == 0) {
		errs = append(errs, fmt.Errorf("REDIS_URL is required unless REDIS_SENTINEL_MASTER and REDIS_SENTINEL_ADDRS are configured"))
	}
	if c.RedisURL != "" {
		u, parseErr := url.Parse(c.RedisURL)
		if parseErr != nil || (u.Scheme != "redis" && u.Scheme != "rediss") {
			errs = append(errs, fmt.Errorf("REDIS_URL must be a valid redis:// or rediss:// URL"))
		}
	}
	if c.SequinBaseURL == "" {
		errs = append(errs, fmt.Errorf("SEQUIN_BASE_URL is required"))
	}
	if c.SequinBaseURL != "" {
		u, parseErr := url.Parse(c.SequinBaseURL)
		if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") {
			errs = append(errs, fmt.Errorf("SEQUIN_BASE_URL must be a valid HTTP(S) URL"))
		}
	}
	if c.SequinConsumerName == "" {
		errs = append(errs, fmt.Errorf("SEQUIN_CONSUMER_NAME is required"))
	}
	if c.SequinAPIToken == "" {
		errs = append(errs, fmt.Errorf("SEQUIN_API_TOKEN is required"))
	}
	if c.SequinBatchSize <= 0 {
		errs = append(errs, fmt.Errorf("SEQUIN_BATCH_SIZE must be > 0, got %d", c.SequinBatchSize))
	}
	if c.SequinWaitTimeMs <= 0 {
		errs = append(errs, fmt.Errorf("SEQUIN_WAIT_TIME_MS must be > 0, got %d", c.SequinWaitTimeMs))
	}
	// Gate on STRAIT_ENV, not SENTRY_ENVIRONMENT (an observability label, not a
	// security boundary): keying CDC webhook auth on Sentry's env let production
	// disable signature verification by setting SENTRY_ENVIRONMENT=development.
	if c.SequinWebhookSecret == "" && !IsRelaxedDeploymentEnvironment(c.DeploymentEnvironment) {
		errs = append(errs, fmt.Errorf("SEQUIN_WEBHOOK_SECRET is required in non-development environments"))
	}

	// Positive-required durations.
	requirePositive := map[string]time.Duration{
		"HEARTBEAT_INTERVAL":               c.HeartbeatInterval,
		"REAPER_INTERVAL":                  c.ReaperInterval,
		"STALE_THRESHOLD":                  c.StaleThreshold,
		"POLLER_INTERVAL":                  c.PollerInterval,
		"REQUEST_TIMEOUT":                  c.RequestTimeout,
		"WORKER_DRAIN_TIMEOUT":             c.WorkerDrainTimeout,
		"DB_STATEMENT_TIMEOUT":             c.DBStatementTimeout,
		"DB_MAX_CONN_LIFETIME":             c.DBMaxConnLifetime,
		"DB_MAX_CONN_IDLE_TIME":            c.DBMaxConnIdleTime,
		"DB_HEALTH_CHECK_PERIOD":           c.DBHealthCheckPeriod,
		"DB_IDLE_IN_TRANSACTION_TIMEOUT":   c.DBIdleInTransactionTimeout,
		"DB_LOCK_TIMEOUT":                  c.DBLockTimeout,
		"DB_LONG_TXN_ALERT_THRESHOLD":      c.DBLongTxnAlertThreshold,
		"DB_WATCHDOG_INTERVAL":             c.DBWatchdogInterval,
		"DB_BACKPRESSURE_SAMPLE_INTERVAL":  c.DBBackpressureSampleInterval,
		"WORKER_DB_SYNC_INTERVAL":          c.WorkerDBSyncInterval,
		"WORKER_HEARTBEAT_TIMEOUT":         c.WorkerHeartbeatTimeout,
		"WORKER_DISCONNECT_SWEEP_INTERVAL": c.WorkerDisconnectSweepInterval,
		"WORKER_DISCONNECT_ACK_TIMEOUT":    c.WorkerDisconnectAckTimeout,
		"GRPC_PUBSUB_STARTUP_TIMEOUT":      c.GRPCPubsubStartupTimeout,
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
	if c.WorkerDBSyncInterval <= c.HeartbeatInterval {
		errs = append(errs, fmt.Errorf("WORKER_DB_SYNC_INTERVAL (%v) must be > HEARTBEAT_INTERVAL (%v)", c.WorkerDBSyncInterval, c.HeartbeatInterval))
	}
	if c.WorkerDBSyncInterval >= c.StaleThreshold {
		errs = append(errs, fmt.Errorf("WORKER_DB_SYNC_INTERVAL (%v) must be < STALE_THRESHOLD (%v)", c.WorkerDBSyncInterval, c.StaleThreshold))
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
	if c.DBBackpressureAcquireWaitThreshold < 0 {
		errs = append(errs, fmt.Errorf("DB_BACKPRESSURE_ACQUIRE_WAIT_THRESHOLD must be >= 0, got %v", c.DBBackpressureAcquireWaitThreshold))
	}
	if c.DBBackpressureOccupancyThreshold <= 0 || c.DBBackpressureOccupancyThreshold > 1 {
		errs = append(errs, fmt.Errorf("DB_BACKPRESSURE_OCCUPANCY_THRESHOLD must be > 0 and <= 1, got %v", c.DBBackpressureOccupancyThreshold))
	}
	if c.BackpressureDefaultMaxTokens < 0 {
		errs = append(errs, fmt.Errorf("BACKPRESSURE_DEFAULT_MAX_TOKENS must be >= 0, got %d", c.BackpressureDefaultMaxTokens))
	}
	if c.BackpressureDefaultRefillPerSec < 0 {
		errs = append(errs, fmt.Errorf("BACKPRESSURE_DEFAULT_REFILL_PER_SEC must be >= 0, got %d", c.BackpressureDefaultRefillPerSec))
	}
	if c.BackpressureLocalLeaseSize < 1 {
		errs = append(errs, fmt.Errorf("BACKPRESSURE_LOCAL_LEASE_SIZE must be >= 1, got %d", c.BackpressureLocalLeaseSize))
	}
	if c.BackpressureEnabled && c.BackpressureDefaultMaxTokens == 0 {
		errs = append(errs, fmt.Errorf("BACKPRESSURE_DEFAULT_MAX_TOKENS must be > 0 when BACKPRESSURE_ENABLED=true"))
	}
	switch strings.ToLower(strings.TrimSpace(c.ExecutionTraceMode)) {
	case "off", "errors", "full":
	default:
		errs = append(errs, fmt.Errorf("EXECUTION_TRACE_MODE must be off, errors, or full, got %q", c.ExecutionTraceMode))
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
