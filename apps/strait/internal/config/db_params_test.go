package config

import (
	"testing"
	"time"
)

func TestApplyDBRuntimeParams_Defaults(t *testing.T) {
	cfg := &Config{
		DBStatementTimeout:         30 * time.Second,
		DBIdleInTransactionTimeout: 30 * time.Second,
		DBLockTimeout:              5 * time.Second,
	}
	params := map[string]string{}
	ApplyDBRuntimeParams(params, cfg)

	tests := map[string]string{
		"statement_timeout":                   "30000",
		"idle_in_transaction_session_timeout": "30000",
		"lock_timeout":                        "5000",
	}
	for k, want := range tests {
		if got := params[k]; got != want {
			t.Errorf("params[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestApplyDBRuntimeParams_ZeroSkips(t *testing.T) {
	cfg := &Config{} // all zero
	params := map[string]string{}
	ApplyDBRuntimeParams(params, cfg)
	if len(params) != 0 {
		t.Errorf("expected no params for zero config, got %v", params)
	}
}

func TestApplyDBRuntimeParams_NilSafe(t *testing.T) {
	// Must not panic.
	ApplyDBRuntimeParams(nil, nil)
	ApplyDBRuntimeParams(map[string]string{}, nil)
	ApplyDBRuntimeParams(nil, &Config{DBStatementTimeout: 1 * time.Second})
}

func TestApplyDBRuntimeParams_DoesNotOverwriteUnrelated(t *testing.T) {
	cfg := &Config{DBStatementTimeout: 1 * time.Second}
	params := map[string]string{"application_name": "strait-test"}
	ApplyDBRuntimeParams(params, cfg)

	if params["application_name"] != "strait-test" {
		t.Errorf("application_name was overwritten: %v", params)
	}
	if params["statement_timeout"] != "1000" {
		t.Errorf("statement_timeout = %q", params["statement_timeout"])
	}
}

func TestApplyDBRuntimeParams_MillisecondPrecision(t *testing.T) {
	cfg := &Config{
		DBStatementTimeout:         2500 * time.Millisecond,
		DBIdleInTransactionTimeout: 750 * time.Millisecond,
		DBLockTimeout:              100 * time.Millisecond,
	}
	params := map[string]string{}
	ApplyDBRuntimeParams(params, cfg)

	if params["statement_timeout"] != "2500" {
		t.Errorf("statement_timeout = %q", params["statement_timeout"])
	}
	if params["idle_in_transaction_session_timeout"] != "750" {
		t.Errorf("idle_in_transaction_session_timeout = %q", params["idle_in_transaction_session_timeout"])
	}
	if params["lock_timeout"] != "100" {
		t.Errorf("lock_timeout = %q", params["lock_timeout"])
	}
}
