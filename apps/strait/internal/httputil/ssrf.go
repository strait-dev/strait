package httputil

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// privateRanges contains CIDR blocks that must be blocked to prevent SSRF.
var privateRanges = mustParsePrivateRanges()

func mustParsePrivateRanges() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",        // loopback
		"10.0.0.0/8",         // RFC 1918
		"172.16.0.0/12",      // RFC 1918
		"192.168.0.0/16",     // RFC 1918
		"169.254.0.0/16",     // link-local
		"100.64.0.0/10",      // CGNAT (RFC 6598)
		"0.0.0.0/8",          // unspecified
		"192.0.0.0/24",       // IETF protocol assignments
		"192.0.2.0/24",       // documentation
		"192.88.99.0/24",     // deprecated 6to4 relay anycast
		"198.18.0.0/15",      // benchmarking
		"198.51.100.0/24",    // documentation
		"203.0.113.0/24",     // documentation
		"224.0.0.0/4",        // multicast
		"240.0.0.0/4",        // reserved
		"255.255.255.255/32", // limited broadcast
		"::1/128",            // IPv6 loopback
		"fc00::/7",           // IPv6 unique local
		"fe80::/10",          // IPv6 link-local
		"fec0::/10",          // deprecated IPv6 site-local
		"ff00::/8",           // IPv6 multicast
		"2001:db8::/32",      // documentation
		"2001:2::/48",        // benchmarking
		"2001:10::/28",       // deprecated ORCHID
		"64:ff9b::/96",       // NAT64 well-known prefix (RFC 6052)
		"64:ff9b:1::/48",     // NAT64 local-use block (RFC 8215)
		"2002::/16",          // 6to4 (RFC 3056)
	}
	ranges := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// This is static package data. A parse failure means the binary was
			// built with an invalid SSRF guardrail and must not start.
			panic(fmt.Sprintf("httputil: bad CIDR %q: %v", cidr, err))
		}
		ranges = append(ranges, network)
	}
	return ranges
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
			return nil, fmt.Errorf("ssrf: invalid dial address")
		}
		if host == "" || port == "" {
			return nil, fmt.Errorf("ssrf: invalid dial address")
		}
		for _, blocked := range blockedHosts {
			if strings.EqualFold(host, blocked) {
				return nil, fmt.Errorf("ssrf: blocked internal host")
			}
		}
		if looksLikeNonStandardIP(host) {
			return nil, fmt.Errorf("ssrf: host uses non-standard IP notation")
		}
		if ip := net.ParseIP(host); ip != nil {
			if isPrivateIP(ip) {
				return nil, fmt.Errorf("ssrf: blocked private dial address")
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		}
		addrs, lookupErr := lookupHost(host)
		if lookupErr != nil {
			return nil, fmt.Errorf("ssrf: DNS lookup failed")
		}
		firstPublic, err := firstPublicResolvedIP(addrs)
		if err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(firstPublic.String(), port))
	}
}

func firstPublicResolvedIP(addrs []string) (net.IP, error) {
	var firstPublic net.IP
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if isPrivateIP(ip) {
			return nil, fmt.Errorf("ssrf: host resolves to private address")
		}
		if firstPublic == nil {
			firstPublic = ip
		}
	}
	if firstPublic == nil {
		return nil, fmt.Errorf("ssrf: DNS lookup returned no usable IP addresses")
	}
	return firstPublic, nil
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
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, network := range privateRanges {
		if network.Contains(ip) {
			return true
		}
	}
	if len(ip) == net.IPv6len {
		if v4 := embeddedIPv4(ip); v4 != nil {
			return isPrivateIP(v4)
		}
	}
	return false
}

