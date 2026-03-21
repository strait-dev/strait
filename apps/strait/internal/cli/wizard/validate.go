package wizard

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ValidateProjectName checks that a project name meets our constraints.
func ValidateProjectName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("project name is required")
	}
	if len(name) > 128 {
		return fmt.Errorf("project name must be at most 128 characters")
	}
	if !slugRegex.MatchString(name) {
		return fmt.Errorf("project name must contain only lowercase letters, numbers, and hyphens, and cannot start or end with a hyphen")
	}
	return nil
}

// ValidateSlug checks that a slug is valid.
func ValidateSlug(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return fmt.Errorf("slug is required")
	}
	if len(slug) > 128 {
		return fmt.Errorf("slug must be at most 128 characters")
	}
	if !slugRegex.MatchString(slug) {
		return fmt.Errorf("slug must contain only lowercase letters, numbers, and hyphens, and cannot start or end with a hyphen")
	}
	return nil
}

// ValidateEndpoint checks that an endpoint URL is valid HTTP/HTTPS.
func ValidateEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return fmt.Errorf("endpoint URL is required")
	}
	if len(endpoint) > 2048 {
		return fmt.Errorf("endpoint URL must be at most 2048 characters")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("endpoint URL must use http or https scheme")
	}
	if parsed.Host == "" {
		return fmt.Errorf("endpoint URL must have a host")
	}
	return nil
}

// ValidateRuntime checks that a runtime is one of the known values.
func ValidateRuntime(runtime string) error {
	runtime = strings.TrimSpace(strings.ToLower(runtime))
	valid := map[string]bool{
		"node":   true,
		"bun":    true,
		"python": true,
		"go":     true,
		"docker": true,
	}
	if !valid[runtime] {
		return fmt.Errorf("runtime must be one of: node, bun, python, go, docker")
	}
	return nil
}

// ValidateCron checks a cron expression (optional — empty string is valid).
func ValidateCron(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	// Accept shorthand aliases
	aliases := map[string]bool{
		"@yearly": true, "@annually": true,
		"@monthly": true, "@weekly": true,
		"@daily": true, "@midnight": true,
		"@hourly": true,
	}
	if aliases[expr] {
		return nil
	}
	parts := strings.Fields(expr)
	if len(parts) != 5 && len(parts) != 6 {
		return fmt.Errorf("cron expression must have 5 or 6 fields (got %d)", len(parts))
	}
	return nil
}

// ValidateTimeout checks that a timeout is within reasonable bounds.
func ValidateTimeout(secs int) error {
	if secs < 1 {
		return fmt.Errorf("timeout must be at least 1 second")
	}
	if secs > 86400 {
		return fmt.Errorf("timeout must be at most 86400 seconds (24 hours)")
	}
	return nil
}

// ValidateMaxAttempts checks that max attempts is within bounds.
func ValidateMaxAttempts(n int) error {
	if n < 1 {
		return fmt.Errorf("max attempts must be at least 1")
	}
	if n > 100 {
		return fmt.Errorf("max attempts must be at most 100")
	}
	return nil
}

// Runtimes returns the list of supported runtimes.
func Runtimes() []string {
	return []string{"node", "bun", "python", "go", "docker"}
}
