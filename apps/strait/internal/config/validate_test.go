package config

import (
	"strings"
	"testing"
	"time"
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
		DatabaseURL:                   "postgres://localhost/test",
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
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestValidate_SequinWebhookSecretRequiredOutsideDevelopment(t *testing.T) {
	t.Parallel()

	c := validConfig()
	c.SentryEnvironment = "production"
	c.SequinWebhookSecret = ""
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "SEQUIN_WEBHOOK_SECRET") {
		t.Fatalf("Validate() error = %v, want SEQUIN_WEBHOOK_SECRET error", err)
	}

	c.SequinWebhookSecret = "sequin-webhook-secret"
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() rejected production Sequin webhook secret: %v", err)
	}
}

func TestValidate_RedisURLScheme(t *testing.T) {
	t.Parallel()

	c := validConfig()
	c.RedisURL = "http://localhost:6379"
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "REDIS_URL") {
		t.Fatalf("Validate() error = %v, want REDIS_URL scheme error", err)
	}
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
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestValidate_NegativeDuration(t *testing.T) {
	c := validConfig()
	c.HeartbeatInterval = -1 * time.Second
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "HEARTBEAT_INTERVAL") {
		t.Errorf("want heartbeat error, got %v", err)
	}
}

func TestValidate_ZeroDuration(t *testing.T) {
	c := validConfig()
	c.DBStatementTimeout = 0
	if err := c.Validate(); err == nil {
		t.Error("zero statement timeout should fail")
	}
}

func TestValidate_PollVsHeartbeat(t *testing.T) {
	c := validConfig()
	c.PollerInterval = 10 * time.Second
	c.HeartbeatInterval = 10 * time.Second
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "POLLER_INTERVAL") {
		t.Errorf("want poller/heartbeat error, got %v", err)
	}
}

func TestValidate_StaleThresholdTooTight(t *testing.T) {
	c := validConfig()
	c.HeartbeatInterval = 10 * time.Second
	c.StaleThreshold = 15 * time.Second
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "STALE_THRESHOLD") {
		t.Errorf("want stale threshold error, got %v", err)
	}
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
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestLoad_WorkerDBSyncIntervalInvariant(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("HEARTBEAT_INTERVAL", "10s")
	t.Setenv("STALE_THRESHOLD", "60s")
	t.Setenv("WORKER_DB_SYNC_INTERVAL", "10s")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "WORKER_DB_SYNC_INTERVAL") {
		t.Fatalf("Load() error = %v, want WORKER_DB_SYNC_INTERVAL", err)
	}
}

func TestLoad_RunsAggregateValidateInvariants(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("DB_MIN_CONNS", "100")
	t.Setenv("DB_MAX_CONNS", "50")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DB_MIN_CONNS") {
		t.Fatalf("Load() error = %v, want DB_MIN_CONNS validation error", err)
	}
}

func TestValidate_LockTimeoutExceedsStatementTimeout(t *testing.T) {
	c := validConfig()
	c.DBLockTimeout = 60 * time.Second
	c.DBStatementTimeout = 30 * time.Second
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "DB_LOCK_TIMEOUT") {
		t.Errorf("want lock timeout error, got %v", err)
	}
}

func TestValidate_MinExceedsMaxConns(t *testing.T) {
	c := validConfig()
	c.DBMinConns = 100
	c.DBMaxConns = 50
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "DB_MIN_CONNS") {
		t.Errorf("want min conns error, got %v", err)
	}
}

func TestValidate_DLQCrossCap(t *testing.T) {
	c := validConfig()
	c.DLQMaxPerJob = 99999
	c.DLQMaxPerProject = 1000
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "DLQ_MAX_PER_JOB") {
		t.Errorf("want DLQ cross-cap error, got %v", err)
	}
}

func TestValidate_DLQPolicyValue(t *testing.T) {
	c := validConfig()
	c.DLQOverflowPolicy = "burn_it_down"
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "DLQ_OVERFLOW_POLICY") {
		t.Errorf("want DLQ policy error, got %v", err)
	}
}

func TestValidate_AbsurdDurationsCaught(t *testing.T) {
	c := validConfig()
	c.DBStatementTimeout = 8 * 24 * time.Hour
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "exceeds reasonable max") {
		t.Errorf("want max-reasonable error, got %v", err)
	}
}

func TestValidate_ZeroWorkerConcurrency(t *testing.T) {
	c := validConfig()
	c.WorkerConcurrency = 0
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "WORKER_CONCURRENCY") {
		t.Errorf("want concurrency error, got %v", err)
	}
}

func TestValidate_InvalidExecutionTraceMode(t *testing.T) {
	c := validConfig()
	c.ExecutionTraceMode = "always"
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "EXECUTION_TRACE_MODE") {
		t.Errorf("want execution trace mode error, got %v", err)
	}
}

