//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/billing"
)

// TestWriteTypedError_LimitError_402 covers the boundary case STR-462 cares
// about: a handler returns a bare *billing.LimitError, the bridge surfaces it
// as a 402 with the canonical quota_exceeded body, NOT the standard
// ErrorResponse envelope.
func TestWriteTypedError_LimitError_402(t *testing.T) {
	t.Parallel()

	le := &billing.LimitError{
		Code:         "plan_cap_reached",
		Message:      "monthly run cap reached",
		CurrentUsage: 10_001,
		Limit:        10_000,
		Plan:         "starter",
		UpgradeURL:   "https://strait.dev/upgrade",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", nil)
	writeTypedError(rec, req, le)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusPaymentRequired)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	// The bridge must emit the canonical envelope with `code: quota_exceeded`
	// at the top level and the granular reason preserved in `kind`.
	if got["code"] != "quota_exceeded" {
		t.Errorf("code = %v, want quota_exceeded", got["code"])
	}
	if got["kind"] != "plan_cap_reached" {
		t.Errorf("kind = %v, want plan_cap_reached", got["kind"])
	}
	if got["message"] != "monthly run cap reached" {
		t.Errorf("message = %v", got["message"])
	}
	if got["limit"].(float64) != 10_000 {
		t.Errorf("limit = %v, want 10000", got["limit"])
	}
	if got["current"].(float64) != 10_001 {
		t.Errorf("current = %v, want 10001", got["current"])
	}
	if got["plan"] != "starter" {
		t.Errorf("plan = %v, want starter", got["plan"])
	}
	if got["upgrade_url"] != "https://strait.dev/upgrade" {
		t.Errorf("upgrade_url = %v", got["upgrade_url"])
	}

	// The standard envelope keys must NOT be present — the SDK expects a
	// raw quota_exceeded body, not an ErrorResponse wrapper.
	if _, ok := got["error"]; ok {
		t.Errorf("envelope leaked: response contains `error` field")
	}
}

// TestWriteTypedError_LimitError_ServiceDegraded_503 documents that the one
// LimitError code that isn't a quota condition (`service_degraded` — billing
// enforcement failed open) must surface as 503 so SDKs treat it as a transient
// availability event, not a plan rejection.
func TestWriteTypedError_LimitError_ServiceDegraded_503(t *testing.T) {
	t.Parallel()

	le := &billing.LimitError{
		Code:    "service_degraded",
		Message: "Billing enforcement is temporarily unavailable. Please retry shortly.",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", nil)
	writeTypedError(rec, req, le)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var got map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got["code"] != "service_degraded" {
		t.Errorf("code = %v, want service_degraded", got["code"])
	}
	if got["kind"] != "service_degraded" {
		t.Errorf("kind = %v, want service_degraded", got["kind"])
	}
}
