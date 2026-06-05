package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MVCC horizon guardrail config tests. These only exercise env parsing
// and default values; the actual session-level effect is verified by the pool
// integration tests in cmd/strait.

func TestLoad_MVCCGuardrailDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	require.NoError(
		t, err)

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
		assert.Equal(t,
			tt.want,
			tt.
				got)

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
	require.NoError(
		t, err)
	assert.Equal(t,
		2*time.
			Minute,

		cfg.DBIdleInTransactionTimeout)
	assert.Equal(t,
		2500*time.
			Millisecond, cfg.DBLockTimeout)
	assert.Equal(t,
		10*time.
			Minute,
		cfg.DBTransactionTimeout)
	assert.Equal(t,
		90*time.
			Second,
		cfg.DBLongTxnAlertThreshold)
	assert.Equal(t,
		45*time.
			Second,
		cfg.DBWatchdogInterval)
	assert.False(t,
		cfg.DBWatchdogEnabled,
	)

}

func TestLoad_MVCCGuardrailZeroRejected(t *testing.T) {
	// Zero database guardrail durations fail validation so deployments do not
	// accidentally disable lock and idle-in-transaction protection.
	setRequiredEnv(t)
	t.Setenv("DB_IDLE_IN_TRANSACTION_TIMEOUT", "0")
	t.Setenv("DB_LOCK_TIMEOUT", "0")
	t.Setenv("DB_TRANSACTION_TIMEOUT", "0")

	_, err := Load()
	require.Error(t,
		err)

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
				require.Failf(t, "Load panicked", "%q: %v", raw, r)
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
			assert.NotContains(t, params, "idle_in_transaction_session_timeout", "raw duration %q", raw)
		} else {
			assert.Contains(t, params, "idle_in_transaction_session_timeout", "raw duration %q", raw)
		}
	})
}
