//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/billing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Equal(t,

		http.StatusPaymentRequired,

		rec.Code)
	assert.Equal(t,
		"application/json",

		rec.Header().Get("Content-Type"))

	var got map[string]any
	require.NoError(
		t,
		json.NewDecoder(rec.Body).Decode(&got))
	assert.Equal(t,
		"quota_exceeded",

		got["code"])
	assert.Equal(t,
		"plan_cap_reached",

		got["kind"])
	assert.Equal(t,
		"monthly run cap reached",

		got["message"])
	assert.EqualValues(t, 10_000,
		got["limit"].(float64))
	assert.EqualValues(t, 10_001,
		got["current"].(float64))
	assert.Equal(t,
		"starter",

		got["plan"])
	assert.Equal(t,
		"https://strait.dev/upgrade",

		got["upgrade_url"])

	// The bridge must emit the canonical envelope with `code: quota_exceeded`
	// at the top level and the granular reason preserved in `kind`.

	// The standard envelope keys must NOT be present — the SDK expects a
	// raw quota_exceeded body, not an ErrorResponse wrapper.
	if _, ok := got["error"]; ok {
		assert.Failf(t, "test failure",

			"envelope leaked: response contains `error` field")
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
	require.Equal(t,

		http.StatusServiceUnavailable,

		rec.Code,
	)

	var got map[string]any
	require.NoError(
		t,
		json.NewDecoder(rec.Body).Decode(&got))
	assert.Equal(t,
		"service_degraded",

		got["code"])
	assert.Equal(t,
		"service_degraded",

		got["kind"])

}
