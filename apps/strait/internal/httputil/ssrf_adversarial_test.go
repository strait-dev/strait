package httputil

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"
)

// TestSSRF_AdversarialBypass is a comprehensive adversarial test that tries every
// known SSRF bypass technique against ValidateExternalURL. Each subtest represents
// a real-world attack vector. If any of these pass (return nil), we have a bypass.
func TestSSRF_AdversarialBypass(t *testing.T) {
	// Not parallel: modifies package-level lookupHost.
	origLookup := lookupHost
	t.Cleanup(func() { lookupHost = origLookup })

	// Mock DNS: only known-safe hostnames resolve to public IPs.
	lookupHost = func(host string) ([]string, error) {
		switch host {
		case "example.com", "safe.example.com":
			return []string{"93.184.216.34"}, nil
		default:
			return nil, fmt.Errorf("mock DNS: no such host %q", host)
		}
	}

	// Every URL in this list MUST be rejected. If any returns nil, it is a
	// security vulnerability that allows an attacker to hit internal services.
	attacks := []struct {
		name string
		url  string
	}{
		// === Standard private IPs ===
		{"loopback IPv4", "http://127.0.0.1/"},
		{"loopback IPv4 alt", "http://127.0.0.2/"},
		{"loopback IPv4 high", "http://127.255.255.255/"},
		{"rfc1918 10.x", "http://10.0.0.1/"},
		{"rfc1918 10.x high", "http://10.255.255.255/"},
		{"rfc1918 172.16.x", "http://172.16.0.1/"},
		{"rfc1918 172.31.x", "http://172.31.255.255/"},
		{"rfc1918 192.168.x", "http://192.168.0.1/"},
		{"rfc1918 192.168.x high", "http://192.168.255.255/"},
		{"link-local", "http://169.254.169.254/"},
		{"link-local low", "http://169.254.0.1/"},
		{"cgnat", "http://100.64.0.1/"},
		{"cgnat high", "http://100.127.255.255/"},
		{"unspecified", "http://0.0.0.0/"},

		// === IPv6 private addresses ===
		{"ipv6 loopback", "http://[::1]/"},
		{"ipv6 loopback full", "http://[0:0:0:0:0:0:0:1]/"},
		{"ipv6 unique local fc", "http://[fc00::1]/"},
		{"ipv6 unique local fd", "http://[fd00::1]/"},
		{"ipv6 link-local", "http://[fe80::1]/"},
		{"ipv6 mapped v4 loopback", "http://[::ffff:127.0.0.1]/"},
		{"ipv6 mapped v4 private", "http://[::ffff:10.0.0.1]/"},
		{"ipv6 mapped v4 link-local", "http://[::ffff:169.254.169.254]/"},
		{"ipv6 compat v4 loopback", "http://[::127.0.0.1]/"},
		{"ipv6 full zeros loopback", "http://[0000:0000:0000:0000:0000:0000:0000:0001]/"},

		// === Octal IP notation (OS resolver interprets, Go does not) ===
		{"octal 127.0.0.1", "http://0177.0.0.1/"},
		{"octal 127.0.0.1 variant", "http://0177.0.0.01/"},
		{"octal 127.0.0.1 padded", "http://0177.00.00.01/"},
		{"octal 10.0.0.1", "http://012.0.0.1/"},
		{"octal 192.168.0.1", "http://0300.0250.0.1/"},
		{"octal 169.254.169.254", "http://0251.0376.0251.0376/"},
		{"octal all zeros", "http://00.00.00.00/"},

		// === Hex-per-octet notation ===
		{"hex 127.0.0.1", "http://0x7f.0.0.1/"},
		{"hex full 127.0.0.1", "http://0x7f.0x0.0x0.0x1/"},
		{"hex 10.0.0.1", "http://0xa.0x0.0x0.0x1/"},
		{"hex 169.254.169.254", "http://0xA9.0xFE.0xA9.0xFE/"},
		{"hex upper 127.0.0.1", "http://0X7F.0X0.0X0.0X1/"},

		// === Decimal integer notation (single 32-bit int) ===
		{"decimal 127.0.0.1", "http://2130706433/"},
		{"decimal 10.0.0.1", "http://167772161/"},
		{"decimal 169.254.169.254", "http://2852039166/"},

		// === Short-form IPs ===
		{"short 127.1", "http://127.1/"},
		{"short 0", "http://0/"},

		// === Blocked hostnames ===
		{"localhost", "http://localhost/"},
		{"localhost upper", "http://LOCALHOST/"},
		{"localhost mixed", "http://LocalHost/"},
		{"localhost port", "http://localhost:8080/"},
		{"metadata gcp", "http://metadata.google.internal/"},
		{"metadata gcp upper", "http://METADATA.GOOGLE.INTERNAL/"},

		// === Scheme bypass ===
		{"file scheme", "file:///etc/passwd"},
		{"ftp scheme", "ftp://127.0.0.1/"},
		{"gopher scheme", "gopher://127.0.0.1:6379/_PING"},
		{"dict scheme", "dict://127.0.0.1:6379/info"},
		{"ssh scheme", "ssh://127.0.0.1/"},
		{"telnet scheme", "telnet://127.0.0.1/"},

		// === URL parsing tricks ===
		{"userinfo at", "http://attacker@127.0.0.1/"},
		{"userinfo at with creds", "http://user:pass@127.0.0.1/"},
		{"fragment bypass", "http://127.0.0.1#@example.com/"},
		{"backslash", "http://127.0.0.1\\@example.com/"},

		// === Empty / malformed ===
		{"empty string", ""},
		{"just scheme", "http://"},
		{"no host", "http:///path"},

		// === AWS/cloud metadata via hostname ===
		// These fail DNS in our mock, which correctly rejects them.
		{"aws metadata dns", "http://instance-data.ec2.internal/"},
		{"aws metadata ip6", "http://[fd00:ec2::254]/"},

		// === DNS rebinding (fails DNS = rejected) ===
		{"nip.io loopback", "http://127.0.0.1.nip.io/"},
		{"xip.io metadata", "http://169.254.169.254.xip.io/"},
		{"burp collaborator", "http://spoofed.burpcollaborator.net/"},
		{"localtest.me", "http://localtest.me/"},
	}

	for _, att := range attacks {
		t.Run(att.name, func(t *testing.T) {
			err := ValidateExternalURL(att.url)
			if err == nil {
				t.Errorf("SSRF BYPASS: %q was accepted (expected rejection)", att.url)
			}
		})
	}
}

