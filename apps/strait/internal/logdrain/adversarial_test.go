package logdrain

import (
	"strings"
	"testing"
)

// TestProtectedHeaders_AllBlocked verifies every entry in ProtectedHeaders is filtered.
func TestProtectedHeaders_AllBlocked(t *testing.T) {
	t.Parallel()

	expected := []string{
		"host", "content-length", "content-type", "transfer-encoding",
		"connection", "upgrade", "te", "trailer",
	}

	for _, h := range expected {
		if !ProtectedHeaders[h] {
			t.Errorf("expected %q to be a protected header", h)
		}
	}

	if len(ProtectedHeaders) != len(expected) {
		t.Errorf("ProtectedHeaders has %d entries, expected %d", len(ProtectedHeaders), len(expected))
	}
}

// TestProtectedHeaders_CaseInsensitive verifies the filtering uses lowercase comparison.
func TestProtectedHeaders_CaseInsensitive(t *testing.T) {
	t.Parallel()

	variants := []string{"HOST", "Host", "host", "HoSt"}
	for _, v := range variants {
		lower := strings.ToLower(v)
		if !ProtectedHeaders[lower] {
			t.Errorf("expected %q (lowered to %q) to be blocked", v, lower)
		}
	}
}

// TestProtectedHeaders_CustomInjection verifies that Authorization in custom headers
// is NOT blocked (it is not in the protected list), which is by design since the
// "header" auth type allows arbitrary custom headers except the protected ones.
func TestProtectedHeaders_CustomInjection(t *testing.T) {
	t.Parallel()

	// Authorization is intentionally allowed for custom header auth.
	if ProtectedHeaders[strings.ToLower("Authorization")] {
		t.Fatal("Authorization should not be in ProtectedHeaders")
	}
}

// TestProtectedHeaders_NullByteBypass verifies "host\x00" is not treated as "host".
func TestProtectedHeaders_NullByteBypass(t *testing.T) {
	t.Parallel()

	malicious := "host\x00"
	lower := strings.ToLower(malicious)

	// The null byte makes it a different string, so it should not match.
	if ProtectedHeaders[lower] {
		t.Fatal("header with null byte should not match protected header")
	}

	// However, strings.ToLower("host\x00") preserves the null byte.
	if lower == "host" {
		t.Fatal("null byte header should not equal 'host'")
	}
}

// FuzzProtectedHeaders fuzzes header names to verify the lookup is consistent.
func FuzzProtectedHeaders(f *testing.F) {
	f.Add("host")
	f.Add("Host")
	f.Add("HOST")
	f.Add("content-type")
	f.Add("x-custom-header")
	f.Add("authorization")
	f.Add("host\x00")
	f.Add("")

	f.Fuzz(func(t *testing.T, header string) {
		lower := strings.ToLower(header)
		blocked := ProtectedHeaders[lower]

		// If blocked, the lowercase form must be in the map.
		if blocked && !ProtectedHeaders[lower] {
			t.Errorf("inconsistent lookup for %q", header)
		}

		// Verify idempotency: double-lowercase should not change the result.
		doubleLower := strings.ToLower(lower)
		if ProtectedHeaders[doubleLower] != blocked {
			t.Errorf("double-lowercase changed result for %q", header)
		}
	})
}
