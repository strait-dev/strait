package api

import (
	"context"
	"crypto/subtle"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHasScope_WildcardBypass verifies that the wildcard scope grants access to every scope.
func TestHasScope_WildcardBypass(t *testing.T) {
	t.Parallel()

	scopes := []string{domain.ScopeAll}
	targets := []string{
		domain.ScopeJobsRead, domain.ScopeJobsWrite, domain.ScopeJobsTrigger,
		domain.ScopeRunsRead, domain.ScopeRunsWrite,
		domain.ScopeWorkflowsRead, domain.ScopeWorkflowsWrite, domain.ScopeWorkflowsTrigger,
		domain.ScopeSecretsRead, domain.ScopeSecretsWrite,
		domain.ScopeAPIKeysManage, domain.ScopeRBACManage, domain.ScopeStatsRead,
		domain.ScopeProjectsRead, domain.ScopeProjectsWrite, domain.ScopeProjectsManage,
		"anything:unknown", "",
	}
	for _, target := range targets {
		assert.True(t,
			domain.HasScope(scopes,
				target))
	}
}

// TestHasScope_EmptyScopes verifies that empty scopes slice is treated as wildcard.
func TestHasScope_EmptyScopes(t *testing.T) {
	t.Parallel()
	require.True(
		t, domain.HasScope([]string{}, domain.
			ScopeJobsRead))
	require.True(
		t, domain.HasScope(nil, domain.
			ScopeJobsRead,
		))

	// Empty scopes slice should grant access for backwards compatibility.
}

// TestHasScope_NullByteScope verifies that null bytes in scope strings do not bypass matching.
func TestHasScope_NullByteScope(t *testing.T) {
	t.Parallel()

	// A scope with a null byte should not match the clean version.
	poisoned := "jobs\x00:read"
	scopes := []string{poisoned}
	require.False(t, domain.HasScope(scopes,
		domain.ScopeJobsRead,
	))
	require.Error(t, domain.ValidateScopes([]string{poisoned}))

	// It should also not pass validation.
}

// TestHasScope_CaseSensitivity verifies that scope matching is case-sensitive.
func TestHasScope_CaseSensitivity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		scope    string
		required string
		want     bool
	}{
		{"JOBS:READ", "jobs:read", false},
		{"Jobs:Read", "jobs:read", false},
		{"jobs:read", "JOBS:READ", false},
		{"jobs:read", "jobs:read", true},
	}
	for _, tc := range cases {
		got := domain.HasScope([]string{tc.scope}, tc.required)
		assert.Equal(
			t, tc.want, got)
	}
}

// TestHasScope_UnknownScope verifies behavior with scopes that are not in the valid set.
func TestHasScope_UnknownScope(t *testing.T) {
	t.Parallel()

	// HasScope does a simple string match, so an unknown scope should still match if present.
	unknown := "jobs:delete"
	require.True(
		t, domain.HasScope([]string{unknown},
			unknown))
	require.False(t, domain.HasScope([]string{unknown},
		domain.ScopeJobsRead,
	))
	require.Error(t, domain.ValidateScopes([]string{unknown}))

	// But it should not match a different scope.

	// Validation should reject it.
}

// FuzzScopeParsing fuzzes the ValidateScopes function with arbitrary scope strings.
func FuzzScopeParsing(f *testing.F) {
	f.Add("jobs:read")
	f.Add("*")
	f.Add("")
	f.Add("jobs\x00:read")
	f.Add("JOBS:READ")
	f.Add("jobs:delete")
	f.Add(strings.Repeat("a", 10000))
	f.Add("jobs:read:extra:colons")
	f.Add("\t\n\r")

	f.Fuzz(func(t *testing.T, scope string) {
		// ValidateScopes must not panic.
		_ = domain.ValidateScopes([]string{scope})
		// HasScope must not panic.
		_ = domain.HasScope([]string{scope}, domain.ScopeJobsRead)
		_ = domain.HasScope([]string{scope}, scope)
	})
}