// embeddedIPv4 returns the IPv4 address embedded inside an IPv6 transition or
// translation form, or nil if ip does not carry one. Callers re-run the
// private-IP check against the returned address so that IMDS, loopback, and
// other IPv4-side ranges still fire when the attacker hands us the IPv6 shape.
//
// Forms handled:
//   - ::ffff:x.x.x.x    IPv4-mapped, caught by net.IP.To4
//   - ::x.x.x.x         IPv4-compatible (deprecated, RFC 4291)
//   - 64:ff9b::/96      NAT64 well-known prefix (RFC 6052)
//   - 2002::/16         6to4 (RFC 3056); embedded IPv4 lives in bytes 2..6
//
// In NAT64/DNS64 deployments (AWS IPv6-only subnets, IPv6-mostly Kubernetes)
// an attacker-controlled hostname can resolve to 64:ff9b::a9fe:a9fe — the
// NAT64 form of 169.254.169.254 — and slip past a guard that only decoded the
// IPv4-mapped and IPv4-compatible shapes.
func embeddedIPv4(ip net.IP) net.IP {
	// IPv4-mapped: To4() returns non-nil for ::ffff:x.x.x.x.
	if v4 := ip.To4(); v4 != nil && !ip.Equal(v4) {
		return v4
	}
	// IPv4-compatible: first 96 bits are zero, last 32 bits are the IPv4
	// address. To4() returns nil for these.
	allZero := true
	for i := range 12 {
		if ip[i] != 0 {
			allZero = false
			break
		}
	}
	if allZero && (ip[12] != 0 || ip[13] != 0 || ip[14] != 0 || ip[15] != 0) {
		return net.IPv4(ip[12], ip[13], ip[14], ip[15])
	}
	// NAT64 well-known prefix 64:ff9b:: — embedded IPv4 in the low 32 bits.
	if ip[0] == 0x00 && ip[1] == 0x64 && ip[2] == 0xff && ip[3] == 0x9b &&
		ip[4] == 0x00 && ip[5] == 0x00 && ip[6] == 0x00 && ip[7] == 0x00 &&
		ip[8] == 0x00 && ip[9] == 0x00 && ip[10] == 0x00 && ip[11] == 0x00 {
		return net.IPv4(ip[12], ip[13], ip[14], ip[15])
	}
	// NAT64 local-use prefix 64:ff9b:1::/96 (RFC 8215) — embedded IPv4 in the low
	// 32 bits. The whole 64:ff9b:1::/48 block is also blocked by privateRanges;
	// decoding here lets the embedded IPv4 itself be range-checked.
	if ip[0] == 0x00 && ip[1] == 0x64 && ip[2] == 0xff && ip[3] == 0x9b &&
		ip[4] == 0x00 && ip[5] == 0x01 && ip[6] == 0x00 && ip[7] == 0x00 &&
		ip[8] == 0x00 && ip[9] == 0x00 && ip[10] == 0x00 && ip[11] == 0x00 {
		return net.IPv4(ip[12], ip[13], ip[14], ip[15])
	}
	// 6to4: 2002:WWXX:YYZZ::/48 — embedded IPv4 in bytes 2..6.
	// 2002:7f00:0001:: is 6to4 of 127.0.0.1.
	if ip[0] == 0x20 && ip[1] == 0x02 {
		return net.IPv4(ip[2], ip[3], ip[4], ip[5])
	}
	return nil
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
			return fmt.Errorf("URL must not point to internal host")
		}
	}

	// Reject ambiguous IP representations that Go's net.ParseIP does not
	// handle but OS-level DNS resolvers may interpret: octal-encoded octets
	// (e.g. 0177.0.0.1 = 127.0.0.1) and other non-standard notations.
	if looksLikeNonStandardIP(host) {
		return fmt.Errorf("ssrf: URL host uses non-standard IP notation")
	}

	// If the host is an IP literal, validate it directly.
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("URL must not point to private or internal address")
		}
		return nil
	}

	// Resolve hostname and check all returned IPs. DNS resolution failure
	// is treated as a rejection to prevent DNS rebinding attacks where an
	// attacker controls a DNS server that intermittently fails.
	addrs, lookupErr := lookupHost(host)
	if lookupErr != nil {
		return fmt.Errorf("ssrf: DNS lookup failed")
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip != nil && isPrivateIP(ip) {
			return fmt.Errorf("URL host resolves to private address")
		}
	}

	return nil
}

// RedactURLForLog returns a URL shape that is useful for operations but does
// not expose path segments, query strings, userinfo, or fragments. Those often
// carry tokens in webhook and callback URLs.
func RedactURLForLog(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "[invalid-url]"
	}
	if u.Scheme == "" || u.Host == "" {
		return "[invalid-url]"
	}
	return u.Scheme + "://" + u.Host
}

// SanitizeHTTPClientError removes request URLs from net/http client errors.
// url.Error includes the full URL, and webhook/callback URLs often carry query
// tokens, userinfo, or path secrets.
func SanitizeHTTPClientError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Err != nil {
			var nested *url.Error
			if errors.As(urlErr.Err, &nested) {
				return SanitizeHTTPClientError(urlErr.Err)
			}
			return fmt.Sprintf("%s: %s", urlErr.Op, sanitizeHTTPClientRootError(urlErr.Err))
		}
		return urlErr.Op
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

func sanitizeHTTPClientRootError(err error) string {
	if err == nil {
		return "request failed"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded.Error()
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "invalid header"):
		return "invalid header"
	case strings.HasPrefix(msg, "ssrf:"):
		return msg
	default:
		return "request failed"
	}
}
