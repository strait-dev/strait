package httputil

import (
	"fmt"
	"net"
	"net/url"
	"strings"
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

	// If the host is an IP literal, validate it directly.
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("URL must not point to private or internal address %s", ip)
		}
		return nil
	}

	// Resolve hostname and check all returned IPs. DNS resolution failure
	// is not treated as SSRF; the request will fail later with a meaningful
	// network error rather than a confusing validation message.
	addrs, lookupErr := lookupHost(host)
	if lookupErr != nil {
		return nil //nolint:nilerr // intentional: let the HTTP client surface the DNS error
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip != nil && isPrivateIP(ip) {
			return fmt.Errorf("URL host %q resolves to private address %s", host, ip)
		}
	}

	return nil
}
