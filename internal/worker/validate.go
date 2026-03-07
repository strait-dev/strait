package worker

import (
	"fmt"
	"net"
	"net/url"
)

// validateEndpointURL checks that a URL is valid and doesn't target private networks.
// This mirrors the api-layer SSRF check (api.validateURL) for runtime URL overrides
// such as environment endpoint overrides that bypass the API validation layer.
func validateEndpointURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must use http or https scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	// Block private/internal IPs (SSRF protection).
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("URL must not point to private or loopback addresses")
		}
	}

	return nil
}
