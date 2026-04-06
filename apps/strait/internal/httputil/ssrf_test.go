package httputil

import (
	"fmt"
	"net"
	"strings"
	"testing"
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
	t.Parallel()

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
			err := ValidateExternalURL(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("error %q does not contain %q", err, tt.errMsg)
				}
			} else if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

func TestValidateExternalURL_DNSResolvesToPrivate(t *testing.T) {
	t.Parallel()

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
			err := ValidateExternalURL(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("error %q does not contain %q", err, tt.errMsg)
				}
			} else if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

func TestValidateExternalURL_DNSLookupFailure(t *testing.T) {
	t.Parallel()

	origLookup := lookupHost
	t.Cleanup(func() { lookupHost = origLookup })
	lookupHost = mockLookupHost

	// "unknown.example.com" is not in the mock, so lookup returns an error.
	// The SSRF check must reject the URL to prevent DNS rebinding attacks.
	err := ValidateExternalURL("https://unknown.example.com/hook")
	if err == nil {
		t.Fatal("expected error for DNS lookup failure, got nil")
	}
	if !strings.Contains(err.Error(), "DNS lookup failed") {
		t.Fatalf("error %q does not mention DNS lookup failure", err)
	}
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
			if ip == nil {
				t.Fatalf("failed to parse IP %q", tt.ip)
			}
			got := isPrivateIP(ip)
			if got != tt.private {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}
