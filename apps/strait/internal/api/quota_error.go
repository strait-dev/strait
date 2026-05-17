package api

import (
	"errors"
	"net/http"

	"strait/internal/billing"
)

// QuotaExceededBody is the canonical 402 response body for plan-quota
// rejections. Top-level `code` is always "quota_exceeded" so clients can
// branch on a single string; `kind` carries the granular reason from the
// underlying *billing.LimitError for telemetry and finer-grained UX.
type QuotaExceededBody struct {
	Code       string `json:"code"`
	Kind       string `json:"kind"`
	Message    string `json:"message"`
	Limit      int64  `json:"limit"`
	Current    int64  `json:"current"`
	Plan       string `json:"plan,omitempty"`
	UpgradeURL string `json:"upgrade_url,omitempty"`
}

const (
	quotaExceededCode    = "quota_exceeded"
	serviceDegradedCode  = "service_degraded"
	serviceDegradedKind  = "service_degraded"
	defaultQuotaErrorMsg = "quota exceeded"
)

// quotaExceededError converts a *billing.LimitError into a 402 PaymentRequired
// response with the canonical structured body. The granular LimitError.Code is
// preserved in `kind` so downstream UX can still distinguish reasons (e.g.
// plan_cap_reached vs project_limit_reached).
//
// `service_degraded` is the one non-quota code that LimitError carries; it
// represents fail-open exhaustion and is mapped to 503 instead.
func newQuotaExceeded(le *billing.LimitError, messagePrefix string) error {
	if le == nil {
		return nil
	}
	if le.Code == serviceDegradedKind {
		return &rawStatusError{
			status: http.StatusServiceUnavailable,
			body: QuotaExceededBody{
				Code:    serviceDegradedCode,
				Kind:    le.Code,
				Message: prefixMessage(messagePrefix, le.Message),
			},
		}
	}
	return &rawStatusError{
		status: http.StatusPaymentRequired,
		body: QuotaExceededBody{
			Code:       quotaExceededCode,
			Kind:       le.Code,
			Message:    prefixMessage(messagePrefix, le.Message),
			Limit:      le.Limit,
			Current:    le.CurrentUsage,
			Plan:       le.Plan,
			UpgradeURL: le.UpgradeURL,
		},
	}
}

// limitErrorTo402 inspects err, converts it to a structured 402/503 if it
// wraps a *billing.LimitError, and otherwise returns the original err.
// Callers use this at the API boundary to standardize the response shape
// without losing non-LimitError failures.
func limitErrorTo402(err error, messagePrefix string) error {
	if err == nil {
		return nil
	}
	var le *billing.LimitError
	if errors.As(err, &le) {
		return newQuotaExceeded(le, messagePrefix)
	}
	return err
}

func prefixMessage(prefix, message string) string {
	if prefix == "" {
		return message
	}
	if message == "" {
		return prefix
	}
	return prefix + ": " + message
}
