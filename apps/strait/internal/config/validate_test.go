package config

import (
	"strings"
	"testing"
	"time"
)

func validConfig() *Config {
	return &Config{
		WorkerConcurrency:              25,
		HeartbeatInterval:              10 * time.Second,
		ReaperInterval:                 30 * time.Second,
		StaleThreshold:                 60 * time.Second,
		PollerInterval:                 5 * time.Second,
		RequestTimeout:                 30 * time.Second,
		WorkerDrainTimeout:             30 * time.Second,
		DBStatementTimeout:             30 * time.Second,
		DBMaxConnLifetime:              30 * time.Minute,
		DBMaxConnIdleTime:              5 * time.Minute,
		DBHealthCheckPeriod:            30 * time.Second,
		DBIdleInTransactionTimeout:     30 * time.Second,
		DBLockTimeout:                  5 * time.Second,
		DBLongTxnAlertThreshold:        60 * time.Second,
		DBWatchdogInterval:             15 * time.Second,
		DBMaxConns:                     50,
		DBMinConns:                     10,
		DLQMaxPerJob:                   1000,
		DLQMaxPerProject:               10000,
		DLQOverflowPolicy:              "drop_oldest",
	}
}

func TestValidate_Happy(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
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