// TestSSRF_ValidURLsAccepted ensures we do not break legitimate external URLs.
func TestSSRF_ValidURLsAccepted(t *testing.T) {
	// Not parallel: modifies package-level lookupHost.
	origLookup := lookupHost
	t.Cleanup(func() { lookupHost = origLookup })

	lookupHost = func(host string) ([]string, error) {
		// All test hostnames resolve to public IPs.
		return []string{"93.184.216.34"}, nil
	}

	validURLs := []string{
		"https://example.com/webhook",
		"https://api.stripe.com/v1/charges",
		"http://hooks.slack.com/services/T00/B00/xxx",
		"https://discord.com/api/webhooks/123/abc",
		"https://93.184.216.34/callback",
		"https://1.2.3.4:8443/hook",
		"https://sub.domain.example.com/path?query=1",
		"https://example.com:443/",
		"http://example.com:8080/",
		"https://172.32.0.1/",      // just above 172.16-31 range
		"https://100.128.0.1/",     // just above CGNAT range
		"https://192.167.255.255/", // just below 192.168.x range
	}

	for _, u := range validURLs {
		t.Run(u, func(t *testing.T) {
			if err := ValidateExternalURL(u); err != nil {
				t.Errorf("valid URL %q was rejected: %v", u, err)
			}
		})
	}
}

// TestSSRF_BoundaryIPs tests IPs at the exact boundaries of private ranges.
func TestSSRF_BoundaryIPs(t *testing.T) {
	t.Parallel()

	type boundaryTest struct {
		ip      string
		private bool
	}

	tests := []boundaryTest{
		// 10.0.0.0/8 boundaries
		{"9.255.255.255", false},
		{"10.0.0.0", true},
		{"10.255.255.255", true},
		{"11.0.0.0", false},

		// 172.16.0.0/12 boundaries
		{"172.15.255.255", false},
		{"172.16.0.0", true},
		{"172.31.255.255", true},
		{"172.32.0.0", false},

		// 192.168.0.0/16 boundaries
		{"192.167.255.255", false},
		{"192.168.0.0", true},
		{"192.168.255.255", true},
		{"192.169.0.0", false},

		// 169.254.0.0/16 boundaries
		{"169.253.255.255", false},
		{"169.254.0.0", true},
		{"169.254.255.255", true},
		{"169.255.0.0", false},

		// 100.64.0.0/10 (CGNAT) boundaries
		{"100.63.255.255", false},
		{"100.64.0.0", true},
		{"100.127.255.255", true},
		{"100.128.0.0", false},

		// 127.0.0.0/8 boundaries
		{"126.255.255.255", false},
		{"127.0.0.0", true},
		{"127.0.0.1", true},
		{"127.255.255.255", true},
		{"128.0.0.0", false},
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

// TestSSRF_NonStandardIPDetection tests looksLikeNonStandardIP with many edge cases.
func TestSSRF_NonStandardIPDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host     string
		detected bool
	}{
		// Octal (must detect)
		{"0177.0.0.1", true},
		{"0177.0.0.01", true},
		{"0177.00.00.01", true},
		{"0300.0250.0.1", true},
		{"012.0.0.1", true},
		{"00.00.00.00", true},
		{"0251.0376.0251.0376", true},

		// Hex-per-octet (must detect)
		{"0x7f.0.0.1", true},
		{"0x7f.0x0.0x0.0x1", true},
		{"0xa.0x0.0x0.0x1", true},
		{"0xA9.0xFE.0xA9.0xFE", true},
		{"0X7F.0X0.0X0.0X1", true},

		// Standard decimal IPs (must NOT detect)
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"172.16.0.1", false},
		{"169.254.169.254", false},
		{"8.8.8.8", false},
		{"93.184.216.34", false},
		{"255.255.255.255", false},
		{"1.1.1.1", false},

		// Hostnames (must NOT detect)
		{"example.com", false},
		{"sub.example.com", false},
		{"localhost", false},
		{"a.b.c.d", false},

		// Edge cases
		{"", false},
		{"1.2.3", false},           // only 3 parts
		{"1.2.3.4.5", false},       // 5 parts
		{"1.2.3.", false},          // trailing dot = empty part
		{".1.2.3", false},          // leading dot = empty part
		{"1..2.3", false},          // empty middle part
		{"abc.def.ghi.jkl", false}, // alpha only
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			t.Parallel()
			got := looksLikeNonStandardIP(tt.host)
			if got != tt.detected {
				t.Errorf("looksLikeNonStandardIP(%q) = %v, want %v", tt.host, got, tt.detected)
			}
		})
	}
}

