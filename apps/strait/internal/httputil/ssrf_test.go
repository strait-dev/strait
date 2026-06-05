package httputil

import (
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLookupHost returns a mock DNS resolver that maps known test hostnames
// to specific IPs and returns an error for unknown hosts.
func mockLookupHost(host string) ([]string, error) {
	switch host {
	case "example.com", "hooks.slack.com":
		return []string{"93.184.216.34"}, nil
	case "internal.example.com":
		return []string{"10.0.0.5"}, nil
	case "sneaky.example.com":
		return []string{"93.184.216.34", "192.168.1.1"}, nil
	case "safe.example.com":
		return []string{"93.184.216.34"}, nil
	case "loopback.example.com":
		return []string{"127.0.0.1"}, nil
	case "ipv6private.example.com":
		return []string{"::1"}, nil
	default:
		return nil, fmt.Errorf("mock DNS: no such host %q", host)
	}
}

func TestValidateExternalURL(t *testing.T) {
	// Not parallel: modifies package-level lookupHost.
	origLookup := lookupHost
	t.Cleanup(func() { lookupHost = origLookup })
	lookupHost = mockLookupHost

	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		// Valid external URLs.
		{name: "valid https", url: "https://example.com/webhook", wantErr: false},
		{name: "valid http", url: "http://example.com/hook", wantErr: false},
		{name: "valid with port", url: "https://example.com:8080/hook", wantErr: false},
		{name: "valid with path and query", url: "https://hooks.slack.com/services/T00/B00/abc?foo=bar", wantErr: false},

		// Empty and invalid.
		{name: "empty string", url: "", wantErr: true, errMsg: "must not be empty"},
		{name: "whitespace only", url: "   ", wantErr: true, errMsg: "scheme must be http or https"},

		// Non-HTTP schemes.
		{name: "ftp scheme", url: "ftp://example.com/file", wantErr: true, errMsg: "scheme must be http or https"},
		{name: "file scheme", url: "file:///etc/passwd", wantErr: true, errMsg: "scheme must be http or https"},
		{name: "gopher scheme", url: "gopher://evil.com", wantErr: true, errMsg: "scheme must be http or https"},
		{name: "javascript scheme", url: "javascript:alert(1)", wantErr: true, errMsg: "scheme must be http or https"},
		{name: "no scheme", url: "example.com/hook", wantErr: true, errMsg: "scheme must be http or https"},

		// Loopback addresses.
		{name: "loopback 127.0.0.1", url: "http://127.0.0.1/hook", wantErr: true, errMsg: "private or internal"},
		{name: "loopback 127.0.0.2", url: "http://127.0.0.2/hook", wantErr: true, errMsg: "private or internal"},
		{name: "loopback 127.255.255.255", url: "http://127.255.255.255/hook", wantErr: true, errMsg: "private or internal"},
		{name: "ipv6 loopback", url: "http://[::1]/hook", wantErr: true, errMsg: "private or internal"},

		// RFC 1918 private ranges.
		{name: "10.0.0.1", url: "http://10.0.0.1/hook", wantErr: true, errMsg: "private or internal"},
		{name: "10.255.255.255", url: "http://10.255.255.255/hook", wantErr: true, errMsg: "private or internal"},
		{name: "172.16.0.1", url: "http://172.16.0.1/hook", wantErr: true, errMsg: "private or internal"},
		{name: "172.31.255.255", url: "http://172.31.255.255/hook", wantErr: true, errMsg: "private or internal"},
		{name: "192.168.0.1", url: "http://192.168.0.1/hook", wantErr: true, errMsg: "private or internal"},
		{name: "192.168.255.255", url: "http://192.168.255.255/hook", wantErr: true, errMsg: "private or internal"},

		// Link-local.
		{name: "link-local 169.254.0.1", url: "http://169.254.0.1/hook", wantErr: true, errMsg: "private or internal"},
		{name: "link-local 169.254.169.254", url: "http://169.254.169.254/latest/meta-data", wantErr: true, errMsg: "private or internal"},
		{name: "ipv6 link-local", url: "http://[fe80::1]/hook", wantErr: true, errMsg: "private or internal"},

		// CGNAT.
		{name: "cgnat 100.64.0.1", url: "http://100.64.0.1/hook", wantErr: true, errMsg: "private or internal"},
		{name: "cgnat 100.127.255.255", url: "http://100.127.255.255/hook", wantErr: true, errMsg: "private or internal"},

		// IPv6 unique local.
		{name: "ipv6 ula fc00::", url: "http://[fc00::1]/hook", wantErr: true, errMsg: "private or internal"},
		{name: "ipv6 ula fd00::", url: "http://[fd12::1]/hook", wantErr: true, errMsg: "private or internal"},

		// Unspecified addresses.
		{name: "unspecified 0.0.0.0", url: "http://0.0.0.0/hook", wantErr: true, errMsg: "private or internal"},

		// Blocked hostnames.
		{name: "localhost", url: "http://localhost/hook", wantErr: true, errMsg: "internal host"},
		{name: "localhost uppercase", url: "http://LOCALHOST/hook", wantErr: true, errMsg: "internal host"},
		{name: "metadata.google.internal", url: "http://metadata.google.internal/computeMetadata/v1", wantErr: true, errMsg: "internal host"},

		// Edge cases: non-private IP literals.
		{name: "172.15.255.255 is not private", url: "http://172.15.255.255/hook", wantErr: false},
		{name: "172.32.0.1 is not private", url: "http://172.32.0.1/hook", wantErr: false},
		{name: "100.63.255.255 not CGNAT", url: "http://100.63.255.255/hook", wantErr: false},
		{name: "100.128.0.0 not CGNAT", url: "http://100.128.0.0/hook", wantErr: false},
		{name: "public ipv4", url: "http://8.8.8.8/hook", wantErr: false},
		{name: "public ipv6", url: "http://[2607:f8b0:4004:800::200e]/hook", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertExternalURLValidation(t, tt.url, tt.wantErr, tt.errMsg)
		})
	}
}

func TestRedactURLForLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "drops path query and fragment",
			in:   "https://hooks.example.com/services/T00/B00/token?secret=value#frag",
			want: "https://hooks.example.com",
		},
		{
			name: "drops userinfo",
			in:   "https://user:pass@example.com:8443/path?token=abc",
			want: "https://example.com:8443",
		},
		{
			name: "invalid",
			in:   "/relative/path?token=abc",
			want: "[invalid-url]",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RedactURLForLog(tt.in))
		})
	}
}

func TestSanitizeHTTPClientError_RemovesURLSecrets(t *testing.T) {
	t.Parallel()

	err := &url.Error{
		Op:  "Post",
		URL: "https://user:pass@hooks.example.com/private/path?token=secret#frag",
		Err: fmt.Errorf("dial tcp: lookup failed"),
	}

	got := SanitizeHTTPClientError(err)
	assert.NotContains(t, got, "token=secret")
	assert.NotContains(t, got, "user:pass")
	assert.NotContains(t, got, "/private/path")
	assert.NotContains(t, got, "hooks.example.com")
	assert.Contains(t, got, "Post")
	assert.Contains(t, got, "request failed")
	assert.NotContains(t, got, "lookup failed")
}

func TestSanitizeHTTPClientError_RemovesNestedURLSecrets(t *testing.T) {
	t.Parallel()

	err := &url.Error{
		Op:  "Post",
		URL: "https://safe.example.com/hook?outer=secret",
		Err: &url.Error{
			Op:  "Get",
			URL: "https://user:pass@internal.example.com/redirect?inner=secret",
			Err: fmt.Errorf("redirect blocked"),
		},
	}

	got := SanitizeHTTPClientError(err)
	assert.NotContains(t, got, "outer=secret")
	assert.NotContains(t, got, "inner=secret")
	assert.NotContains(t, got, "user:pass")
	assert.NotContains(t, got, "internal.example.com")
	assert.Contains(t, got, "Get")
	assert.Contains(t, got, "request failed")
	assert.NotContains(t, got, "redirect blocked")
}

