package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"strait/internal/domain"
)

func classifyError(err error) string {
	if err == nil {
		return domain.ErrorClassUnknown
	}

	// Budget errors take highest priority.
	if isBudgetError(err) {
		return domain.ErrorClassBudget
	}

	// Deadline / timeout.
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.ErrorClassTimeout
	}

	// OOM signals.
	if isOOMError(err) {
		return domain.ErrorClassOOM
	}

	// Endpoint HTTP status classification.
	var endpointErr *domain.EndpointError
	if errors.As(err, &endpointErr) {
		switch {
		case endpointErr.StatusCode == http.StatusTooManyRequests:
			return domain.ErrorClassRateLimited
		case endpointErr.StatusCode == http.StatusUnauthorized || endpointErr.StatusCode == http.StatusForbidden:
			return domain.ErrorClassAuth
		case endpointErr.StatusCode >= http.StatusBadRequest && endpointErr.StatusCode < http.StatusInternalServerError:
			return domain.ErrorClassClient
		case endpointErr.StatusCode >= http.StatusInternalServerError:
			return domain.ErrorClassServer
		}
	}

	// Connection errors.
	if isConnectionError(err) {
		return domain.ErrorClassConnection
	}

	// Generic network errors.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return domain.ErrorClassTransient
	}

	// Context canceled (not deadline) is transient.
	if errors.Is(err, context.Canceled) {
		return domain.ErrorClassTransient
	}

	return domain.ErrorClassUnknown
}

func isOOMError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "out of memory") ||
		strings.Contains(msg, "OOM") ||
		strings.Contains(msg, "memory limit exceeded") ||
		strings.Contains(msg, "ENOMEM")
}

func isConnectionError(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}

func isBudgetError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "budget exceeded") ||
		strings.Contains(msg, "cost limit")
}

// errorHash returns a 16-char hex digest of the first 200 runes of an error
// message. Used for poison pill detection to identify identical errors across
// retry attempts without storing the full error string in metadata. Truncates
// by rune so multi-byte UTF-8 sequences are never split mid-character.
func errorHash(errMsg string) string {
	runes := []rune(errMsg)
	if len(runes) > 200 {
		runes = runes[:200]
	}
	h := sha256.Sum256([]byte(string(runes)))
	return hex.EncodeToString(h[:8])
}

func errorHashForError(err error) string {
	var endpointErr *domain.EndpointError
	if errors.As(err, &endpointErr) {
		return errorHash(fmt.Sprintf("endpoint returned %d: %s", endpointErr.StatusCode, endpointErr.Body))
	}
	return errorHash(err.Error())
}

func shouldRetryForClass(errClass string) bool {
	switch errClass {
	case domain.ErrorClassClient, domain.ErrorClassAuth, domain.ErrorClassBudget, domain.ErrorClassOOM:
		return false
	default:
		return true
	}
}

func shouldUseFallbackForClass(errClass string) bool {
	switch errClass {
	case domain.ErrorClassTransient, domain.ErrorClassRateLimited, domain.ErrorClassConnection, domain.ErrorClassTimeout:
		return true
	default:
		return false
	}
}
