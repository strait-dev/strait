package config

import (
	"fmt"
)

// ApplyDBRuntimeParams writes Postgres runtime parameters (statement_timeout,
// idle_in_transaction_session_timeout, lock_timeout) derived from cfg into
// the supplied params map. Callers pass pgx's poolConfig.ConnConfig.RuntimeParams
// here so these settings are applied to every new connection via the startup
// message. A value of zero in cfg means "leave the param unset".
//
// transaction_timeout is NOT written here because it is Postgres 17+ only and
// must be issued via SET after connection (see services.go's AfterConnect).
func ApplyDBRuntimeParams(params map[string]string, cfg *Config) {
	if params == nil || cfg == nil {
		return
	}
	if cfg.DBStatementTimeout > 0 {
		params["statement_timeout"] = fmt.Sprintf("%d", cfg.DBStatementTimeout.Milliseconds())
	}
	if cfg.DBIdleInTransactionTimeout > 0 {
		params["idle_in_transaction_session_timeout"] = fmt.Sprintf("%d", cfg.DBIdleInTransactionTimeout.Milliseconds())
	}
	if cfg.DBLockTimeout > 0 {
		params["lock_timeout"] = fmt.Sprintf("%d", cfg.DBLockTimeout.Milliseconds())
	}
}
