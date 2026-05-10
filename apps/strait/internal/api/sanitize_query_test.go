package api

import (
	"strings"
	"testing"
	"testing/quick"
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := sanitizeQuery(map[string][]string{tc.key: {tc.value}})
			if strings.Contains(out, tc.value) {
				t.Fatalf("sanitizeQuery leaked value %q in output %q", tc.value, out)
			}
			if !strings.Contains(strings.ToLower(out), strings.ToLower(tc.key)) {
				t.Fatalf("sanitizeQuery dropped key %q from output %q", tc.key, out)
			}
			if !strings.Contains(out, "[REDACTED]") {
				t.Fatalf("sanitizeQuery output missing redaction marker: %q", out)
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
			if !strings.Contains(out, tc.value) {
				t.Fatalf("sanitizeQuery dropped benign value %q from output %q", tc.value, out)
			}
			if strings.Contains(out, "[REDACTED]") {
				t.Fatalf("sanitizeQuery wrongly redacted benign param %q: %q", tc.key, out)
			}
		})
	}
}

// FuzzSanitizeQueryNeverEchoesSensitiveSubstring exercises the
// substring contract under random inputs: when the param name contains
// any redaction keyword (case-insensitive), the value must never appear
// in the sanitized output.
func FuzzSanitizeQueryNeverEchoesSensitiveSubstring(f *testing.F) {
	keywords := []string{"secret", "password", "token", "key", "auth", "credential", "sig", "jwt"}
	seeds := []struct {
		key   string
		value string
	}{
		{"my_secret_token", "VALUE-A"},
		{"X-CSRF-TOKEN", "VALUE-B"},
		{"prefix_password_suffix", "VALUE-C"},
		{"WeirdKey", "VALUE-D"},
		{"some_credential_thing", "VALUE-E"},
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
		if strings.Contains(out, value) {
			t.Fatalf("value %q leaked through sanitizeQuery for key %q: %q", value, key, out)
		}
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
	if err := quick.Check(prop, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}
