package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

const databaseAdmissionRetryAfterSeconds = 1

func isRetryableDatabaseAdmissionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if pgconn.Timeout(err) || pgconn.SafeToRetry(err) {
		return true
	}
	if isRetryableDatabaseAdmissionMessage(err) {
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

func isRetryableDatabaseAdmissionMessage(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"broken pipe",
		"conn closed",
		"connection closed",
		"connection reset by peer",
		"failed to connect",
		"i/o timeout",
		"server closed the connection",
		"unexpected eof",
		"use of closed network connection",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func newDatabaseAdmission429() *typedAPIError {
	retryAfter := strconv.Itoa(databaseAdmissionRetryAfterSeconds)
	return &typedAPIError{
		status: http.StatusTooManyRequests,
		apiError: APIError{
			Code:    ErrorCodeRateLimited,
			Message: "database admission control throttled",
			Details: []string{
				"retry_after_seconds=" + retryAfter,
			},
		},
		headers: map[string]string{
			"Retry-After": retryAfter,
		},
	}
}

func respondDatabaseAdmission429(w http.ResponseWriter, r *http.Request) {
	apiErr := newDatabaseAdmission429()
	for key, value := range apiErr.headers {
		w.Header().Set(key, value)
	}
	respondError(w, r, apiErr.status, apiErr.apiError)
}
