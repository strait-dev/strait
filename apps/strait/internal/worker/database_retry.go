package worker

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

func isRetryablePostgresError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if pgconn.Timeout(err) || pgconn.SafeToRetry(err) {
		return true
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	switch pgErr.Code {
	case "25P02", // in_failed_sql_transaction
		"40P01", // deadlock_detected
		"53300", // too_many_connections
		"53400", // configuration_limit_exceeded
		"55P03", // lock_not_available
		"57014", // query_canceled
		"57P03": // cannot_connect_now
		return true
	default:
		return len(pgErr.Code) >= 2 && pgErr.Code[:2] == "08"
	}
}
