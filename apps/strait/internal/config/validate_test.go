package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validConfig() *Config {
	return &Config{
		WorkerConcurrency:             25,
		HeartbeatInterval:             10 * time.Second,
		ReaperInterval:                30 * time.Second,
		StaleThreshold:                60 * time.Second,
		PollerInterval:                5 * time.Second,
		RequestTimeout:                30 * time.Second,
		WorkerDrainTimeout:            30 * time.Second,
		DBStatementTimeout:            30 * time.Second,
		DBMaxConnLifetime:             30 * time.Minute,
		DBMaxConnIdleTime:             5 * time.Minute,
		DBHealthCheckPeriod:           30 * time.Second,
		DBIdleInTransactionTimeout:    30 * time.Second,
		DBLockTimeout:                 5 * time.Second,
		DBLongTxnAlertThreshold:       60 * time.Second,
		DBWatchdogInterval:            15 * time.Second,
		WorkerDBSyncInterval:          15 * time.Second,
		WorkerHeartbeatTimeout:        30 * time.Second,
		WorkerDisconnectSweepInterval: 30 * time.Second,
		WorkerDisconnectAckTimeout:    5 * time.Second,
		GRPCPubsubStartupTimeout:      30 * time.Second,
		DatabaseURL:                   "postgres://localhost/test?sslmode=require",
		RedisURL:                      "redis://localhost:6379",
		SequinBaseURL:                 "http://localhost:7376",
		SequinConsumerName:            "strait-cdc",
		SequinAPIToken:                "sequin-api-token",
		SequinBatchSize:               200,
		SequinWaitTimeMs:              5000,
		DBMaxConns:                    50,
		DBMinConns:                    10,
		SentryEnvironment:             "development",
		ExecutionTraceMode:            "off",
		DLQMaxPerJob:                  1000,
		DLQMaxPerProject:              10000,
		DLQOverflowPolicy:             "drop_oldest",
	}
}

func TestValidate_Happy(t *testing.T) {
	require.NoError(
		t, validConfig().Validate())
}

func TestValidate_SequinWebhookSecretRequiredOutsideDevelopment(t *testing.T) {
	t.Parallel()

	c := validConfig()
	c.SentryEnvironment = "production"
	c.SequinWebhookSecret = ""
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "SEQUIN_WEBHOOK_SECRET",
	)

	c.SequinWebhookSecret = "sequin-webhook-secret"
	require.NoError(
		t, c.Validate())
}

func TestValidate_RedisURLScheme(t *testing.T) {
	t.Parallel()

	c := validConfig()
	c.RedisURL = "http://localhost:6379"
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "REDIS_URL",
	)
}

func TestValidate_SequinPollingSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mut  func(*Config)
		want string
	}{
		{
			name: "batch_size_zero",
			mut:  func(c *Config) { c.SequinBatchSize = 0 },
			want: "SEQUIN_BATCH_SIZE",
		},
		{
			name: "wait_time_zero",
			mut:  func(c *Config) { c.SequinWaitTimeMs = 0 },
			want: "SEQUIN_WAIT_TIME_MS",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := validConfig()
			tt.mut(c)
			err := c.Validate()
			require.Error(t,
				err)
			assert.Contains(
				t, err.Error(), tt.
					want,
			)
		})
	}
}

func TestValidate_NegativeDuration(t *testing.T) {
	c := validConfig()
	c.HeartbeatInterval = -1 * time.Second
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "HEARTBEAT_INTERVAL",
	)
}

func TestValidate_ZeroDuration(t *testing.T) {
	c := validConfig()
	c.DBStatementTimeout = 0
	assert.Error(t,
		c.Validate())
}

func TestValidate_PollVsHeartbeat(t *testing.T) {
	c := validConfig()
	c.PollerInterval = 10 * time.Second
	c.HeartbeatInterval = 10 * time.Second
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "POLLER_INTERVAL",
	)
}

func TestValidate_StaleThresholdTooTight(t *testing.T) {
	c := validConfig()
	c.HeartbeatInterval = 10 * time.Second
	c.StaleThreshold = 15 * time.Second
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "STALE_THRESHOLD",
	)
}

func TestValidate_WorkerDBSyncIntervalInvariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name:   "equal to heartbeat",
			mutate: func(c *Config) { c.WorkerDBSyncInterval = c.HeartbeatInterval },
			want:   "WORKER_DB_SYNC_INTERVAL",
		},
		{
			name:   "less than heartbeat",
			mutate: func(c *Config) { c.WorkerDBSyncInterval = c.HeartbeatInterval - time.Second },
			want:   "WORKER_DB_SYNC_INTERVAL",
		},
		{
			name:   "equal to stale threshold",
			mutate: func(c *Config) { c.WorkerDBSyncInterval = c.StaleThreshold },
			want:   "STALE_THRESHOLD",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(c)
			err := c.Validate()
			require.Error(t,
				err)
			assert.Contains(
				t, err.Error(), tt.
					want,
			)
		})
	}
}

