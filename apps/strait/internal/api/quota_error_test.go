package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"strait/internal/billing"
)

// TestNewQuotaExceeded_BodyShape asserts the canonical 402 body shape that
// STR-462 requires: top-level `code` is always "quota_exceeded", with the
// granular limit reason preserved in `kind`, and numeric/plan/upgrade fields
// surfaced verbatim from the underlying *billing.LimitError.
func TestNewQuotaExceeded_BodyShape(t *testing.T) {
	t.Parallel()

	le := &billing.LimitError{
		Code:         "plan_cap_reached",
		Message:      "monthly run cap reached",
		CurrentUsage: 10_001,
		Limit:        10_000,
		Plan:         "starter",
		UpgradeURL:   "https://strait.dev/upgrade",
	}

	out := newQuotaExceeded(le, "")
	if out == nil {
		t.Fatal("newQuotaExceeded returned nil")
	}

	var rse *rawStatusError
	if !errors.As(out, &rse) {
		t.Fatalf("expected *rawStatusError, got %T", out)
	}
	if rse.status != http.StatusPaymentRequired {
		t.Errorf("status = %d, want %d", rse.status, http.StatusPaymentRequired)
	}

	raw, err := json.Marshal(rse.body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

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
}

// TestNewQuotaExceeded_ServiceDegradedMapsTo503 documents the one LimitError
// code that is not a plan-quota condition: service_degraded is fail-open
// exhaustion and must surface as 503 with code "service_degraded" so clients
// don't treat it as a billing problem.
func TestNewQuotaExceeded_ServiceDegradedMapsTo503(t *testing.T) {
	t.Parallel()

	le := &billing.LimitError{
		Code:    "service_degraded",
		Message: "Billing enforcement is temporarily unavailable. Please retry shortly.",
	}

	out := newQuotaExceeded(le, "")
	var rse *rawStatusError
	if !errors.As(out, &rse) {
		t.Fatalf("expected *rawStatusError, got %T", out)
	}
	if rse.status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rse.status, http.StatusServiceUnavailable)
	}

	raw, _ := json.Marshal(rse.body)
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	if got["code"] != "service_degraded" {
		t.Errorf("code = %v, want service_degraded", got["code"])
	}
	if got["kind"] != "service_degraded" {
		t.Errorf("kind = %v, want service_degraded", got["kind"])
	}
}

// TestNewQuotaExceeded_PrefixComposesMessage covers the bulk-item prefix
// path (`item 3: monthly run cap reached`) so callers using the prefix
// argument get a deterministic message format.
func TestNewQuotaExceeded_PrefixComposesMessage(t *testing.T) {
	t.Parallel()

	le := &billing.LimitError{Code: "plan_cap_reached", Message: "boom"}
	out := newQuotaExceeded(le, "item 3")
	var rse *rawStatusError
	if !errors.As(out, &rse) {
		t.Fatalf("expected *rawStatusError, got %T", out)
	}
	body, _ := json.Marshal(rse.body)
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	if got["message"] != "item 3: boom" {
		t.Errorf("message = %v, want %q", got["message"], "item 3: boom")
	}
}

// TestLimitErrorTo402_PassThroughNonLimitError ensures non-billing errors are
// returned untouched so callers can fall back to their existing error paths.
func TestLimitErrorTo402_PassThroughNonLimitError(t *testing.T) {
	t.Parallel()

	original := errors.New("network read failed")
	got := limitErrorTo402(original, "")
	if !errors.Is(got, original) {
		t.Errorf("got %v, want pass-through of original error", got)
	}
}
