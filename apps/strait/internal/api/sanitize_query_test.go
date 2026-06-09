package api

import (
	"fmt"
	"strings"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

// TestSanitizeQueryRedactsExpandedKeys pins the broader
// keyword-substring contract: any param name containing a known
// credential keyword (case-insensitive) must have its value redacted.
// The previous implementation only matched three exact keys
// ("api_key", "token", "secret"), so common shapes like "access_token",
// "client_secret", "apikey", "x-api-key", "signature", "jwt", and
// "authorization" leaked the value into structured logs.
func TestSanitizeQueryRedactsExpandedKeys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		key   string
		value string
	}{
		{"password", "password", "hunter2"},
		{"access_token", "access_token", "secret-access-token-value"},
		{"refresh_token", "refresh_token", "secret-refresh-token-value"},
		{"jwt", "jwt", "eyJhbGciOiJIUzI1NiJ9"},
		{"auth_token", "auth_token", "secret-auth-token"},
		{"authorization", "authorization", "Bearer secret-bearer"},
		{"client_secret", "client_secret", "shh"},
		{"apikey", "apikey", "sk-live-leak"},
		{"x_api_key", "x-api-key", "sk-live-leak-2"},
		{"signature", "signature", "deadbeef"},
		{"sig", "sig", "deadbeef2"},
		{"credential", "credential", "creds123"},
		{"credentials", "credentials", "creds456"},
		{"private_key", "private_key", "BEGIN RSA"},
		{"mixed_case_AccessToken", "AccessToken", "leaky"},
		{"mixed_case_API_KEY", "API_KEY", "leaky2"},
		{"bearer", "bearer", "bearer-leak"},
		{"bearer_token", "bearer_token", "bearer-leak-2"},
		{"hmac", "hmac", "hmac-leak"},
		{"hmac_signature", "hmac_signature", "hmac-leak-2"},
		{"nonce", "nonce", "nonce-leak"},
		{"x_nonce", "X-Nonce", "nonce-leak-2"},
		{"csrf", "csrf", "csrf-token-value"},
		{"x_csrf_token", "X-CSRF-Token", "csrf-leak-2"},
		{"oauth_state", "state", "oauth-state-nonce"},
		{"code_verifier", "code_verifier", "pkce-verifier-leak"},
		{"code_challenge", "code_challenge", "pkce-challenge-leak"},
		{"session", "session", "session-id-leak"},
		{"session_id", "session_id", "session-id-leak-2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := sanitizeQuery(map[string][]string{tc.key: {tc.value}})
			require.NotContains(t, out, tc.value)
			require.Contains(t, strings.ToLower(out), strings.ToLower(tc.key))
			require.Contains(t, out, "[REDACTED]")
		})
	}
}

func TestShouldLogRequest_AlwaysLogsErrorsAndSamplesSuccess(t *testing.T) {
	t.Parallel()
	require.True(t, shouldLogRequest(500, ""))
	require.True(t, shouldLogRequest(404, ""))
	require.False(t, shouldLogRequest(200, ""))

	var sampled string
	for i := range successRequestLogSampleModulo * 4 {
		candidate := fmt.Sprintf("req-%d", i)
		if shouldLogRequest(200, candidate) {
			sampled = candidate
			break
		}
	}
	require.NotEmpty(t,
		sampled,
	)
	require.True(t, shouldLogRequest(200, sampled))
}

func BenchmarkShouldLogRequestSuccess(b *testing.B) {
	requestID := "req-benchmark"
	b.ReportAllocs()
	for b.Loop() {
		_ = shouldLogRequest(200, requestID)
	}
}

func BenchmarkSanitizeQuery(b *testing.B) {
	benchmarks := []struct {
		name   string
		params map[string][]string
	}{
		{name: "empty", params: map[string][]string{}},
		{name: "safe", params: map[string][]string{
			"cursor": {"abc123"},
			"limit":  {"100"},
			"status": {"active"},
		}},
		{name: "sensitive_lower", params: map[string][]string{
			"access_token": {"secret-token"},
			"cursor":       {"abc123"},
		}},
		{name: "sensitive_mixed", params: map[string][]string{
			"X-CSRF-Token": {"csrf-token-value"},
			"page":         {"3"},
		}},
		{name: "multi_values", params: map[string][]string{
			"sort":      {"created_at", "updated_at"},
			"signature": {"abc", "def"},
		}},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				out := sanitizeQuery(bm.params)
				if len(bm.params) > 0 && out == "" {
					b.Fatal("sanitizeQuery returned empty output")
				}
			}
		})
	}
}

// TestSanitizeQueryPreservesSafeParams regresses the happy
// path: benign pagination/sort params keep their values.
func TestSanitizeQueryPreservesSafeParams(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key   string
		value string
	}{
		{"page", "3"},
		{"limit", "100"},
		{"sort", "created_at"},
		{"cursor", "abc123"},
		{"status", "active"},
		{"q", "search-term"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			out := sanitizeQuery(map[string][]string{tc.key: {tc.value}})
			require.Contains(t, out, tc.value)
			require.NotContains(t, out, "[REDACTED]")
		})
	}
}

// FuzzSanitizeQueryNeverEchoesSensitiveSubstring exercises the
// substring contract under random inputs: when the param name contains
// any redaction keyword (case-insensitive), the value must never appear
// in the sanitized output.
func FuzzSanitizeQueryNeverEchoesSensitiveSubstring(f *testing.F) {
	keywords := []string{"secret", "password", "token", "key", "auth", "credential", "sig", "jwt", "bearer", "hmac", "nonce", "csrf", "state", "code_verifier", "code_challenge", "session"}
	seeds := []struct {
		key   string
		value string
	}{
		{"my_secret_token", "VALUE-A"},
		{"X-CSRF-TOKEN", "VALUE-B"},
		{"prefix_password_suffix", "VALUE-C"},
		{"WeirdKey", "VALUE-D"},
		{"some_credential_thing", "VALUE-E"},
		{"oauth_state", "VALUE-F"},
		{"code_verifier", "VALUE-G"},
		{"session_cookie", "VALUE-H"},
	}
	for _, s := range seeds {
		f.Add(s.key, s.value)
	}

	f.Fuzz(func(t *testing.T, key, value string) {
		if value == "" {
			return
		}
		// Only assert the contract when the key actually contains a
		// keyword. Random keys without keywords are not in scope here.
		hasKeyword := false
		lower := strings.ToLower(key)
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				hasKeyword = true
				break
			}
		}
		if !hasKeyword {
			return
		}
		out := sanitizeQuery(map[string][]string{key: {value}})
		require.NotContains(t, out, value)
	})
}

// TestSanitizeQueryPropertyRedactsKeywordKeys is a redundant
// property check via testing/quick.
func TestSanitizeQueryPropertyRedactsKeywordKeys(t *testing.T) {
	t.Parallel()

	// A value made of bytes that are unlikely to appear in the literal
	// "[REDACTED]" marker, so accidental substring matches are noise-free.
	const probe = "ZZprobe-value-ZZ"

	prop := func(prefix, suffix string) bool {
		key := prefix + "token" + suffix
		out := sanitizeQuery(map[string][]string{key: {probe}})
		return !strings.Contains(out, probe)
	}
	require.NoError(t, quick.
		Check(
			prop, &quick.Config{
				MaxCount: 200}))
}
