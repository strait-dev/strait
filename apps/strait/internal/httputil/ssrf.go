package httputil

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// privateRanges contains CIDR blocks that must be blocked to prevent SSRF.
var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // RFC 1918
		"172.16.0.0/12",  // RFC 1918
		"192.168.0.0/16", // RFC 1918
		"169.254.0.0/16", // link-local
		"100.64.0.0/10",  // CGNAT (RFC 6598)
		"0.0.0.0/8",      // unspecified
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("httputil: bad CIDR %q: %v", cidr, err))
		}
		privateRanges = append(privateRanges, network)
	}
}

// blockedHosts are hostnames (case-insensitive) that must never be targeted.
var blockedHosts = []string{
	"localhost",
	"metadata.google.internal",
}

// lookupHost is the DNS resolver function, replaceable in tests.
var lookupHost = net.LookupHost

// SetLookupHostForTest replaces the DNS resolver used by ValidateExternalURL.
// It returns a restore function that must be called to reset the original resolver.
// This is intended for use in tests outside the httputil package.
func SetLookupHostForTest(fn func(string) ([]string, error)) func() {
	orig := lookupHost
	lookupHost = fn
	return func() { lookupHost = orig }
}

// SafeDialContext returns a DialContext that prevents DNS rebinding SSRF by
// resolving the target host itself, rejecting every private/internal answer,
// and dialing the vetted IP literal. When allowPrivate is true, it falls back
// to the standard net.Dialer behavior for local/self-host deployments.
func SafeDialContext(allowPrivate bool) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	if allowPrivate {
		return dialer.DialContext
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("ssrf: invalid dial address %q: %w", address, err)
		}
		if host == "" || port == "" {
			return nil, fmt.Errorf("ssrf: invalid dial address %q", address)
		}
		for _, blocked := range blockedHosts {
			if strings.EqualFold(host, blocked) {
				return nil, fmt.Errorf("ssrf: blocked internal host %q", blocked)
			}
		}
		if looksLikeNonStandardIP(host) {
			return nil, fmt.Errorf("ssrf: host %q uses non-standard IP notation", host)
		}
		if ip := net.ParseIP(host); ip != nil {
			if isPrivateIP(ip) {
				return nil, fmt.Errorf("ssrf: blocked private dial address %s", ip)
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		}
		addrs, lookupErr := lookupHost(host)
		if lookupErr != nil {
			return nil, fmt.Errorf("ssrf: DNS lookup failed for %q: %w", host, lookupErr)
		}
		var firstPublic net.IP
		for _, addr := range addrs {
			ip := net.ParseIP(addr)
			if ip == nil {
				continue
			}
			if isPrivateIP(ip) {
				return nil, fmt.Errorf("ssrf: host %q resolves to private address %s", host, ip)
			}
			if firstPublic == nil {
				firstPublic = ip
			}
		}
		if firstPublic == nil {
			return nil, fmt.Errorf("ssrf: DNS lookup for %q returned no usable IP addresses", host)
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(firstPublic.String(), port))
	}
}

// NewExternalTransport returns an HTTP transport for untrusted, user-supplied
// URLs. It validates resolved addresses at dial time, which closes the gap
// between registration-time URL validation and delivery-time DNS answers.
func NewExternalTransport(allowPrivate bool) *http.Transport {
	return &http.Transport{
		DialContext:         SafeDialContext(allowPrivate),
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
}

// isPrivateIP reports whether ip belongs to a private, loopback, link-local,
// CGNAT, or otherwise internal network range.
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, network := range privateRanges {
		if network.Contains(ip) {
			return true
		}
	}
	// Check IPv6 addresses that embed IPv4:
	// - ::ffff:127.0.0.1 (IPv4-mapped, caught by To4)
	// - ::127.0.0.1 (IPv4-compatible, NOT caught by To4)
	if len(ip) == net.IPv6len {
		// IPv4-mapped: To4() returns non-nil.
		if v4 := ip.To4(); v4 != nil && !ip.Equal(v4) {
			return isPrivateIP(v4)
		}
		// IPv4-compatible (deprecated RFC 4291): first 96 bits are zero,
		// last 32 bits are the IPv4 address. To4() returns nil for these.
		allZero := true
		for i := range 12 {
			if ip[i] != 0 {
				allZero = false
				break
			}
		}
		if allZero && (ip[12] != 0 || ip[13] != 0 || ip[14] != 0 || ip[15] != 0) {
			v4 := net.IPv4(ip[12], ip[13], ip[14], ip[15])
			return isPrivateIP(v4)
		}
	}
	return false
}

// looksLikeNonStandardIP detects IP-like hostnames that use non-standard
// notation (octal, hex-per-octet, or decimal-encoded) which Go's net.ParseIP
// rejects but OS DNS resolvers may interpret as loopback/private addresses.
// Examples: "0177.0.0.1" (octal for 127.0.0.1), "0x7f.0.0.1".
func looksLikeNonStandardIP(host string) bool {
	// Must look like a dotted-quad (4 parts separated by dots).
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	allNumeric := true
	hasLeadingZero := false
	for _, part := range parts {
		if part == "" {
			return false
		}
		// Check if this part is numeric (decimal, octal, or hex).
		isNumeric := true
		for _, c := range part {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') && c != 'x' && c != 'X' {
				isNumeric = false
				break
			}
		}
		if !isNumeric {
			allNumeric = false
		}
		// Leading zero in a numeric part signals octal notation.
		if len(part) > 1 && part[0] == '0' && isNumeric {
			hasLeadingZero = true
		}
	}
	// If all parts are numeric and any has a leading zero (octal) or hex prefix,
	// this is a non-standard IP representation.
	if allNumeric && hasLeadingZero {
		return true
	}
	// Hex-per-octet: 0x7f.0.0.1
	for _, part := range parts {
		if len(part) > 2 && (part[:2] == "0x" || part[:2] == "0X") {
			return true
		}
	}
	return false
}

// ValidateExternalURL checks that rawURL is a safe, externally-routable HTTP(S) URL.
// It rejects:
//   - non-HTTP(S) schemes
//   - empty or missing host
//   - localhost and cloud metadata hostnames
//   - IP literals in private/loopback/link-local/CGNAT ranges
//   - hostnames that resolve to private IPs
func ValidateExternalURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL must not be empty")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https, got %q", u.Scheme)
	}

	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	host := u.Hostname()

	// Block well-known internal hostnames.
	for _, blocked := range blockedHosts {
		if strings.EqualFold(host, blocked) {
			return fmt.Errorf("URL must not point to internal host %q", blocked)
		}
	}

	// Reject ambiguous IP representations that Go's net.ParseIP does not
	// handle but OS-level DNS resolvers may interpret: octal-encoded octets
	// (e.g. 0177.0.0.1 = 127.0.0.1) and other non-standard notations.
	if looksLikeNonStandardIP(host) {
		return fmt.Errorf("ssrf: URL host %q uses non-standard IP notation", host)
	}

	// If the host is an IP literal, validate it directly.
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("URL must not point to private or internal address %s", ip)
		}
		return nil
	}

	// Resolve hostname and check all returned IPs. DNS resolution failure
	// is treated as a rejection to prevent DNS rebinding attacks where an
	// attacker controls a DNS server that intermittently fails.
	addrs, lookupErr := lookupHost(host)
	if lookupErr != nil {
		return fmt.Errorf("ssrf: DNS lookup failed for %q: %w", host, lookupErr)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip != nil && isPrivateIP(ip) {
			return fmt.Errorf("URL host %q resolves to private address %s", host, ip)
		}
	}

	return nil
}