func TestValidateExternalURL_DNSResolvesToPrivate(t *testing.T) {
	// Not parallel: modifies package-level lookupHost.
	origLookup := lookupHost
	t.Cleanup(func() { lookupHost = origLookup })
	lookupHost = mockLookupHost

	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{name: "resolves to private 10.x", url: "https://internal.example.com/hook", wantErr: true, errMsg: "resolves to private"},
		{name: "mixed public and private", url: "https://sneaky.example.com/hook", wantErr: true, errMsg: "resolves to private"},
		{name: "resolves to public only", url: "https://safe.example.com/hook", wantErr: false},
		{name: "resolves to loopback", url: "https://loopback.example.com/hook", wantErr: true, errMsg: "resolves to private"},
		{name: "resolves to ipv6 loopback", url: "https://ipv6private.example.com/hook", wantErr: true, errMsg: "resolves to private"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertExternalURLValidation(t, tt.url, tt.wantErr, tt.errMsg)
		})
	}
}

func TestValidateExternalURL_DNSLookupFailure(t *testing.T) {
	// Not parallel: modifies package-level lookupHost.
	origLookup := lookupHost
	t.Cleanup(func() { lookupHost = origLookup })
	lookupHost = mockLookupHost

	// "unknown.example.com" is not in the mock, so lookup returns an error.
	// The SSRF check must reject the URL to prevent DNS rebinding attacks.
	err := ValidateExternalURL("https://unknown.example.com/hook")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DNS lookup failed")
	assert.NotContains(t, err.Error(), "unknown.example.com")
}

func TestSafeDialContext_BlocksPrivateResolvedAddresses(t *testing.T) {
	// Not parallel: modifies package-level lookupHost.
	origLookup := lookupHost
	t.Cleanup(func() { lookupHost = origLookup })
	lookupHost = mockLookupHost

	dial := SafeDialContext(false)
	_, err := dial(t.Context(), "tcp", "internal.example.com:80")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolves to private")
	assert.NotContains(t, err.Error(), "internal.example.com")
	assert.NotContains(t, err.Error(), "10.0.0.5")
}

func TestSafeDialContext_BlocksPrivateLiteralAddresses(t *testing.T) {
	t.Parallel()

	dial := SafeDialContext(false)
	_, err := dial(t.Context(), "tcp", "169.254.169.254:80")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked private")
}

func TestIsPrivateIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ip      string
		private bool
	}{
		// Private.
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"100.64.0.1", true},
		{"0.0.0.0", true},
		{"::1", true},
		{"fe80::1", true},
		{"fc00::1", true},
		{"fd00::1", true},

		// Public.
		{"8.8.8.8", false},
		{"93.184.216.34", false},
		{"172.32.0.1", false},
		{"100.128.0.1", false},
		{"2607:f8b0:4004:800::200e", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "failed to parse IP %q", tt.ip)
			got := isPrivateIP(ip)
			assert.Equal(t, tt.private, got)
		})
	}
}

func TestIsPrivateIP_IPv4CompatibleIPv6(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ip      net.IP
		private bool
	}{
		{
			name:    "only ip[12] non-zero (127.0.0.0 loopback range)",
			ip:      net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 127, 0, 0, 0},
			private: true,
		},
		{
			name:    "only ip[13] non-zero (0.10.0.0 unspecified range)",
			ip:      net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0},
			private: true,
		},
		{
			name:    "only ip[14] non-zero (0.0.168.0 unspecified range)",
			ip:      net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 168, 0},
			private: true,
		},
		{
			name:    "only ip[15] non-zero (0.0.0.2 in 0/8 range)",
			ip:      net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
			private: true,
		},
		{
			name:    "all last 4 octets zero (unspecified address)",
			ip:      net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			private: true,
		},
		{
			name:    "IPv4-compatible public IP (::8.8.8.8)",
			ip:      net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 8, 8, 8},
			private: false,
		},
		{
			name:    "all four octets non-zero private (::10.20.30.40)",
			ip:      net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 10, 20, 30, 40},
			private: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isPrivateIP(tt.ip)
			assert.Equal(t, tt.private, got)
		})
	}
}