func TestValidate_AccumulatesMultipleErrors(t *testing.T) {
	c := validConfig()
	c.WorkerConcurrency = 0
	c.DBStatementTimeout = 0
	c.DBLockTimeout = 100 * time.Hour // above reasonable? actually within 7d
	c.DLQOverflowPolicy = "bogus"
	err := c.Validate()
	if err == nil {
		t.Fatal("want errors")
	}
	msg := err.Error()
	if !strings.Contains(msg, "WORKER_CONCURRENCY") ||
		!strings.Contains(msg, "DB_STATEMENT_TIMEOUT") ||
		!strings.Contains(msg, "DLQ_OVERFLOW_POLICY") {
		t.Errorf("missing accumulated errors: %v", msg)
	}
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
				t.Fatalf("Validate panicked: %v", r)
			}
		}()
		_ = c.Validate()
	})
}

func setRequiredAuditEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
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
	if err == nil {
		t.Fatal("expected error for negative audit retention")
	}
}

func TestValidate_AuditRetentionTooLarge(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_RETENTION_DEFAULT_DAYS", "36501")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for oversized audit retention")
	}
	if !strings.Contains(err.Error(), "AUDIT_RETENTION_DEFAULT_DAYS") {
		t.Fatalf("error = %v, want AUDIT_RETENTION_DEFAULT_DAYS", err)
	}
}

func TestValidate_AuditBufferTooSmall(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_ASYNC_BUFFER_SIZE", "100")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for buffer size < 256")
	}
}

func TestValidate_AuditSIEMEndpointWithoutToken(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_SIEM_ENDPOINT", "https://siem.example.com/audit")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for SIEM endpoint without token")
	}
}

func TestValidate_AuditSIEMEndpointWithToken(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_SIEM_ENDPOINT", "https://siem.example.com/audit")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "secret-bearer-token")

	_, err := Load()
	if err != nil {
		t.Fatalf("unexpected error for valid SIEM config: %v", err)
	}
}

func TestValidate_AuditSIEMEndpointWithUserinfo(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_SIEM_ENDPOINT", "https://u:p@siem.example.com/audit")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "secret-bearer-token")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for SIEM endpoint containing userinfo")
	}
}

func TestValidate_AuditSIEMEndpointUnparseable(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_SIEM_ENDPOINT", "://not-a-valid-url")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "secret-bearer-token")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unparseable SIEM endpoint")
	}
}

func TestValidate_AuditDefaultsValid(t *testing.T) {
	setRequiredAuditEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error with valid audit defaults: %v", err)
	}
	if cfg.AuditRetentionDefaultDays != 365 {
		t.Errorf("AuditRetentionDefaultDays = %d, want 365", cfg.AuditRetentionDefaultDays)
	}
	if cfg.AuditAsyncBufferSize != 4096 {
		t.Errorf("AuditAsyncBufferSize = %d, want 4096", cfg.AuditAsyncBufferSize)
	}
}

func TestValidate_AuditDLQReclaimBatchZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_RECLAIM_BATCH", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for zero DLQ reclaim batch")
	}
}

func TestValidate_AuditDLQReclaimBatchOne(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_RECLAIM_BATCH", "1")

	_, err := Load()
	if err != nil {
		t.Fatalf("DLQ reclaim batch=1 should be valid: %v", err)
	}
}

func TestValidate_AuditBufferExactly256(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_ASYNC_BUFFER_SIZE", "256")

	_, err := Load()
	if err != nil {
		t.Fatalf("buffer size=256 should be valid: %v", err)
	}
}

func TestValidate_AuditBuffer255(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_ASYNC_BUFFER_SIZE", "255")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for buffer size=255")
	}
}

func TestValidate_AuditDLQMaxAgeDaysZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_AGE_DAYS", "0")

	_, err := Load()
	if err != nil {
		t.Fatalf("DLQ max age=0 should be valid (disables sweep): %v", err)
	}
}

func TestValidate_AuditDLQMaxAgeDaysTooLarge(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_AGE_DAYS", "36501")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for oversized DLQ max age")
	}
}

func TestValidate_AuditDLQMaxReclaimAttemptsZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_RECLAIM_ATTEMPTS", "0")

	_, err := Load()
	if err != nil {
		t.Fatalf("DLQ max reclaim attempts=0 should be valid: %v", err)
	}
}

func TestValidate_AuditRetentionZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_RETENTION_DEFAULT_DAYS", "0")

	_, err := Load()
	if err != nil {
		t.Fatalf("retention=0 should be valid: %v", err)
	}
}

func TestValidate_JWTSigningKeyExactly32Chars(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("JWT_SIGNING_KEY", "exactly-32-characters-key-value!")

	_, err := Load()
	if err != nil {
		t.Fatalf("32-char JWT key should be valid: %v", err)
	}
}

func TestValidate_JWTSigningKey31Chars(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("JWT_SIGNING_KEY", "exactly-31-characters-key-valu")

	_, err := Load()
	if err == nil {
		t.Fatal("31-char JWT key should be rejected")
	}
}

func TestValidate_AuditDLQMaxAgeDaysNegative(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_AGE_DAYS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative DLQ max age days")
	}
}

func TestValidate_AuditDLQMaxReclaimAttemptsNegative(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_RECLAIM_ATTEMPTS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative DLQ max reclaim attempts")
	}
}

func TestValidate_AuditDLQReclaimBatchNegative(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_RECLAIM_BATCH", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative DLQ reclaim batch")
	}
}
