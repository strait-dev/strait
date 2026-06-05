package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"strait/internal/billing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NotNil(t, out)

	var rse *rawStatusError
	require.True(t, errors.As(out,
		&rse))
	assert.Equal(t, http.
		StatusPaymentRequired,

		rse.status,
	)

	raw, err := json.Marshal(rse.body)
	require.NoError(t,
		err)

	var got map[string]any
	require.NoError(t,
		json.Unmarshal(raw, &got))
	assert.Equal(t, "quota_exceeded",

		got["code"])
	assert.Equal(t, "plan_cap_reached",

		got["kind"])
	assert.Equal(t, "monthly run cap reached",

		got["message"])
	assert.EqualValues(t, 10_000,
		got["limit"].(float64))
	assert.EqualValues(t, 10_001,
		got["current"].(float64))
	assert.Equal(t, "starter",
		got["plan"])
	assert.Equal(t, "https://strait.dev/upgrade",

		got["upgrade_url"])

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
	require.True(t, errors.As(out,
		&rse))
	assert.Equal(t, http.
		StatusServiceUnavailable,

		rse.status,
	)

	raw, _ := json.Marshal(rse.body)
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	assert.Equal(t, "service_degraded",

		got["code"])
	assert.Equal(t, "service_degraded",

		got["kind"])

}

// TestNewQuotaExceeded_PrefixComposesMessage covers the bulk-item prefix
// path (`item 3: monthly run cap reached`) so callers using the prefix
// argument get a deterministic message format.
func TestNewQuotaExceeded_PrefixComposesMessage(t *testing.T) {
	t.Parallel()

	le := &billing.LimitError{Code: "plan_cap_reached", Message: "boom"}
	out := newQuotaExceeded(le, "item 3")
	var rse *rawStatusError
	require.True(t, errors.As(out,
		&rse))

	body, _ := json.Marshal(rse.body)
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	assert.Equal(t, "item 3: boom",

		got["message"])

}

// TestLimitErrorTo402_PassThroughNonLimitError ensures non-billing errors are
// returned untouched so callers can fall back to their existing error paths.
func TestLimitErrorTo402_PassThroughNonLimitError(t *testing.T) {
	t.Parallel()

	original := errors.New("network read failed")
	got := limitErrorTo402(original, "")
	assert.True(t, errors.Is(got,
		original))

}