func TestValidateExternalURL_OctalIPBypass(t *testing.T) {
	// Not parallel: modifies package-level lookupHost.
	origLookup := lookupHost
	t.Cleanup(func() { lookupHost = origLookup })
	lookupHost = func(host string) ([]string, error) {
		return []string{"93.184.216.34"}, nil
	}

	octalPayloads := []string{
		"http://0177.0.0.1/",          // octal 127.0.0.1
		"http://0177.0.0.01/",         // octal variant
		"http://0177.00.00.01/",       // octal with extra zeros
		"http://0300.0250.0.1/",       // octal 192.168.0.1
		"http://012.0.0.1/",           // octal 10.0.0.1
		"http://0x7f.0.0.1/",          // hex-per-octet 127.0.0.1
		"http://0x7f.0x0.0x0.0x1/",    // full hex-per-octet
		"http://0xA9.0xFE.0xA9.0xFE/", // hex 169.254.169.254
	}

	for _, payload := range octalPayloads {
		t.Run(payload, func(t *testing.T) {
			err := ValidateExternalURL(payload)
			assert.Error(t, err)
		})
	}

	// Verify normal IPs still work
	validURLs := []string{
		"https://93.184.216.34/",
		"https://example.com/",
		"https://1.2.3.4/webhook",
	}
	for _, u := range validURLs {
		t.Run("valid:"+u, func(t *testing.T) {
			assert.NoError(t, ValidateExternalURL(u))
		})
	}
}

func TestLooksLikeNonStandardIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		host     string
		expected bool
	}{
		{"0177.0.0.1", true},
		{"0177.0.0.01", true},
		{"0300.0250.0.1", true},
		{"012.0.0.1", true},
		{"0x7f.0.0.1", true},
		{"0x7f.0x0.0x0.0x1", true},
		{"127.0.0.1", false},   // standard decimal, no leading zero
		{"10.0.0.1", false},    // standard decimal
		{"example.com", false}, // hostname
		{"192.168.1.1", false}, // standard decimal
		{"1.2.3", false},       // not 4 parts
		{"", false},            // empty

		// Boundary characters for ssrf.go:109 mutant killing.
		{"09.0.0.1", true},     // '9' at boundary of 0-9 range
		{"0a.0.0.1", true},     // 'a' at boundary of a-f range
		{"0f.0.0.1", true},     // 'f' at boundary of a-f range
		{"0A.0.0.1", true},     // 'A' at boundary of A-F range
		{"0F.0.0.1", true},     // 'F' at boundary of A-F range
		{"01x.02.03.04", true}, // 'x' in non-prefix position
		{"01X.02.03.04", true}, // 'X' in non-prefix position
		{"0:.0.0.1", false},    // ':' (ASCII 58, just past '9')
		{"0`.0.0.1", false},    // '`' (ASCII 96, just before 'a')
		{"0g.0.0.1", false},    // 'g' (just past 'f')
		{"0@.0.0.1", false},    // '@' (ASCII 64, just before 'A')
		{"0G.0.0.1", false},    // 'G' (just past 'F')
		{"0x.gg.0.1", false},   // hex prefix with len=2 (boundary for ssrf.go:129)
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			t.Parallel()
			got := looksLikeNonStandardIP(tt.host)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func assertExternalURLValidation(t *testing.T, rawURL string, wantErr bool, errMsg string) {
	t.Helper()

	err := ValidateExternalURL(rawURL)
	if wantErr {
		require.Error(t, err)
		if errMsg != "" {
			assert.Contains(t, err.Error(), errMsg)
		}
		return
	}
	assert.NoError(t, err)
}