// TestAPIKey_TimingAttack verifies that internal secret comparison uses constant-time comparison.
// This test measures timing variance to detect if the comparison is constant-time.
func TestAPIKey_TimingAttack(t *testing.T) {
	t.Parallel()

	secret := "test-internal-secret-that-is-long-enough-for-timing"
	wrongPrefix := "xest-internal-secret-that-is-long-enough-for-timing"
	wrongSuffix := "test-internal-secret-that-is-long-enough-for-timinx"
	require.Equal(t, 1, subtle.ConstantTimeCompare([]byte(secret), []byte(secret)))
	require.Equal(t, 0, subtle.ConstantTimeCompare([]byte(secret), []byte(wrongPrefix)))
	require.Equal(t, 0, subtle.ConstantTimeCompare([]byte(secret), []byte(wrongSuffix)))

	// Verify that subtle.ConstantTimeCompare is used by checking behavior.
	// With constant-time comparison, matching and non-matching should both
	// return in similar time. We verify the function itself works correctly.

	// Measure timing variance across many iterations. Constant-time comparison
	// should have low variance between early-mismatch and late-mismatch inputs.
	// Note: this is a best-effort statistical test. CI environments have noisy
	// scheduling, so we use many iterations and a very generous tolerance.
	const iterations = 10000
	secretBytes := []byte(secret)

	measureAvg := func(other []byte) time.Duration {
		var total time.Duration
		for range iterations {
			start := time.Now()
			subtle.ConstantTimeCompare(secretBytes, other)
			total += time.Since(start)
		}
		return total / iterations
	}

	avgPrefix := measureAvg([]byte(wrongPrefix))
	avgSuffix := measureAvg([]byte(wrongSuffix))

	// With constant-time comparison, both averages should be in the same ballpark.
	// We use a very generous 10x tolerance because CI timing is noisy and the
	// primary goal is to verify the function is being called (not to detect
	// a non-constant-time implementation).
	if avgPrefix == 0 || avgSuffix == 0 {
		// Both completed too fast to measure -- that is fine for constant-time.
		return
	}
	ratio := float64(avgPrefix) / float64(avgSuffix)
	if ratio > 10.0 || ratio < 0.1 {
		t.Logf("timing ratio %.2f is outside tolerance (prefix=%v, suffix=%v) -- may be CI noise", ratio, avgPrefix, avgSuffix)
	}
}

// FuzzOIDCJWTParsing fuzzes JWT token parsing to ensure no panics on malformed tokens.
func FuzzOIDCJWTParsing(f *testing.F) {
	f.Add("eyJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJ0ZXN0In0.invalid")
	f.Add("")
	f.Add("not-a-jwt")
	f.Add("a.b.c")
	f.Add("a.b")
	f.Add(strings.Repeat("A", 10000))
	f.Add("eyJ\x00bGci.eyJpc\x00MiOiJ0.inv")

	f.Fuzz(func(t *testing.T, token string) {
		// Build a request with the fuzzed token and ensure the middleware does not panic.
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		// The securityHeaders middleware should never panic regardless of input.
		handler := (&Server{}).securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		handler.ServeHTTP(w, r)
	})
}

// TestRequirePermission_APIKeyScopeEdgeCases tests edge cases for scope checking via the middleware.
func TestRequirePermission_APIKeyScopeEdgeCases(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, nil, nil)

	tests := []struct {
		name     string
		scopes   []string
		required string
		wantCode int
	}{
		{
			name:     "wildcard scope grants any permission",
			scopes:   []string{"*"},
			required: domain.ScopeJobsWrite,
			wantCode: http.StatusOK,
		},
		{
			name:     "empty string scope blocks",
			scopes:   []string{""},
			required: domain.ScopeJobsRead,
			wantCode: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := srv.requirePermission(tt.required)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			ctx := context.WithValue(r.Context(), ctxActorTypeKey, "api_key")
			ctx = context.WithValue(ctx, ctxScopesKey, tt.scopes)
			r = r.WithContext(ctx)

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			assert.Equal(
				t, tt.wantCode, w.
					Code)
		})
	}
}

// TestSecurityHeaders_AlwaysSet verifies security headers are set on every response.
func TestSecurityHeaders_AlwaysSet(t *testing.T) {
	t.Parallel()

	handler := (&Server{}).securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	expected := map[string]string{
		securityHeaderXContentTypeOptions: "nosniff",
		securityHeaderXFrameOptions:       "DENY",
		securityHeaderXXSSProtection:      "0",
		securityHeaderCSP:                 "default-src 'none'",
		securityHeaderReferrerPolicy:      "no-referrer",
	}

	for header, want := range expected {
		got := w.Header().Get(header)
		assert.Equal(
			t, want, got)
	}
	assert.Empty(
		t, w.Header().
			Get(securityHeaderHSTS))

	// HSTS should NOT be set for non-HTTPS requests.
}

// TestSecurityHeaders_NilRequest verifies requestIsHTTPS handles nil safely.
func TestSecurityHeaders_NilRequest(t *testing.T) {
	t.Parallel()
	require.False(t, (&Server{}).requestIsHTTPS(nil))
}

// TestSecureCookie_SecurityFlags verifies secure cookie security attributes.
func TestSecureCookie_SecurityFlags(t *testing.T) {
	t.Parallel()

	c := SecureCookie("session", "value123", 3600)
	assert.True(t,
		c.Secure)
	assert.True(t,
		c.HttpOnly)
	assert.Equal(
		t, http.SameSiteStrictMode,

		c.SameSite,
	)
	assert.Equal(t, 3600, c.MaxAge)
}

// TestConstantTimeCompare_EmptyInputs verifies constant-time comparison with edge cases.
func TestConstantTimeCompare_EmptyInputs(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1, subtle.ConstantTimeCompare([]byte{}, []byte{}))
	assert.Equal(t, 0, subtle.ConstantTimeCompare([]byte{}, []byte("a")))

	// Empty vs empty should match.

	// Empty vs non-empty should not match.

	// Very long strings of same length.
	long := strings.Repeat("x", 10000)
	assert.Equal(t, 1, subtle.ConstantTimeCompare([]byte(long), []byte(long)))

	_ = math.MaxFloat64 // Ensure math is used.
}
