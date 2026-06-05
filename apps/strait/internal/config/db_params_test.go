package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
		assert.Equal(t, want, params[k], "params[%q]", k)
	}
}

func TestApplyDBRuntimeParams_ZeroSkips(t *testing.T) {
	cfg := &Config{} // all zero
	params := map[string]string{}
	ApplyDBRuntimeParams(params, cfg)
	assert.Len(t, params,
		0)

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
	assert.Equal(t,
		"strait-test",

		params["application_name"])
	assert.Equal(t,
		"1000",
		params["statement_timeout"])

}

func TestApplyDBRuntimeParams_MillisecondPrecision(t *testing.T) {
	cfg := &Config{
		DBStatementTimeout:         2500 * time.Millisecond,
		DBIdleInTransactionTimeout: 750 * time.Millisecond,
		DBLockTimeout:              100 * time.Millisecond,
	}
	params := map[string]string{}
	ApplyDBRuntimeParams(params, cfg)
	assert.Equal(t,
		"2500",
		params["statement_timeout"])
	assert.Equal(t,
		"750", params["idle_in_transaction_session_timeout"])
	assert.Equal(t,
		"100", params["lock_timeout"])

}
