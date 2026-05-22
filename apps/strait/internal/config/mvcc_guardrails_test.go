package config

import (
	"testing"
	"time"
)

// MVCC horizon guardrail config tests. These only exercise env parsing
// and default values; the actual session-level effect is verified by the pool
// integration tests in cmd/strait.

func TestLoad_MVCCGuardrailDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"DBIdleInTransactionTimeout", cfg.DBIdleInTransactionTimeout, 30 * time.Second},
		{"DBLockTimeout", cfg.DBLockTimeout, 5 * time.Second},
		{"DBTransactionTimeout", cfg.DBTransactionTimeout, 60 * time.Second},
		{"DBLongTxnAlertThreshold", cfg.DBLongTxnAlertThreshold, 60 * time.Second},
		{"DBWatchdogInterval", cfg.DBWatchdogInterval, 15 * time.Second},
		{"DBWatchdogEnabled", cfg.DBWatchdogEnabled, true},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestLoad_MVCCGuardrailOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_IDLE_IN_TRANSACTION_TIMEOUT", "120s")
	t.Setenv("DB_LOCK_TIMEOUT", "2500ms")
	t.Setenv("DB_TRANSACTION_TIMEOUT", "10m")
	t.Setenv("DB_LONG_TXN_ALERT_THRESHOLD", "90s")
	t.Setenv("DB_WATCHDOG_INTERVAL", "45s")
	t.Setenv("DB_WATCHDOG_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DBIdleInTransactionTimeout != 2*time.Minute {
		t.Errorf("DBIdleInTransactionTimeout = %v", cfg.DBIdleInTransactionTimeout)
	}
	if cfg.DBLockTimeout != 2500*time.Millisecond {
		t.Errorf("DBLockTimeout = %v", cfg.DBLockTimeout)
	}
	if cfg.DBTransactionTimeout != 10*time.Minute {
		t.Errorf("DBTransactionTimeout = %v", cfg.DBTransactionTimeout)
	}
	if cfg.DBLongTxnAlertThreshold != 90*time.Second {
		t.Errorf("DBLongTxnAlertThreshold = %v", cfg.DBLongTxnAlertThreshold)
	}
	if cfg.DBWatchdogInterval != 45*time.Second {
		t.Errorf("DBWatchdogInterval = %v", cfg.DBWatchdogInterval)
	}
	if cfg.DBWatchdogEnabled {
		t.Error("DBWatchdogEnabled should be false")
	}
}

func TestLoad_MVCCGuardrailZeroRejected(t *testing.T) {
	// Zero database guardrail durations fail validation so deployments do not
	// accidentally disable lock and idle-in-transaction protection.
	setRequiredEnv(t)
	t.Setenv("DB_IDLE_IN_TRANSACTION_TIMEOUT", "0")
	t.Setenv("DB_LOCK_TIMEOUT", "0")
	t.Setenv("DB_TRANSACTION_TIMEOUT", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected validation error for zero MVCC guardrail durations")
	}
}

// FuzzDurationParsing asserts that arbitrary duration-ish strings fed to the
// MVCC config vars never cause Load to panic, and that ApplyDBRuntimeParams
// treats non-positive results as "skip" (so a negative or zero value never
// propagates a bogus Postgres session param).
func FuzzDurationParsing(f *testing.F) {
	seeds := []string{"30s", "1m", "250ms", "0", "", "1h", "1.5s", "-30s", "not-a-duration", "9999999999999s"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		// Oversized inputs can blow up aconfig; cap it.
		if len(raw) > 64 {
			return
		}
		// setenv rejects NUL bytes; skip those inputs.
		for _, r := range raw {
			if r == 0 {
				return
			}
		}
		setRequiredEnv(t)
		t.Setenv("DB_IDLE_IN_TRANSACTION_TIMEOUT", raw)
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Load panicked on %q: %v", raw, r)
			}
		}()
		cfg, err := Load()
		if err != nil {
			return
		}
		// Whatever the parsed value, the runtime param application must be
		// safe: non-positive values result in no emitted param.
		params := map[string]string{}
		ApplyDBRuntimeParams(params, cfg)
		if cfg.DBIdleInTransactionTimeout <= 0 {
			if _, ok := params["idle_in_transaction_session_timeout"]; ok {
				t.Errorf("non-positive duration %v leaked into params from %q", cfg.DBIdleInTransactionTimeout, raw)
			}
		} else {
			if _, ok := params["idle_in_transaction_session_timeout"]; !ok {
				t.Errorf("positive duration %v missing from params for %q", cfg.DBIdleInTransactionTimeout, raw)
			}
		}
	})
}