func TestLoad_WorkerDBSyncIntervalInvariant(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("HEARTBEAT_INTERVAL", "10s")
	t.Setenv("STALE_THRESHOLD", "60s")
	t.Setenv("WORKER_DB_SYNC_INTERVAL", "10s")

	_, err := Load()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "WORKER_DB_SYNC_INTERVAL",
	)
}

func TestLoad_RunsAggregateValidateInvariants(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("DB_MIN_CONNS", "100")
	t.Setenv("DB_MAX_CONNS", "50")

	_, err := Load()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "DB_MIN_CONNS",
	)
}

func TestValidate_LockTimeoutExceedsStatementTimeout(t *testing.T) {
	c := validConfig()
	c.DBLockTimeout = 60 * time.Second
	c.DBStatementTimeout = 30 * time.Second
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "DB_LOCK_TIMEOUT",
	)
}

func TestValidate_MinExceedsMaxConns(t *testing.T) {
	c := validConfig()
	c.DBMinConns = 100
	c.DBMaxConns = 50
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "DB_MIN_CONNS",
	)
}

func TestValidate_DLQCrossCap(t *testing.T) {
	c := validConfig()
	c.DLQMaxPerJob = 99999
	c.DLQMaxPerProject = 1000
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "DLQ_MAX_PER_JOB",
	)
}

func TestValidate_DLQPolicyValue(t *testing.T) {
	c := validConfig()
	c.DLQOverflowPolicy = "burn_it_down"
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "DLQ_OVERFLOW_POLICY",
	)
}

func TestValidate_AbsurdDurationsCaught(t *testing.T) {
	c := validConfig()
	c.DBStatementTimeout = 8 * 24 * time.Hour
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "exceeds reasonable max",
	)
}

func TestValidate_ZeroWorkerConcurrency(t *testing.T) {
	c := validConfig()
	c.WorkerConcurrency = 0
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "WORKER_CONCURRENCY",
	)
}

func TestValidate_InvalidExecutionTraceMode(t *testing.T) {
	c := validConfig()
	c.ExecutionTraceMode = "always"
	err := c.Validate()
	require.Error(t,
		err)
	assert.Contains(
		t, err.Error(), "EXECUTION_TRACE_MODE",
	)
}

func TestValidate_AccumulatesMultipleErrors(t *testing.T) {
	c := validConfig()
	c.WorkerConcurrency = 0
	c.DBStatementTimeout = 0
	c.DBLockTimeout = 100 * time.Hour // above reasonable? actually within 7d
	c.DLQOverflowPolicy = "bogus"
	err := c.Validate()
	require.Error(t,
		err)

	msg := err.Error()
	assert.False(t,
		!strings.Contains(msg,

			"WORKER_CONCURRENCY") || !strings.Contains(msg,
			"DB_STATEMENT_TIMEOUT") || !strings.Contains(msg, "DLQ_OVERFLOW_POLICY"))
}

func FuzzValidateNeverPanics(f *testing.F) {
	f.Add(int64(0), int64(0), int64(0))
	f.Add(int64(1e9), int64(1e9), int64(1e9))
	f.Fuzz(func(t *testing.T, a, b, d int64) {
		c := validConfig()
		c.HeartbeatInterval = time.Duration(a)
		c.PollerInterval = time.Duration(b)
		c.StaleThreshold = time.Duration(d)
		defer func() {
			if r := recover(); r != nil {
				require.Failf(t, "Validate panicked", "%v", r)
			}
		}()
		_ = c.Validate()
	})
}

func setRequiredAuditEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/test?sslmode=require")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("SEQUIN_BASE_URL", "http://localhost:7376")
	t.Setenv("SEQUIN_CONSUMER_NAME", "strait-cdc")
	t.Setenv("SEQUIN_API_TOKEN", "sequin-api-token")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	t.Setenv("AUDIT_RETENTION_DEFAULT_DAYS", "365")
	t.Setenv("AUDIT_ASYNC_BUFFER_SIZE", "4096")
}

func TestValidate_AuditRetentionNegative(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_RETENTION_DEFAULT_DAYS", "-1")

	_, err := Load()
	require.Error(t,
		err)
}

func TestValidate_AuditRetentionTooLarge(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_RETENTION_DEFAULT_DAYS", "36501")

	_, err := Load()
	require.Error(t,
		err)
	require.Contains(t,
		err.
			Error(), "AUDIT_RETENTION_DEFAULT_DAYS")
}

func TestValidate_AuditBufferTooSmall(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_ASYNC_BUFFER_SIZE", "100")

	_, err := Load()
	require.Error(t,
		err)
}

func TestValidate_AuditSIEMEndpointWithoutToken(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_SIEM_ENDPOINT", "https://siem.example.com/audit")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "")

	_, err := Load()
	require.Error(t,
		err)
}

