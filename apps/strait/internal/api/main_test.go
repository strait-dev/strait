package api

import (
	"os"
	"testing"

	"strait/internal/httputil"
)

// TestMain installs a mock DNS resolver for the entire API test suite so that
// ValidateExternalURL (which rejects DNS lookup failures to prevent SSRF) does
// not fail on synthetic hostnames used in tests.
func TestMain(m *testing.M) {
	installHumaErrorOverride()

	restore := httputil.SetLookupHostForTest(func(host string) ([]string, error) {
		switch host {
		case "internal.example.com":
			return []string{"10.0.0.5"}, nil
		case "sneaky.example.com":
			return []string{"93.184.216.34", "192.168.1.1"}, nil
		case "loopback.example.com":
			return []string{"127.0.0.1"}, nil
		case "ipv6private.example.com":
			return []string{"::1"}, nil
		default:
			// Resolve all other hostnames as public for test convenience.
			return []string{"93.184.216.34"}, nil
		}
	})

	code := m.Run()
	restore()
	os.Exit(code)
}