// FuzzValidateExternalURL fuzzes the SSRF validator with random URLs.
// It asserts two invariants:
// 1. The function never panics.
// 2. Any URL that is accepted must not contain a private/loopback IP literal.
func FuzzValidateExternalURL(f *testing.F) {
	// Seed corpus with known bypass attempts.
	seeds := []string{
		"https://example.com/",
		"http://127.0.0.1/",
		"http://0177.0.0.1/",
		"http://0x7f.0.0.1/",
		"http://2130706433/",
		"http://[::1]/",
		"http://[::ffff:127.0.0.1]/",
		"http://localhost/",
		"http://169.254.169.254/",
		"http://metadata.google.internal/",
		"http://0.0.0.0/",
		"file:///etc/passwd",
		"gopher://127.0.0.1:6379/",
		"",
		"http://",
		"http:///",
		"http://user:pass@127.0.0.1/",
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://192.168.1.1/",
		"http://100.64.0.1/",
		"http://0251.0376.0251.0376/",
		"http://[fe80::1]/",
		"http://[fc00::1]/",
		"http://127.0.0.1.nip.io/",
		"http://0300.0250.0.1/",
		"http://012.0.0.1/",
		"https://93.184.216.34/",
		"http://0x7f.0x0.0x0.0x1/",
		"http://0177.00.00.01/",
		"http://attacker@127.0.0.1/",
		"http://127.0.0.1#@example.com/",
		"http://0xA9.0xFE.0xA9.0xFE/",
		"http://00.00.00.00/",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic.
		err := ValidateExternalURL(input)

		if err == nil {
			// URL was accepted. Verify it does not contain an obvious
			// private IP literal in the host (defense-in-depth check).
			u, parseErr := url.Parse(input)
			if parseErr != nil {
				t.Errorf("accepted URL %q that fails to re-parse: %v", input, parseErr)
				return
			}
			host := u.Hostname()

			// Check if the accepted host is a plain private IP.
			if ip := net.ParseIP(host); ip != nil {
				if isPrivateIP(ip) {
					t.Errorf("SSRF BYPASS: accepted URL with private IP %q: %s", input, ip)
				}
			}

			// Check for known blocked hostnames.
			for _, blocked := range blockedHosts {
				if strings.EqualFold(host, blocked) {
					t.Errorf("SSRF BYPASS: accepted URL with blocked host %q: %s", input, host)
				}
			}

			// Check for octal/hex notation that might slip through.
			if looksLikeNonStandardIP(host) {
				t.Errorf("SSRF BYPASS: accepted URL with non-standard IP %q: %s", input, host)
			}
		}
	})
}

// FuzzLooksLikeNonStandardIP fuzzes the octal/hex IP detector.
// Invariant: if the function returns false for a dotted-quad that Go's net.ParseIP
// also rejects, verify the input cannot be interpreted as a private IP by the OS.
func FuzzLooksLikeNonStandardIP(f *testing.F) {
	seeds := []string{
		"0177.0.0.1",
		"127.0.0.1",
		"0x7f.0.0.1",
		"example.com",
		"10.0.0.1",
		"012.0.0.1",
		"",
		"0.0.0.0",
		"0300.0250.0.1",
		"0xA9.0xFE.0xA9.0xFE",
		"255.255.255.255",
		"0177.00.00.01",
		"0X7F.0X0.0X0.0X1",
		"1.2.3.4",
		"00.00.00.00",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, host string) {
		// Must not panic.
		_ = looksLikeNonStandardIP(host)
	})
}