func TestValidate_AuditSIEMEndpointWithToken(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_SIEM_ENDPOINT", "https://siem.example.com/audit")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "secret-bearer-token")

	_, err := Load()
	require.NoError(
		t, err)
}

func TestValidate_AuditSIEMEndpointWithUserinfo(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_SIEM_ENDPOINT", "https://u:p@siem.example.com/audit")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "secret-bearer-token")

	_, err := Load()
	require.Error(t,
		err)
}

func TestValidate_AuditSIEMEndpointUnparseable(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_SIEM_ENDPOINT", "://not-a-valid-url")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "secret-bearer-token")

	_, err := Load()
	require.Error(t,
		err)
}

func TestValidate_AuditDefaultsValid(t *testing.T) {
	setRequiredAuditEnv(t)

	cfg, err := Load()
	require.NoError(
		t, err)
	assert.Equal(t,
		365, cfg.AuditRetentionDefaultDays,
	)
	assert.Equal(t,
		4096, cfg.AuditAsyncBufferSize,
	)
}

func TestValidate_AuditDLQReclaimBatchZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_RECLAIM_BATCH", "0")

	_, err := Load()
	require.Error(t,
		err)
}

func TestValidate_AuditDLQReclaimBatchOne(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_RECLAIM_BATCH", "1")

	_, err := Load()
	require.NoError(
		t, err)
}

func TestValidate_AuditBufferExactly256(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_ASYNC_BUFFER_SIZE", "256")

	_, err := Load()
	require.NoError(
		t, err)
}

func TestValidate_AuditBuffer255(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_ASYNC_BUFFER_SIZE", "255")

	_, err := Load()
	require.Error(t,
		err)
}

func TestValidate_AuditDLQMaxAgeDaysZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_AGE_DAYS", "0")

	_, err := Load()
	require.NoError(
		t, err)
}

func TestValidate_AuditDLQMaxAgeDaysTooLarge(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_AGE_DAYS", "36501")

	_, err := Load()
	require.Error(t,
		err)
}

func TestValidate_AuditDLQMaxReclaimAttemptsZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_RECLAIM_ATTEMPTS", "0")

	_, err := Load()
	require.NoError(
		t, err)
}

func TestValidate_AuditRetentionZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_RETENTION_DEFAULT_DAYS", "0")

	_, err := Load()
	require.NoError(
		t, err)
}

func TestValidate_JWTSigningKeyExactly32Chars(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("JWT_SIGNING_KEY", "exactly-32-characters-key-value!")

	_, err := Load()
	require.NoError(
		t, err)
}

func TestValidate_JWTSigningKey31Chars(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("JWT_SIGNING_KEY", "exactly-31-characters-key-valu")

	_, err := Load()
	require.Error(t,
		err)
}

func TestValidate_AuditDLQMaxAgeDaysNegative(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_AGE_DAYS", "-1")

	_, err := Load()
	require.Error(t,
		err)
}

func TestValidate_AuditDLQMaxReclaimAttemptsNegative(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_RECLAIM_ATTEMPTS", "-1")

	_, err := Load()
	require.Error(t,
		err)
}

func TestValidate_AuditDLQReclaimBatchNegative(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_RECLAIM_BATCH", "-1")

	_, err := Load()
	require.Error(t,
		err)
}

// TestValidateDatabaseSSLMode is the regression guard for the TLS-downgrade
// finding: outside development, any sslmode that permits an unencrypted
// connection — including an unset sslmode (libpq defaults to "prefer") — must
// be rejected, while explicit secure modes are accepted. Development aliases
// (development, dev, test) stay permissive.
func TestValidateDatabaseSSLMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		databaseURL string
		environment string
		wantErr     bool
	}{
		{"require in prod", "postgres://h/db?sslmode=require", "production", false},
		{"verify-full in prod", "postgres://h/db?sslmode=verify-full", "production", false},
		{"disable in prod", "postgres://h/db?sslmode=disable", "production", true},
		{"prefer in prod", "postgres://h/db?sslmode=prefer", "production", true},
		{"allow in prod", "postgres://h/db?sslmode=allow", "production", true},
		{"absent in prod", "postgres://h/db", "production", true},
		{"absent empty env defaults non-dev", "postgres://h/db", "", true},
		{"uppercase DISABLE in prod", "postgres://h/db?sslmode=DISABLE", "production", true},
		{"dsn form require in prod", "host=h dbname=db sslmode=require", "production", false},
		{"dsn form absent in prod", "host=h dbname=db", "production", true},
		{"disable in development", "postgres://h/db?sslmode=disable", "development", false},
		{"absent in dev alias", "postgres://h/db", "dev", false},
		{"disable in test", "postgres://h/db?sslmode=disable", "test", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateDatabaseSSLMode(tt.databaseURL, tt.environment)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
