// Package tunnel provides utilities for creating Cloudflare Quick Tunnels
// to expose local development ports to the internet.
package tunnel

import (
	"fmt"
	"regexp"
	"strings"
)

// tunnelURLPattern matches a Cloudflare Quick Tunnel URL in cloudflared output.
var tunnelURLPattern = regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)

// JobEndpoint describes a job with its slug and local path for tunnel mapping.
type JobEndpoint struct {
	Slug string
	Path string
}

// ParseTunnelURL extracts the tunnel URL from cloudflared stdout output.
// It searches for a URL matching the pattern https://*.trycloudflare.com.
func ParseTunnelURL(output string) (string, error) {
	match := tunnelURLPattern.FindString(output)
	if match == "" {
		return "", fmt.Errorf("no tunnel URL found in cloudflared output")
	}
	return match, nil
}

// BuildJobEndpoints maps job slugs to their full tunnel URL paths.
// Each job's path is appended to the tunnel base URL. If a job has no
// explicit path, the slug is used as the path segment.
func BuildJobEndpoints(tunnelURL string, jobs []JobEndpoint) map[string]string {
	endpoints := make(map[string]string, len(jobs))
	base := strings.TrimRight(tunnelURL, "/")

	for _, job := range jobs {
		path := job.Path
		if path == "" {
			path = "/" + job.Slug
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		endpoints[job.Slug] = base + path
	}

	return endpoints
}
