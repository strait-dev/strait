package worker

import (
	"fmt"
	"net"
	"net/url"
)

func ValidateEndpointURL(rawURL string, opts ...func(*endpointValidationOpts)) error {
	var o endpointValidationOpts
	for _, opt := range opts {
		opt(&o)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must use http or https scheme")
	}
	if o.requireTLS && u.Scheme != "https" {
		return fmt.Errorf("URL must use https when TLS is required")
	}
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	// Block private/internal IPs (SSRF protection).
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip != nil && !o.allowPrivate {
		if isPrivateOrLinkLocalIP(ip) {
			return fmt.Errorf("URL must not point to private or loopback addresses")
		}
		if isCGNAT(ip) {
			return fmt.Errorf("URL must not point to CGNAT addresses (100.64.0.0/10)")
		}
		if isIPv6ULA(ip) {
			return fmt.Errorf("URL must not point to IPv6 unique local addresses (fc00::/7)")
		}
	}

	return nil
}

func isPrivateOrLinkLocalIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast()
}

// EndpointValidationOpts holds options for endpoint URL validation.
type EndpointValidationOpts = endpointValidationOpts

type endpointValidationOpts struct {
	allowPrivate bool
	requireTLS   bool
}

// WithAllowPrivateEndpoints allows private and loopback IP literals.
func WithAllowPrivateEndpoints(allow bool) func(*endpointValidationOpts) {
	return func(o *endpointValidationOpts) {
		o.allowPrivate = allow
	}
}

// WithRequireTLS returns an option that enforces HTTPS scheme.
func WithRequireTLS(require bool) func(*endpointValidationOpts) {
	return func(o *endpointValidationOpts) {
		o.requireTLS = require
	}
}

func validateEndpointURL(rawURL string) error {
	return ValidateEndpointURL(rawURL)
}

// ValidateEndpointURLWithTLS validates a URL with optional TLS requirement.
func ValidateEndpointURLWithTLS(rawURL string, requireTLS bool) error {
	return ValidateEndpointURL(rawURL, WithRequireTLS(requireTLS))
}

// cgnatNet is the CGNAT range 100.64.0.0/10 (RFC 6598).
var cgnatNet = net.IPNet{
	IP:   net.IP{100, 64, 0, 0},
	Mask: net.CIDRMask(10, 32),
}

// isCGNAT reports whether ip falls in 100.64.0.0/10.
func isCGNAT(ip net.IP) bool {
	return cgnatNet.Contains(ip)
}

// isIPv6ULA reports whether ip falls in fc00::/7.
func isIPv6ULA(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		return false
	}
	ip6 := ip.To16()
	if ip6 == nil {
		return false
	}
	return ip6[0]&0xfe == 0xfc
}
